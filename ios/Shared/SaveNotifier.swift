import Foundation
import OSLog
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
    /// 확장에는 화면이 없으므로 로그가 유일한 흔적이다.
    /// `xcrun simctl spawn booted log stream --predicate 'subsystem == "com.pushpoint.app"'`
    private static let log = Logger(subsystem: "com.pushpoint.app", category: "share")

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
    /// 탭한 알림이 어느 링크였는지 앱이 알아보는 열쇠.
    static let linkIDKey = "pushpoint.linkID"

    static func notifySaved(title: String, host: String, tags: [String], duplicate: Bool,
                            linkID: Int64? = nil) async {
        let content = UNMutableNotificationContent()
        content.title = duplicate ? t("notify.duplicate") : t("notify.saved")
        // 무엇이 저장됐는지 — 제목이 없으면 도메인이라도 보여준다(엉뚱한 링크를 저장했을 때
        // 알아챌 수 있는 유일한 단서다).
        content.subtitle = title.isEmpty ? host : title
        // 태그는 이 앱의 차별점이다. 서버도 네트워크도 없이 그 자리에서 붙었다는 증거라
        // 본문에 그대로 노출한다.
        content.body = tags.isEmpty ? host : tags.prefix(4).joined(separator: " · ")
        content.sound = nil // 저장은 조용해야 한다 — 소리는 방해다
        // **어느 링크인지 싣는다.** 이게 없으면 알림을 눌러도 목록만 열리고, 방금 저장한
        // 것을 다시 찾아야 한다 — 저장이 한 번에 끝난다는 약속이 마지막 한 걸음에서 깨진다.
        if let linkID { content.userInfo[linkIDKey] = linkID }
        await post(content)
    }

    /// 실패는 소리와 함께 남긴다 — 사용자가 놓치면 그 링크는 저장되지 않은 채 사라진다.
    static func notifyFailed(message: String) async {
        let content = UNMutableNotificationContent()
        content.title = t("notify.failed")
        content.body = message
        content.sound = .default
        await post(content)
    }

    private static func post(_ content: UNMutableNotificationContent) async {
        // 띄울 수 없으면 **버렸다는 사실을 남긴다.** 조용히 지나가면 사용자는 저장이
        // 됐는지조차 알 수 없고, 다음에 앱을 열어도 그 사실이 어디에도 없다.
        guard await canNotify() else {
            countDrop("권한 없음")
            return
        }
        // trigger가 nil이면 즉시 전달된다.
        let request = UNNotificationRequest(identifier: UUID().uuidString, content: content, trigger: nil)
        do {
            try await UNUserNotificationCenter.current().add(request)
        } catch {
            // **`try?`로 두면 안 된다.** `canNotify()`가 true를 준 뒤 `add`가 던지면
            // 카운트도 안 늘고 로그도 없어서, 사용자에게는 **저장 성공과 완전히 같아
            // 보인다.** 확장은 화면을 안 그리므로 알림이 유일한 통로이고, 이 클래스
            // 주석이 스스로 그렇게 적어 뒀다 — "사용자가 놓치면 그 링크는 저장되지 않은
            // 채 사라진다."
            //
            // 2026-07-29에 `canNotify()` 갈래는 막았는데 이 한 줄이 열려 있었다.
            countDrop("add 실패: \(error)")
        }
    }

    /// 알림을 못 띄운 사실을 공유 defaults에 센다. 본체 앱이 다음에 앞으로 나올 때 배너로 말한다.
    ///
    /// **defaults가 nil이면 그 사실도 남긴다.** `if let`으로 조용히 빠지면 "버린 게 없다"와
    /// "보고 채널이 아예 없다"가 구분되지 않는다 — 그리고 후자는 실제로 일어난다:
    /// `AppGroup.swift`가 적어 둔 대로 무료 개인 팀에서는 entitlement가 없을 수 있고,
    /// 그때 `UserDefaults(suiteName:)`는 nil을 준다. 같은 파일의 `dataDirectory()`는
    /// 그 경우 폴백을 거부하고 화면에 오류를 띄우는데, defaults만 조용했다.
    private static func countDrop(_ reason: String) {
        guard let d = AppGroup.defaults else {
            log.error("알림을 버렸고 그 사실도 기록할 수 없다 — App Group defaults가 없다 (\(reason, privacy: .public))")
            return
        }
        d.set(d.integer(forKey: droppedKey) + 1, forKey: droppedKey)
        log.error("알림을 버렸다 (\(reason, privacy: .public))")
    }
}
