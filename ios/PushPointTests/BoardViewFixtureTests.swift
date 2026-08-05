import XCTest

/// 되살림 카드가 보드에 미치는 영향 — **웹과 같은 파일을 읽는다**
/// (`testdata/resurface-board-cases.json`, 웹은 `src/lib/board.test.ts`).
///
/// 이 규칙은 이미 한 번 갈렸다: 웹이 `!tag`를 보고 이쪽이 "필터 없음"을 봐서, 웹에서는
/// "실패"로 좁힌 목록 맨 위에 멀쩡한 링크가 얹혀 있었다. **양쪽 화면이 각각은 멀쩡해
/// 보였다는 것이 요점이다** — 갈라진 것을 알려면 두 결과를 나란히 놓는 수밖에 없었다.
///
/// 고정하는 것은 필터 입력이 아니라 **결과**(`cardId`·`boardIds`)다. 입력을 고정하면 두
/// 구현이 같은 것을 받는다는 것만 확인되고 무엇을 그리는지는 안 본다 — `cover-cases.json`이
/// 통과하는 동안 `cover-ops.json`이 잡아낸 그 차이다.
@MainActor
final class BoardViewFixtureTests: XCTestCase {
    private struct Fixture: Decodable {
        struct Case: Decodable {
            let name: String
            let links: [Int]
            let resurfaced: Int?
            let filtered: Bool
            let cardId: Int?
            let boardIds: [Int]
        }
        let cases: [Case]
    }

    private func loadFixture() throws -> Fixture {
        let url = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "resurface-board-cases", withExtension: "json"),
            "픽스처가 테스트 번들에 없다 — project.yml의 resources를 볼 것"
        )
        return try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))
    }

    private static func link(_ id: Int) -> Components.Schemas.Link {
        .init(id: id, url: "https://e.com/\(id)", domain: "e.com", title: "제목 \(id)",
              description: "", content_type: .article, thumb_url: nil, status: .done,
              tags: [], note: "", created_at: 1_700_000_000, error: "", retry_state: .none)
    }

    func testMatchesTheSharedFixture() throws {
        let fixture = try loadFixture()
        // 파일을 잘못 읽어 0건이 되면 아무것도 안 돌고 초록이 된다 — 검사가 사라진 것이
        // 통과로 보이는 자리라 못 박는다. 웹 테스트에도 같은 단언이 있다.
        XCTAssertGreaterThan(fixture.cases.count, 5, "픽스처가 비었거나 덜 읽혔다")

        for c in fixture.cases {
            let view = FeedModel.boardView(links: c.links.map(Self.link),
                                           resurfaced: c.resurfaced.map(Self.link),
                                           filtered: c.filtered)

            XCTAssertEqual(view.card?.id, c.cardId, "카드가 다르다 — \(c.name)")
            XCTAssertEqual(view.board.map(\.id), c.boardIds, "보드가 다르다 — \(c.name)")
        }
    }

    /// 보드는 링크를 잃지 않는다 — 카드로 간 한 건만 빠진다. 픽스처 값과 별개로 규칙
    /// 자체를 본다(웹 테스트에도 같은 케이스가 있다).
    func testBoardLosesNothingButTheCard() throws {
        for c in try loadFixture().cases {
            let view = FeedModel.boardView(links: c.links.map(Self.link),
                                           resurfaced: c.resurfaced.map(Self.link),
                                           filtered: c.filtered)

            var shown = Set(view.board.map(\.id))
            if let card = view.card, c.links.contains(card.id) { shown.insert(card.id) }
            XCTAssertEqual(shown.sorted(), c.links.sorted(), "링크가 화면에서 사라졌다 — \(c.name)")
        }
    }
}
