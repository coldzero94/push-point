import SwiftUI

/// 목록 — 시간 척추로 끊은 카드 보드.
///
/// 행이 아니라 카드인 이유(§4.4): 계약이 이미 주는 `description`을 행이 한 글자도 쓰지
/// 않아 화면이 "내가 모은 것"이 아니라 "레코드 목록"으로 읽혔다. 그리고 개인 아카이브에서
/// 회상의 단서는 대개 "언제"이므로 날짜로 끊는다 — keyset 커서가 `(created_at, id)`
/// 정렬이라 페이지 경계와 섹션 경계가 자연스럽게 맞는다.
struct ContentView: View {
    /// 태그 이름 → facet. 탭 컨테이너(RootView)가 받아 두 탭이 나눠 쓴다 —
    /// 탭마다 따로 받으면 같은 태그가 화면마다 다른 색이 될 수 있다.
    let facets: [String: PP.Facet]
    /// 통계에서 넘어온 필터. 켜져 있으면 목록이 좁혀지고 해제 칩이 뜬다.
    @Binding var filter: ListFilter?

    @EnvironmentObject private var backend: Backend
    @State private var links: [Components.Schemas.Link] = []
    @State private var loadError: String?
    /// 방금 지운 링크. 되돌릴 수 있는 동안만 들고 있는다.
    @State private var justDeleted: Components.Schemas.Link?
    @State private var undoTask: Task<Void, Never>?
    /// 열어 볼 링크. NavigationLink 대신 이 값으로 이동한다 — 아래 row 주석 참조.
    @State private var opening: OpeningLink?
    /// 검색어. 비어 있으면 평소의 보드, 있으면 검색 결과가 그 자리를 대신한다.
    @State private var query = ""
    @State private var results: [Components.Schemas.Link] = []
    /// 서버가 어느 경로로 찾았는지(fts | like). 두 글자 이하로 친 사용자가 결과가
    /// 적은 이유를 알 수 있게 화면에 남긴다 — 계약이 이걸 알려주는 이유가 그것이다.
    @State private var searchMode: String?
    @State private var searching = false
    /// 검색 실패. **loadError와 따로 둔다** — searchContent는 loadError를 읽지 않고,
    /// 검색이 실패했다고 목록 화면 전체를 오류로 바꾸면 저장한 것이 사라진 것처럼 보인다.
    @State private var searchError: String?
    /// 검색 결과 다음 장 실패. 첫 장 실패와 화면에서 다뤄야 할 자리가 다르다 —
    /// 이쪽은 이미 보이는 결과 아래에 붙는 푸터다.
    @State private var searchMoreError: String?
    /// 다음 페이지 커서. nil이면 마지막 페이지다 — 계약이 그렇게 말한다.
    /// **페이지 번호가 아니다.** keyset 커서라 목록에 쓰기가 일어나도 항목이 건너뛰거나
    /// 중복되지 않는다(.claude/rules/ios.md).
    @State private var nextCursor: String?
    @State private var searchCursor: String?
    /// 다음 장을 이미 받고 있는지. 없으면 마지막 카드가 화면에 머무는 동안 같은 요청이
    /// 여러 번 나간다 — onAppear는 스크롤 중 여러 번 불린다.
    @State private var loadingMore = false

    var body: some View {
        NavigationStack {
            content
                .background(PP.Palette.canvas)
                .navigationTitle("Push-Point")
            .navigationDestination(item: $opening) { target in
                LinkDetailView(linkID: target.id,
                               facetOf: { facets[$0] ?? .neutral },
                               dictionary: facets)
            }
        }
        // 검색은 목록 안에 있다 — 별도 탭으로 빼지 않았다. 찾는 대상이 바로 이 목록이고,
        // 탭을 나누면 "필터가 걸린 목록"과 "검색 결과"라는 거의 같은 두 화면을 사용자가
        // 구분해 가며 써야 한다. `.searchable`은 iOS가 이미 가르쳐 둔 자리이기도 하다.
        .searchable(text: $query, prompt: "제목 · 메모 · 태그")
        .task(id: backend.state) { await load() }
        .task(id: filter) { await load() }
        // 타이핑마다 요청을 보내지 않는다. 한 글자마다 FTS를 때리면 폰 안에서 도는
        // 서버라 더 잘 보인다 — 입력이 멎은 뒤에 한 번만 간다.
        .task(id: query) { await runSearch() }
        // 확인창 대신 되돌리기. 확인창은 **모든** 삭제를 느리게 만들어 흔한 경우에
        // 세금을 매기고, 서버가 소프트 삭제라 되살릴 수단이 실제로 있다.
        .overlay(alignment: .bottom) {
            if let link = justDeleted {
                UndoToast(message: "삭제했습니다") { Task { await undo(link) } }
            }
        }
        .animation(.smooth(duration: 0.25), value: justDeleted?.id)
    }

    @ViewBuilder
    private var content: some View {
        switch backend.state {
        case .idle, .starting:
            ProgressView("서버 시작 중…").padding(.top, 60)
        case let .failed(message):
            ContentUnavailableView("서버를 시작하지 못했습니다",
                                   systemImage: "exclamationmark.triangle",
                                   description: Text(message))
        case .running:
            if !query.isEmpty {
                searchContent
            } else if let loadError {
                // 빈/오류 상태도 스크롤 뷰에 담는다. ContentUnavailableView는 스크롤이
                // 아니라서 그대로 두면 **바로 그때** — 다시 시도하고 싶은 순간 — 당겨서
                // 새로고침이 안 된다.
                refreshableState {
                    ContentUnavailableView("목록을 불러오지 못했습니다",
                                           systemImage: "wifi.exclamationmark",
                                           description: Text(loadError))
                }
            } else if links.isEmpty {
                refreshableState {
                    ContentUnavailableView("아직 저장한 링크가 없습니다", systemImage: "tray",
                                           description: Text("공유 시트로 링크를 보내면 여기에 쌓입니다."))
                }
            } else {
                board
            }
        }
    }

    /// 검색 결과.
    ///
    /// 결과도 **같은 카드**로 그린다. 검색이 따로 생긴 화면이 아니라 같은 보드를 좁혀 본
    /// 것이라는 감각이 유지돼야 하고, 카드가 이미 제목·설명·태그·커버를 다 보여주므로
    /// 검색 전용 행을 새로 만들 이유가 없다.
    ///
    /// 다만 날짜 척추는 없앤다. 검색 결과의 정렬은 bm25 관련도이지 시간이 아니라서,
    /// 시간으로 끊으면 있지도 않은 시간 순서를 주장하게 된다.
    @ViewBuilder
    private var searchContent: some View {
        if searching && results.isEmpty {
            refreshableState { ProgressView().padding(.top, 60) }
        } else if let searchError {
            // **빈 결과보다 먼저 본다.** 순서가 뒤바뀌면 실패가 "결과가 없습니다"라는
            // 단정문으로 위장되고, 그건 사용자에게 "이 검색어로는 저장한 게 없다"는
            // 거짓말이 된다 — 아카이브에서 가장 나쁜 실패 방식이다.
            refreshableState {
                ContentUnavailableView {
                    Label("검색하지 못했습니다", systemImage: "wifi.exclamationmark")
                } description: {
                    Text(searchError)
                } actions: {
                    Button("다시 시도") { Task { await runSearch(debounce: false) } }
                }
            }
        } else if results.isEmpty {
            refreshableState {
                ContentUnavailableView("결과가 없습니다", systemImage: "magnifyingglass",
                                       description: Text("\u{201C}\(query)\u{201D}와 맞는 링크를 찾지 못했습니다."))
            }
        } else {
            List {
                if let mode = searchMode, mode == "like" {
                    // 세 글자 미만은 FTS가 아니라 LIKE로 간다(계약). 결과가 적을 때
                    // 사용자가 "없구나"가 아니라 "더 치면 되는구나"로 읽어야 한다.
                    Text("두 글자 이하는 제목·메모만 훑습니다. 세 글자부터 전문 검색으로 바뀝니다.")
                        .font(PP.Typo.meta)
                        .foregroundStyle(PP.Palette.fg3)
                        .plainRow(top: 10, bottom: 2)
                }
                ForEach(results, id: \.id) { link in
                    row(link)
                        .plainRow(top: 5, bottom: 5)
                        .onAppear {
                            if link.id == results.last?.id { Task { await searchMore() } }
                        }
                }
                // 다음 장 실패는 화면상 "여기가 끝"과 구분되지 않는다. 커서는 보존돼
                // 있으므로 같은 자리에서 그대로 재시도된다.
                if let searchMoreError {
                    HStack(spacing: 10) {
                        Text(searchMoreError)
                            .font(PP.Typo.meta)
                            .foregroundStyle(PP.Palette.danger)
                        Button("다시 시도") { Task { await searchMore() } }
                            .font(PP.Typo.label)
                            .foregroundStyle(PP.Palette.accent)
                        Spacer()
                    }
                    .plainRow(top: 10, bottom: 10)
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .refreshable { await runSearch() }
        }
    }

    /// 스크롤되지 않는 상태 화면을 당길 수 있게 감싼다. 내용이 화면보다 짧아도
    /// 제스처가 잡히도록 최소 높이를 준다.
    private func refreshableState<Content: View>(@ViewBuilder _ content: () -> Content)
        -> some View {
        ScrollView {
            content()
                .frame(maxWidth: .infinity, minHeight: 420)
        }
        .scrollBounceBehavior(.always)
        .refreshable { await load() }
    }

    /// 보드를 `List`로 만든다. 카드 모양은 그대로지만 스와이프 액션과 셀 재활용이
    /// 딸려 온다 — §8.4가 행 액션으로 `.swipeActions`를 지정하고 있고, 링크 10만 건이
    /// 목표라 재활용도 공짜로 얻을 이유가 없다. List가 기본으로 얹는 구분선·배경·여백은
    /// 전부 걷어내야 보드처럼 보인다.
    private var board: some View {
        List {
            if let filter {
                filterBar(filter).plainRow(top: 8, bottom: 0)
            }
            ForEach(sections, id: \.title) { section in
                Section {
                    ForEach(section.links, id: \.id) { link in
                        row(link)
                            .plainRow(top: 5, bottom: 5)
                            // 마지막 카드가 보이면 다음 장을 받는다. **날짜 구간이 아니라
                            // 전체 목록의 마지막**을 기준으로 삼는다 — 구간 단위로 보면
                            // "오늘" 섹션 끝에서도 발동해 아직 필요 없는 장을 당겨 온다.
                            .onAppear {
                                if link.id == links.last?.id { Task { await loadMore() } }
                            }
                    }
                } header: {
                    spine(section).plainRow(top: 14, bottom: 6)
                }
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .environment(\.defaultMinListHeaderHeight, 0)
        // 당겨서 새로고침은 **스크롤 뷰 자신**에 붙어야 한다. 바깥 컨테이너에 걸면
        // 목록이 비어 있을 때(ContentUnavailableView는 스크롤이 아니다) 제스처를 받을
        // 대상이 없어 조용히 아무 일도 일어나지 않는다.
        .refreshable { await load() }
    }

    /// `NavigationLink` 대신 버튼 + `navigationDestination`을 쓴다.
    ///
    /// List 안의 NavigationLink는 오른쪽에 화살표(disclosure indicator)를 붙이는데,
    /// 그건 **행의 어휘이지 카드의 어휘가 아니다** — §4.4의 카드 명세에 화살표는 없고,
    /// 카드는 그 자체가 탭 대상이라 "여기를 눌러라" 표시가 따로 필요 없다. 게다가
    /// 화살표는 카드 바깥에 떠서 카드와 분리돼 보인다.
    ///
    /// 스와이프가 있다는 힌트를 대신 넣지는 않았다. 목록을 옆으로 미는 것은 iOS에서
    /// 이미 배워진 동작이고, 발견되지 않는 경우를 위해 길게 누르기(컨텍스트 메뉴)를
    /// 함께 뒀다. 힌트 UI를 더하면 그건 화면에 상시로 남는 비용인데, 한 번 배우면
    /// 다시 필요 없는 정보에 그 자리를 내줄 이유가 없다(§1.3이 온보딩 투어를 금지하는 것과 같은 판단).
    private func row(_ link: Components.Schemas.Link) -> some View {
        Button {
            opening = OpeningLink(id: link.id)
        } label: {
            LinkCard(link: link,
                     facetOf: { facets[$0] ?? .neutral },
                     activeTag: activeTagName,
                     resolveThumb: backend.absoluteURL)
        }
        .buttonStyle(.plain)
        // 스와이프와 길게 누르기 둘 다 둔다(§8.4). 스와이프는 빠르지만 발견되지 않고,
        // 컨텍스트 메뉴는 느리지만 항상 찾을 수 있다.
        // 끝까지 밀면 버튼을 거치지 않고 바로 지운다 — 메시지 앱과 같은 동작이고,
        // 손이 이미 그렇게 배워 있다. 안전망은 아래 되돌리기 토스트다.
        .swipeActions(edge: .trailing, allowsFullSwipe: true) {
            Button(role: .destructive) { Task { await delete(link) } } label: {
                Label("삭제", systemImage: "trash")
            }
        }
        // 방향으로 성격을 나눈다. **오른쪽 끝은 파괴적인 것만** — iOS 전반의 관용이고,
        // 그래야 손이 기억한 방향이 다른 화면에서 배신하지 않는다.
        //
        // 왼쪽 끝은 되돌릴 수 있는 것. 정상 링크에는 **공유**를 둔다 — 원문 열기는
        // 카드를 눌러 들어간 상세의 기본 버튼과 같은 동작이라 여기 두면 중복이고,
        // "저장한 것을 남에게 보낸다"는 목록에서만 할 수 있는 다른 동작이다.
        // 실패한 링크는 공유할 내용 자체가 없으므로 재시도가 그 자리를 가져간다.
        .swipeActions(edge: .leading, allowsFullSwipe: false) {
            if link.status == .failed {
                Button { Task { await retry(link) } } label: {
                    Label("재시도", systemImage: "arrow.clockwise")
                }
                .tint(PP.Palette.warn)
            } else if let url = URL(string: link.url) {
                ShareLink(item: url) { Label("공유", systemImage: "square.and.arrow.up") }
                    .tint(PP.Palette.accent)
            }
        }
        .contextMenu {
            if let url = URL(string: link.url) {
                Link(destination: url) { Label("원문 열기", systemImage: "safari") }
                    .simultaneousGesture(TapGesture().onEnded { recordOpen(link) })
                ShareLink(item: url) { Label("공유", systemImage: "square.and.arrow.up") }
            }
            Button(role: .destructive) { Task { await delete(link) } } label: {
                Label("삭제", systemImage: "trash")
            }
        }
    }

    /// 켜진 필터를 화면에 남긴다. 통계에서 넘어왔는데 목록이 조용히 좁아져 있으면
    /// 사용자는 링크가 사라졌다고 읽는다 — 무엇이 걸려 있는지와 푸는 법이 같이 있어야 한다.
    private func filterBar(_ active: ListFilter) -> some View {
        HStack(spacing: 8) {
            Text(label(for: active))
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.fg2)
            Button { filter = nil } label: {
                Image(systemName: "xmark.circle.fill")
                    .font(PP.Typo.label)
                    .foregroundStyle(PP.Palette.fg3)
            }
            .buttonStyle(.plain)
            Spacer()
        }
        .padding(.horizontal, 11)
        .padding(.vertical, 7)
        .background(PP.Palette.hover)
        .clipShape(Capsule())
        .overlay(Capsule().strokeBorder(PP.Palette.lineControl, lineWidth: 1))
    }

    private func label(for active: ListFilter) -> String {
        switch active {
        case let .tag(name): "태그: \(name)"
        case .failed: "수집 실패만"
        }
    }

    /// 시간 척추 — serif 머리글 + 건수 + 하한선. serif가 쓰이는 유일한 자리다(§2.2.5).
    private func spine(_ section: DaySection) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 9) {
            Text(section.title)
                .font(PP.Typo.spine)
                .tracking(PP.Tracking.spine)
                .foregroundStyle(PP.Palette.fg1)
            Text("\(section.links.count)")
                .font(PP.Typo.metaMono)
                .foregroundStyle(PP.Palette.fg3)
            Rectangle().fill(PP.Palette.line1).frame(height: 1)
        }
    }

    private var activeTagName: String? {
        if case let .tag(name) = filter { return name }
        return nil
    }

    /// navigationDestination(item:)이 Identifiable을 요구해서 id만 감싼다.
    private struct OpeningLink: Identifiable, Hashable {
        let id: Int
    }

    // MARK: - 구간

    private struct DaySection {
        let title: String
        let links: [Components.Schemas.Link]
    }

    /// 오늘 · 어제 · 이번 주 · 이전. 절대 날짜가 아니라 상대 구간인 이유는, 찾을 때
    /// 떠오르는 것이 "며칠 전"이지 "7월 12일"이 아니기 때문이다.
    private var sections: [DaySection] {
        let cal = Calendar.current
        let now = Date()
        var buckets: [(String, [Components.Schemas.Link])] = [
            ("오늘", []), ("어제", []), ("이번 주", []), ("이전", []),
        ]
        for link in links {
            let date = Date(timeIntervalSince1970: TimeInterval(link.created_at))
            let index: Int
            if cal.isDateInToday(date) {
                index = 0
            } else if cal.isDateInYesterday(date) {
                index = 1
            } else if let days = cal.dateComponents([.day], from: cal.startOfDay(for: date),
                                                     to: cal.startOfDay(for: now)).day, days < 7 {
                // **달력 하루 단위로 센다.** 앞의 두 분기가 isDateInToday/isDateInYesterday라
                // 달력 기준인데 여기만 경과 시간으로 재면 축이 섞인다 — 6일 2시간 전 링크가
                // 달력으로는 7일 차이라 "이번 주"와 "이전" 사이에서 경계가 어긋난다.
                index = 2
            } else {
                index = 3
            }
            buckets[index].1.append(link)
        }
        return buckets.filter { !$0.1.isEmpty }.map { DaySection(title: $0.0, links: $0.1) }
    }

    /// 수집에 실패한 링크를 다시 큐에 넣는다. 실패는 통계가 아니라 할 일이라(통계 탭의
    /// "손이 필요한 것"과 같은 판단), 목록에서 바로 손댈 수 있어야 한다.
    private func retry(_ link: Components.Schemas.Link) async {
        guard let client = backend.client else { return }
        do {
            _ = try await client.retryLink(.init(path: .init(id: link.id)))
            // 상태가 pending으로 돌아가면 레일이 다시 켜지므로 목록을 새로 받는다.
            await load()
        } catch {
            loadError = error.localizedDescription
        }
    }

    /// 삭제 후 목록을 다시 받지 않고 **그 자리에서 빼는** 이유: 재조회는 왕복이 있어
    /// 방금 지운 카드가 잠깐 남아 있고, 그 사이 다시 누르면 404가 난다.
    private func delete(_ link: Components.Schemas.Link) async {
        guard let client = backend.client else { return }
        do {
            _ = try await client.deleteLink(.init(path: .init(id: link.id))).noContent
            withAnimation(.smooth(duration: 0.25)) {
                links.removeAll { $0.id == link.id }
            }
            justDeleted = link
            // 되돌리기 창을 닫는 타이머. 새로 지우면 앞의 타이머는 취소된다 —
            // 그러지 않으면 먼저 걸린 타이머가 나중 토스트를 지운다.
            undoTask?.cancel()
            undoTask = Task {
                try? await Task.sleep(for: .seconds(5))
                guard !Task.isCancelled else { return }
                if justDeleted?.id == link.id { justDeleted = nil }
            }
        } catch {
            loadError = error.localizedDescription
        }
    }

    /// 되돌리기는 같은 URL을 다시 저장하는 것이다 — store가 소프트 삭제된 행을 만나면
    /// undelete한다(별도 복구 엔드포인트가 없다). 단, 그 경로는 링크를 pending으로
    /// 되돌리고 다시 스크랩하므로 **태그·요약은 새로 만들어진다**. 링크가 돌아오는 것이
    /// 요점이고 그 값들은 어차피 파생물이라 받아들일 만하다.
    private func undo(_ link: Components.Schemas.Link) async {
        guard let client = backend.client else { return }
        justDeleted = nil
        undoTask?.cancel()
        do {
            _ = try await client.createLink(.init(body: .json(.init(url: link.url))))
            await load()
        } catch {
            loadError = error.localizedDescription
        }
    }

    /// 열람 기록 — fire-and-forget. 실패는 무시한다(계측이 흐름을 막으면 안 된다).
    private func recordOpen(_ link: Components.Schemas.Link) {
        guard let client = backend.client else { return }
        Task { _ = try? await client.markOpened(.init(path: .init(id: link.id))) }
    }

    // MARK: - 검색

    /// 입력이 멎은 뒤에 한 번만 요청한다.
    ///
    /// `.task(id:)`는 id가 바뀌면 이전 작업을 **취소**하므로, 앞에 슬립을 두면 그것만으로
    /// 디바운스가 된다 — 타이머도 상태도 필요 없다. 취소된 작업은 슬립에서 죽고 요청까지
    /// 가지 않는다.
    private func runSearch(debounce: Bool = true) async {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else {
            results = []
            searchMode = nil
            searchCursor = nil
            searchError = nil
            searchMoreError = nil
            return
        }
        if debounce {
            do {
                try await Task.sleep(for: .milliseconds(250))
            } catch {
                return // 취소됨 — 다음 글자가 들어왔다
            }
        }
        guard case .running = backend.state, let client = backend.client else { return }
        searching = true
        defer { searching = false }
        do {
            var q2 = Operations.search.Input.Query(q: q, limit: 50)
            // 태그 필터가 걸려 있으면 검색에도 그대로 적용한다 — 화면에 필터 칩이 떠
            // 있는데 검색만 그걸 무시하면 사용자가 본 것과 다른 결과가 나온다.
            if case let .tag(name) = filter { q2.tag = name }
            let page = try await client.search(.init(query: q2)).ok.body.json
            // SearchResult는 allOf(Link + rank)라 생성기가 value1/value2로 감싼다.
            // 카드가 필요한 것은 Link 쪽이다.
            results = page.links.map(\.value1)
            searchCursor = page.next_cursor
            searchMode = page.mode.rawValue
            searchError = nil
            searchMoreError = nil
        } catch {
            // 검색 실패를 목록 오류 자리에 쓰지 않는다 — 목록은 멀쩡한데 화면 전체가
            // 오류로 바뀌면 사용자는 저장한 것이 사라졌다고 읽는다. 대신 **검색 화면에서**
            // 드러낸다. 이걸 안 하면 실패가 "결과가 없습니다"라는 단정문이 되어,
            // 저장해 둔 것이 없다는 거짓말을 하게 된다.
            results = []
            searchMode = nil
            searchCursor = nil
            searchError = error.localizedDescription
        }
    }

    /// 검색 결과의 다음 장. 목록과 같은 이유로 필요하다 — 50건에서 끊긴 결과는
    /// "더 없다"로 읽히는데, 검색에서 그 오해는 목록에서보다 비싸다.
    ///
    /// 검색 커서는 목록 커서와 **형식이 다르고 서로 호환되지 않는다**(계약). 그래서 상태를
    /// 따로 들고 있고, 검색어가 바뀌면 버린다.
    private func searchMore() async {
        guard !loadingMore, let cursor = searchCursor,
              case .running = backend.state, let client = backend.client else { return }
        loadingMore = true
        defer { loadingMore = false }
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return }
        var q2 = Operations.search.Input.Query(q: q, limit: 50)
        q2.cursor = cursor
        if case let .tag(name) = filter { q2.tag = name }
        do {
            let page = try await client.search(.init(query: q2)).ok.body.json
            let known = Set(results.map(\.id))
            results.append(contentsOf: page.links.map(\.value1).filter { !known.contains($0.id) })
            searchCursor = page.next_cursor
            searchMoreError = nil
        } catch {
            // 삼키면 "여기가 끝"과 구분되지 않는다 — 이 함수의 주석이 경고하는 바로 그
            // 오해를 만들게 된다. searchCursor를 그대로 두므로 재시도가 같은 자리에서 이어진다.
            searchMoreError = "다음 결과를 불러오지 못했습니다"
        }
    }

    // MARK: - 로드

    /// 첫 장. 커서를 버리고 처음부터 다시 받는다 — 당겨서 새로고침·필터 변경이 여기로 온다.
    private func load() async {
        guard case .running = backend.state, let client = backend.client else { return }
        do {
            let page = try await fetchPage(client, cursor: nil)
            links = page.links
            nextCursor = page.next_cursor
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }

    /// 다음 장. 마지막 카드가 보이면 불린다.
    ///
    /// **더보기 버튼을 두지 않는 이유**는 취향이 아니다. 이 앱은 아카이브이고, 목록을
    /// 거슬러 올라가는 것이 저장한 것을 되찾는 주된 방법이다 — 그 길목마다 버튼을 세우면
    /// 되찾기 자체가 일이 된다. 대신 실패했을 때 조용하지 않아야 해서 loadError로 드러낸다.
    private func loadMore() async {
        guard !loadingMore, let cursor = nextCursor,
              case .running = backend.state, let client = backend.client else { return }
        loadingMore = true
        defer { loadingMore = false }
        do {
            let page = try await fetchPage(client, cursor: cursor)
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

    /// 목록 한 장. 커서만 다르고 필터는 항상 같이 간다 — 커서에 필터가 실려 있지 않으므로
    /// 빠뜨리면 두 번째 장부터 필터가 풀린다.
    private func fetchPage(_ client: Client, cursor: String?)
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
