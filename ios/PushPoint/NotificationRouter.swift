import Foundation
import UserNotifications

/// 저장 알림을 누르면 **그 링크로 간다.**
///
/// 이게 없으면 알림은 "저장됐다"만 말하고 앱은 목록 맨 위에서 시작한다. 대부분은 그게
/// 맞지만, 방금 저장한 것에 메모를 붙이거나 태그를 고치려는 순간에는 한 걸음이 더 든다 —
/// 한 번에 끝난다는 약속이 마지막에서 깨지는 자리다.
///
/// **델리게이트는 앱이 뜰 때 바로 걸어야 한다.** iOS는 알림 탭으로 앱을 깨우면서
/// `didReceive`를 곧장 부르는데, 그 시점에 델리게이트가 없으면 그 탭은 조용히 버려진다.
/// 그래서 `PushPointApp.init`에서 등록하고, 도착한 id는 화면이 준비될 때까지 여기 담아 둔다.
@MainActor
final class NotificationRouter: NSObject, ObservableObject, UNUserNotificationCenterDelegate {
    static let shared = NotificationRouter()

    /// 열어야 할 링크. 화면이 읽고 나면 `nil`로 되돌린다.
    @Published var pendingLinkID: Int64?

    func install() {
        UNUserNotificationCenter.current().delegate = self
    }

    /// **`async` 변형이 아니라 완료 핸들러 변형을 쓴다.** async 쪽은 UIKit이 기대하는
    /// 시점보다 늦게 끝나서, 알림 탭으로 차갑게 뜬 앱이
    /// `-[UIApplication _performBlockAfterCATransactionCommitSynchronizes]`
    /// 어서션에서 죽었다(2026-08-03, 홈 화면으로 떨어졌다). 여기서는 값만 남기고 즉시
    /// 완료를 알린 뒤, 화면 이동은 `ContentView`가 첫 프레임 뒤에 맡는다.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        didReceive response: UNNotificationResponse,
        withCompletionHandler completionHandler: @escaping () -> Void
    ) {
        let info = response.notification.request.content.userInfo
        // `Int`로 캐스팅하면 안 된다 — 저장은 `Int64`로 싣고, 알림 페이로드가 직렬화를
        // 거치면서 어느 쪽으로 되살아날지 보장이 없다. 숫자로 받아 넓힌다.
        let id = (info[SaveNotifier.linkIDKey] as? NSNumber)?.int64Value
        DispatchQueue.main.async {
            if let id { Self.shared.pendingLinkID = id }
            completionHandler()
        }
    }

    /// 앱이 앞에 있는 동안에도 배너를 띄운다. 공유 시트에서 저장하면 사파리가 앞이지만,
    /// 앱을 열어 둔 채 다른 앱에서 공유하는 경우도 있고 그때 아무 표시가 없으면
    /// 저장이 됐는지 알 방법이 없다.
    nonisolated func userNotificationCenter(
        _ center: UNUserNotificationCenter,
        willPresent notification: UNNotification
    ) async -> UNNotificationPresentationOptions {
        [.banner, .list]
    }
}
