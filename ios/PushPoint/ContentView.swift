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

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: 12) {
                    if let filter { filterBar(filter) }
                    content
                }
                .padding(.horizontal, 16).padding(.bottom, 24)
            }
            .background(PP.Palette.canvas)
            .navigationTitle("Push-Point")
            .refreshable { await load() }
        }
        .task(id: backend.state) { await load() }
        .task(id: filter) { await load() }
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

    private var board: some View {
        LazyVStack(alignment: .leading, spacing: 20) {
            ForEach(sections, id: \.title) { section in
                VStack(alignment: .leading, spacing: 10) {
                    spine(section)
                    ForEach(section.links, id: \.id) { link in
                        NavigationLink {
                            LinkDetailView(linkID: link.id, facetOf: { facets[$0] ?? .neutral })
                        } label: {
                            LinkCard(link: link,
                                     facetOf: { facets[$0] ?? .neutral },
                                     activeTag: activeTagName,
                                     resolveThumb: backend.absoluteURL)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
        .padding(.top, 8)
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
    private func spine(_ section: Section) -> some View {
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

    private struct Section {
        let title: String
        let links: [Components.Schemas.Link]
    }

    /// 오늘 · 어제 · 이번 주 · 이전. 절대 날짜가 아니라 상대 구간인 이유는, 찾을 때
    /// 떠오르는 것이 "며칠 전"이지 "7월 12일"이 아니기 때문이다.
    private var sections: [Section] {
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
        return buckets.filter { !$0.1.isEmpty }.map { Section(title: $0.0, links: $0.1) }
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
