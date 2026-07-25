// Package ppcore는 iOS 본체 앱이 쓰는 바인드 표면이다. 저장·목록·검색·수정을 함수로
// 하나씩 노출하는 대신 **폰 안에서 진짜 서버를 인프로세스로 띄우고 주소만 돌려준다.**
//
// 왜 이렇게 하나 — 개별 함수로 내보내면 자립 모드의 JSON이 서버 모드(api/openapi.yaml)와
// 갈라지고, Swift 앱은 같은 화면을 두 벌의 디코더로 그려야 한다. 여기서 chi 라우터를
// 그대로 띄우면 두 모드가 **문자 그대로 같은 계약**을 서빙하므로, 앱은 생성된 OpenAPI
// 클라이언트 한 벌로 base URL만 바꾸면 된다:
//
//	자립 모드   http://127.0.0.1:<Start가 돌려준 포트>
//	홈서버 모드 http://<Tailscale IP>:8420
//
// 배선은 internal/app 한 곳에 있어 서버 바이너리와 동일하다(docs/v2/04 §7.4).
//
// Share Extension은 이걸 쓰지 않는다 — 확장은 서버를 띄울 수 없고, 띄울 필요도 없다.
// 확장은 mobile/ppshare로 SQLite에 직접 쓴다(메모리 예산 근거는 그쪽 패키지 주석).
package ppcore

import (
	"errors"
	"fmt"
	"sync"

	"github.com/coby/push-point/backend/internal/app"
)

// scrapeConcurrency는 폰에서의 스크랩 동시성. 서버(기본 8)보다 낮게 둔다 —
// 모바일은 CPU·배터리·셀룰러 대역폭이 모두 제약이고, 개인용 저장 빈도에서 8은 과하다.
const scrapeConcurrency = 2

// 패키지 전역으로 두는 것은 gomobile이 강요해서가 아니다 — 바인드되는 패키지 안에
// 선언한 구조체는 생성자와 함께 내보낼 수 있다. 앱 하나에 서버 하나라는 도메인이
// 실제로 싱글턴이라 택한 **의도적 단순화**이고, Swift 쪽 수명 관리를 없애 준다.
// 인스턴스가 여럿 필요해지면(테스트 병렬화 등) 핸들 타입으로 바꾸는 것이 정석이다.
var (
	mu            sync.Mutex
	inst          *app.App
	instDir       string // 실행 중 인스턴스가 연 디렉터리 — 재진입 시 인자 비교용
	instKey       string
	errNotRunning = errors.New("ppcore: 서버가 실행 중이 아니다")
)

// Start는 인프로세스 서버를 127.0.0.1의 **임의 포트**에 띄우고 "127.0.0.1:54321" 형태의
// 실제 주소를 돌려준다. 같은 인자로 다시 부르면 실행 중인 주소를 그대로 돌려준다
// (앱 포그라운드 복귀 등 재진입에 안전).
//
// **인자가 다르면 에러다.** 조용히 무시하면 앱이 새 키로 요청을 보내는데 서버는 옛 키로
// 검사해 전부 401이 되고, 원인이 드러나지 않는다(dataDir가 다르면 확장과 본체가 서로
// 다른 DB를 보게 된다). 키를 갈아야 하면 Stop 후 다시 Start한다.
//
// apiKey는 **앱이 실행마다 새로 만든 난수**를 넘기는 것이 맞다. iOS에서 루프백은 앱
// 샌드박스를 넘어 공유되므로 같은 기기의 다른 앱이 이 포트에 접속을 시도할 수 있다 —
// 포트가 매번 바뀌고 키를 모르면 아무것도 못 한다. 고정 키를 코드에 박지 말 것.
//
// dataDir는 App Group 컨테이너 경로를 넘긴다 — Share Extension(ppshare)이 쓰는 것과
// **같은 디렉터리**여야 확장이 저장한 링크를 본체가 이어받는다.
func Start(dataDir, apiKey string) (string, error) {
	mu.Lock()
	defer mu.Unlock()
	if inst != nil {
		if dataDir != instDir || apiKey != instKey {
			return "", fmt.Errorf("ppcore: 다른 인자로 재시작할 수 없다 — Stop 후 다시 Start할 것 (dataDir 일치=%v, apiKey 일치=%v)",
				dataDir == instDir, apiKey == instKey)
		}
		return inst.Addr(), nil
	}

	logger := newLogger(dataDir)
	a, err := app.Start(app.Config{
		DataDir: dataDir,
		APIKey:  apiKey,
		// 포트 0 = OS가 고른다. 루프백에 묶어 같은 네트워크의 다른 기기에는 노출하지 않는다.
		Addr:              "127.0.0.1:0",
		ScrapeConcurrency: scrapeConcurrency,
		// SSRF 가드는 폰에서도 켜 둔다 — 저장한 링크가 사설망(집 공유기·회사 내부)으로
		// 나가지 못하게 막는다. 이 값을 true로 두는 것은 로컬 fixture 테스트 전용이다.
		AllowPrivateHosts: false,
	}, logger)
	if err != nil {
		return "", err
	}
	inst, instDir, instKey = a, dataDir, apiKey

	// 서버나 dispatcher가 죽으면 조용히 사라지지 않게 남긴다(dataDir/pushpoint.log —
	// 기기에서 stderr는 버려지므로 파일이어야 한다. logger.go 참조).
	// 핸들을 비워 둬야 앱이 다음 Start 호출로 복구할 수 있다.
	go func() {
		if werr := a.Wait(); werr != nil {
			logger.Error("인프로세스 서버 비정상 종료", "err", werr)
			mu.Lock()
			if inst == a {
				inst, instDir, instKey = nil, "", ""
			}
			mu.Unlock()
		}
	}()
	return a.Addr(), nil
}

// Addr는 실행 중인 서버 주소를 반환한다. 실행 중이 아니면 빈 문자열.
func Addr() string {
	mu.Lock()
	defer mu.Unlock()
	if inst == nil {
		return ""
	}
	return inst.Addr()
}

// Stop은 서버를 graceful 종료한다. 앱이 백그라운드로 갈 때 호출한다 — iOS는 백그라운드
// 앱의 CPU·네트워크를 곧 정지시키므로, 진행 중 잡을 어중간하게 흘리는 것보다 명시적으로
// 접고 다음 포그라운드에서 다시 여는 편이 낫다(큐는 SQLite에 남아 이어서 처리된다).
//
// **메인 스레드에서 부르지 말 것.** 드레인은 진행 중 잡을 기다리는데 스크랩 한 건이
// HTTP 타임아웃(10초)까지 갈 수 있고, 그동안 이 패키지의 뮤텍스를 쥐고 있어 Addr·Start도
// 함께 막힌다. gomobile 호출은 Swift에서 동기이므로 메인 스레드면 그 시간만큼 UI가 멈추고,
// 백그라운드 전환 중이라면 iOS watchdog에 걸릴 수 있다. Swift 쪽에서 Task.detached 등으로
// 메인 액터 밖에서 호출한다.
func Stop() error {
	mu.Lock()
	defer mu.Unlock()
	if inst == nil {
		return errNotRunning
	}
	err := inst.Shutdown()
	inst, instDir, instKey = nil, "", ""
	return err
}
