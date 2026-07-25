import Foundation
import UserNotifications

/// 저장 결과를 **로컬 알림 배너**로 알린다.
///
/// 왜 확장 화면이 아니라 알림인가 — 확장은 시스템이 시트로 띄우므로 무엇을 하든 보고 있던
/// 페이지를 가린다. 저장 한 건 알리자고 화면을 덮는 것은 과하고, 커스텀 detent로 높이를
/// 줄이려는 시도도 시스템이 시트 크기를 쥐고 있어 통하지 않았다.
///
/// 그래서 방향을 뒤집었다: **확장은 저장하자마자 즉시 닫고**(저장은 밀리초 단위다),
/// 결과는 위에서 내려오는 배너로 보여준다. 사용자 입장에서는 공유 → 즉시 원래 화면 복귀 →
/// 메시지처럼 알림이 뜨는 흐름이 되어, 화면을 전혀 가리지 않으면서 무엇이 저장됐는지도 안다.
enum SaveNotifier {
    /// 알림 권한을 요청한다. 본체 앱이 부른다 — 확장에서 시스템 권한 창을 띄우면
    /// 공유 흐름 한복판에 모달이 끼어들어 "2초 저장"이 깨진다.
    static func requestAuthorization() async {
        _ = try? await UNUserNotificationCenter.current()
            .requestAuthorization(options: [.alert, .sound])
    }

    /// 저장 결과를 배너로 띄운다.
    ///
    /// 권한이 없으면 조용히 아무 일도 일어나지 않는다 — 저장 자체는 이미 끝났으므로
    /// 알림 실패로 사용자를 붙잡을 이유가 없다. 진단은 os_log가 맡는다.
    static func notifySaved(title: String, host: String, tags: [String], duplicate: Bool) async {
        let content = UNMutableNotificationContent()
        content.title = duplicate ? "이미 저장된 링크" : "저장했습니다"
        // 무엇이 저장됐는지 — 제목이 없으면 도메인이라도 보여준다(엉뚱한 링크를 저장했을 때
        // 알아챌 수 있는 유일한 단서다).
        content.subtitle = title.isEmpty ? host : title
        // 태그는 이 앱의 차별점이다. 서버도 네트워크도 없이 그 자리에서 붙었다는 증거라
        // 본문에 그대로 노출한다.
        content.body = tags.isEmpty ? host : tags.prefix(4).joined(separator: " · ")
        content.sound = nil // 저장은 조용해야 한다 — 소리는 방해다
        await post(content)
    }

    /// 실패는 소리와 함께 남긴다 — 사용자가 놓치면 그 링크는 저장되지 않은 채 사라진다.
    static func notifyFailed(message: String) async {
        let content = UNMutableNotificationContent()
        content.title = "저장하지 못했습니다"
        content.body = message
        content.sound = .default
        await post(content)
    }

    private static func post(_ content: UNMutableNotificationContent) async {
        // trigger가 nil이면 즉시 전달된다.
        let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        try? await UNUserNotificationCenter.current().add(request)
    }
}
