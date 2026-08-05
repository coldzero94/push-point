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
    @State private var facets: [String: PP.Facet] = [:]
    @State private var tab: Tab = .list
    /// 통계에서 넘어온 필터. 목록이 이 값을 보고 좁혀 보여준다.
    @State private var filter: ListFilter?

    /// 언어가 바뀌면 화면 전체를 다시 그린다 — 문자열은 뷰 본문에서 `t()`로 읽히므로
    /// 부분 갱신으로는 두 언어가 섞인 화면이 남는다.
    @ObservedObject private var langStore = L.Store.shared


    enum Tab { case list, stats }

    var body: some View {
        TabView(selection: $tab) {
            ContentView(facets: facets, filter: $filter)
                .tabItem { Label(t("nav.list"), systemImage: "square.stack") }
                .tag(Tab.list)
            StatsView(facetOf: { facets[$0] ?? .neutral }) { selected in
                // 통계에서 무언가를 누르면 목록으로 데려간다 — 통계가 막다른 길이 아니라
                // 목록으로 들어가는 입구가 된다.
                filter = selected
                tab = .list
            }
            .tabItem { Label(t("nav.stats"), systemImage: "chart.bar") }
            .tag(Tab.stats)
        }
        .id(langStore.lang)
        .tint(PP.Palette.accent)
        // 루트에 걸지만 **여기서 끝이 아니다** — 시트는 별도 표현 컨텍스트라 각자
        // 걸어야 한다(`Theme.Applying`). 처음에는 여기 한 곳이면 된다고 적어 뒀는데,
        // 화면을 보니 뜬 시트가 안 따라왔다.
        .pushPointTheme()
        .task(id: backend.state) { await loadFacets() }
    }

    private func loadFacets() async {
        guard let client = backend.client else { return }
        // 사전은 30개 남짓이라 매번 받아도 부담이 없고, 태그가 추가돼도 즉시 따라간다.
        // 목록 응답의 LinkTag에는 facet이 없어서 이 사전이 그 자리를 메운다.
        guard let tags = try? await client.listTags(.init()).ok.body.json else { return }
        facets = Dictionary(uniqueKeysWithValues: tags.map {
            ($0.name, PP.Facet(apiValue: $0.facet.rawValue))
        })
    }
}
