import XCTest

/// facet 한글 라벨을 **웹과 같은 파일로** 대조한다.
///
/// 13 §3의 판정표에서 마지막까지 "일치 검사: 없음"으로 남아 있던 항목이다. 그 표에 실린
/// 다른 규칙들은 픽스처를 붙이자마자 실제 갈라짐이 드러났다 — streak은 셋 중 둘이 **틀린
/// 답에서** 일치하고 있었고, 상대 시각은 픽스처 자신이 먼저 틀렸다.
///
/// 상태 라벨(`StatusLabelFixtureTests`)과 달리 **여기서는 함수를 직접 부른다.**
/// `PP.Facet.label`은 뷰가 아니라 열거형이라 테스트 타깃에서 보인다 — 그래서 "소스에
/// 문자열이 있다"가 아니라 "이 입력에 이 출력을 낸다"를 검사한다.
final class FacetLabelFixtureTests: XCTestCase {
    private struct Fixture: Decodable { let labels: [String: String] }

    func testMatchesTheSharedFixture() throws {
        let url = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "facet-labels", withExtension: "json"),
            "픽스처가 번들에 없다 — project.yml의 buildPhase: resources 확인"
        )
        let fx = try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))

        // 계약의 네 값이 전부 픽스처에 있어야 한다 — 하나가 빠지면 그 facet만 조용히
        // 검사 밖에 남는다.
        XCTAssertEqual(fx.labels.count, PP.Facet.allCases.count,
                       "픽스처와 enum의 개수가 다르다")

        for facet in PP.Facet.allCases {
            let want = try XCTUnwrap(fx.labels[facet.rawValue],
                                     "\(facet.rawValue)가 픽스처에 없다")
            XCTAssertEqual(facet.label, want,
                           "\(facet.rawValue)의 라벨이 웹과 다르다 — 두 화면이 같은 태그를 "
                               + "다른 이름으로 부르면 사용자가 같은 것으로 못 읽는다(§8.1)")
        }
    }
}
