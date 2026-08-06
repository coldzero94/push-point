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

        /// 오늘의 한 건. `nil`을 돌려주면 204(후보 없음)로 답한다. 기본값은 `nil`을
        /// 돌려주는 것이 아니라 **없음**이라, 이 기능을 안 보는 테스트에서 `getResurfaced`를
        /// 부르면 여전히 터진다 — 부르지 않는다는 사실도 검사 대상이다.
        var resurfaced: (@Sendable () async -> Components.Schemas.Link?)?

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
        func getResurfaced(_: Operations.getResurfaced.Input) async throws
            -> Operations.getResurfaced.Output {
            guard let resurfaced else { fatalError("getResurfaced를 기대하지 않은 테스트가 불렀다") }
            if let link = await resurfaced() { return .ok(.init(body: .json(link))) }
            return .noContent(.init())
        }
        func search(_: Operations.search.Input) async throws -> Operations.search.Output { fatalError() }
        func getThumb(_: Operations.getThumb.Input) async throws -> Operations.getThumb.Output { fatalError() }
        func listTags(_: Operations.listTags.Input) async throws -> Operations.listTags.Output { fatalError() }
        func createTag(_: Operations.createTag.Input) async throws -> Operations.createTag.Output { fatalError() }
        func updateTag(_: Operations.updateTag.Input) async throws -> Operations.updateTag.Output { fatalError() }
        func deleteTag(_: Operations.deleteTag.Input) async throws -> Operations.deleteTag.Output { fatalError() }
        func getStats(_: Operations.getStats.Input) async throws -> Operations.getStats.Output { fatalError() }
        func getSheetsStatus(_: Operations.getSheetsStatus.Input) async throws -> Operations.getSheetsStatus.Output { fatalError() }
        func getSheetsScript(_: Operations.getSheetsScript.Input) async throws -> Operations.getSheetsScript.Output { fatalError() }
        func connectSheets(_: Operations.connectSheets.Input) async throws -> Operations.connectSheets.Output { fatalError() }
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

    // MARK: - 오늘의 한 건
    //
    // 이건 화면의 **세 번째 목록**이고, 이 파일 맨 위 문단이 예고한 자리다: 목록이 하나
    // 더 생겼는데 변경이 그걸 안 건드리면 카드가 유령으로 남는다. 아래 둘은 그 사고를
    // 만들어 두고 막혔는지 본다.

    /// 목록에 7이 **들어 있는** 스텁. 되살림 링크는 아카이브의 일부이므로 이게 실제
    /// 형편이고, 빈 목록에서 돌리면 `links.removeAll`과 `resurfaced = nil`이 같은 링크에
    /// 대해 함께 도는 경로를 한 번도 안 밟는다 — "카드가 지워진다"만 보고 "목록에서도
    /// 지워진다"는 안 보게 된다.
    private static func resurfacing(_ id: Int?) -> StubClient {
        var c = Self.stub { _ in Self.page([1, 7, 2], next: nil) }.0
        c.resurfaced = { id.map { Self.link($0) } }
        return c
    }

    /// **유령 카드.** 되살림 카드를 밀어 지우면 토스트는 뜨는데 카드가 남고, 눌러
    /// 들어가면 404다 — 검색 결과에서 이미 한 번 난 사고를 목록 하나 더로 재현한 것.
    func testDeletingTheResurfacedLinkClearsTheCard() async {
        let client = Self.resurfacing(7)
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        await feed.loadResurfaced(client)
        XCTAssertEqual(feed.resurfaced?.id, 7)
        XCTAssertTrue(feed.links.contains { $0.id == 7 }, "전제가 틀렸다 — 목록에 7이 없다")

        feed.apply(.removed(7))

        XCTAssertNil(feed.resurfaced, "지운 링크가 되살림 카드로 화면에 남았다")
        XCTAssertFalse(feed.links.contains { $0.id == 7 }, "목록에서는 안 지워졌다")
    }

    /// 재시도로 상태가 바뀌면 카드도 같이 바뀌어야 한다. 안 바뀌면 목록의 같은 링크는
    /// `done`인데 맨 위 카드만 `failed`로 남아, 한 화면이 자기 자신과 모순된다.
    func testRetryingTheResurfacedLinkUpdatesTheCard() async {
        let client = Self.resurfacing(7)
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        await feed.loadResurfaced(client)
        XCTAssertEqual(feed.resurfaced?.status, .done)

        feed.apply(.replaced(Self.link(7, status: .failed)))

        XCTAssertEqual(feed.resurfaced?.status, .failed, "카드가 옛 상태로 남았다")
        XCTAssertEqual(feed.links.first { $0.id == 7 }?.status, .failed, "목록 쪽이 안 바뀌었다")
    }

    /// 후보가 없으면 서버가 204를 준다. 그때 **직전 답을 붙들고 있으면 안 된다** —
    /// 오늘 그 링크를 열어서 후보에서 빠졌는데 카드가 그대로 있으면, 되살리기는
    /// "이미 본 것"을 매일 다시 들이미는 기능이 된다.
    func testNoContentClearsTheCard() async {
        let feed = FeedModel()
        await feed.loadResurfaced(Self.resurfacing(7))
        XCTAssertNotNil(feed.resurfaced)

        await feed.loadResurfaced(Self.resurfacing(nil))

        XCTAssertNil(feed.resurfaced, "204를 받고도 옛 카드를 들고 있다")
    }

    /// 방금 지운 링크가 되살림 자리로 돌아오는 것을 막는다. 서버는 삭제를 아직 모를 수
    /// 있고(요청이 엇갈리면), 그러면 되돌리기 토스트가 떠 있는 채로 같은 링크가 맨 위에
    /// 카드로 뜬다 — `pollRefresh`가 막는 것과 같은 사고다.
    func testDeletedLinkDoesNotComeBackAsTheCard() async {
        let feed = FeedModel()
        feed.apply(.removed(7))

        await feed.loadResurfaced(Self.resurfacing(7))

        XCTAssertNil(feed.resurfaced, "지운 링크가 되살림 카드로 되돌아왔다")
    }

    /// **되살림 링크가 목록의 꼬리일 때 페이지네이션이 멈추지 않아야 한다.**
    ///
    /// 화면에서 이게 회귀하면 아무 표시 없이 목록이 1장에서 끝난다 — 오류도, 스피너도,
    /// 더보기 버튼도 없다(iOS에는 그 폴백이 아예 없어서, `onAppear` 조건이 한 번 안 걸리면
    /// 2장에 도달할 경로가 하나도 남지 않는다). 되살림 후보는 `created_at <= now-7d`라
    /// 1장의 **꼬리 쪽에 몰리므로**, 하필 끝이 되는 것은 드문 경우가 아니다.
    ///
    /// 그래서 확인하는 것은 트리거가 쓰는 상수가 아니라 **두 꼬리가 실제로 다르다는 것**이다.
    func testBoardTailMovesWhenTheResurfacedLinkIsLast() async {
        let (client, _) = Self.stub { _ in Self.page([1, 2, 3], next: "c1") }
        let feed = FeedModel()
        await feed.load(client, filter: nil)
        let card = Self.link(3)

        let board = feed.board(hidingCard: card)

        XCTAssertEqual(board.map(\.id), [1, 2], "되살림 링크가 보드에 남았다 — 카드가 두 번 그려진다")
        XCTAssertEqual(board.last?.id, 2, "트리거가 걸릴 카드가 사라졌다 — 다음 장을 영영 못 받는다")
        XCTAssertNotEqual(board.last?.id, feed.links.last?.id,
                          "두 꼬리가 같으면 이 테스트는 아무것도 안 본다")
    }

    /// 카드가 없으면(204·필터 중) 보드는 목록 그대로다 — 여기서 한 건이라도 빠지면
    /// 그건 링크가 화면에서 사라지는 것이다.
    func testBoardIsUntouchedWithoutACard() async {
        let (client, _) = Self.stub { _ in Self.page([1, 2, 3], next: nil) }
        let feed = FeedModel()
        await feed.load(client, filter: nil)

        XCTAssertEqual(feed.board(hidingCard: nil).map(\.id), [1, 2, 3])
    }

    /// 되돌리면 그 보호도 풀려야 한다. 안 풀면 그 id는 세션 내내 "방금 지운 것"으로 남아
    /// 되살림 카드가 하루 종일 빈다 — 링크는 목록에 멀쩡히 돌아와 있는데.
    ///
    /// `forgetDeletion`은 이 테스트가 생기기 전에도 있었고 통과했지만, **앱은 그걸 부르지
    /// 않았다.** 모델이 맞는 것과 앱이 거기 배선된 것은 다른 사실이다.
    func testUndoLetsTheCardComeBackToo() async {
        let feed = FeedModel()
        feed.apply(.removed(7))
        feed.forgetDeletion(of: 7)

        await feed.loadResurfaced(Self.resurfacing(7))

        XCTAssertEqual(feed.resurfaced?.id, 7, "되돌린 링크가 되살림 카드로 안 돌아온다")
    }
}
