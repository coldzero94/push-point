import Foundation
import OpenAPIRuntime
import OpenAPIURLSession
import PPCore

/// Backend는 폰 안에서 도는 인프로세스 서버의 수명주기를 감싸고, 그 서버를 향한
/// 생성 클라이언트를 내준다.
///
/// 자립 모드에서 이 앱은 자기 자신의 서버다 — `PpcoreStart`가 `internal/app`의 배선을
/// 그대로 띄우므로 홈서버 모드와 **같은 코드가 같은 계약**을 서빙한다. 그래서 화면 쪽
/// 코드는 두 모드를 구분하지 않는다: 달라지는 것은 `Client`에 넣는 serverURL뿐이다.
@MainActor
final class Backend: ObservableObject {
    enum State: Equatable {
        case idle
        case running(baseURL: URL)
        case failed(String)
    }

    @Published private(set) var state: State = .idle

    /// 계약(api/openapi.yaml)에서 생성된 클라이언트. 실행 중일 때만 존재한다.
    private(set) var client: Client?

    /// apiKey는 **실행마다 새로 만든 난수**다. iOS에서 루프백은 앱 샌드박스를 넘어
    /// 공유되므로 같은 기기의 다른 앱이 포트를 찾아낼 수 있다 — 포트가 매번 바뀌고
    /// 키를 모르면 아무것도 못 한다. 고정 키를 코드에 박으면 이 방어가 사라진다.
    // 저장 프로퍼티 초기화에서는 Self를 못 쓴다(클래스는 covariant Self) — 타입명을 직접 쓴다.
    private let apiKey = Backend.randomKey()

    func start() {
        if case .running = state { return }
        guard let dir = AppGroup.dataDirectory() else {
            state = .failed("App Group 컨테이너를 열 수 없습니다 (entitlement 확인 필요)")
            return
        }
        do {
            try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
            // PpcoreStart는 "127.0.0.1:54321" 형태의 실제 주소를 돌려준다 —
            // 포트는 OS가 고르므로 하드코딩할 수 없고 매 실행 달라진다.
            var err: NSError?
            let addr = PpcoreStart(dir.path, apiKey, &err)
            if let err {
                state = .failed(err.localizedDescription)
                return
            }
            guard let url = URL(string: "http://\(addr)") else {
                state = .failed("서버 주소를 해석할 수 없습니다: \(addr)")
                return
            }
            client = Client(
                serverURL: url,
                transport: URLSessionTransport(),
                // 생성기가 securityScheme 코드를 만들지 않으므로 Bearer는 미들웨어로 붙인다.
                middlewares: [AuthMiddleware(apiKey: apiKey)]
            )
            state = .running(baseURL: url)
        } catch {
            state = .failed(error.localizedDescription)
        }
    }

    /// 백그라운드 전환 시 호출한다. iOS는 백그라운드 앱의 CPU·네트워크를 곧 정지시키므로,
    /// 진행 중 잡을 어중간하게 흘리는 것보다 명시적으로 접는 편이 낫다 —
    /// 큐는 SQLite에 남아 다음 포그라운드에서 이어서 처리된다.
    func stop() {
        var err: NSError?
        PpcoreStop(&err)
        client = nil
        state = .idle
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
