import Foundation

/// 목록에 반영해야 할 링크 한 건의 변화. `FeedModel.apply(_:)`가 유일한 입구다.
///
/// 화면에 목록이 둘(보드·검색 결과)인데 삭제·재시도가 하나만 건드리고 있었다. 검색
/// 결과에서 카드를 밀어 지우면 토스트는 뜨는데 카드가 남고, 눌러 들어가면 404가 났다.
/// 목록이 하나 더 생겨도 같은 사고가 나지 않도록 입구를 하나로 둔다.
enum LinkChange {
    case removed(Int)
    case replaced(Components.Schemas.Link)
}

/// 보드가 보여주는 링크 목록과 그 페이지네이션·폴링.
///
/// ## 왜 뷰에서 떼어 냈는가 (2026-07-29)
///
/// `ContentView`가 672줄에 함수·상태 33개였고, 그중 여섯 개의 async 함수가 같은 배열
/// 넷을 직렬화 없이 건드리고 있었다. 이 세션에 그 파일에서 난 사고가 그 형태다:
///
/// - 폴링이 `load()`를 불러 **스크롤해 받아 둔 뒷장을 통째로 버렸다**
/// - 삭제와 폴링 응답이 엇갈려 **지운 링크가 맨 위로 되살아났다**
/// - 목록이 둘인데 변경이 하나만 건드려 **검색 결과의 카드가 안 지워졌다**
///
/// **셋 다 뷰 안에 있는 한 UI 테스트로만 잡힌다.** 실제로 첫 번째는 60건을 심고 40번
/// 스와이프하는 20초짜리 테스트가 잡았다. 여기로 옮기면 같은 것을 스텁 클라이언트로
/// 밀리초에 잡는다 — **이 파일의 값어치는 구조가 아니라 그 테스트다.**
///
/// 동작은 옮기면서 바꾸지 않았다. 커서·중복 방지·폴링 조건·삭제 보호가 전부 그대로다.
@MainActor
@Observable
final class FeedModel {
    private(set) var links: [Components.Schemas.Link] = []
    private(set) var loadError: String?

    /// 오늘의 한 건 — 잊고 있던 링크 하나. 후보가 없으면 서버가 204를 주고 여기는 `nil`이다.
    ///
    /// **뷰가 아니라 여기에 두는 이유**는 이 파일 맨 위 문단 그대로다. 이건 화면의 세 번째
    /// 목록이고, 목록이 하나 더 생겼는데 변경이 그걸 안 건드리면 나는 사고는 이미 한 번
    /// 났다 — 카드를 밀어 지우면 토스트는 뜨는데 카드가 남고, 눌러 들어가면 404다.
    /// `apply(_:)` 안에 두면 그 사고가 20초짜리 UI 테스트가 아니라 밀리초에 잡힌다.
    private(set) var resurfaced: Components.Schemas.Link?

    private var nextCursor: String?
    private var loadingMore = false

    /// 방금 지운 링크 id. **폴러가 되살리는 것을 막는다.**
    ///
    /// `pollRefresh`는 서버의 1장을 받아 덮어쓰는데, 삭제 요청과 폴링 응답이 엇갈리면
    /// (응답이 삭제 전 상태로 계산됨) 지운 링크가 `links`에 없으므로 "새 링크"로 판정돼
    /// 맨 위에 다시 붙는다 — 되돌리기 토스트가 떠 있는 채로.
    private var recentlyDeleted: Set<Int> = []

    /// 더 받을 장이 남았는가. 뷰가 마지막 카드에서 `loadMore`를 부를지 정할 때 쓴다.
    var hasMore: Bool { nextCursor != nil }

    /// 진행 중(종단이 아닌) 링크가 있는가 — 폴링을 돌릴 조건.
    var hasWorkInFlight: Bool {
        links.contains { $0.status != .done && $0.status != .failed }
    }

    /// 폴링 task의 identity. 진행 중 링크의 **집합이 바뀔 때만** 값이 바뀌므로
    /// 매 갱신마다 task가 재시작되지 않는다.
    var pollKey: String {
        let working = links.filter { $0.status != .done && $0.status != .failed }
        return working.isEmpty ? "idle" : working.map { String($0.id) }.joined(separator: ",")
    }

    // MARK: - 읽기

    func load(_ client: any APIProtocol, filter: ListFilter?) async {
        do {
            let page = try await fetchPage(client, filter: filter, cursor: nil)
            links = page.links
            nextCursor = page.next_cursor
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }

    /// 오늘의 한 건을 받아 둔다. **실패는 조용히 넘긴다** — 이건 목록이 아니라 덤이고,
    /// 없으면 카드 한 장이 안 보일 뿐이다. 여기서 `loadError`를 쓰면 목록이 멀쩡히 있는
    /// 화면에 "불러오지 못했습니다"가 뜬다.
    ///
    /// 서버가 하루 동안 같은 답을 주므로 자주 부를 이유가 없지만, 부른다고 답이 흔들리지도
    /// 않는다 — 웹의 1시간 staleTime과 같은 판단이고 여기서는 새로고침에 얹는 것으로 족하다.
    func loadResurfaced(_ client: any APIProtocol) async {
        guard let out = try? await client.getResurfaced(.init()) else { return }
        switch out {
        case let .ok(r):
            // 방금 지운 링크가 되살림 자리로 돌아오는 것을 막는다. 서버는 삭제를 아직
            // 모를 수 있고(요청이 엇갈리면), 그러면 되돌리기 토스트가 떠 있는 채로 같은
            // 링크가 맨 위에 카드로 뜬다 — `pollRefresh`가 막는 것과 같은 사고다.
            if let link = try? r.body.json, !recentlyDeleted.contains(link.id) {
                resurfaced = link
            }
        case .noContent:
            resurfaced = nil
        default:
            break
        }
    }

    /// 다음 장. 마지막 카드가 보이면 불린다.
    ///
    /// **더보기 버튼을 두지 않는 이유**는 취향이 아니다. 이 앱은 아카이브이고, 목록을
    /// 거슬러 올라가는 것이 저장한 것을 되찾는 주된 방법이다 — 그 길목마다 버튼을 세우면
    /// 되찾기 자체가 일이 된다. 대신 실패했을 때 조용하지 않아야 해서 loadError로 드러낸다.
    func loadMore(_ client: any APIProtocol, filter: ListFilter?) async {
        guard !loadingMore, let cursor = nextCursor else { return }
        loadingMore = true
        defer { loadingMore = false }
        do {
            let page = try await fetchPage(client, filter: filter, cursor: cursor)
            // 중복 방지. keyset 커서는 원리상 안 겹치지만, 같은 커서로 두 번 불리는 경합이
            // 남아 있으면 같은 카드가 두 장 생기고 ForEach(id:)가 런타임에 경고를 뱉는다.
            let known = Set(links.map(\.id))
            links.append(contentsOf: page.links.filter { !known.contains($0.id) })
            nextCursor = page.next_cursor
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }

    /// 폴링용 갱신. **`load`를 부르면 안 된다** — 그건 links를 1장으로 갈아치워서
    /// 이미 스크롤해 받아 둔 뒷장을 버린다. 진행 중인 링크는 거의 항상 맨 위에 있으므로
    /// 첫 장만 다시 받아 **제자리로 덮어쓴다.** 커서와 뒷장은 건드리지 않는다.
    func pollRefresh(_ client: any APIProtocol, filter: ListFilter?) async {
        guard let page = try? await fetchPage(client, filter: filter, cursor: nil) else { return }
        var byID = [Int: Int]()
        for (i, l) in links.enumerated() { byID[l.id] = i }
        var fresh: [Components.Schemas.Link] = []
        for l in page.links {
            if recentlyDeleted.contains(l.id) { continue }
            if let i = byID[l.id] { links[i] = l } else { fresh.append(l) }
        }
        // 다른 경로(공유 시트)로 들어온 링크는 앞에 붙인다.
        if !fresh.isEmpty { links.insert(contentsOf: fresh, at: 0) }
    }

    // MARK: - 쓰기

    /// 링크 한 건의 변화를 반영한다. 삭제·교체가 지나는 유일한 입구다.
    func apply(_ change: LinkChange) {
        switch change {
        case let .removed(id):
            links.removeAll { $0.id == id }
            if resurfaced?.id == id { resurfaced = nil }
            recentlyDeleted.insert(id)
        case let .replaced(link):
            if let i = links.firstIndex(where: { $0.id == link.id }) { links[i] = link }
            if resurfaced?.id == link.id { resurfaced = link }
        }
    }

    /// 되돌리기가 성공했을 때 — 그 id는 다시 살아 있으므로 폴러가 가져와야 한다.
    func forgetDeletion(of id: Int) { recentlyDeleted.remove(id) }

    /// UI 테스트가 되살림 카드를 세운다. **`-uitest`로 뜬 앱에서만 동작한다.**
    ///
    /// 서버를 거치지 않는 유일한 상태 설정이라 문을 좁게 낸다. 왜 이 문이 필요한지는
    /// `UITestMode.seedResurfaceFlag`에 있다 — 실제 시간으로는 이 상태를 만들 수 없고,
    /// 네 가지 우회로가 전부 막혀 있다.
    func seedResurfaced(_ link: Components.Schemas.Link) {
        guard UITestModeFlags.isActive else { return }
        resurfaced = link
    }

    // MARK: - 보드가 그리는 것

    /// 보드가 실제로 그릴 목록. 되살림 카드로 올라간 한 건은 뺀다.
    ///
    /// **사본이 아니라 이동이다.** 빼지 않으면 같은 카드가 한 화면에 두 번 나온다 —
    /// 되살림은 아카이브 전체에서 고르는데 7일 지난 링크도 1장 안에 흔히 들어 있어서,
    /// 아카이브가 작을수록 반드시 겹친다(5건짜리 웹 아카이브에서 실제로 두 번 나왔다).
    ///
    /// **뷰가 아니라 여기 있는 이유는 페이지네이션이다.** 다음 장을 당기는 조건이
    /// "마지막 카드가 보이면"인데, 그 마지막은 `links`의 끝이 아니라 **보드가 그린 것**의
    /// 끝이어야 한다. 되살림으로 올라간 링크가 하필 `links`의 끝이면 그 id를 가진 카드가
    /// 보드에 없어 조건이 영영 성립하지 않고, iOS에는 더보기 버튼도 없어서 **목록이 1장에서
    /// 조용히 끝난다** — 오류도 스피너도 없이. 뷰 안에 두면 이걸 검사할 방법이 없다.
    func board(hidingCard card: Components.Schemas.Link?) -> [Components.Schemas.Link] {
        guard let id = card?.id else { return links }
        return links.filter { $0.id != id }
    }

    /// 화면이 그릴 것 — 위에 얹을 카드 하나와 보드가 그리는 목록.
    ///
    /// **규칙 둘이 여기 함께 있어야 한다.** 카드를 고르는 규칙(좁혀진 화면에는 안 그린다)이
    /// 뷰에, 빼는 규칙이 모델에 나뉘어 있으면 공유 픽스처가 절반만 잰다. 웹의
    /// `lib/board.ts`가 같은 두 규칙을 한 함수로 들고 있고,
    /// `testdata/resurface-board-cases.json`이 **두 구현의 결과**를 대조한다.
    ///
    /// `filtered`만 받고 무엇으로 좁혔는지는 묻지 않는다 — 웹은 tag·status·unopened
    /// 셋이고 여기는 tag·failed 둘이라, 공유할 수 있는 것은 "좁혀졌다"까지다.
    func boardView(filtered: Bool) -> (card: Components.Schemas.Link?,
                                       board: [Components.Schemas.Link]) {
        Self.boardView(links: links, resurfaced: resurfaced, filtered: filtered)
    }

    /// 규칙 자체. **상태를 안 읽는 순수 함수인 것이 핵심이다** — 픽스처 테스트가
    /// 이걸 그대로 부르므로 모델에 테스트 전용 setter를 뚫지 않아도 되고, 웹의
    /// `boardView`와 모양이 같아 두 구현을 나란히 읽을 수 있다.
    nonisolated static func boardView(links: [Components.Schemas.Link],
                                      resurfaced: Components.Schemas.Link?,
                                      filtered: Bool)
        -> (card: Components.Schemas.Link?, board: [Components.Schemas.Link]) {
        guard !filtered, let card = resurfaced else { return (nil, links) }
        return (card, links.filter { $0.id != card.id })
    }

    /// 변경(삭제·재시도)이 실패했을 때 목록 오류 자리를 쓰지 않기 위해 뷰가 지운다.
    func clearLoadError() { loadError = nil }

    // MARK: -

    /// 목록 한 장. 커서만 다르고 필터는 항상 같이 간다 — 커서에 필터가 실려 있지 않으므로
    /// 빠뜨리면 두 번째 장부터 필터가 풀린다.
    private func fetchPage(_ client: any APIProtocol, filter: ListFilter?, cursor: String?)
        async throws -> Components.Schemas.LinkPage {
        var query = Operations.listLinks.Input.Query(limit: 50)
        query.cursor = cursor
        switch filter {
        case let .tag(name): query.tag = name
        case .failed: query.status = .failed
        case nil: break
        }
        return try await client.listLinks(.init(query: query)).ok.body.json
    }
}
