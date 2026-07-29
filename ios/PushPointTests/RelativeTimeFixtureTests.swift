import XCTest

/// 상대 시각 규칙을 **웹과 같은 파일로** 대조한다.
///
/// `RelativeTimeTests`는 이 구현의 경계를 검증하고, 이 파일은 **두 구현이 같은 답을 내는지**를
/// 검증한다. 둘은 다른 일이다 — streak가 그것을 보여 줬다: 웹과 sh를 픽스처로 묶자마자
/// 다른 세 규칙이 갈라져 있던 것이 드러났고, 그때까지 "일치한다"는 문서의 한 문장이었다.
///
/// 픽스처가 `testdata/`에 있는 이유도 같다. `frontend/src/lib/time.test.ts`가 같은 파일을
/// 읽으므로, 한쪽만 고치면 **반대쪽이 빨개진다.**
final class RelativeTimeFixtureTests: XCTestCase {
    private struct Fixture: Decodable {
        struct Case: Decodable {
            let name: String
            let at: String
            let plain: String
            let dayStated: String
        }
        let now: String
        let cases: [Case]
    }

    /// 픽스처의 로컬타임 문자열 → Date. 타임존을 붙이지 않는 것이 요점이다 —
    /// 규칙이 **달력 하루 차이**로 판정하므로 기준도 로컬이어야 웹과 같아진다.
    private func parse(_ s: String) throws -> Date {
        let f = DateFormatter()
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd'T'HH:mm:ss"
        f.timeZone = TimeZone.current
        return try XCTUnwrap(f.date(from: s), "픽스처 시각을 못 읽었다: \(s)")
    }

    func testMatchesTheSharedFixture() throws {
        let url = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "relative-time-cases", withExtension: "json"),
            "픽스처가 번들에 없다 — project.yml의 buildPhase: resources를 확인할 것. "
                + "XcodeGen은 잘못 쓴 resources 키를 조용히 무시한다."
        )
        let fx = try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))
        let now = try parse(fx.now)

        for c in fx.cases {
            let epoch = Int(try parse(c.at).timeIntervalSince1970)

            XCTAssertEqual(RelativeTime.label(epoch, now: now, dayStated: false), c.plain,
                           "\(c.name) — 구간 없는 화면(검색)에서 웹과 갈라졌다")
            XCTAssertEqual(RelativeTime.label(epoch, now: now, dayStated: true), c.dayStated,
                           "\(c.name) — 구간이 날을 말하는 화면(보드)에서 웹과 갈라졌다")
        }
    }
}
