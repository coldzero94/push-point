import XCTest

/// `FeedModel`의 목록·커서·폴링 규칙.
///
/// **이 파일이 이 리팩토링의 값어치다.** 아래 케이스들은 전부 이번 세션에 실제로 난
/// 사고이고, 모델이 뷰 안에 있던 동안에는 **UI 테스트로만** 잡혔다 — 첫 번째는 링크
/// 60건을 심고 40번 스와이프하는 20초짜리 테스트가 잡았다. 여기서는 스텁 클라이언트로
/// 밀리초에 잡는다.
@MainActor
final class FeedModelTests: XCTestCase {
    // MARK: - 스텁

    /// 계약을 구현하되 목록만 답한다. 나머지는 부르면 테스트가 터지게 둔다 —
    /// 조용히 기본값을 돌려주면 "안 불렸다"와 "불렸는데 무시했다"가 같아진다.
    private struct StubClient: APIProtocol {
        /// 호출 순서대로 돌려줄 응답. cursor가 nil이면 첫 장 요청이다.
        let pages: @Sendable (String?) async throws -> Components.Schemas.LinkPage
        /// 실제로 나간 요청의 커서들. 페이지네이션이 정말 커서를 태우는지 본다.
        let recorder: Recorder

        final class Recorder: @unchecked Sendable {
            private(set) var cursors: [String?] = []
            private(set) var filters: [String?] = []
            func record(_ c: String?, _ tag: String?) { cursors.append(c); filters.append(tag) }
        }

        func listLinks(_ input: Operations.listLinks.Input) async throws
            -> Operations.listLinks.Output {
            recorder.record(input.query.cursor, input.query.tag)
            let page = try await pages(input.query.cursor)
            return .ok(.init(body: .json(page)))
        }

        // 아래는 이 테스트가 쓰지 않는다.
        func healthz(_: Operations.healthz.Input) async throws -> Operations.healthz.Output { fatalError() }
        func createLink(_: Operations.createLink.Input) async throws -> Operations.createLink.Output { fatalError() }
        func getLink(_: Operations.getLink.Input) async throws -> Operations.getLink.Output { fatalError() }
        func updateLink(_: Operations.updateLink.Input) async throws -> Operations.updateLink.Output { fatalError() }
        func deleteLink(_: Operations.deleteLink.Input) async throws -> Operations.deleteLink.Output { fatalError() }
        func retryLink(_: Operations.retryLink.Input) async throws -> Operations.retryLink.Output { fatalError() }
        func markOpened(_: Operations.markOpened.Input) async throws -> Operations.markOpened.Output { fatalError() }
        func search(_: Operations.search.Input) async throws -> Operations.search.Output { fatalError() }
        func getThumb(_: Operations.getThumb.Input) async throws -> Operations.getThumb.Output { fatalError() }
        func listTags(_: Operations.listTags.Input) async throws -> Operations.listTags.Output { fatalError() }
        func createTag(_: Operations.createTag.Input) async throws -> Operations.createTag.Output { fatalError() }
        func updateTag(_: Operations.updateTag.Input) async throws -> Operations.updateTag.Output { fatalError() }
        func deleteTag(_: Operations.deleteTag.Input) async throws -> Operations.deleteTag.Output { fatalError() }
        func getStats(_: Operations.getStats.Input) async throws -> Operations.getStats.Output { fatalError() }
        func getSheetsStatus(_: Operations.getSheetsStatus.Input) async throws -> Operations.getSheetsStatus.Output { fatalError() }
        func syncSheets(_: Operations.syncSheets.Input) async throws -> Operations.syncSheets.Output { fatalError() }
    }

    private nonisolated static func link(_ id: Int, status: Components.Schemas.LinkStatus = .done)
        -> Components.Schemas.Link {
        .init(id: id, url: "https://e.com/\(id)", domain: "e.com", title: "제목 \(id)",
              description: "", content_type: .article, thumb_url: nil, status: status,
              tags: [], note: "", created_at: 1_700_000_000,
              // 이 테스트는 목록·커서·폴링 규칙만 본다 — 실패 이력은 그 규칙과 무관하다.
              error: "", retry_state: .none)
    }

    private nonisolated static func page(_ ids: [Int], next: String?,
                      status: Components.Schemas.LinkStatus = .done)
        -> Components.Schemas.LinkPage {
        .init(links: ids.map { Self.link($0, status: status) }, next_cursor: next)
    }

    private nonisolated static func stub(_ f: @escaping @Sendable (String?) async throws -> Components.Schemas.LinkPage)
        -> (StubClient, StubClient.Recorder) {
        let r = StubClient.Recorder()
        return (StubClient(pages: f, recorder: r), r)
    }

    // MARK: - 페이지네이션

    func testLoadMoreAppendsAndCarriesTheCursor() async {
        let (client, rec) = Self.stub { cursor in
            cursor == nil ? Self.page([1, 2], next: "c1") : Self.page([3, 4], next: nil)
        }
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        await feed.loadMore(client, filter: nil)

        XCTAssertEqual(feed.links.map(\.id), [1, 2, 3, 4])
        XCTAssertEqual(rec.cursors, [nil, "c1"], "두 번째 요청이 커서를 태우지 않았다")
        XCTAssertFalse(feed.hasMore)
    }

    /// **이 세션에 실제로 난 사고.** 폴링이 `load()`를 불러 목록을 1장으로 갈아치웠고,
    /// 스크롤해 받아 둔 뒷장이 통째로 사라졌다. 60건을 심고 40번 스와이프하는 UI
    /// 테스트가 잡았다 — 여기서는 두 줄이면 된다.
    func testPollRefreshDoesNotDiscardLoadedPages() async {
        let (client, _) = Self.stub { cursor in
            cursor == nil ? Self.page([1, 2], next: "c1") : Self.page([3, 4], next: nil)
        }
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        await feed.loadMore(client, filter: nil)
        XCTAssertEqual(feed.links.count, 4)

        await feed.pollRefresh(client, filter: nil)

        XCTAssertEqual(feed.links.map(\.id), [1, 2, 3, 4],
                       "폴링이 뒷장을 버렸다 — load()를 부르고 있다")
    }

    func testPollRefreshUpdatesInPlace() async {
        var served = 0
        let (client, _) = Self.stub { _ in
            served += 1
            return Self.page([1], next: nil, status: served == 1 ? .pending : .done)
        }
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        XCTAssertTrue(feed.hasWorkInFlight)

        await feed.pollRefresh(client, filter: nil)

        XCTAssertEqual(feed.links.count, 1, "제자리 갱신이 아니라 중복으로 붙었다")
        XCTAssertFalse(feed.hasWorkInFlight, "상태가 갱신되지 않았다")
    }

    /// **삭제와 폴링 응답이 엇갈리는 경합.** 서버가 삭제 전에 계산한 1장이 오면 그 링크는
    /// `links`에 없으므로 "새 링크"로 판정돼 맨 위에 다시 붙는다 — 되돌리기 토스트가
    /// 떠 있는 채로. 시뮬레이터에서는 이 타이밍을 만들 수가 없다.
    func testPollRefreshDoesNotResurrectADeletedLink() async {
        let (client, _) = Self.stub { _ in Self.page([1, 2], next: nil) }
        let feed = FeedModel()
        await feed.load(client, filter: nil)

        feed.apply(.removed(1))
        XCTAssertEqual(feed.links.map(\.id), [2])

        await feed.pollRefresh(client, filter: nil) // 서버는 아직 1을 준다

        XCTAssertEqual(feed.links.map(\.id), [2], "지운 링크가 되살아났다")
    }

    func testUndoLetsTheLinkComeBack() async {
        let (client, _) = Self.stub { _ in Self.page([1, 2], next: nil) }
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        feed.apply(.removed(1))
        feed.forgetDeletion(of: 1)

        await feed.pollRefresh(client, filter: nil)

        XCTAssertEqual(Set(feed.links.map(\.id)), [1, 2], "되돌린 링크가 안 돌아왔다")
    }

    // MARK: - 필터

    /// 커서에 필터가 실려 있지 않으므로 매 요청에 같이 보내야 한다. 빠뜨리면
    /// **두 번째 장부터 필터가 풀린다** — 사용자는 태그 필터가 켜진 화면에서
    /// 스크롤하다가 관계없는 링크를 만난다.
    func testFilterRidesEveryPage() async {
        let (client, rec) = Self.stub { cursor in
            cursor == nil ? Self.page([1], next: "c1") : Self.page([2], next: nil)
        }
        let feed = FeedModel()
        await feed.load(client, filter: .tag("개발"))
        await feed.loadMore(client, filter: .tag("개발"))

        XCTAssertEqual(rec.filters, ["개발", "개발"], "두 번째 장에서 필터가 풀렸다")
    }

    // MARK: - 실패

    func testLoadFailureIsSurfaced() async {
        struct Boom: Error, LocalizedError { var errorDescription: String? { "터졌다" } }
        let (client, _) = Self.stub { _ in throw Boom() }
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        XCTAssertEqual(feed.loadError, "터졌다")
        XCTAssertTrue(feed.links.isEmpty)
    }

    /// 폴링 실패는 **조용해야 한다.** 1.5초마다 도는 것이 실패했다고 화면을 오류로
    /// 덮으면 목록이 깜빡인다. 대신 갱신이 안 될 뿐 기존 목록은 그대로 남는다.
    func testPollFailureKeepsTheList() async {
        struct Boom: Error {}
        var first = true
        let (client, _) = Self.stub { _ in
            if first { first = false; return Self.page([1, 2], next: nil) }
            throw Boom()
        }
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        await feed.pollRefresh(client, filter: nil)

        XCTAssertEqual(feed.links.map(\.id), [1, 2], "폴링 실패가 목록을 지웠다")
        XCTAssertNil(feed.loadError, "폴링 실패를 화면 오류로 올렸다")
    }

    // MARK: - 중복

    func testLoadMoreIgnoresDuplicates() async {
        let (client, _) = Self.stub { cursor in
            cursor == nil ? Self.page([1, 2], next: "c1") : Self.page([2, 3], next: nil)
        }
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        await feed.loadMore(client, filter: nil)
        XCTAssertEqual(feed.links.map(\.id), [1, 2, 3], "같은 링크가 두 장 생겼다")
    }
}
