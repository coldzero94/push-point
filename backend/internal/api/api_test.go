package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	"github.com/coby/push-point/backend/internal/store"
)

// ---- fake Store (인메모리) ----
// 핸들러 계층 테스트가 sqlite 구현과 결합하지 않도록 store.Store 계약만 흉내 낸다.

type fakeStore struct {
	mu        sync.Mutex
	nextID    int64
	nextTag   int64
	links     map[int64]*store.LinkDetail
	byURL     map[string]int64
	deleted   map[int64]bool
	tags      map[int64]*store.Tag
	lastSave  store.SaveInput  // 핸들러가 마지막으로 넘긴 SaveInput (테스트 관찰용)
	savedBody map[int64]string // 링크별 클라이언트 캡처 본문
}

var _ store.Store = (*fakeStore)(nil)

func newFakeStore() *fakeStore {
	f := &fakeStore{
		links:     make(map[int64]*store.LinkDetail),
		byURL:     make(map[string]int64),
		deleted:   make(map[int64]bool),
		tags:      make(map[int64]*store.Tag),
		savedBody: make(map[int64]string),
	}
	f.mustAddTag("dev", store.FacetCraft)
	f.mustAddTag("golang", store.FacetCraft)
	return f
}

func (f *fakeStore) mustAddTag(name, facet string) {
	f.nextTag++
	f.tags[f.nextTag] = &store.Tag{ID: f.nextTag, Name: name, Aliases: []string{}, Facet: facet}
}

// addLink는 테스트 픽스처 주입용 (Store 계약 밖).
func (f *fakeStore) addLink(url, status string, createdAt int64) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	id := f.nextID
	f.links[id] = &store.LinkDetail{
		Link: store.Link{
			ID: id, URL: url, Domain: "example.com", Title: "title " + url,
			Status: status, CreatedAt: createdAt, Tags: []store.LinkTag{},
			// 실제 스토어의 CASE는 **항상** 세 값 중 하나를 주므로(linkCols) 페이크도
			// 그 불변식을 지킨다. 빈 문자열로 두면 enum 밖 값이 계약을 타고 나가고,
			// 생성된 클라이언트가 디코드에서 터진다.
			RetryState: "none",
		},
		Jobs: store.JobSummary{Scrape: "pending"},
	}
	f.byURL[url] = id
	return id
}

// setThumb은 thumb_path를 주입한다 (Store 계약 밖 픽스처).
// setError·setRetryState는 계약이 2026-07-30에 받은 두 필드의 픽스처다 (Store 계약 밖).
func (f *fakeStore) setError(id int64, msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links[id].Error = msg
}

func (f *fakeStore) setRetryState(id int64, st string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links[id].RetryState = st
}

func (f *fakeStore) setThumb(id int64, path string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links[id].ThumbPath = &path
}

// setDescription은 절단 검증용 description을 주입한다 (Store 계약 밖 픽스처).
func (f *fakeStore) setDescription(id int64, desc string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.links[id].Description = desc
}

func (f *fakeStore) SaveLink(ctx context.Context, in store.SaveInput) (int64, int64, bool, error) {
	// 실제 store와 같은 계약 — SaveLink는 자기 입력을 스스로 정규화한다(진입점 무관).
	in, nerr := in.Normalize()
	if nerr != nil {
		return 0, 0, false, nerr
	}
	url, note := in.URL, in.Note
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastSave = in // 정규화까지 마친 값 — 저장 계층이 실제로 받는 것
	if id, ok := f.byURL[url]; ok {
		if !f.deleted[id] {
			return id, f.links[id].CreatedAt, true, nil
		}
		// 소프트 삭제된 URL 재저장 — undelete 후 신규처럼 (sqlite 구현과 동일 계약)
		delete(f.deleted, id)
		l := f.links[id]
		l.Status = "pending"
		l.Note = note
		l.Error = ""
		return id, l.CreatedAt, false, nil
	}
	f.nextID++
	id := f.nextID
	f.links[id] = &store.LinkDetail{
		Link: store.Link{
			ID: id, URL: url, Status: "pending", Note: note,
			CreatedAt: 1000 + id, Tags: []store.LinkTag{},
		},
		Jobs: store.JobSummary{Scrape: "pending"},
	}
	f.byURL[url] = id
	return id, 1000 + id, false, nil
}

func (f *fakeStore) GetLink(ctx context.Context, id int64) (*store.LinkDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[id]
	if !ok || f.deleted[id] {
		return nil, store.ErrNotFound
	}
	cp := *l
	return &cp, nil
}

func (f *fakeStore) ListLinks(ctx context.Context, cursor string, limit int, tag, status string, unopened bool) ([]store.Link, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var all []store.Link
	for id, l := range f.links {
		if f.deleted[id] {
			continue
		}
		if status != "" && l.Status != status {
			continue
		}
		if tag != "" {
			found := false
			for _, t := range l.Tags {
				if strings.EqualFold(t.Name, tag) {
					found = true
				}
			}
			if !found {
				continue
			}
		}
		all = append(all, l.Link)
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].CreatedAt != all[j].CreatedAt {
			return all[i].CreatedAt > all[j].CreatedAt
		}
		return all[i].ID > all[j].ID
	})
	if cursor != "" {
		ca, id, err := store.DecodeCursor(cursor)
		if err != nil {
			return nil, "", err
		}
		kept := all[:0]
		for _, l := range all {
			if l.CreatedAt < ca || (l.CreatedAt == ca && l.ID < id) {
				kept = append(kept, l)
			}
		}
		all = kept
	}
	next := ""
	if len(all) > limit {
		all = all[:limit]
		last := all[len(all)-1]
		next = store.EncodeCursor(last.CreatedAt, last.ID)
	}
	return all, next, nil
}

func (f *fakeStore) UpdateLink(ctx context.Context, id int64, note *string, tags []string) (*store.LinkDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[id]
	if !ok || f.deleted[id] {
		return nil, store.ErrNotFound
	}
	if note != nil {
		l.Note = *note
	}
	if tags != nil {
		newTags := []store.LinkTag{}
		for _, name := range tags {
			var found *store.Tag
			for _, t := range f.tags {
				if strings.EqualFold(t.Name, name) {
					found = t
				}
			}
			if found == nil {
				return nil, store.ErrUnknownTag
			}
			newTags = append(newTags, store.LinkTag{ID: found.ID, Name: found.Name, Source: "manual"})
		}
		l.Tags = newTags
	}
	cp := *l
	return &cp, nil
}

func (f *fakeStore) DeleteLink(ctx context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.links[id]; !ok || f.deleted[id] {
		return store.ErrNotFound
	}
	f.deleted[id] = true
	return nil
}

func (f *fakeStore) RetryLink(ctx context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[id]
	if !ok || f.deleted[id] {
		return store.ErrNotFound
	}
	if l.Status != "failed" {
		return store.ErrNotFailed
	}
	l.Status = "pending"
	return nil
}

// setTagLastSaved는 신선도 픽스처다 (Store 계약 밖).
func (f *fakeStore) setTagLastSaved(id int64, at int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.tags[id].LinkCount = 1
	f.tags[id].LastSavedAt = &at
}

func (f *fakeStore) ListTags(ctx context.Context) ([]store.Tag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []store.Tag
	for _, t := range f.tags {
		out = append(out, *t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (f *fakeStore) CreateTag(ctx context.Context, name string, aliases []string, facet string) (*store.Tag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, t := range f.tags {
		if strings.EqualFold(t.Name, name) {
			return nil, store.ErrDuplicateTag
		}
	}
	if facet == "" {
		facet = store.FacetNeutral // 계약 default — sqliteStore와 같은 규칙
	}
	f.nextTag++
	t := &store.Tag{ID: f.nextTag, Name: name, Aliases: aliases, Facet: facet}
	f.tags[t.ID] = t
	cp := *t
	return &cp, nil
}

func (f *fakeStore) UpdateTag(ctx context.Context, id int64, name *string, aliases []string, facet *string) (*store.Tag, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	t, ok := f.tags[id]
	if !ok {
		return nil, store.ErrNotFound
	}
	if name != nil {
		t.Name = *name
	}
	if aliases != nil {
		t.Aliases = aliases
	}
	if facet != nil {
		// sqliteStore.UpdateTag의 normalizeFacet과 같은 규칙 — 빈 문자열은 neutral로 접는다.
		if *facet == "" {
			t.Facet = store.FacetNeutral
		} else {
			t.Facet = *facet
		}
	}
	cp := *t
	return &cp, nil
}

func (f *fakeStore) DeleteTag(ctx context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.tags[id]; !ok {
		return store.ErrNotFound
	}
	delete(f.tags, id)
	return nil
}

func (f *fakeStore) Search(ctx context.Context, q, tag string, from, to *int64, cursor string, limit int) ([]store.SearchResult, string, store.SearchMode, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mode := store.SearchModeFTS
	if utf8.RuneCountInString(q) < 3 {
		mode = store.SearchModeLike
	}
	var out []store.SearchResult
	lq := strings.ToLower(q)
	for id, l := range f.links {
		if f.deleted[id] {
			continue
		}
		if !strings.Contains(strings.ToLower(l.Title+" "+l.Description+" "+l.Note), lq) {
			continue
		}
		r := store.SearchResult{Link: l.Link}
		if mode == store.SearchModeFTS {
			rank := -1.5
			r.Rank = &rank
		}
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt > out[j].CreatedAt })
	return out, "", mode, nil
}

func (f *fakeStore) Stats(ctx context.Context) (*store.Stats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var failed int64
	for id, l := range f.links {
		if !f.deleted[id] && l.Status == "failed" {
			failed++
		}
	}
	return &store.Stats{
		TotalLinks:  int64(len(f.links) - len(f.deleted)),
		FailedLinks: failed,
	}, nil
}

// scrape/thumb 잡 핸들러용 메서드 — API 핸들러 테스트 경로에서는 호출되지 않아 최소 구현.
func (f *fakeStore) GetLinkURL(ctx context.Context, linkID int64) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[linkID]
	if !ok || f.deleted[linkID] {
		return "", "", store.ErrNotFound
	}
	return l.URL, "", nil
}

func (f *fakeStore) ApplyScrape(ctx context.Context, linkID int64, m store.ScrapeResult) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[linkID]
	if !ok || f.deleted[linkID] {
		return store.ErrNotFound
	}
	l.Title, l.Description, l.ContentType, l.Status = m.Title, m.Description, m.ContentType, "done"
	return nil
}

func (f *fakeStore) SetThumbPath(ctx context.Context, linkID int64, relPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[linkID]
	if !ok || f.deleted[linkID] {
		return store.ErrNotFound
	}
	l.ThumbPath = &relPath
	return nil
}

// tag 잡 경로는 API 테스트 범위 밖 — 인터페이스 충족용 스텁 (실경로는 store 단위 테스트가 커버).
func (f *fakeStore) GetLinkContent(ctx context.Context, linkID int64) (store.LinkContent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	l, ok := f.links[linkID]
	if !ok || f.deleted[linkID] {
		return store.LinkContent{}, store.ErrNotFound
	}
	return store.LinkContent{Title: l.Title, Description: l.Description}, nil
}

func (f *fakeStore) LoadTagDict(ctx context.Context) ([]store.TagDictEntry, error) { return nil, nil }

func (f *fakeStore) ApplyTags(ctx context.Context, linkID int64, scored []store.ScoredTag, terms []string) error {
	return nil
}

func (f *fakeStore) SetSummary(ctx context.Context, linkID int64, summary string) error { return nil }

func (f *fakeStore) Close() error { return nil }

// ---- 테스트 하네스 ----

const testKey = "test-key"

func newTestRouter(t *testing.T) (*fakeStore, http.Handler, string) {
	t.Helper()
	fs := newFakeStore()
	dataDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	h := NewRouter(NewServer(fs, dataDir, logger), testKey, logger)
	return fs, h, dataDir
}

func do(t *testing.T, h http.Handler, method, target, body, key string) *httptest.ResponseRecorder {
	t.Helper()
	var rd io.Reader
	if body != "" {
		rd = strings.NewReader(body)
	}
	req := httptest.NewRequest(method, target, rd)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func decodeJSON(t *testing.T, rec *httptest.ResponseRecorder, v any) {
	t.Helper()
	if err := json.Unmarshal(rec.Body.Bytes(), v); err != nil {
		t.Fatalf("JSON 디코드 실패: %v (body=%q)", err, rec.Body.String())
	}
}

func errCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var e struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	decodeJSON(t, rec, &e)
	return e.Error.Code
}

// ---- 테스트 ----

func TestBearerAuth(t *testing.T) {
	_, h, _ := newTestRouter(t)
	tests := []struct {
		name       string
		method     string
		target     string
		key        string
		wantStatus int
		wantCode   string // 에러 응답일 때만 검사
	}{
		{"healthz는 인증 면제", http.MethodGet, "/healthz", "", http.StatusOK, ""},
		{"키 없음 401", http.MethodGet, "/api/v1/links", "", http.StatusUnauthorized, "unauthorized"},
		{"틀린 키 401", http.MethodGet, "/api/v1/links", "wrong-key", http.StatusUnauthorized, "unauthorized"},
		{"길이 다른 키 401", http.MethodGet, "/api/v1/links", "x", http.StatusUnauthorized, "unauthorized"},
		{"맞는 키 200", http.MethodGet, "/api/v1/links", testKey, http.StatusOK, ""},
		{"thumbs는 인증 면제 (401 아닌 404)", http.MethodGet, "/thumbs/aa/none.jpg", "", http.StatusNotFound, "not_found"},
		{"tags도 인증 필요", http.MethodGet, "/api/v1/tags", "", http.StatusUnauthorized, "unauthorized"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, tc.method, tc.target, "", tc.key)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body=%q)", rec.Code, tc.wantStatus, rec.Body.String())
			}
			if tc.wantCode != "" {
				if got := errCode(t, rec); got != tc.wantCode {
					t.Fatalf("error code = %q, want %q", got, tc.wantCode)
				}
			}
		})
	}
}

func TestPprofLoopbackOnly(t *testing.T) {
	_, h, _ := newTestRouter(t)
	tests := []struct {
		name       string
		remoteAddr string
		wantStatus int
	}{
		{"루프백 허용 (인증 불요)", "127.0.0.1:54321", http.StatusOK},
		{"IPv6 루프백 허용", "[::1]:54321", http.StatusOK},
		{"비루프백은 404", "192.0.2.1:1234", http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
			req.RemoteAddr = tc.remoteAddr
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			if rec.Code != tc.wantStatus {
				t.Fatalf("status = %d, want %d", rec.Code, tc.wantStatus)
			}
		})
	}
}

func TestCreateLink(t *testing.T) {
	_, h, _ := newTestRouter(t)

	// 신규 저장 → 201 {id, status, created_at}
	rec := do(t, h, http.MethodPost, "/api/v1/links", `{"url":"https://example.com/a","note":"메모"}`, testKey)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%q)", rec.Code, rec.Body.String())
	}
	var created struct {
		ID        int64  `json:"id"`
		Status    string `json:"status"`
		CreatedAt int64  `json:"created_at"`
	}
	decodeJSON(t, rec, &created)
	if created.Status != "pending" || created.ID == 0 || created.CreatedAt == 0 {
		t.Fatalf("201 응답 필드 이상: %+v", created)
	}
	// F8: created_at은 time.Now()가 아니라 store가 돌려준 실제 저장값이어야 한다
	// (fakeStore는 1000+id를 기록).
	if created.CreatedAt != 1000+created.ID {
		t.Fatalf("created_at = %d, want 저장값 %d", created.CreatedAt, 1000+created.ID)
	}

	// 같은 URL 재저장 → 200 {id, duplicate:true} (멱등)
	rec = do(t, h, http.MethodPost, "/api/v1/links", `{"url":"https://example.com/a"}`, testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("dup status = %d, want 200", rec.Code)
	}
	var dup struct {
		ID        int64 `json:"id"`
		Duplicate bool  `json:"duplicate"`
	}
	decodeJSON(t, rec, &dup)
	if !dup.Duplicate || dup.ID != created.ID {
		t.Fatalf("duplicate 응답 이상: %+v (want id=%d)", dup, created.ID)
	}

	// 잘못된 입력 → 400 invalid_input
	invalid := []struct{ name, body string }{
		{"url 누락", `{"note":"x"}`},
		{"상대 경로", `{"url":"notaurl"}`},
		{"스킴 불허", `{"url":"ftp://example.com/x"}`},
		{"본문 아님", `not-json`},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			rec := do(t, h, http.MethodPost, "/api/v1/links", tc.body, testKey)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body=%q)", rec.Code, rec.Body.String())
			}
			if got := errCode(t, rec); got != "invalid_input" {
				t.Fatalf("error code = %q, want invalid_input", got)
			}
		})
	}
}

func TestListLinksCursorPagination(t *testing.T) {
	fs, h, _ := newTestRouter(t)
	for i := 1; i <= 5; i++ {
		fs.addLink(fmt.Sprintf("https://example.com/%d", i), "done", int64(1000+i))
	}

	type page struct {
		Links []struct {
			ID int64 `json:"id"`
		} `json:"links"`
		NextCursor *string `json:"next_cursor"`
	}
	var ids []int64
	cursor := ""
	pages := 0
	for {
		target := "/api/v1/links?limit=2"
		if cursor != "" {
			target += "&cursor=" + cursor
		}
		rec := do(t, h, http.MethodGet, target, "", testKey)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d (body=%q)", rec.Code, rec.Body.String())
		}
		var p page
		decodeJSON(t, rec, &p)
		for _, l := range p.Links {
			ids = append(ids, l.ID)
		}
		pages++
		if p.NextCursor == nil {
			break
		}
		cursor = *p.NextCursor
		if pages > 10 {
			t.Fatal("페이지네이션이 끝나지 않음")
		}
	}
	if pages != 3 {
		t.Fatalf("pages = %d, want 3", pages)
	}
	want := []int64{5, 4, 3, 2, 1} // created_at DESC, id DESC
	if len(ids) != len(want) {
		t.Fatalf("ids = %v, want %v", ids, want)
	}
	for i := range want {
		if ids[i] != want[i] {
			t.Fatalf("ids = %v, want %v", ids, want)
		}
	}

	// 깨진 커서 → 400
	rec := do(t, h, http.MethodGet, "/api/v1/links?cursor=%2A%2A", "", testKey)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("깨진 커서 status = %d, want 400", rec.Code)
	}
}

func TestGetLinkDetail(t *testing.T) {
	// 상세 200: description은 절단 없이 전문, jobs는 scrape만 (tag/thumb 필드 부재).
	// 같은 링크가 목록에서는 description이 200룬(멀티바이트 안전)으로 절단된다.
	fs, h, _ := newTestRouter(t)
	id := fs.addLink("https://example.com/detail", "done", 3000)
	desc := strings.Repeat("가", 250) // 250 rune 멀티바이트(각 3바이트 UTF-8)
	fs.setDescription(id, desc)

	// --- 상세: 절단 없음 + jobs.scrape만 존재 ---
	rec := do(t, h, http.MethodGet, fmt.Sprintf("/api/v1/links/%d", id), "", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("상세 status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	var detail struct {
		Description string                     `json:"description"`
		Jobs        map[string]json.RawMessage `json:"jobs"`
	}
	decodeJSON(t, rec, &detail)
	if detail.Description != desc || utf8.RuneCountInString(detail.Description) != 250 {
		t.Fatalf("상세 description rune 수 = %d, want 250 (절단 없음)", utf8.RuneCountInString(detail.Description))
	}
	if _, ok := detail.Jobs["scrape"]; !ok {
		t.Fatalf("jobs.scrape 없음: %v", detail.Jobs)
	}
	if _, ok := detail.Jobs["tag"]; ok {
		t.Fatal("jobs.tag 존재 — 잡 없는 kind는 필드 생략이어야 함")
	}
	if _, ok := detail.Jobs["thumb"]; ok {
		t.Fatal("jobs.thumb 존재 — 잡 없는 kind는 필드 생략이어야 함")
	}

	// --- 목록: 같은 description이 200룬으로 절단, 멀티바이트 안전 ---
	rec = do(t, h, http.MethodGet, "/api/v1/links", "", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("목록 status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	var page struct {
		Links []struct {
			ID          int64  `json:"id"`
			Description string `json:"description"`
		} `json:"links"`
	}
	decodeJSON(t, rec, &page)
	found := false
	for _, l := range page.Links {
		if l.ID != id {
			continue
		}
		found = true
		if n := utf8.RuneCountInString(l.Description); n != 200 {
			t.Fatalf("목록 description rune 수 = %d, want 200 (절단)", n)
		}
		if !utf8.ValidString(l.Description) {
			t.Fatal("목록 description이 유효한 UTF-8이 아님 — 멀티바이트 경계 절단 깨짐")
		}
	}
	if !found {
		t.Fatalf("목록에서 id %d 미발견", id)
	}
}

func TestEmptyListSerializesAsArray(t *testing.T) {
	// 빈 결과의 links는 null이 아니라 [] — iOS 디코더가 옵셔널 처리 없이 안전하게 읽는다.
	_, h, _ := newTestRouter(t)

	rec := do(t, h, http.MethodGet, "/api/v1/links", "", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("목록 status = %d, want 200", rec.Code)
	}
	assertLinksEmptyArray(t, rec)

	// FTS 경로(q 3자 이상) 미매칭 → 빈 결과
	rec = do(t, h, http.MethodGet, "/api/v1/search?q=존재하지않는검색어", "", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("검색 status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	assertLinksEmptyArray(t, rec)
}

// assertLinksEmptyArray는 응답 JSON의 links가 정확히 [] (null·부재 아님)인지 단언한다.
func assertLinksEmptyArray(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	var m map[string]json.RawMessage
	decodeJSON(t, rec, &m)
	raw, ok := m["links"]
	if !ok {
		t.Fatalf("links 필드 없음: %s", rec.Body.String())
	}
	if string(raw) != "[]" {
		t.Fatalf("links = %s, want [] (null 아님)", raw)
	}
}

func TestUpdateLinkTagsReplace(t *testing.T) {
	fs, h, _ := newTestRouter(t)
	id := fs.addLink("https://example.com/u", "done", 2000)

	// 태그 전체 교체 → source=manual
	rec := do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/links/%d", id), `{"tags":["dev","golang"],"note":"바꾼 메모"}`, testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	var detail struct {
		Note string `json:"note"`
		Tags []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"tags"`
	}
	decodeJSON(t, rec, &detail)
	if detail.Note != "바꾼 메모" || len(detail.Tags) != 2 {
		t.Fatalf("PATCH 응답 이상: %+v", detail)
	}
	for _, tg := range detail.Tags {
		if tg.Source != "manual" {
			t.Fatalf("source = %q, want manual", tg.Source)
		}
	}

	// 사전에 없는 태그 → 400 invalid_input
	rec = do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/links/%d", id), `{"tags":["없는태그"]}`, testKey)
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_input" {
		t.Fatalf("unknown tag: status=%d code=%q, want 400 invalid_input", rec.Code, rec.Body.String())
	}

	// 없는 링크 → 404
	rec = do(t, h, http.MethodPatch, "/api/v1/links/9999", `{"note":"x"}`, testKey)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("없는 링크 status = %d, want 404", rec.Code)
	}
}

func TestDeleteThenGet(t *testing.T) {
	fs, h, _ := newTestRouter(t)
	id := fs.addLink("https://example.com/d", "done", 2000)

	rec := do(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/links/%d", id), "", testKey)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	rec = do(t, h, http.MethodGet, fmt.Sprintf("/api/v1/links/%d", id), "", testKey)
	if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Fatalf("삭제 후 조회: status=%d body=%q, want 404 not_found", rec.Code, rec.Body.String())
	}
	// 재삭제도 404 (소프트 삭제 후 부재 취급)
	rec = do(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/links/%d", id), "", testKey)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("재삭제 status = %d, want 404", rec.Code)
	}
}

func TestRetryLink(t *testing.T) {
	fs, h, _ := newTestRouter(t)
	failedID := fs.addLink("https://example.com/f", "failed", 2000)
	doneID := fs.addLink("https://example.com/ok", "done", 2001)

	rec := do(t, h, http.MethodPost, fmt.Sprintf("/api/v1/links/%d/retry", failedID), "", testKey)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("retry status = %d, want 202 (body=%q)", rec.Code, rec.Body.String())
	}
	var out struct {
		ID     int64  `json:"id"`
		Status string `json:"status"`
	}
	decodeJSON(t, rec, &out)
	if out.ID != failedID || out.Status != "pending" {
		t.Fatalf("202 응답 이상: %+v", out)
	}

	// failed가 아닌 링크 → 400
	rec = do(t, h, http.MethodPost, fmt.Sprintf("/api/v1/links/%d/retry", doneID), "", testKey)
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_input" {
		t.Fatalf("not-failed retry: status=%d, want 400 invalid_input", rec.Code)
	}
	// 없는 링크 → 404
	rec = do(t, h, http.MethodPost, "/api/v1/links/9999/retry", "", testKey)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("없는 링크 retry status = %d, want 404", rec.Code)
	}
}

func TestSearchModeBranch(t *testing.T) {
	fs, h, _ := newTestRouter(t)
	fs.addLink("https://example.com/s", "done", 2000) // title: "title https://example.com/s"

	// q 3자 이상 → fts, rank 존재
	rec := do(t, h, http.MethodGet, "/api/v1/search?q=title", "", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	var p struct {
		Mode  string `json:"mode"`
		Links []struct {
			Rank *float64 `json:"rank"`
		} `json:"links"`
	}
	decodeJSON(t, rec, &p)
	if p.Mode != "fts" || len(p.Links) != 1 || p.Links[0].Rank == nil {
		t.Fatalf("fts 응답 이상: mode=%q links=%+v", p.Mode, p.Links)
	}

	// q 3자 미만 → like 폴백, rank null
	rec = do(t, h, http.MethodGet, "/api/v1/search?q=ti", "", testKey)
	var p2 struct {
		Mode  string `json:"mode"`
		Links []struct {
			Rank *float64 `json:"rank"`
		} `json:"links"`
	}
	decodeJSON(t, rec, &p2)
	if p2.Mode != "like" || len(p2.Links) != 1 || p2.Links[0].Rank != nil {
		t.Fatalf("like 응답 이상: mode=%q links=%+v", p2.Mode, p2.Links)
	}

	// q 누락 → 400 (필수 파라미터)
	rec = do(t, h, http.MethodGet, "/api/v1/search", "", testKey)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("q 누락 status = %d, want 400", rec.Code)
	}
}

func TestSearchValidation(t *testing.T) {
	// F6+F12: TrimSpace 후 빈 q → 400 (스펙 minLength: 1), limit < 1 → 400,
	// limit > 100 → 100 클램프 (스펙 maximum: 100).
	fs, h, _ := newTestRouter(t)
	fs.addLink("https://example.com/v", "done", 2000)

	// 빈 q → 400 invalid_input (전량 스캔 경로 차단)
	for _, target := range []string{
		"/api/v1/search?q=",
		"/api/v1/search?q=%20%20", // 공백만 — TrimSpace 후 빈 문자열
	} {
		rec := do(t, h, http.MethodGet, target, "", testKey)
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_input" {
			t.Fatalf("%s: status=%d body=%q, want 400 invalid_input", target, rec.Code, rec.Body.String())
		}
	}

	// limit < 1 → 400 invalid_input (검색·목록 공통)
	for _, target := range []string{
		"/api/v1/search?q=title&limit=0",
		"/api/v1/search?q=title&limit=-1",
		"/api/v1/links?limit=0",
	} {
		rec := do(t, h, http.MethodGet, target, "", testKey)
		if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_input" {
			t.Fatalf("%s: status=%d body=%q, want 400 invalid_input", target, rec.Code, rec.Body.String())
		}
	}

	// limit > 100 → 100으로 클램프하고 200
	rec := do(t, h, http.MethodGet, "/api/v1/search?q=title&limit=500", "", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("limit 클램프 status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
}

func TestUpdateLinkTagsNullVsEmpty(t *testing.T) {
	// F7 확정 의미론: tags null/생략 = 유지, 빈 배열 [] = 전체 제거.
	fs, h, _ := newTestRouter(t)
	id := fs.addLink("https://example.com/nve", "done", 2000)

	// 태그 부착
	rec := do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/links/%d", id), `{"tags":["dev"]}`, testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("태그 부착 status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	var detail struct {
		Tags []struct {
			Name string `json:"name"`
		} `json:"tags"`
	}

	// null → 유지
	rec = do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/links/%d", id), `{"tags":null,"note":"n1"}`, testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("tags null status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	decodeJSON(t, rec, &detail)
	if len(detail.Tags) != 1 || detail.Tags[0].Name != "dev" {
		t.Fatalf("tags null 후 tags = %+v, want dev 유지", detail.Tags)
	}

	// 필드 생략 → 유지
	rec = do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/links/%d", id), `{"note":"n2"}`, testKey)
	decodeJSON(t, rec, &detail)
	if len(detail.Tags) != 1 {
		t.Fatalf("tags 생략 후 tags = %+v, want dev 유지", detail.Tags)
	}

	// 빈 배열 → 전체 제거
	rec = do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/links/%d", id), `{"tags":[]}`, testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("tags [] status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	decodeJSON(t, rec, &detail)
	if len(detail.Tags) != 0 {
		t.Fatalf("tags [] 후 tags = %+v, want 전체 제거", detail.Tags)
	}
}

func TestThumbsServingAndTraversal(t *testing.T) {
	_, h, dataDir := newTestRouter(t)

	// 픽스처: DATA_DIR/thumbs/aa/hash.jpg + 탈출 대상 DATA_DIR/secret.txt
	if err := os.MkdirAll(filepath.Join(dataDir, "thumbs", "aa"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "thumbs", "aa", "hash.jpg"), []byte("jpeg-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "secret.txt"), []byte("비밀"), 0o600); err != nil {
		t.Fatal(err)
	}

	// 정상 서빙 (인증 없음) + Cache-Control
	rec := do(t, h, http.MethodGet, "/thumbs/aa/hash.jpg", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != "jpeg-bytes" {
		t.Fatalf("본문 불일치: %q", rec.Body.String())
	}
	if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("Cache-Control = %q, immutable 캐시 헤더 없음", cc)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "image/jpeg" {
		t.Fatalf("Content-Type = %q, want image/jpeg", ct)
	}

	// 경로 탈출 시도 → 404 (thumbs 루트 밖 접근 차단)
	for _, target := range []string{
		"/thumbs/../secret.txt",
		"/thumbs/%2e%2e/secret.txt",
		"/thumbs/aa/..",
	} {
		t.Run(target, func(t *testing.T) {
			rec := do(t, h, http.MethodGet, target, "", "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("%s: status = %d, want 404 (body=%q)", target, rec.Code, rec.Body.String())
			}
		})
	}

	// 없는 파일 → 404 not_found
	rec = do(t, h, http.MethodGet, "/thumbs/bb/none.jpg", "", "")
	if rec.Code != http.StatusNotFound || errCode(t, rec) != "not_found" {
		t.Fatalf("없는 썸네일: status=%d body=%q", rec.Code, rec.Body.String())
	}
}

func TestTagsCRUD(t *testing.T) {
	_, h, _ := newTestRouter(t)

	// 생성 201
	rec := do(t, h, http.MethodPost, "/api/v1/tags", `{"name":"ml","aliases":["머신러닝"]}`, testKey)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	var tag struct {
		ID      int64    `json:"id"`
		Name    string   `json:"name"`
		Aliases []string `json:"aliases"`
	}
	decodeJSON(t, rec, &tag)
	if tag.Name != "ml" || len(tag.Aliases) != 1 {
		t.Fatalf("201 응답 이상: %+v", tag)
	}

	// 중복 이름 → 400
	rec = do(t, h, http.MethodPost, "/api/v1/tags", `{"name":"ML"}`, testKey)
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_input" {
		t.Fatalf("dup tag: status=%d, want 400 invalid_input", rec.Code)
	}

	// 수정 200
	rec = do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/tags/%d", tag.ID), `{"aliases":["머신러닝","ml옵스"]}`, testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status = %d (body=%q)", rec.Code, rec.Body.String())
	}

	// 삭제 204 → 재삭제 404
	rec = do(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/tags/%d", tag.ID), "", testKey)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	rec = do(t, h, http.MethodDelete, fmt.Sprintf("/api/v1/tags/%d", tag.ID), "", testKey)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("재삭제 status = %d, want 404", rec.Code)
	}
}

// apiTag는 태그 응답 디코드용 — facet이 계약상 required라 값 타입으로 받는다
// (누락되면 빈 문자열이 되어 단언에서 걸린다).
type apiTag struct {
	ID      int64    `json:"id"`
	Name    string   `json:"name"`
	Aliases []string `json:"aliases"`
	Facet   string   `json:"facet"`
}

// TestTagFacet은 계약(TagFacet: craft|media|life|neutral, default neutral)의
// HTTP 표면을 검증한다 — 생략 시 neutral, 생성 시 지정, 목록 포함, enum 밖 값은 400.
func TestTagFacet(t *testing.T) {
	_, h, _ := newTestRouter(t)

	// facet 생략 → 계약 default neutral
	rec := do(t, h, http.MethodPost, "/api/v1/tags", `{"name":"신규태그"}`, testKey)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	var created apiTag
	decodeJSON(t, rec, &created)
	if created.Facet != "neutral" {
		t.Fatalf("facet 생략 시 = %q, want neutral", created.Facet)
	}

	// 생성 시 facet 지정
	rec = do(t, h, http.MethodPost, "/api/v1/tags", `{"name":"팟캐스트","facet":"media"}`, testKey)
	if rec.Code != http.StatusCreated {
		t.Fatalf("facet 지정 create status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	var media apiTag
	decodeJSON(t, rec, &media)
	if media.Facet != "media" {
		t.Fatalf("생성 시 지정 facet = %q, want media", media.Facet)
	}

	// enum 밖 값 → 400 invalid_input (CHECK 제약이 500을 내기 전에 핸들러가 막는다)
	rec = do(t, h, http.MethodPost, "/api/v1/tags", `{"name":"엉터리","facet":"bogus"}`, testKey)
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_input" {
		t.Fatalf("잘못된 facet: status=%d body=%q, want 400 invalid_input", rec.Code, rec.Body.String())
	}

	// PATCH로 facet 교체
	rec = do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/tags/%d", media.ID), `{"facet":"life"}`, testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("facet 수정 status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	var updated apiTag
	decodeJSON(t, rec, &updated)
	if updated.Facet != "life" {
		t.Fatalf("수정 후 facet = %q, want life", updated.Facet)
	}

	// facet 생략 PATCH → 기존 값 유지 (aliases만 교체)
	rec = do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/tags/%d", media.ID), `{"aliases":["오디오"]}`, testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("aliases 수정 status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	decodeJSON(t, rec, &updated)
	if updated.Facet != "life" {
		t.Fatalf("facet 생략 수정 후 = %q, want life 유지", updated.Facet)
	}

	// PATCH도 enum 밖 값은 400
	rec = do(t, h, http.MethodPatch, fmt.Sprintf("/api/v1/tags/%d", media.ID), `{"facet":"rainbow"}`, testKey)
	if rec.Code != http.StatusBadRequest || errCode(t, rec) != "invalid_input" {
		t.Fatalf("잘못된 facet 수정: status=%d body=%q, want 400 invalid_input", rec.Code, rec.Body.String())
	}

	// 목록에 facet이 실린다 — 클라이언트는 이 응답만으로 Map<tagId, facet>을 만든다
	rec = do(t, h, http.MethodGet, "/api/v1/tags", "", testKey)
	if rec.Code != http.StatusOK {
		t.Fatalf("list status = %d (body=%q)", rec.Code, rec.Body.String())
	}
	var list []apiTag
	decodeJSON(t, rec, &list)
	if len(list) == 0 {
		t.Fatal("태그 목록이 비었다")
	}
	valid := map[string]bool{"craft": true, "media": true, "life": true, "neutral": true}
	got := map[string]string{}
	for _, tg := range list {
		if !valid[tg.Facet] {
			t.Fatalf("목록의 facet = %q (tag=%q), want enum 값", tg.Facet, tg.Name)
		}
		got[tg.Name] = tg.Facet
	}
	if got["dev"] != "craft" || got["신규태그"] != "neutral" || got["팟캐스트"] != "life" {
		t.Fatalf("목록 facet = %+v", got)
	}
}

// 클라이언트 캡처 필드는 핸들러에서 정제·절단된 뒤 store로 넘어가야 한다 — 상한 초과는
// 400이 아니라 절단이고(클라이언트가 경계를 서버와 맞출 수 없다), body_text만 개행을 남긴다.
func TestCreateLinkCleansCaptureFields(t *testing.T) {
	fs, h, _ := newTestRouter(t)
	key := testKey

	long := strings.Repeat("가", 20000) // 60000바이트 — body 상한(32KB) 초과
	payload, err := json.Marshal(map[string]string{
		"url":         "https://cap.example/a",
		"title":       "제목\x00제어\n줄바꿈",
		"description": "설명\t탭",
		"body_text":   "첫 문장이다.\n둘째 문장이다.\n" + long, // 개행은 앞쪽 — 절단 뒤에도 남아야 한다
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := do(t, h, http.MethodPost, "/api/v1/links", string(payload), key)
	if rec.Code != http.StatusCreated {
		t.Fatalf("상태 = %d, want 201 (본문: %s)", rec.Code, rec.Body.String())
	}

	in := fs.lastSave
	if strings.ContainsRune(in.Title, 0) {
		t.Errorf("제목에 제어문자가 남음: %q", in.Title)
	}
	if strings.Contains(in.Title, "\n") || strings.Contains(in.Description, "\t") {
		t.Errorf("제목·설명은 개행·탭을 접어야: %q / %q", in.Title, in.Description)
	}
	if !strings.Contains(in.BodyText, "\n") {
		t.Error("body_text는 개행을 유지해야 한다(요약이 문장 구분에 쓴다)")
	}
	if len(in.BodyText) > 32<<10 {
		t.Errorf("body_text가 상한을 넘김: %d바이트", len(in.BodyText))
	}
	if !utf8.ValidString(in.BodyText) {
		t.Error("절단이 룬 경계를 깼다")
	}
}

func (f *fakeStore) CorpusDF(ctx context.Context) (int64, map[string]int64, error) {
	return 0, nil, nil
}

func (f *fakeStore) MarkOpened(ctx context.Context, linkID int64) error { return nil }
