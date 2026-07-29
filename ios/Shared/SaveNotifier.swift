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
    /// 확장이 알림을 띄우지 못하고 버린 횟수. 본체 앱이 읽어 배너로 알린다.
    ///
    /// **왜 세는가**: 확장은 화면을 그리지 않고 즉시 닫히므로(ShareViewController 주석)
    /// 알림이 저장 결과를 알리는 **유일한 통로**다. 권한이 없으면 성공·중복·실패가 전부
    /// "시트가 닫히고 아무 일도 없음"으로 똑같이 보이고, 사용자는 저장된 것과 잃은 것을
    /// 구분할 수 없다. 2초 DoD 경로 전체가 그 상태였다(2026-07-29 리뷰).
    static let droppedKey = "pushpoint.notificationsDropped"

    /// 알림 권한을 요청한다. 본체 앱이 부른다 — 확장에서 시스템 권한 창을 띄우면
    /// 공유 흐름 한복판에 모달이 끼어들어 "2초 저장"이 깨진다.
    static func requestAuthorization() async {
        _ = try? await UNUserNotificationCenter.current()
            .requestAuthorization(options: [.alert, .sound])
    }

    /// 지금 알림을 띄울 수 있는가.
    ///
    /// **`add(_:)`의 성공 여부로는 알 수 없다** — 권한이 거부돼 있어도 add는 던지지 않고
    /// 성공한 뒤 알림만 버려진다. 그래서 상태를 직접 물어보는 것 말고는 방법이 없다.
    static func canNotify() async -> Bool {
        let s = await status()
        return s == .authorized || s == .provisional
    }

    /// 현재 권한 상태. **아직 물어본 적 없음(.notDetermined)과 거부됨(.denied)은 다르다** —
    /// 전자는 앱이 그 자리에서 물어보면 되고, 후자만 설정으로 보내야 한다.
    static func status() async -> UNAuthorizationStatus {
        await UNUserNotificationCenter.current().notificationSettings().authorizationStatus
    }

    /// 저장 결과를 배너로 띄운다.
    ///
    /// 권한이 없으면 확장 흐름은 막지 않는다(저장은 이미 끝났다). 대신 **버린 횟수를
    /// 공유 defaults에 남겨** 본체 앱이 배너로 알린다 — 원래는 그냥 사라져서, 저장 성공과
    /// 완전 실패가 사용자에게 똑같이 보였다.
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
        // 띄울 수 없으면 **버렸다는 사실을 남긴다.** 조용히 지나가면 사용자는 저장이
        // 됐는지조차 알 수 없고, 다음에 앱을 열어도 그 사실이 어디에도 없다.
        guard await canNotify() else {
            if let d = AppGroup.defaults {
                d.set(d.integer(forKey: droppedKey) + 1, forKey: droppedKey)
            }
            return
        }
        // trigger가 nil이면 즉시 전달된다.
        let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        try? await UNUserNotificationCenter.current().add(request)
    }
}
