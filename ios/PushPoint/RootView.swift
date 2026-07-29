import SwiftUI

/// 탭 컨테이너.
///
/// 화면이 셋이 되면서 `NavigationStack` 하나로는 이동할 방법이 없어졌다. 탭 바가 iOS의
/// 정식 관용구이고, 웹의 상단 내비게이션과 같은 자리를 차지한다(§8.5 — 플랫폼 관용을
/// 따르는 것은 의도적으로 다르다).
///
/// 태그 사전(name → facet)을 여기서 한 번 받아 두 탭이 나눠 쓴다. 탭마다 따로 받으면
/// 같은 태그가 목록과 통계에서 다른 색으로 보일 수 있다 — facet은 색의 유일한 출처라
/// 그 불일치가 곧 제품 결함이다.
struct RootView: View {
    @EnvironmentObject private var backend: Backend
    @State private var facetsFailed = false
    @State private var facets: [String: PP.Facet] = [:]
    @State private var tab: Tab = .list
    /// 통계에서 넘어온 필터. 목록이 이 값을 보고 좁혀 보여준다.
    @State private var filter: ListFilter?

    enum Tab { case list, stats }

    var body: some View {
        TabView(selection: $tab) {
            ContentView(facets: facets, filter: $filter)
                .tabItem { Label("목록", systemImage: "square.stack") }
                .tag(Tab.list)
            StatsView(facetOf: { facets[$0] ?? .neutral }) { selected in
                // 통계에서 무언가를 누르면 목록으로 데려간다 — 통계가 막다른 길이 아니라
                // 목록으로 들어가는 입구가 된다.
                filter = selected
                tab = .list
            }
            .tabItem { Label("통계", systemImage: "chart.bar") }
            .tag(Tab.stats)
        }
        .tint(PP.Palette.accent)
        .task(id: backend.state) { await loadFacets() }
        // 사전을 못 받았으면 알린다. 색이 사라진 화면은 그 자체로는 정상으로 보인다.
        .overlay(alignment: .top) {
            if facetsFailed {
                Button { Task { await loadFacets() } } label: {
                    HStack(spacing: 8) {
                        Image(systemName: "exclamationmark.triangle.fill").font(PP.Typo.label)
                        Text("태그 사전을 불러오지 못해 분류 색이 표시되지 않습니다")
                            .font(PP.Typo.label)
                        Spacer(minLength: 4)
                        Text("다시 시도").font(PP.Typo.label).underline()
                    }
                    .foregroundStyle(PP.Palette.fg1)
                    .padding(.horizontal, 14).padding(.vertical, 12)
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .background(PP.Palette.warnTint)
                }
                .buttonStyle(.plain)
            }
        }
    }

    /// 태그 사전을 받는다. **이 앱의 색 전체가 여기에 달려 있다.**
    ///
    /// 실패하면 `facets`가 빈 채로 남고, 모든 소비자가 `?? .neutral`로 떨어진다 —
    /// 칩도 커버도 통계 묶음도 전부 회색이 되고, 통계 문단에서 "주로 'X'에 관심이
    /// 갔고" 절은 아예 사라진다. 그리고 **그 화면은 정상으로 보인다**: 사전에 facet이
    /// 없는 상태와 구분되지 않는다.
    ///
    /// 예전에는 `try?` + `return` 하나였다. `.task(id: backend.state)`는 백엔드 상태가
    /// 바뀔 때만 다시 도므로, 시작 직후 한 번 실패하면 **그 세션 내내 회색**이었다.
    /// 그래서 재시도한다 — 사전은 40행짜리 정적 데이터라 몇 번 더 받아도 부담이 없다.
    private func loadFacets() async {
        guard let client = backend.client else { return }
        for attempt in 0 ..< 3 {
            if attempt > 0 {
                try? await Task.sleep(for: .milliseconds(400 << attempt))
                if Task.isCancelled { return }
            }
            // 목록 응답의 LinkTag에는 facet이 없어서 이 사전이 그 자리를 메운다.
            guard let tags = try? await client.listTags(.init()).ok.body.json else { continue }
            facets = Dictionary(uniqueKeysWithValues: tags.map {
                ($0.name, PP.Facet(apiValue: $0.facet.rawValue))
            })
            facetsFailed = false
            return
        }
        // 세 번 다 실패했으면 **회색으로 위장하지 않는다.** 색이 의미를 지는 앱에서
        // 색이 전부 사라진 화면은 "분류가 없는 아카이브"라는 거짓말이다.
        facetsFailed = true
    }
}
