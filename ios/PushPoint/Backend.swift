import Foundation
import OpenAPIRuntime
import OpenAPIURLSession
import PPCore
import UIKit

/// Backend는 폰 안에서 도는 인프로세스 서버의 수명주기를 감싸고, 그 서버를 향한
/// 생성 클라이언트를 내준다.
///
/// 자립 모드에서 이 앱은 자기 자신의 서버다 — `PpcoreStart`가 `internal/app`의 배선을
/// 그대로 띄우므로 홈서버 모드와 **같은 코드가 같은 계약**을 서빙한다. 그래서 화면 쪽
/// 코드는 두 모드를 구분하지 않는다: 달라지는 것은 `Client`에 넣는 serverURL뿐이다.
///
/// **gomobile 호출은 Swift에서 동기다.** `PpcoreStart`는 마이그레이션까지 돌고
/// `PpcoreStop`은 진행 중 잡을 드레인하는데(스크랩 한 건이 HTTP 타임아웃 10초까지 갈 수
/// 있다), 그동안 호출한 스레드가 통째로 막힌다. 메인 스레드에서 부르면 UI가 멈추고,
/// 백그라운드 전환 중이라면 iOS watchdog에 걸린다 — 그래서 이 클래스는 상태만 메인 액터에
/// 두고 **블로킹 호출은 전부 메인 밖으로** 내보낸다.
@MainActor
final class Backend: ObservableObject {
    enum State: Equatable {
        case idle
        case starting
        case running(baseURL: URL)
        case failed(String)
    }

    @Published private(set) var state: State = .idle

    /// 계약(api/openapi.yaml)에서 생성된 클라이언트. 실행 중일 때만 존재한다.
    private(set) var client: Client?

    /// apiKey는 **실행마다 새로 만든 난수**다. iOS에서 루프백은 앱 샌드박스를 넘어
    /// 공유되므로 같은 기기의 다른 앱이 포트를 찾아낼 수 있다 — 포트가 매번 바뀌고
    /// 키를 모르면 아무것도 못 한다. 고정 키를 코드에 박으면 이 방어가 사라진다.
    ///
    /// 저장 프로퍼티 초기화에서는 Self를 못 쓴다(클래스는 covariant Self) — 타입명을 직접 쓴다.
    private let apiKey = Backend.randomKey()

    /// 포그라운드 복귀마다 불린다. 이미 실행 중이거나 시작하는 중이면 아무것도 하지 않는다 —
    /// ppcore.Start는 인자가 다르면 에러이고 같으면 기존 주소를 주므로 중복 호출 자체는
    /// 안전하지만, 여기서 걸러 불필요한 스레드 전환을 없앤다.
    func start() {
        switch state {
        case .running, .starting: return
        case .idle, .failed: break
        }
        state = .starting

        // UI 테스트는 App Group이 아니라 임시 디렉터리를 쓴다 — 테스트가 사용자의 실제
        // 아카이브를 건드리는 일이 구조적으로 불가능해야 한다(UITestMode 주석).
        let container: URL?
        if UITestMode.isActive {
            // 데이터만으로는 격리가 안 된다 — 공유 defaults도 되돌려야 한다.
            UITestMode.resetSharedDefaults()
            container = UITestMode.dataDirectory()
        } else {
            container = AppGroup.dataDirectory()
        }
        guard let dir = container else {
            state = .failed(t("status.appGroupUnavailable"))
            return
        }
        let path = dir.path
        let key = apiKey

        Task {
            do {
                try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            } catch {
                state = .failed(error.localizedDescription)
                return
            }
            // PpcoreStart는 "127.0.0.1:54321" 형태의 실제 주소를 돌려준다 — 포트는 OS가
            // 고르므로 하드코딩할 수 없고 매 실행 달라진다. 마이그레이션이 이 안에서
            // 돌기 때문에 메인 밖에서 호출한다.
            let outcome = await Task.detached(priority: .userInitiated) { () -> Result<String, Error> in
                var err: NSError?
                let addr = PpcoreStart(path, key, &err)
                if let err { return .failure(err) }
                return .success(addr)
            }.value

            switch outcome {
            case let .failure(error):
                state = .failed(error.localizedDescription)
            case let .success(addr):
                guard let url = URL(string: "http://\(addr)") else {
                    state = .failed(t("status.badServerAddress", ["addr": addr]))
                    return
                }
                client = Client(
                    serverURL: url,
                    transport: URLSessionTransport(),
                    // 생성기가 securityScheme 코드를 만들지 않으므로 Bearer는 미들웨어로 붙인다.
                    middlewares: [AuthMiddleware(apiKey: key)]
                )
                // 시딩은 **.running으로 넘어가기 전에** 끝내야 한다. 화면은
                // `.task(id: backend.state)`로 목록을 읽는데, 상태를 먼저 바꾸면
                // 빈 DB를 읽고 다시 읽을 계기가 없다 — 실제로 그렇게 실패했다.
                // 시딩도 실제 저장 경로(POST /links)로 한다: SQLite에 직접 쓰면
                // 저장 계약이 깨져도 테스트는 멀쩡히 통과한다.
                if UITestMode.isActive, let client {
                    await UITestMode.seed(using: client)
                }
                state = .running(baseURL: url)
            }
        }
    }

    /// 백그라운드 전환 시 호출한다. iOS는 백그라운드 앱의 CPU·네트워크를 곧 정지시키므로,
    /// 진행 중 잡을 어중간하게 흘리는 것보다 명시적으로 접는 편이 낫다 —
    /// 큐는 SQLite에 남아 다음 포그라운드에서 이어서 처리된다.
    ///
    /// UI 상태는 즉시 정리하고 드레인만 메인 밖으로 보낸다. 그리고 그 드레인이 잘리지
    /// 않도록 `beginBackgroundTask`로 실행 시간을 요청한다 — 요청하지 않으면 iOS가
    /// 셧다운 도중 프로세스를 정지시켜 WAL 체크포인트가 끝나지 않을 수 있다.
    func stop() {
        guard case .running = state else { return }
        state = .idle
        client = nil

        // UIApplication은 메인 액터 전용이고 지금 메인이므로 여기서 시간을 요청한다.
        let tracker = BackgroundTaskTracker()
        tracker.begin(name: "pushpoint.shutdown")

        Task.detached(priority: .utility) {
            var err: NSError?
            PpcoreStop(&err) // 여기서 막힌다 — 메인 밖이라 UI는 영향받지 않는다
            await tracker.end()
        }
    }

    /// 계약의 `thumb_url`은 `/thumbs/{dir}/{file}` **상대 경로**다(인증 면제 —
    /// AsyncImage가 커스텀 헤더를 못 실기 때문이다). 그대로 `URL(string:)`에 넣으면
    /// host 없는 URL이 되어 **조용히 아무것도 안 그린다** — 실제로 그렇게 썸네일이
    /// 전부 비어 보였다. 서버 주소를 아는 곳은 여기뿐이므로 해석도 여기서 한다.
    func absoluteURL(_ path: String) -> URL? {
        guard case let .running(baseURL) = state else { return nil }
        return URL(string: path, relativeTo: baseURL)
    }

    private static func randomKey() -> String {
        var bytes = [UInt8](repeating: 0, count: 32)
        // 실패하면 예측 가능한 키를 쓰느니 크래시가 낫다 — 이 값이 유일한 접근 통제다.
        guard SecRandomCopyBytes(kSecRandomDefault, bytes.count, &bytes) == errSecSuccess else {
            fatalError("보안 난수 생성 실패 — API 키를 만들 수 없습니다")
        }
        return bytes.map { String(format: "%02x", $0) }.joined()
    }
}

/// 백그라운드 실행 시간 토큰을 한 번만 반납하도록 감싼다.
///
/// 만료 핸들러와 정상 완료가 **경쟁**한다 — 둘 다 반납하면 크래시고, 둘 다 안 하면 앱이
/// 강제 종료된다. 토큰을 메인 액터에 가둬 두 경로가 같은 순서로 지나가게 한다.
@MainActor
final class BackgroundTaskTracker {
    private var token = UIBackgroundTaskIdentifier.invalid

    func begin(name: String) {
        token = UIApplication.shared.beginBackgroundTask(withName: name) { [weak self] in
            // 시간이 다 되면 iOS가 여기를 부른다. 드레인이 안 끝났어도 반납해야 한다 —
            // 반납하지 않으면 앱이 강제 종료된다.
            Task { @MainActor in await self?.end() }
        }
    }

    func end() async {
        guard token != .invalid else { return }
        UIApplication.shared.endBackgroundTask(token)
        token = .invalid
    }
}
