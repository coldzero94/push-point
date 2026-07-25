package ppshare

import (
	"encoding/json"
	"os/exec"
	"strings"
	"testing"
)

// body는 요약 가드(MinBodyRunes=200, MinProseSents=3)를 통과하고 시드 사전의
// kubernetes·golang에 걸리는 산문을 만든다.
const body = "쿠버네티스 클러스터에서 파드를 배포하고 롤링 업데이트를 수행하는 방법을 설명한다. " +
	"golang으로 작성한 컨트롤러가 리소스 변화를 감지해 원하는 상태로 수렴시키는 과정을 살펴본다. " +
	"서비스 오브젝트가 파드 집합에 안정적인 네트워크 엔드포인트를 부여하는 원리를 다룬다. " +
	"오토스케일러가 관측 지표를 기준으로 레플리카 수를 조정하는 과정을 정리한다. " +
	"인그레스 컨트롤러가 외부 트래픽을 클러스터 내부로 라우팅하는 흐름을 따라간다."

func save(t *testing.T, p map[string]string) result {
	t.Helper()
	in, err := json.Marshal(p)
	if err != nil {
		t.Fatal(err)
	}
	out, err := Save(string(in))
	if err != nil {
		t.Fatalf("Save 실패: %v", err)
	}
	var r result
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatalf("결과 JSON 파싱 실패 (%q): %v", out, err)
	}
	return r
}

// Save의 반환 JSON은 **Swift가 키로 디코드하는 계약**이다. 그런데 위 save() 헬퍼는
// 같은 Go 구조체로 마샬·언마샬하므로 키 이름이 바뀌어도 전부 통과한다 — 실제로 세 키를
// 바꿔 보니 5개 테스트가 그대로 녹색이었다. 그래서 키를 문자열로 직접 확인한다.
func TestSaveResultJSONKeys(t *testing.T) {
	if err := Open(t.TempDir()); err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	defer Close() //nolint:errcheck // 테스트 정리

	in, err := json.Marshal(map[string]string{"url": "https://example.com/keys", "body_text": body})
	if err != nil {
		t.Fatal(err)
	}
	out, err := Save(string(in))
	if err != nil {
		t.Fatalf("Save 실패: %v", err)
	}
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("결과가 JSON 객체가 아니다 (%q): %v", out, err)
	}
	// 성공 경로에 반드시 있어야 하는 키. tag_error·summary_error는 omitempty라 여기 없다.
	for _, key := range []string{"id", "created_at", "duplicate", "tags", "tag_names", "summary_len"} {
		if _, ok := got[key]; !ok {
			t.Errorf("결과에 %q 키가 없다 — Swift 디코더가 조용히 0/false를 읽는다: %s", key, out)
		}
	}
	if _, ok := got["tag_error"]; ok {
		t.Errorf("성공 경로에는 tag_error가 없어야 한다(omitempty): %s", out)
	}
}

// 확장의 핵심 시나리오 — 서버 없이, 저장과 동시에 태그·요약까지 붙는다.
func TestSaveTagsAndSummarizesInline(t *testing.T) {
	if err := Open(t.TempDir()); err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	defer Close() //nolint:errcheck // 테스트 정리

	r := save(t, map[string]string{
		"url": "https://example.com/k8s", "title": "쿠버네티스 배포 전략",
		"description": "짧은 설명", "body_text": body,
	})
	if r.ID == 0 || r.CreatedAt == 0 {
		t.Errorf("id·created_at이 채워져야 한다: %+v", r)
	}
	if r.Duplicate {
		t.Error("첫 저장은 duplicate가 아니어야 한다")
	}
	if r.TagError != "" {
		t.Errorf("태깅이 실패했다: %s", r.TagError)
	}
	// 본문에 kubernetes·golang이 있으므로 시드 사전에 반드시 걸린다.
	if r.Tags == 0 {
		t.Error("태그가 하나도 안 붙었다 — 인라인 태깅이 동작하지 않는다")
	}
	// 확장 UI가 칩으로 그리므로 이름이 개수와 맞아야 한다.
	if len(r.TagNames) != r.Tags {
		t.Errorf("태그 이름 수(%d)가 개수(%d)와 다르다 — UI가 빈 칩을 그린다", len(r.TagNames), r.Tags)
	}
	// 산문 5문장이라 요약 가드를 통과해야 한다.
	if r.SummaryLen == 0 {
		t.Error("요약이 비었다 — 인라인 요약이 동작하지 않는다")
	}
}

// pendingJobKinds는 링크에 남아 있는 미처리 잡의 kind별 개수를 센다.
// done인 잡은 세지 않는다 — 이미 끝난 잡은 실패를 되살릴 수단이 아니다.
func pendingJobKinds(t *testing.T, linkID int64) map[string]int {
	t.Helper()
	rows, err := db.Reader.Query(
		`SELECT kind, COUNT(*) FROM jobs WHERE link_id = ?
		   AND status IN ('pending','running') GROUP BY kind`, linkID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close() //nolint:errcheck // 테스트
	out := map[string]int{}
	for rows.Next() {
		var kind string
		var n int
		if err := rows.Scan(&kind, &n); err != nil {
			t.Fatal(err)
		}
		out[kind] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// 인라인 태깅이 실패했을 때의 안전망이 경로마다 다르다는 사실을 고정한다.
// Save의 문서 주석이 이 구분에 의존하므로, SaveLink의 enqueue 조건이 바뀌면
// 주석이 조용히 거짓이 되는 대신 여기서 걸려야 한다.
func TestSaveTagJobFallbackByPath(t *testing.T) {
	if err := Open(t.TempDir()); err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	defer Close() //nolint:errcheck // 테스트 정리

	// 본문이 있으면 tag 잡이 함께 들어간다 — 인라인 태깅 실패를 워커가 되살릴 수 있다.
	withBody := save(t, map[string]string{"url": "https://example.com/with-body", "body_text": body})
	if got := pendingJobKinds(t, withBody.ID); got["tag"] == 0 {
		t.Errorf("본문이 있으면 tag 잡이 있어야 한다: %v", got)
	}

	// 본문이 없으면 tag 잡은 없고 scrape 잡만 있다. 태그는 스크랩이 성공해야
	// ApplyScrape가 만든다 — 스크랩이 끝내 실패하면 태그 없이 남는다.
	noBody := save(t, map[string]string{"url": "https://example.com/no-body", "title": "제목만"})
	got := pendingJobKinds(t, noBody.ID)
	if got["tag"] != 0 {
		t.Errorf("본문이 없으면 tag 잡이 없어야 한다(현재 동작): %v", got)
	}
	if got["scrape"] == 0 {
		t.Errorf("본문이 없으면 최소한 scrape 잡은 있어야 한다 — 없으면 태그 경로가 완전히 끊긴다: %v", got)
	}

	// 재공유(중복): 저장된 본문이 이미 client 출처면 SaveLink가 곧바로 반환하므로
	// 본문을 다시 실어 보내도 잡이 생기지 않는다. 같은 페이지를 또 공유하는 평범한
	// 동작이라 이 경로가 조용히 바뀌면 안 된다.
	before := pendingJobKinds(t, withBody.ID)
	again := save(t, map[string]string{"url": "https://example.com/with-body", "body_text": body})
	if !again.Duplicate {
		t.Fatal("같은 URL 재공유는 duplicate여야 한다")
	}
	after := pendingJobKinds(t, withBody.ID)
	if after["tag"] != before["tag"] || after["scrape"] != before["scrape"] {
		t.Errorf("재공유가 잡을 새로 만들었다 — Save 주석의 세 번째 경로 설명이 어긋난다: %v → %v", before, after)
	}
}

func TestSaveDuplicate(t *testing.T) {
	if err := Open(t.TempDir()); err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	defer Close() //nolint:errcheck // 테스트 정리

	p := map[string]string{"url": "https://example.com/dup", "title": "제목", "body_text": body}
	first := save(t, p)
	second := save(t, p)
	if second.Duplicate != true {
		t.Error("같은 URL 재저장은 duplicate=true여야 한다")
	}
	if second.ID != first.ID {
		t.Errorf("중복은 기존 id를 돌려줘야 한다: %d != %d", second.ID, first.ID)
	}
}

func TestSaveBeforeOpen(t *testing.T) {
	_ = Close() // 앞선 테스트의 핸들이 남아 있으면 정리
	if _, err := Save(`{"url":"https://example.com/x"}`); err == nil {
		t.Error("Open 없이 Save하면 에러여야 한다")
	}
	if err := Close(); err == nil {
		t.Error("Open 없이 Close하면 에러여야 한다")
	}
}

func TestSaveRejectsBadInput(t *testing.T) {
	if err := Open(t.TempDir()); err != nil {
		t.Fatalf("Open 실패: %v", err)
	}
	defer Close() //nolint:errcheck // 테스트 정리

	for _, c := range []struct{ name, in string }{
		{"JSON이 아님", `not json`},
		{"URL 없음", `{"title":"제목만"}`},
		{"http(s)가 아님", `{"url":"ftp://example.com/x"}`},
	} {
		if _, err := Save(c.in); err == nil {
			t.Errorf("%s: 에러여야 한다", c.name)
		}
	}
}

// 이 패키지의 존재 이유를 지키는 테스트다. scraper를 실수로 import하면 확장의
// RSS가 13.4MB → 64.2MB로 뛰어 메모리 예산을 위협하는데, 그건 **빌드도 테스트도
// 통과하면서** 조용히 벌어진다. 그래서 의존성 그래프를 직접 본다.
func TestExtensionBudget_noHeavyDeps(t *testing.T) {
	// go가 아예 없을 때만 건너뛴다. 아무 exec 오류에나 Skip을 걸면 빌드가 깨져
	// go list가 실패하는 순간 이 가드가 조용히 사라진다 — 그건 이 테스트가 막으려는
	// 실패 양상과 정확히 같다(CI에는 항상 툴체인이 있다).
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go 툴체인 없음 — 건너뜀: %v", err)
	}
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatalf("go list -deps 실패 — 의존성 가드를 확인할 수 없다: %v", err)
	}
	// scraper 자신과, scraper만 끌고 오는 무거운 라이브러리들.
	banned := []string{"internal/scraper", "trafilatura", "go-readability", "domdistiller", "goquery"}
	for _, dep := range strings.Split(string(out), "\n") {
		for _, b := range banned {
			if strings.Contains(dep, b) {
				t.Errorf("확장 바인드가 %q를 링크한다 — RSS가 ~50MB 늘어 메모리 예산을 위협한다 (경유: %s)", b, dep)
			}
		}
	}
}
