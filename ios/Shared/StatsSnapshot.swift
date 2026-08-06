import Foundation
import OSLog

/// 위젯이 읽는 통계 스냅샷.
///
/// **위젯은 인프로세스 서버에 붙을 수 없다** — 서버는 앱 프로세스 안에서 돌고, 위젯이
/// 그려질 때 앱은 대개 떠 있지 않다. 세 갈래를 재고 이걸 골랐다(10 §8.6):
///
/// - `ppcore`(49MB)를 링크 — 위젯 예산 밖이고, 타일 하나 그리자고 HTTP 서버를 띄우는 꼴이다
/// - `ppshare`에 `Stats()` 추가 — 19MB로 들어가지만 **JSON이 두 벌이 된다.** 08 §M4가
///   `ppcore`에서 CRUD를 함수로 안 내보낸 이유가 정확히 그것이다
/// - **스냅샷** ← 앱이 계약 그대로의 `Stats`를 써 두고 위젯이 같은 생성 타입으로 읽는다
///
/// 그래서 여기서 다루는 것은 **`Components.Schemas.Stats`의 JSON 그대로**다. 요약하거나
/// 평평하게 만들지 않는다 — 그 순간 모양이 둘이 되고, 이 파일이 존재하는 이유가 사라진다.
enum StatsSnapshot {
    private static let key = "pushpoint.statsSnapshot"
    private static let log = Logger(subsystem: "com.pushpoint.app", category: "widget")

    /// 앱이 통계를 받을 때마다 쓴다.
    static func write(_ data: Data) {
        guard let d = AppGroup.defaults else {
            // 조용히 빠지지 않는다. 스냅샷이 없으면 위젯은 빈 상태를 그리는데, 그건
            // "아직 아무것도 저장 안 함"과 화면에서 같아 보인다 — 통로가 없어서인지
            // 진짜 비어서인지 나중에 구분할 방법이 로그밖에 없다.
            log.error("통계 스냅샷을 쓸 수 없다 — App Group defaults가 없다")
            return
        }
        d.set(data, forKey: key)
    }

    /// 위젯이 읽는다. 없으면 `nil`이고, 그때 위젯은 **숫자를 지어내지 않고** 빈 상태를 그린다.
    static func read() -> Data? {
        AppGroup.defaults?.data(forKey: key)
    }

    /// 공유 확장이 저장에 성공했을 때 **오늘 칸을 1 올린다.**
    ///
    /// 앱을 한 번도 안 열어도 위젯이 거짓말하지 않게 하는 유일한 장치다 — 공유 시트로만
    /// 쓰는 사용자에게는 앱이 스냅샷을 갱신할 기회가 없다. 연속을 바꾸는 행동이 정확히
    /// 저장이므로 여기가 그 자리다.
    ///
    /// **모양을 그대로 두고 값만 고친다.** `Stats`를 디코드해 고치고 다시 인코드하면
    /// 생성 타입을 확장에도 링크해야 하는데, 확장은 예산이 가장 빡빡한 자리다. 대신
    /// `by_day`의 **마지막 원소의 count**만 JSON 위에서 찾아 올린다 — 계약이 마지막 칸을
    /// 오늘로 보장하므로(§Stats.by_day) 날짜를 계산할 필요가 없다.
    ///
    /// 스냅샷이 없거나 모양이 예상과 다르면 **아무것도 하지 않는다.** 여기서 새 스냅샷을
    /// 지어내면 총계·주간·태그가 전부 0인 그럴듯한 거짓이 화면에 나간다.
    static func bumpToday() {
        guard let data = read(),
              var root = (try? JSONSerialization.jsonObject(with: data)) as? [String: Any],
              var byDay = root["by_day"] as? [[String: Any]],
              !byDay.isEmpty,
              let last = byDay.last,
              let count = last["count"] as? Int else {
            log.notice("스냅샷을 올리지 않았다 — 없거나 모양이 다르다")
            return
        }
        var today = last
        today["count"] = count + 1
        byDay[byDay.count - 1] = today
        root["by_day"] = byDay
        // 오늘 첫 저장이면 주간 합도 함께 오른다 — 둘이 같은 창에서 나오므로(계약상
        // links_this_week는 by_day 마지막 7칸 합) 한쪽만 올리면 화면이 자기와 모순된다.
        if let week = root["links_this_week"] as? Int { root["links_this_week"] = week + 1 }
        if let total = root["total_links"] as? Int { root["total_links"] = total + 1 }

        guard let updated = try? JSONSerialization.data(withJSONObject: root) else { return }
        write(updated)
    }
}
