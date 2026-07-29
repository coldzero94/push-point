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
    /// epoch 초 → 한국어 상대 라벨.
    ///
    /// - Parameters:
    ///   - now: 경계를 테스트할 수 있게 주입받는다.
    ///   - dayStated: **화면이 이미 그 날을 말했는가.** 보드는 시간 척추로 끊으므로
    ///     "어제" 구간의 카드가 다시 "어제"라고 적으면 머리글을 한 줄씩 되풀이한다 —
    ///     60건이면 같은 글자가 60번이다. 검색 결과에는 구간이 없어서 그 라벨이 유일한
    ///     날짜 정보이므로 거기서는 그대로 둔다.
    ///
    ///     원래 이 함수는 `RelativeDateTimeFormatter`의 "1일 전"이 "어제" 머리글과
    ///     **모순되는 것**을 피하려고 "어제"를 냈다. 모순을 피해 일치를 택했는데,
    ///     일치는 곧 반복이었다 — 세 번째 선택지가 **그 날의 시각**을 말하는 것이다.
    static func label(_ epoch: Int, now: Date = Date(), dayStated: Bool = false) -> String {
        let then = Date(timeIntervalSince1970: TimeInterval(epoch))
        let diff = Int(now.timeIntervalSince(then))
        let cal = Calendar.current
        let gap = cal.dateComponents([.day], from: cal.startOfDay(for: then),
                                     to: cal.startOfDay(for: now)).day ?? 0

        // **머리글이 날을 말했으면 달력을 먼저 본다.**
        //
        // 경과 시간을 먼저 재면 24시간 미만인 어제 항목이 "N시간 전"으로 빠져나가,
        // 같은 "어제" 구간 안에 "9시간 전"과 "오전 9:05"가 섞인다. 실제로 그렇게 나왔고
        // **테스트는 통과했다** — 픽스처의 어제 케이스가 전부 24시간을 넘겼기 때문이다.
        // 화면을 보고서야 드러났다(2026-07-30).
        if dayStated, gap > 0 {
            return gap == 1 ? Self.timeOfDay(then) : Self.monthDay(then, cal)
        }

        if diff < 60 { return "방금" }
        if diff < 3600 { return "\(diff / 60)분 전" }
        if diff < 86400 { return "\(diff / 3600)시간 전" }

        // "어제"는 48시간 창이 아니라 **달력 하루 차이**다 — 웹과 같은 규칙.
        // 경과 시간으로 재면 새벽에 저장한 것이 하루 종일 "N시간 전"으로 남는다.
        if gap == 1 { return "어제" }

        return Self.monthDay(then, cal)
    }

    static func monthDay(_ date: Date, _ cal: Calendar) -> String {
        let c = cal.dateComponents([.month, .day], from: date)
        return "\(c.month ?? 0)월 \(c.day ?? 0)일"
    }

    /// 그 날 안에서의 시각. 웹 `time.ts`와 같은 형식이어야 한다.
    ///
    /// `DateFormatter`를 쓰지 않는다 — 로케일이 형식을 정하면 두 클라이언트가 갈라지고,
    /// 이 파일이 존재하는 이유가 정확히 그것을 막는 것이다(`Locale`도 이미 ko_KR 고정).
    static func timeOfDay(_ date: Date) -> String {
        let c = Calendar.current.dateComponents([.hour, .minute], from: date)
        let h = c.hour ?? 0, m = c.minute ?? 0
        let period = h < 12 ? "오전" : "오후"
        let h12 = h % 12 == 0 ? 12 : h % 12
        return "\(period) \(h12):\(String(format: "%02d", m))"
    }
}
