// Package api는 gen.StrictServerInterface(oapi-codegen strict-server)를 구현한다.
// 핸들러는 store/queue 호출과 타입 매핑만 담당 — 비즈니스 로직 없음.
// 시각은 store의 int64 epoch 초를 정수 그대로 내보낸다.
package api

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/coby/push-point/backend/internal/api/gen"
	"github.com/coby/push-point/backend/internal/store"
)

// Server는 StrictServerInterface 구현체. Store만 의존한다
// (큐는 store.SaveLink/RetryLink 트랜잭션 내부에서 결합 — store.go 참조).
type Server struct {
	store     store.Store
	thumbsDir string // DATA_DIR/thumbs 절대화된 루트 — 경로 탈출 검증 기준
	logger    *slog.Logger
}

// NewServer는 핸들러 구현체를 만든다. dataDir는 config.DataDir.
func NewServer(st store.Store, dataDir string, logger *slog.Logger) *Server {
	abs, err := filepath.Abs(filepath.Join(dataDir, "thumbs"))
	if err != nil {
		// Abs 실패는 cwd 소실 등 비정상 환경 — 상대 경로로라도 동작시키고 로그.
		logger.Error("thumbs 루트 절대화 실패", "err", err)
		abs = filepath.Clean(filepath.Join(dataDir, "thumbs"))
	}
	return &Server{store: st, thumbsDir: abs, logger: logger}
}

var _ gen.StrictServerInterface = (*Server)(nil)

// ---- 공용 매핑 헬퍼 ----

// apiErr는 {error:{code,message}} 공통 에러 바디를 만든다.
func apiErr(code gen.ErrorErrorCode, msg string) gen.Error {
	var e gen.Error
	e.Error.Code = code
	e.Error.Message = msg
	return e
}

// clampLimit는 limit 파라미터를 검증·보정한다 — 생략 시 기본 20,
// 1 미만은 400 invalid_input(스펙 minimum: 1), 100 초과는 100으로 클램프(스펙 maximum: 100).
func clampLimit(p *gen.Limit) (int, error) {
	if p == nil {
		return 20, nil
	}
	if *p < 1 {
		return 0, badRequestErr("limit must be >= 1")
	}
	if *p > 100 {
		return 100, nil
	}
	return *p, nil
}

func cursorOf(p *gen.Cursor) string {
	if p == nil {
		return ""
	}
	return *p
}

// nextCursorPtr는 빈 커서를 null로 매핑한다 (마지막 페이지).
func nextCursorPtr(c string) *string {
	if c == "" {
		return nil
	}
	return &c
}

func intPtr64(p *int64) *int {
	if p == nil {
		return nil
	}
	v := int(*p)
	return &v
}

func f32Ptr(p *float64) *float32 {
	if p == nil {
		return nil
	}
	v := float32(*p)
	return &v
}

// truncate200은 목록/검색용 description 200자(룬 기준) 절단.
func truncate200(s string) string {
	r := []rune(s)
	if len(r) <= 200 {
		return s
	}
	return string(r[:200])
}

// thumbURL은 thumb_path("aa/hash.jpg")를 서버 상대 경로 "/thumbs/aa/hash.jpg"로 변환.
func thumbURL(p *string) *string {
	if p == nil || *p == "" {
		return nil
	}
	u := "/thumbs/" + strings.TrimPrefix(*p, "/")
	return &u
}

func toAPITags(tags []store.LinkTag) []gen.LinkTag {
	out := make([]gen.LinkTag, 0, len(tags))
	for _, t := range tags {
		out = append(out, gen.LinkTag{
			Id:         int(t.ID),
			Name:       t.Name,
			Source:     gen.LinkTagSource(t.Source),
			Confidence: f32Ptr(t.Confidence),
		})
	}
	return out
}

func toAPILink(l store.Link) gen.Link {
	return gen.Link{
		Id:          int(l.ID),
		Url:         l.URL,
		Domain:      l.Domain,
		Title:       l.Title,
		Description: truncate200(l.Description),
		ContentType: gen.ContentType(l.ContentType),
		ThumbUrl:    thumbURL(l.ThumbPath),
		Status:      gen.LinkStatus(l.Status),
		Tags:        toAPITags(l.Tags),
		Note:        l.Note,
		CreatedAt:   int(l.CreatedAt),
	}
}

func toAPIDetail(d *store.LinkDetail) gen.LinkDetail {
	out := gen.LinkDetail{
		Id:          int(d.ID),
		Url:         d.URL,
		Domain:      d.Domain,
		Title:       d.Title,
		Description: d.Description, // 상세는 절단 없음
		ContentType: gen.ContentType(d.ContentType),
		ThumbUrl:    thumbURL(d.ThumbPath),
		Status:      gen.LinkStatus(d.Status),
		Tags:        toAPITags(d.Tags),
		Note:        d.Note,
		CreatedAt:   int(d.CreatedAt),
		Author:      d.Author,
		PublishedAt: intPtr64(d.PublishedAt),
		DurationSec: intPtr64(d.DurationSec),
		WordCount:   intPtr64(d.WordCount),
		Lang:        d.Lang,
		Summary:     d.Summary, // 상세 전용 — 목록·검색 매핑에는 없다(계약이 그렇게 좁다)
		Error:       d.Error,
	}
	// 잡이 아직 없는 kind는 store가 빈 문자열을 준다 → 계약상 필드 생략(nil).
	// scrape 잡은 저장 트랜잭션에서 항상 생성되므로 필수 필드.
	out.Jobs.Scrape = gen.JobStatus(d.Jobs.Scrape)
	out.Jobs.Tag = jobStatusPtr(d.Jobs.Tag)
	out.Jobs.Thumb = jobStatusPtr(d.Jobs.Thumb)
	return out
}

// jobStatusPtr는 잡 미존재(빈 문자열)를 nil로 바꾼다 — enum 밖 값("")이 응답에 나가지 않게.
func jobStatusPtr(s string) *gen.JobStatus {
	if s == "" {
		return nil
	}
	v := gen.JobStatus(s)
	return &v
}

func toAPITag(t *store.Tag) gen.Tag {
	aliases := t.Aliases
	if aliases == nil {
		aliases = []string{}
	}
	facet := gen.TagFacet(t.Facet)
	if !facet.Valid() {
		// 저장된 값이 enum 밖일 수 없지만(tags.facet CHECK), 계약 밖 값을 응답에
		// 흘리는 것보다 default로 접는 쪽이 안전하다.
		facet = gen.Neutral
	}
	return gen.Tag{
		Id:        int(t.ID),
		Name:      t.Name,
		Aliases:   aliases,
		Facet:     facet,
		LinkCount: int(t.LinkCount),
	}
}

// facetPtr는 요청 바디의 optional facet을 store 인자(*string)로 바꾼다.
// nil이면 nil (생성=계약 default neutral, 수정=기존 값 유지), enum 밖 값이면 ok=false → 400.
// JSON 디코더는 enum을 검증하지 않으므로 여기가 CHECK 제약 위반(=500)을 400으로 바꾸는 지점이다.
func facetPtr(f *gen.TagFacet) (*string, bool) {
	if f == nil {
		return nil, true
	}
	if !f.Valid() {
		return nil, false
	}
	v := string(*f)
	return &v, true
}

// ---- 오퍼레이션 구현 (14개) ----

// Healthz — 인증 면제, 생존 확인.
func (s *Server) Healthz(ctx context.Context, request gen.HealthzRequestObject) (gen.HealthzResponseObject, error) {
	return gen.Healthz200JSONResponse{Status: gen.Ok}, nil
}

// CreateLink — INSERT links + scrape 잡을 한 트랜잭션으로 (store 책임). 신규 201, 중복 200.
func (s *Server) CreateLink(ctx context.Context, request gen.CreateLinkRequestObject) (gen.CreateLinkResponseObject, error) {
	if request.Body == nil {
		return gen.CreateLink400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "url is required"))}, nil
	}
	deref := func(p *string) string {
		if p == nil {
			return ""
		}
		return *p
	}
	// 여기는 HTTP 표현 → 저장 입력 매핑만 한다. **검증·정제는 store.SaveInput.Normalize가**
	// 하므로 HTTP가 아닌 진입점(임베드 iOS의 로컬 큐 드레인)도 같은 규칙을 받는다.
	in := store.SaveInput{
		URL:         request.Body.Url,
		Note:        deref(request.Body.Note),
		Title:       deref(request.Body.Title),
		Description: deref(request.Body.Description),
		BodyText:    deref(request.Body.BodyText),
	}
	id, createdAt, duplicate, err := s.store.SaveLink(ctx, in)
	if errors.Is(err, store.ErrInvalidURL) {
		return gen.CreateLink400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "url must be absolute http(s)"))}, nil
	}
	if err != nil {
		return nil, err // → 500 internal (server.go의 ResponseErrorHandlerFunc)
	}
	if duplicate {
		return gen.CreateLink200JSONResponse{Id: int(id), Duplicate: gen.True}, nil
	}
	// created_at은 INSERT ... RETURNING이 돌려준 실제 저장값 —
	// 상세 재조회 없이 응답 (동기 경로 최소화, p99 < 50ms).
	return gen.CreateLink201JSONResponse{
		Id:        int(id),
		Status:    gen.LinkStatusPending,
		CreatedAt: int(createdAt),
	}, nil
}

// ListLinks — keyset 커서 목록. 커서 형식 오류는 400.
func (s *Server) ListLinks(ctx context.Context, request gen.ListLinksRequestObject) (gen.ListLinksResponseObject, error) {
	tag, status := "", ""
	if request.Params.Tag != nil {
		tag = *request.Params.Tag
	}
	if request.Params.Status != nil {
		if !request.Params.Status.Valid() {
			return nil, badRequestErr("invalid status filter")
		}
		status = string(*request.Params.Status)
	}
	limit, err := clampLimit(request.Params.Limit)
	if err != nil {
		return nil, err
	}
	items, next, err := s.store.ListLinks(ctx, cursorOf(request.Params.Cursor), limit, tag, status)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			return nil, badRequestErr("invalid cursor")
		}
		return nil, err
	}
	links := make([]gen.Link, 0, len(items))
	for _, l := range items {
		links = append(links, toAPILink(l))
	}
	return gen.ListLinks200JSONResponse(gen.LinkPage{Links: links, NextCursor: nextCursorPtr(next)}), nil
}

// GetLink — 상세 조회.
func (s *Server) GetLink(ctx context.Context, request gen.GetLinkRequestObject) (gen.GetLinkResponseObject, error) {
	d, err := s.store.GetLink(ctx, int64(request.Id))
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return gen.GetLink404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "link not found"))}, nil
		}
		return nil, err
	}
	return gen.GetLink200JSONResponse(toAPIDetail(d)), nil
}

// UpdateLink — note 교체 / tags 전체 교체 (feedback 기록은 store 책임).
func (s *Server) UpdateLink(ctx context.Context, request gen.UpdateLinkRequestObject) (gen.UpdateLinkResponseObject, error) {
	if request.Body == nil {
		return gen.UpdateLink400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "body is required"))}, nil
	}
	// tags 의미론 (openapi.yaml updateLink 확정): null이거나 필드 생략 = 유지
	// (json이 *[]string을 nil로 두므로 이 분기를 타지 않음), 빈 배열 [] = 전체 제거.
	var tags []string
	if request.Body.Tags != nil {
		tags = *request.Body.Tags
	}
	d, err := s.store.UpdateLink(ctx, int64(request.Id), request.Body.Note, tags)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			return gen.UpdateLink404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "link not found"))}, nil
		case errors.Is(err, store.ErrUnknownTag):
			return gen.UpdateLink400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "unknown tag name"))}, nil
		}
		return nil, err
	}
	return gen.UpdateLink200JSONResponse(toAPIDetail(d)), nil
}

// DeleteLink — 소프트 삭제, 204.
func (s *Server) DeleteLink(ctx context.Context, request gen.DeleteLinkRequestObject) (gen.DeleteLinkResponseObject, error) {
	if err := s.store.DeleteLink(ctx, int64(request.Id)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return gen.DeleteLink404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "link not found"))}, nil
		}
		return nil, err
	}
	return gen.DeleteLink204Response{}, nil
}

// RetryLink — failed 링크 재-enqueue, 202.
func (s *Server) RetryLink(ctx context.Context, request gen.RetryLinkRequestObject) (gen.RetryLinkResponseObject, error) {
	if err := s.store.RetryLink(ctx, int64(request.Id)); err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			return gen.RetryLink404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "link not found"))}, nil
		case errors.Is(err, store.ErrNotFailed):
			return gen.RetryLink400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "link is not in failed status"))}, nil
		}
		return nil, err
	}
	return gen.RetryLink202JSONResponse{Id: request.Id, Status: gen.LinkStatusPending}, nil
}

// Search — q 3자 이상 FTS5(mode=fts) / 미만 LIKE 폴백(mode=like). 분기는 store 책임.
// TrimSpace 후 빈 q는 400 invalid_input (스펙 minLength: 1 — 전량 스캔 경로 차단).
func (s *Server) Search(ctx context.Context, request gen.SearchRequestObject) (gen.SearchResponseObject, error) {
	q := strings.TrimSpace(request.Params.Q)
	if q == "" {
		return gen.Search400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "q must not be empty"))}, nil
	}
	limit, err := clampLimit(request.Params.Limit)
	if err != nil {
		return nil, err
	}
	tag := ""
	if request.Params.Tag != nil {
		tag = *request.Params.Tag
	}
	var from, to *int64
	if request.Params.From != nil {
		v := int64(*request.Params.From)
		from = &v
	}
	if request.Params.To != nil {
		v := int64(*request.Params.To)
		to = &v
	}
	items, next, mode, err := s.store.Search(ctx, q, tag, from, to, cursorOf(request.Params.Cursor), limit)
	if err != nil {
		if errors.Is(err, store.ErrInvalidCursor) {
			return nil, badRequestErr("invalid cursor")
		}
		return nil, err
	}
	links := make([]gen.SearchResult, 0, len(items))
	for _, it := range items {
		l := toAPILink(it.Link)
		links = append(links, gen.SearchResult{
			Id:          l.Id,
			Url:         l.Url,
			Domain:      l.Domain,
			Title:       l.Title,
			Description: l.Description,
			ContentType: l.ContentType,
			ThumbUrl:    l.ThumbUrl,
			Status:      l.Status,
			Tags:        l.Tags,
			Note:        l.Note,
			CreatedAt:   l.CreatedAt,
			Rank:        f32Ptr(it.Rank),
		})
	}
	return gen.Search200JSONResponse(gen.SearchPage{
		Links:      links,
		Mode:       gen.SearchPageMode(mode),
		NextCursor: nextCursorPtr(next),
	}), nil
}

// GetStats — 위젯용 통계.
func (s *Server) GetStats(ctx context.Context, request gen.GetStatsRequestObject) (gen.GetStatsResponseObject, error) {
	st, err := s.store.Stats(ctx)
	if err != nil {
		return nil, err
	}
	out := gen.Stats{
		TotalLinks:    int(st.TotalLinks),
		LinksThisWeek: int(st.LinksThisWeek),
	}
	out.ByTag = make([]struct {
		Count int    `json:"count"`
		Name  string `json:"name"`
	}, 0, len(st.ByTag))
	for _, t := range st.ByTag {
		out.ByTag = append(out.ByTag, struct {
			Count int    `json:"count"`
			Name  string `json:"name"`
		}{Count: int(t.Count), Name: t.Name})
	}
	out.ByDay = make([]struct {
		Count int    `json:"count"`
		Date  string `json:"date"`
	}, 0, len(st.ByDay))
	for _, d := range st.ByDay {
		out.ByDay = append(out.ByDay, struct {
			Count int    `json:"count"`
			Date  string `json:"date"`
		}{Count: int(d.Count), Date: d.Date})
	}
	return gen.GetStats200JSONResponse(out), nil
}

// ListTags — 태그 사전 + link_count.
func (s *Server) ListTags(ctx context.Context, request gen.ListTagsRequestObject) (gen.ListTagsResponseObject, error) {
	tags, err := s.store.ListTags(ctx)
	if err != nil {
		return nil, err
	}
	out := make(gen.ListTags200JSONResponse, 0, len(tags))
	for i := range tags {
		out = append(out, toAPITag(&tags[i]))
	}
	return out, nil
}

// CreateTag — 201. 이름 중복(NOCASE)은 400. facet 생략 시 neutral.
func (s *Server) CreateTag(ctx context.Context, request gen.CreateTagRequestObject) (gen.CreateTagResponseObject, error) {
	if request.Body == nil || strings.TrimSpace(request.Body.Name) == "" {
		return gen.CreateTag400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "name is required"))}, nil
	}
	var aliases []string
	if request.Body.Aliases != nil {
		aliases = *request.Body.Aliases
	}
	fp, ok := facetPtr(request.Body.Facet)
	if !ok {
		return gen.CreateTag400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "invalid facet"))}, nil
	}
	facet := "" // 생략 = 계약 default (store가 neutral로 접는다)
	if fp != nil {
		facet = *fp
	}
	t, err := s.store.CreateTag(ctx, strings.TrimSpace(request.Body.Name), aliases, facet)
	if err != nil {
		if errors.Is(err, store.ErrDuplicateTag) {
			return gen.CreateTag400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "duplicate tag name"))}, nil
		}
		return nil, err
	}
	return gen.CreateTag201JSONResponse(toAPITag(t)), nil
}

// UpdateTag — name/aliases/facet 각각 optional (전달한 필드만 교체).
func (s *Server) UpdateTag(ctx context.Context, request gen.UpdateTagRequestObject) (gen.UpdateTagResponseObject, error) {
	if request.Body == nil {
		return gen.UpdateTag400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "body is required"))}, nil
	}
	var aliases []string
	if request.Body.Aliases != nil {
		aliases = *request.Body.Aliases
		if aliases == nil {
			aliases = []string{}
		}
	}
	facet, ok := facetPtr(request.Body.Facet)
	if !ok {
		return gen.UpdateTag400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "invalid facet"))}, nil
	}
	t, err := s.store.UpdateTag(ctx, int64(request.Id), request.Body.Name, aliases, facet)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrNotFound):
			return gen.UpdateTag404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "tag not found"))}, nil
		case errors.Is(err, store.ErrDuplicateTag):
			return gen.UpdateTag400JSONResponse{BadRequestJSONResponse: gen.BadRequestJSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "duplicate tag name"))}, nil
		}
		return nil, err
	}
	return gen.UpdateTag200JSONResponse(toAPITag(t)), nil
}

// DeleteTag — 204. link_tags 등은 FK CASCADE (store/DB 책임).
func (s *Server) DeleteTag(ctx context.Context, request gen.DeleteTagRequestObject) (gen.DeleteTagResponseObject, error) {
	if err := s.store.DeleteTag(ctx, int64(request.Id)); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return gen.DeleteTag404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "tag not found"))}, nil
		}
		return nil, err
	}
	return gen.DeleteTag204Response{}, nil
}

// GetThumb — DATA_DIR/thumbs 하위만 서빙. filepath.Clean + 루트 접두 검증으로
// "../" 경로 탈출을 차단한다. 인증 면제 (iOS AsyncImage가 커스텀 헤더 미지원).
func (s *Server) GetThumb(ctx context.Context, request gen.GetThumbRequestObject) (gen.GetThumbResponseObject, error) {
	full := filepath.Clean(filepath.Join(s.thumbsDir, request.Dir, request.File))
	if !strings.HasPrefix(full, s.thumbsDir+string(os.PathSeparator)) {
		// 탈출 시도는 존재 여부를 흘리지 않도록 동일하게 404.
		return gen.GetThumb404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "thumb not found"))}, nil
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return gen.GetThumb404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "thumb not found"))}, nil
		}
		return nil, err
	}
	fi, err := f.Stat()
	if err != nil || fi.IsDir() {
		f.Close()
		if err != nil {
			return nil, err
		}
		return gen.GetThumb404JSONResponse{NotFoundJSONResponse: gen.NotFoundJSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "thumb not found"))}, nil
	}
	// Body가 ReadCloser면 Visit 쪽에서 Close해 준다 (*os.File 해당).
	return gen.GetThumb200ImagejpegResponse{Body: f, ContentLength: fi.Size()}, nil
}

// badRequestErr — strict 핸들러의 에러 반환 경로에서 400으로 매핑되는 센티널.
// (server.go의 ResponseErrorHandlerFunc가 errors.Is로 판별)
type badRequest struct{ msg string }

func (e badRequest) Error() string { return e.msg }

func badRequestErr(msg string) error { return badRequest{msg: msg} }
