import Foundation
import OSLog

/// 되살림 위젯이 읽는 스냅샷 — App Group에 놓인 **계약 그대로의 `Link` JSON**.
///
/// `StatsSnapshot`과 같은 이유로 같은 모양이다. 위젯이 그려질 때 앱은 대개 떠 있지 않고,
/// 이 앱의 서버는 앱 프로세스 안에서 돈다. 그래서 위젯은 서버를 못 부르고, 앱이 받아 둔
/// 것을 읽는다.
///
/// **왜 되살림을 위젯으로 내보내는가.** 되살림은 `GET /links/resurfaced`로 이미 있었지만
/// **앱을 열어야만** 왔다. 잊어버린 것에 대한 앱을, 열어야 한다는 걸 기억해야 하는 구조다.
/// 홈 화면은 이미 하루에 수십 번 보는 자리이므로, 링크가 사람에게 오는 유일한 경로가 된다.
enum ResurfaceSnapshot {
    private static let key = "pushpoint.resurfaceSnapshot"
    private static let log = Logger(subsystem: "com.pushpoint.app", category: "widget")

    /// 앱이 되살림을 받을 때마다 쓴다.
    static func write(_ data: Data) {
        guard let d = AppGroup.defaults else {
            // `StatsSnapshot.write`와 같은 이유로 조용히 빠지지 않는다 — 스냅샷이 없는
            // 위젯과 되살릴 게 없는 위젯은 화면에서 같아 보인다.
            log.error("되살림 스냅샷을 쓸 수 없다 — App Group defaults가 없다")
            return
        }
        d.set(data, forKey: key)
    }

    /// 되살릴 후보가 없을 때(204) 지운다.
    ///
    /// **비우는 것이 안 쓰는 것과 다르다.** 지난주에 되살린 링크가 스냅샷에 남아 있으면
    /// 후보가 없어진 뒤에도 위젯이 계속 그것을 보여준다 — 읽고 지운 링크가 홈 화면에
    /// 며칠씩 붙어 있는 모양이 된다.
    static func clear() {
        AppGroup.defaults?.removeObject(forKey: key)
    }

    /// 위젯이 읽는다. 없으면 `nil`이고, 그때 위젯은 **아무것도 지어내지 않고** 빈 상태를 그린다.
    static func read() -> Data? {
        AppGroup.defaults?.data(forKey: key)
    }
}
