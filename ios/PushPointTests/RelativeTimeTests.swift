import XCTest

/// 상대 시각이 웹(`frontend/src/lib/time.ts`)과 **같은 다섯 갈래**를 내야 한다.
///
/// 원래는 `RelativeDateTimeFormatter`를 썼는데 기본 `dateTimeStyle`이 `.numeric`이라
/// **"어제"를 절대 출력하지 않았다** — 목록의 "어제" 머리글 아래 카드에 "1일 전"이 찍혀
/// 한 화면이 스스로 모순됐다. 눈으로는 자연스러워 보여서 잡히지 않는 종류다.
final class RelativeTimeTests: XCTestCase {
    /// 고정 기준 시각. 실행 시각에 따라 결과가 달라지면 경계 테스트가 의미를 잃는다.
    private let now = Calendar.current.date(from: DateComponents(
        year: 2026, month: 7, day: 26, hour: 15, minute: 0))!

    private func label(_ secondsAgo: Int) -> String {
        RelativeTime.label(Int(now.timeIntervalSince1970) - secondsAgo, now: now)
    }

    func testFiveBuckets() {
        XCTAssertEqual(label(0), "방금")
        XCTAssertEqual(label(59), "방금")
        XCTAssertEqual(label(60), "1분 전")
        XCTAssertEqual(label(59 * 60), "59분 전")
        XCTAssertEqual(label(3600), "1시간 전")
        XCTAssertEqual(label(14 * 3600), "14시간 전")
    }

    /// 24시간 안쪽은 달력 날짜가 달라도 "N시간 전"이다 — 웹과 같은 순서다.
    /// (이 기대값을 처음에 "어제"로 잘못 썼고 테스트가 잡았다.)
    func testWithinADayStaysInHours() {
        // 기준이 7/26 15:00이므로 16시간 전은 7/25 23:00 — 달력으로는 어제지만
        // 24시간 안쪽이라 시간으로 말한다.
        XCTAssertEqual(label(16 * 3600), "16시간 전")
        XCTAssertEqual(label(23 * 3600), "23시간 전")
    }

    /// 24시간을 넘기면 "어제"는 48시간 창이 아니라 **달력 하루 차이**로 판정한다 —
    /// 웹과 같은 규칙. 경과 시간으로 재면 새벽에 저장한 것이 이틀째까지 "어제"로 남는다.
    func testYesterdayIsACalendarDay() {
        // 7/25 14:00 — 25시간 전, 달력 하루 차이라 어제.
        XCTAssertEqual(label(25 * 3600), "어제")
        // 7/25 01:00 — 38시간 전, 여전히 달력 하루 차이라 어제.
        XCTAssertEqual(label(38 * 3600), "어제")
        // 7/24 23:00 — 40시간 전, 달력 이틀 차이라 날짜로 떨어진다.
        XCTAssertEqual(label(40 * 3600), "7월 24일")
    }

    func testOlderFallsBackToDate() {
        XCTAssertEqual(label(10 * 86400), "7월 16일")
    }
}
