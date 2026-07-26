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
    /// 삭제 확인을 기다리는 링크. 삭제는 되돌릴 수 없는 동작이라 한 번 묻는다.
    @State private var pendingDelete: Components.Schemas.Link?

    var body: some View {
        NavigationStack {
            content
                .background(PP.Palette.canvas)
            .navigationTitle("Push-Point")
            .refreshable { await load() }
        }
        .task(id: backend.state) { await load() }
        .task(id: filter) { await load() }
        // 삭제는 소프트 삭제지만 화면에서는 사라지므로, 실수로 지운 것을 되돌릴
        // 방법이 UI에 없다. 그래서 지우기 전에 무엇을 지우는지 이름을 보여주고 묻는다.
        .confirmationDialog(
            pendingDelete.map { "'\($0.title.isEmpty ? $0.domain : $0.title)'을 삭제할까요?" } ?? "",
            isPresented: .init(get: { pendingDelete != nil },
                               set: { if !$0 { pendingDelete = nil } }),
            titleVisibility: .visible
        ) {
            Button("삭제", role: .destructive) {
                if let link = pendingDelete { Task { await delete(link) } }
            }
            Button("취소", role: .cancel) { pendingDelete = nil }
        }
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
            if let loadError {
                ContentUnavailableView("목록을 불러오지 못했습니다",
                                       systemImage: "wifi.exclamationmark",
                                       description: Text(loadError))
            } else if links.isEmpty {
                ContentUnavailableView("아직 저장한 링크가 없습니다", systemImage: "tray",
                                       description: Text("사파리나 앱에서 공유 시트로 보내 보세요."))
            } else {
                board
            }
        }
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
                        row(link).plainRow(top: 5, bottom: 5)
                    }
                } header: {
                    spine(section).plainRow(top: 14, bottom: 6)
                }
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .environment(\.defaultMinListHeaderHeight, 0)
    }

    private func row(_ link: Components.Schemas.Link) -> some View {
        NavigationLink {
            LinkDetailView(linkID: link.id, facetOf: { facets[$0] ?? .neutral })
        } label: {
            LinkCard(link: link,
                     facetOf: { facets[$0] ?? .neutral },
                     activeTag: activeTagName,
                     resolveThumb: backend.absoluteURL)
        }
        .buttonStyle(.plain)
        // 스와이프와 길게 누르기 둘 다 둔다(§8.4). 스와이프는 빠르지만 발견되지 않고,
        // 컨텍스트 메뉴는 느리지만 항상 찾을 수 있다.
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            Button(role: .destructive) { pendingDelete = link } label: {
                Label("삭제", systemImage: "trash")
            }
        }
        // 방향으로 성격을 나눈다. **오른쪽 끝은 파괴적인 것만** — iOS 전반의 관용이고,
        // 그래야 손이 기억한 방향이 다른 화면에서 배신하지 않는다. 왼쪽 끝은 되돌릴 수
        // 있는 것: 실패한 링크는 재시도가 가장 필요한 동작이므로 그 자리를 내주고,
        // 정상 링크는 원문 열기를 둔다.
        .swipeActions(edge: .leading, allowsFullSwipe: false) {
            if link.status == .failed {
                Button { Task { await retry(link) } } label: {
                    Label("재시도", systemImage: "arrow.clockwise")
                }
                .tint(PP.Palette.warn)
            } else if let url = URL(string: link.url) {
                Link(destination: url) { Label("원문", systemImage: "safari") }
                    .tint(PP.Palette.accent)
            }
        }
        .contextMenu {
            if let url = URL(string: link.url) {
                Link(destination: url) { Label("원문 열기", systemImage: "safari") }
            }
            Button(role: .destructive) { pendingDelete = link } label: {
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
            } else if let days = cal.dateComponents([.day], from: date, to: now).day, days < 7 {
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
        pendingDelete = nil
        do {
            _ = try await client.deleteLink(.init(path: .init(id: link.id))).noContent
            withAnimation(.smooth(duration: 0.25)) {
                links.removeAll { $0.id == link.id }
            }
        } catch {
            loadError = error.localizedDescription
        }
    }

    // MARK: - 로드

    private func load() async {
        guard case .running = backend.state, let client = backend.client else { return }
        do {
            var query = Operations.listLinks.Input.Query(limit: 50)
            switch filter {
            case let .tag(name): query.tag = name
            case .failed: query.status = .failed
            case nil: break
            }
            let list = try await client.listLinks(.init(query: query))
            links = try list.ok.body.json.links
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }
}
