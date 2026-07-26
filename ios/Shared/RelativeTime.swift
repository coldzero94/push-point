import Foundation

/// 상대 시각 — 웹(`frontend/src/lib/time.ts`)과 **같은 다섯 갈래**를 낸다.
///
/// `Shared/`에 두는 이유는 `CoverPattern`과 같다: 웹과 값이 맞아야 하는 규칙이고,
/// 갈라져도 **양쪽 다 정상 동작하는 것처럼 보인다.** 그래서 기준값을 테스트로 박는다.
///
/// **`RelativeDateTimeFormatter`를 쓰지 않는다.** 기본 `dateTimeStyle`이 `.numeric`이라
/// "어제"를 절대 출력하지 않고 "1일 전"을 낸다 — 그러면 목록의 "어제" 머리글 아래 카드에
/// "1일 전"이 찍혀 한 화면이 스스로 모순된다. `.named`로 바꿔도 나머지 네 갈래가 웹과
/// 어긋나고, 무엇보다 OS가 문구를 정하면 두 클라이언트가 갈라진다 — 이 프로젝트가
/// `.primary`/`.secondary` 같은 시스템 색을 쓰지 않는 것과 같은 판단이다(§8.1).
/// `Locale`을 이미 ko_KR로 못박고 있어 OS에 맡겨 얻는 로케일 적응성도 없다.
enum RelativeTime {
    /// epoch 초 → 한국어 상대 라벨. `now`를 주입받아 경계를 테스트할 수 있게 한다.
    static func label(_ epoch: Int, now: Date = Date()) -> String {
        let then = Date(timeIntervalSince1970: TimeInterval(epoch))
        let diff = Int(now.timeIntervalSince(then))
        if diff < 60 { return "방금" }
        if diff < 3600 { return "\(diff / 60)분 전" }
        if diff < 86400 { return "\(diff / 3600)시간 전" }

        // "어제"는 48시간 창이 아니라 **달력 하루 차이**다 — 웹과 같은 규칙.
        // 경과 시간으로 재면 새벽에 저장한 것이 하루 종일 "N시간 전"으로 남는다.
        let cal = Calendar.current
        let gap = cal.dateComponents([.day], from: cal.startOfDay(for: then),
                                     to: cal.startOfDay(for: now)).day ?? 0
        if gap == 1 { return "어제" }

        let c = cal.dateComponents([.month, .day], from: then)
        return "\(c.month ?? 0)월 \(c.day ?? 0)일"
    }
}
