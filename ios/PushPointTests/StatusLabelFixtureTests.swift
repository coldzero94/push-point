import XCTest

/// 상태 라벨을 **웹과 같은 파일로** 대조한다.
///
/// §8.1은 두 클라이언트가 같은 단어를 쓰라고 하는데, 그동안 그건 주장이었다 — 13 §3의
/// 판정표에도 "일치 검사: 없음"으로 적혀 있었다. streak과 상대 시각을 픽스처로 묶었을 때
/// 곧바로 갈라짐이 드러났으니, 이것도 파일로 묶는다.
///
/// **이제 함수를 직접 부른다.** 처음 판은 라벨이 뷰의 `private var`라 소스 파일에 문자열이
/// 실재하는지만 보는 약한 검사였고, 판정표의 네 규칙 중 유일하게 그랬다. 규칙을
/// `Shared/StatusAnnounce.swift`로 꺼내면서 "이 입력에 이 출력을 낸다"가 됐다.
final class StatusLabelFixtureTests: XCTestCase {

    /// 공용 픽스처(`testdata/*.json`)는 웹과 공유하는 **한국어** 기준값이다. 시뮬레이터의
    /// 선호 언어는 영어라 고정하지 않으면 영어 문구와 비교하게 된다 — 웹의 vitest도
    /// 같은 이유로 `setLang('ko')`를 부른다. 픽스처가 두 언어를 갖게 되면 그때 두 번 돈다.
    override func setUp() {
        super.setUp()
        L.set(.ko)
    }
    private struct Fixture: Decodable {
        let labels: [String: String]
        let retryWaiting: String
    }

    private func fixture() throws -> Fixture {
        let url = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "status-labels", withExtension: "json"),
            "픽스처가 번들에 없다 — project.yml의 buildPhase: resources 확인"
        )
        return try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))
    }

    func testLabelsMatchTheFixture() throws {
        let fx = try fixture()
        let all: [Components.Schemas.LinkStatus] = [.pending, .scraping, .tagging, .done, .failed]

        // 다섯 상태가 전부 픽스처에 있어야 한다 — 하나가 빠지면 그 상태만 조용히 검사 밖이다.
        XCTAssertEqual(fx.labels.count, all.count, "픽스처와 enum의 개수가 다르다")

        for status in all {
            let want = try XCTUnwrap(fx.labels[status.rawValue],
                                     "\(status.rawValue)가 픽스처에 없다")
            XCTAssertEqual(StatusAnnounce.label(status), want,
                           "\(status.rawValue)의 라벨이 웹과 다르다(§8.1)")
        }
    }

    func testBackoffOutranksStatus() throws {
        let fx = try fixture()
        XCTAssertEqual(StatusAnnounce.announcement(.pending, retryWaiting: true), fx.retryWaiting,
                       "백오프 문구가 웹과 다르다")
        // 우선순위가 뒤집히면 done이 백오프를 삼킨다 — 웹 테스트와 같은 단언이다.
        XCTAssertEqual(StatusAnnounce.announcement(.done, retryWaiting: true), fx.retryWaiting,
                       "status를 먼저 보고 있다 — 백오프가 조용히 사라진다")
    }
}
