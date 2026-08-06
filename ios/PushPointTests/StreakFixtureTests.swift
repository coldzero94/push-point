import XCTest

/// 연속 저장 규칙 — **웹·셸과 같은 파일을 읽는다**(`testdata/streak-cases.json`).
///
/// 이 픽스처는 웹 `rhythm.test.ts`와 `scripts/streak.sh --self-test`가 이미 읽고 있었고,
/// iOS만 빠져 있었다. 규칙이 뷰 안의 `private func`이라 테스트가 부를 수 없었기 때문이고,
/// 13 §3이 그 사실을 "(iOS는 아직)"으로 적어 두고 있었다.
///
/// 위젯이 같은 숫자를 그리게 되면서 구현이 넷이 될 참이었다. 그래서 꺼내 묶었다 — **넷이
/// 갈라진 뒤에 알아내는 것보다 갈라지기 전에 묶는 편이 싸다**는 것이 이 저장소가 커버 기하와
/// 상대 시각에서 이미 두 번 배운 것이다.
final class StreakFixtureTests: XCTestCase {
    private struct Fixture: Decodable {
        struct Case: Decodable {
            let name: String
            let counts: [Int]
            let streak: Int
            let capped: Bool
        }
        let cases: [Case]
    }

    private func loadFixture() throws -> Fixture {
        let url = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "streak-cases", withExtension: "json"),
            "픽스처가 테스트 번들에 없다 — project.yml의 resources를 볼 것"
        )
        return try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))
    }

    func testMatchesTheSharedFixture() throws {
        let fixture = try loadFixture()
        // 0건이면 아무것도 안 돌고 초록이 된다 — 검사가 사라진 것이 통과로 보이는 자리다.
        XCTAssertGreaterThan(fixture.cases.count, 5, "픽스처가 비었거나 덜 읽혔다")

        for c in fixture.cases {
            XCTAssertEqual(Streak.days(c.counts), c.streak, "연속 일수가 다르다 — \(c.name)")
            XCTAssertEqual(Streak.isCapped(c.counts, days: c.streak), c.capped,
                           "상한 판정이 다르다 — \(c.name)")
        }
    }

    /// 오늘 저장 여부는 위젯이 **목적지를 정하는 데** 쓴다(10 §8.6). 픽스처에 그 열이 없어
    /// 여기서 규칙만 못 박는다 — 마지막 칸이 곧 오늘이라는 계약에 기대는 것이 요점이다.
    func testSavedTodayReadsTheLastBucket() {
        XCTAssertTrue(Streak.savedToday([0, 0, 3]))
        XCTAssertFalse(Streak.savedToday([5, 5, 0]))
        XCTAssertFalse(Streak.savedToday([]), "빈 창에서 오늘 저장했다고 말하면 안 된다")
    }
}
