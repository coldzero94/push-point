// Package ppshare는 iOS Share Extension이 쓰는 **최소** 바인드 표면이다.
//
// 왜 mobile/ppcore와 따로 있나 — 확장의 메모리 예산(~120MB) 때문이다. scraper는
// 호출하지 않고 링크만 해도 패키지 init()이 정규식·셀렉터 테이블을 만들어 RSS를
// 13.4MB → 64.2MB로 올린다(실측: 20,000건·97MB DB 콜드 저장 기준). 확장이 시작부터
// 64MB를 깔고 앉으면 UI와 시스템 오버헤드를 얹었을 때 jetsam 사정권이다.
// 그래서 이 패키지는 **scraper를 import하지 않는다** — 이것이 유일하고 중요한 불변식이다.
//
// 대신 tagger·summarizer는 RSS에 잡히지 않아(측정 오차 내, store+queue 기준) 그대로 넣었다.
// 덕분에 확장은 공유 버튼을 누른 자리에서 **오프라인으로** 저장을 끝낸다. 다만 얼마나
// 채워지는지는 공유 출처에 달렸다(04 §7.3.1):
//
//   - 사파리 공유는 JS 전처리기가 캡처 규칙을 돌려 본문을 함께 주므로 태그·요약이 다 붙는다.
//   - 네이티브 앱 공유는 대개 URL뿐이라 요약 가드를 통과하지 못하고, 태깅도 도메인·제목
//     수준으로 약해진다. 본문 보강은 나중에 앱이 열렸을 때 scrape가 맡는다.
//
// 확장이 scraper를 링크하지 않아도 되는 이유는 이 둘 중 어느 쪽도 확장 안에서 HTML을
// 파싱할 필요가 없기 때문이다 — 본문은 캡처 규칙이 주거나, 나중에 서버/본체가 가져온다.
//
// 페이로드는 HTTP 계약(api/openapi.yaml의 LinkInput)과 **같은 JSON**이다. 저장의 단위가
// HTTP 호출이 아니라 페이로드라서, 같은 문자열이 서버 모드에서는 POST 본문으로,
// 자립 모드에서는 이 함수의 인자로 간다(docs/v2/04-DATA-FLOW.md §7.4).
package ppshare

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/coby/push-point/backend/internal/queue"
	"github.com/coby/push-point/backend/internal/store"
	"github.com/coby/push-point/backend/internal/tagjob"
)

// saveTimeout은 Save 한 번의 상한. 전부 로컬 SQLite + 순수 CPU라 정상 경로는 밀리초
// 단위지만, 확장은 시스템이 언제든 죽일 수 있는 짧은 수명이라 무한 대기를 만들지 않는다.
const saveTimeout = 15 * time.Second

// gomobile은 Swift에서 임의 스레드로 호출될 수 있으므로 전역 핸들을 뮤텍스로 감싼다.
// 확장 프로세스는 수명이 짧아 핸들 하나면 충분하다.
var (
	mu sync.Mutex
	db *store.DB
	st store.Store
)

// errNotOpen은 Open 없이 Save/Close를 부른 경우.
var errNotOpen = errors.New("ppshare: Open이 먼저 호출돼야 한다")

// payload는 저장 요청 — api/openapi.yaml의 LinkInput과 같은 키를 쓴다.
type payload struct {
	URL         string `json:"url"`
	Note        string `json:"note"`
	Title       string `json:"title"`
	Description string `json:"description"`
	BodyText    string `json:"body_text"`
}

// result는 Save의 반환 JSON. 확장 UI가 "저장됨/이미 있음"을 구분하고 태그 수를 보여줄 수 있게 한다.
//
// 이 구조는 HTTP 저장 응답(api/openapi.yaml)과 **같지 않다** — 201/200 두 갈래를 하나로
// 합치고, status를 빼고, tags·summary_len·tag_error를 더한다. 확장은 인라인 태깅 결과를
// 그 자리에서 보여줘야 하는데 API는 그 값을 돌려주지 않기 때문이다. 즉 "자립 모드와 서버
// 모드가 같은 JSON"이라는 원칙의 의도된 예외이고, 적용 범위는 **입력 페이로드**다.
type result struct {
	ID        int64 `json:"id"`
	CreatedAt int64 `json:"created_at"`
	Duplicate bool  `json:"duplicate"`
	Tags      int   `json:"tags"`
	// TagNames는 붙은 태그 이름. 확장 UI가 "서버 없이 태그가 붙었다"를 그 자리에서
	// 보여주기 위해 필요하다 — 개수만으로는 무엇이 붙었는지 알 수 없다.
	TagNames   []string `json:"tag_names"`
	SummaryLen int      `json:"summary_len"`
	// TagError는 **태깅 자체가 실패**했을 때만 채워진다 — 이 경우 Tags는 0이고, 링크는
	// 태그 없이 저장된 것이다.
	TagError string `json:"tag_error,omitempty"`
	// SummaryError는 태깅은 성공했지만 요약 기록만 실패했을 때 채워진다. TagError와
	// 나눠 두는 이유: 한 필드에 담으면 "태그가 아예 없다"와 "태그는 멀쩡한데 요약이 없다"를
	// 호출자가 Tags 값으로 역추론해야 하고, 그 규칙은 어디에도 적혀 있지 않게 된다.
	// 특히 본문 없이 저장된 링크는 재시도 잡이 없어(Save 주석 참조) 태깅 실패가 영구적이라,
	// 두 경우의 무게가 다르다.
	SummaryError string `json:"summary_error,omitempty"`
}

// Open은 App Group 컨테이너의 데이터 디렉터리를 열고 마이그레이션을 적용한다.
// 이미 열려 있으면 새로 연 뒤 교체한다(확장이 재사용될 때 경로가 바뀌는 경우를 처리).
//
// 순서가 중요하다: **새 핸들을 먼저 열고 성공했을 때만 기존 것을 닫는다.** 반대로 하면
// store.Open 실패 시 멀쩡히 동작하던 핸들까지 잃고, 다음 Save가 errNotOpen("Open이 먼저
// 호출돼야 한다")을 돌려준다 — Open은 불렸고 실패한 것인데 정반대를 가리키는 메시지다.
func Open(dataDir string) error {
	mu.Lock()
	defer mu.Unlock()
	d, err := store.Open(dataDir)
	if err != nil {
		return err // 기존 핸들이 있었다면 그대로 살아 있다
	}
	if st != nil {
		_ = st.Close() // 교체 대상의 close 실패는 복구할 것이 없다
	}
	st = store.New(d, queue.NewSQLite(d.Writer))
	db = d
	return nil
}

// Save는 캡처 페이로드를 저장하고, 이어서 같은 프로세스에서 태깅·요약까지 끝낸다.
// 반환값은 result JSON이다.
//
// 태깅 실패는 저장을 실패시키지 않는다 — 링크는 이미 커밋됐다. 다만 "나중에 워커가
// 다시 한다"는 안전망의 범위가 경로마다 다르므로 정확히 적어 둔다:
//
//   - **신규 저장 + body_text**: SaveLink가 tag 잡을 같은 트랜잭션에서 enqueue한다 →
//     인라인 태깅이 실패해도 앱이 열릴 때 워커가 다시 한다.
//   - **신규 저장 + 본문 없음**(공유 시트가 URL만 주는 흔한 경우): tag 잡은 없고 scrape
//     잡만 있다. 스크랩이 성공해야 ApplyScrape가 그때 tag 잡을 만든다.
//   - **재공유(중복)**: 저장된 본문이 이미 client 출처면 SaveLink가 곧바로 반환하므로
//     본문을 실어 보내도 **아무 잡도 만들어지지 않는다**. 같은 페이지를 다시 공유하는
//     평범한 동작이 여기 해당한다.
//
// 즉 안전망은 첫 번째 경우에만 있다. 나머지 둘에서 **인라인 태깅은 유일한 기회**이고,
// 실패하면 그 링크는 태그 없이 남을 수 있다 — 그래서 실패를 result.TagError로 올려
// 확장이 사용자에게 보여줄 수 있게 한다. 이 구분은 TestSaveTagJobFallbackByPath가
// 고정한다 — SaveLink의 enqueue 조건이 바뀌면 거기서 걸린다.
func Save(payloadJSON string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if st == nil {
		return "", errNotOpen
	}

	var p payload
	if err := json.Unmarshal([]byte(payloadJSON), &p); err != nil {
		return "", fmt.Errorf("ppshare: 페이로드 파싱 실패: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
	defer cancel()

	id, createdAt, dup, err := st.SaveLink(ctx, store.SaveInput{
		URL:         p.URL,
		Note:        p.Note,
		Title:       p.Title,
		Description: p.Description,
		BodyText:    p.BodyText,
	})
	if err != nil {
		return "", err
	}

	res := result{ID: id, CreatedAt: createdAt, Duplicate: dup}
	if tr, tagErr := tagjob.Run(ctx, st, id); tagErr != nil {
		res.TagError = tagErr.Error()
	} else {
		res.Tags, res.TagNames, res.SummaryLen = tr.Tags, tr.Names, tr.SummaryLen
		if tr.SummaryErr != nil {
			res.SummaryError = tr.SummaryErr.Error()
		}
	}

	out, err := json.Marshal(res)
	if err != nil {
		return "", fmt.Errorf("ppshare: 결과 직렬화 실패: %w", err)
	}
	return string(out), nil
}

// Close는 커넥션 풀을 닫는다. 확장이 끝날 때 호출한다.
func Close() error {
	mu.Lock()
	defer mu.Unlock()
	if st == nil {
		return errNotOpen
	}
	err := st.Close()
	db, st = nil, nil
	return err
}
