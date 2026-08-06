import Charts
import SwiftUI
import WidgetKit

/// 통계 — "이 습관이 살아 있나"를 보는 화면.
///
/// 총 개수 같은 허영 지표를 늘어놓지 않는다. 이 제품의 목표가 "내가 매일 쓰는 앱"이고
/// M6의 완료 기준이 **4주 연속 일일 사용**이므로(08 §2), 통계의 주인공은 쌓인 양이 아니라
/// **리듬**이다. 저장이 끊긴 구간이 보이는 것이 이 화면의 값이다.
///
/// 그리고 **모든 항목이 어딘가로 닿는다.** 숫자만 보여주는 화면은 한 번 보고 다시 오지
/// 않는다 — 태그를 누르면 그 태그의 목록으로, 실패를 누르면 실패한 것들로 간다.
///
/// 연속일·활동일은 계약이 이미 주는 `by_day`(30일)에서 계산한다. 서버에 새 필드를
/// 요구하지 않고 클라이언트가 파생시킬 수 있는 값이면 그렇게 하는 편이 낫다.
struct StatsView: View {
    @EnvironmentObject private var backend: Backend
    let facetOf: (String) -> PP.Facet
    /// 태그·실패를 눌렀을 때 목록 탭으로 넘기는 통로.
    let onFilter: (ListFilter) -> Void

    @State private var stats: Components.Schemas.Stats?
    /// 실패 개수는 이제 `stats.failed_links`가 준다 — **따로 세지 않는다.**
    ///
    /// 예전에는 `GET /links?status=failed&limit=100`을 한 번 더 보내서 배열 길이를 셌다.
    /// 그래서 100에서 포화됐고("100개 이상"), 그 요청이 실패하면 개수를 모르는 상태가 되고,
    /// **웹에는 애초에 그 수단이 없어 같은 섹션이 한쪽에만 있었다**(13 §2). 계약에 수를
    /// 넣으니 셋 다 사라진다.
    private var failedCount: Int? { stats.map { $0.failed_links } }
    @State private var loadError: String?

    var body: some View {
        NavigationStack {
            ScrollView {
                content.padding(.horizontal, 16).padding(.vertical, 16)
            }
            .background(PP.Palette.canvas)
            .navigationTitle(t("nav.stats"))
            .refreshable { await load() }
        }
        // **`id:`가 있어야 한다.** 없으면 화면이 처음 뜰 때 한 번만 돌고, 그때 인프로세스
        // 서버가 아직 시작 전이면 load()가 조용히 반환한 뒤 다시 돌지 않는다 —
        // stats도 loadError도 nil이라 화면은 **영원히 스피너**다. 사용자에게는 로딩으로
        // 보이므로 당겨서 새로고침할 이유조차 없다. ContentView는 처음부터 이 형태다.
        .task(id: backend.state) { await load() }
    }

    @ViewBuilder
    private var content: some View {
        if let stats {
            if stats.total_links == 0 {
                // 빈 상태에서 0을 세 개 보여주는 것은 정보가 아니라 소음이다.
                ContentUnavailableView(t("rhythm.emptyTitle"), systemImage: "chart.bar",
                                       description: Text(t("rhythm.emptyBody")))
                    .padding(.top, 40)
            } else {
                // **순차 등장 안무를 뺐다.** §6.1의 모션 표에 없었고 §6.2는 목록 항목의
                // stagger 진입을 이름으로 금지한다. §1.4의 절제 규칙도 "다른 화면 전환·목록
                // 진입에 안무를 추가하지 않는다"고 못박는다 — 웹에는 없어서 두 클라이언트가
                // 갈라져 있기도 했다. 문서에 없는 연출은 코드를 되돌리는 쪽으로 판정한다.
                VStack(alignment: .leading, spacing: 26) {
                    streakBlock(stats)
                    rhythm(stats)
                    topTags(stats)
                    needsAttention()
                }
            }
        } else if let loadError {
            ContentUnavailableView(t("common.loadFailed"), systemImage: "exclamationmark.triangle",
                                   description: Text(loadError))
        } else {
            ProgressView().padding(.top, 60)
        }
    }

    // MARK: - 연속 저장

    /// 이번 주를 **문단으로** 말한다.
    ///
    /// 대시보드는 숫자를 놓고 해석을 사람에게 떠넘긴다. "1 · 4 · 14"를 보고 잘 되고
    /// 있는지 판단하려면 매번 머릿속에서 문장을 만들어야 하는데, 그 일은 화면이 해야 한다.
    /// 아래 섹션들은 이 문단의 **근거**이지 그 자체가 결론이 아니다.
    private func streakBlock(_ s: Components.Schemas.Stats) -> some View {
        let days = streak(s.by_day)
        return VStack(alignment: .leading, spacing: 10) {
            Text(narrative(s))
                .font(PP.Typo.head)
                .tracking(PP.Tracking.head)
                .foregroundStyle(PP.Palette.fg1)
                .lineSpacing(4)
                .fixedSize(horizontal: false, vertical: true)
            Text(goalLine(days, streakCapped(s.by_day, days)))
                .font(PP.Typo.meta)
                .tracking(PP.Tracking.meta)
                .foregroundStyle(PP.Palette.fg2)
        }
    }

    /// 화면이 사람에게 하는 말. **지지되는 수만 쓴다.**
    ///
    /// 예전 문장은 네 절이었고 그중 둘을 데이터가 받치지 못했다(14 §1). "지난주보다 N개"는
    /// 행동이 전혀 안 변해도 평균 2.41개가 나오고 방향 단어가 사흘에 한 번 뒤집혔다.
    /// "최근 30일은 X요일에 가장 많이"는 **어떤 저장 속도에서도** 성립하지 않았다 —
    /// 30일은 4주+2일이라 오늘·어제 요일만 5칸을 갖고 그 둘이 매일 회전하므로, 하루 두
    /// 건씩 한 번도 거르지 않는 사용자조차 매일 다른 답을 들었다.
    ///
    /// 남은 것은 **사실의 개수**다. 활성 일수와 연속일은 추론이 아니라 세는 것이고,
    /// 흔들릴 때는 진짜로 무언가 일어났다는 뜻이다. 이 프로젝트는 이미 그 둘을 알고
    /// 있었다 — M6 판정을 `scripts/streak.sh`에 맡길 때 고른 것이 정확히 이 둘이다.
    ///
    /// facet 절도 뺐다. `by_tag`에 날짜 조건이 없어 전 기간 누계인데 "이번 주"로 시작한
    /// 문단의 두 번째 자리에서 최근성 주장으로 읽혔다. 이 화면은 **같은 판단을 이미 한 번
    /// 내렸다** — 아래 목록 주석의 facet 도넛 제거 이유가 그대로 이 절에도 적용된다.
    ///
    /// 웹과 **글자까지 같다**(13 §3).
    private func narrative(_ s: Components.Schemas.Stats) -> String {
        // `total_links == 0`은 여기 안 온다 — `content`가 먼저 ContentUnavailableView로
        // 가로챈다. 그 분기를 여기 또 두면 **닿지 않는 문구**가 생기고, 웹과 갈라져도
        // 아무도 모른다(실제로 그렇게 두 문장이 갈라져 있었다).
        let active = Self.activeDays(s.by_day)
        let days = streak(s.by_day)
        let capped = days > 0 && days >= s.by_day.count

        if active == 0 {
            let key = s.total_links == 1 ? "rhythm.narrativeNoRecentOne" : "rhythm.narrativeNoRecent"
            return t(key, ["count": s.total_links])
        }

        // 이어 붙이는 조각은 전부 그 자체로 완결된 문장이다 — 조각으로 문장을 조립하면
        // 어순이 다른 언어에서 무너진다. 웹도 같은 자리를 같은 키로 나눠 두었다.
        let first = t("rhythm.narrativeActive", ["n": active])
        if days > 0 {
            let rest = capped
                ? t("rhythm.narrativeStreakCapped")
                : t("rhythm.narrativeStreak", ["n": days])
            return "\(first) \(rest)"
        }

        // 끊긴 것은 사실이므로 말하되, 되돌리라고 요구하지 않는다(14 §D1).
        guard let gap = Self.daysSinceLastSave(s.by_day) else { return first }
        let last = gap == 1
            ? t("rhythm.narrativeLastYesterday")
            : t("rhythm.narrativeLastDaysAgo", ["n": gap])
        return "\(first) \(last)"
    }

    /// 30일 창 안에서 저장한 날의 수. 웹 `activeDays`와 같은 계산이다.
    private static func activeDays(_ byDay: Components.Schemas.Stats.by_dayPayload) -> Int {
        byDay.filter { $0.count > 0 }.count
    }

    /// 마지막 저장이 며칠 전인가. 창에 아무것도 없으면 nil.
    ///
    /// **위치로 센다** — 계약이 `by_day`를 "정확히 30개, 마지막이 서버 로컬 오늘"로
    /// 보장하므로 뒤에서부터 걸으면 끝이고, 날짜 연산이 없으니 타임존도 없다.
    private static func daysSinceLastSave(_ byDay: Components.Schemas.Stats.by_dayPayload) -> Int? {
        for (i, d) in byDay.enumerated().reversed() where d.count > 0 {
            return byDay.count - 1 - i
        }
        return nil
    }

    private func goalLine(_ days: Int, _ capped: Bool) -> String {
        switch days {
        case 0: t("rhythm.goalStart")
        case _ where capped: t("rhythm.goalMetCapped", ["n": days])
        // 영어는 남은 날이 하루일 때만 단수형이 필요하다. `t()`에 복수 규칙이 없으므로
        // 여기서 갈라 쓴다 — 웹은 같은 문장을 값 안의 `|`로 처리한다.
        case 1 ..< 28:
            t(28 - days == 1 ? "rhythm.goalProgressOne" : "rhythm.goalProgress",
              ["n": days, "count": 28 - days])
        default: t("rhythm.goalMet", ["n": days])
        }
    }


    /// 연속 규칙은 `Shared/Streak`에 있다 — 웹·셸과 **같은 픽스처**로 대조되고
    /// (`testdata/streak-cases.json`), 위젯도 같은 것을 부른다. 뷰 안의 `private func`이던
    /// 동안에는 테스트가 부를 수 없어 iOS만 그 대조 밖이었다(13 §3).
    private func streak(_ byDay: Components.Schemas.Stats.by_dayPayload) -> Int {
        Streak.days(byDay.map(\.count))
    }

    private func streakCapped(_ byDay: Components.Schemas.Stats.by_dayPayload, _ days: Int) -> Bool {
        Streak.isCapped(byDay.map(\.count), days: days)
    }

    // MARK: - 30일 리듬

    @ViewBuilder
    private func rhythm(_ s: Components.Schemas.Stats) -> some View {
        let active = s.by_day.filter { $0.count > 0 }.count
        VStack(alignment: .leading, spacing: 10) {
            sectionTitle(t("common.last30Days"),
                         trailing: t(active == 1 ? "rhythm.daysSavedOne" : "rhythm.daysSaved",
                                     ["count": active]))
            // 계약이 by_day를 **빈 날까지 채운 30칸**으로 보장하므로 i번째 칸이 곧
            // i번째 날이다. 그 보장이 없던 2026-07-28 이전에는 같은 코드가 거짓말을
            // 했다 — 행만 있는 배열을 위치로 그리는 바람에 한 달에 다섯 번 저장한
            // 사람의 막대 다섯 개가 **왼쪽 끝에 붙어서** "한 달 전에 몰아서 저장하고
            // 그 뒤로 안 함"으로 보였다. 웹은 같은 실수를 오른쪽으로 해서, 같은 응답을
            // 두 화면이 반대로 그리고 있었다.
            Chart {
                ForEach(Array(s.by_day.enumerated()), id: \.offset) { index, day in
                    BarMark(
                        x: .value("일", index),
                        y: .value("건수", day.count)
                    )
                    .foregroundStyle(day.count > 0 ? PP.Palette.accent : PP.Palette.line2)
                    .cornerRadius(1.5)
                }
            }
            .chartXScale(domain: 0 ... Swift.max(s.by_day.count - 1, 29))
            .chartXAxis(.hidden) // 30개 라벨은 읽히지 않는다 — 모양만 본다
            .chartYAxis {
                AxisMarks(position: .leading, values: .automatic(desiredCount: 3)) {
                    AxisValueLabel().font(PP.Typo.label)
                }
            }
            .frame(height: 110)

            HStack {
                Text(t("rhythm.axisStart")).font(PP.Typo.label).foregroundStyle(PP.Palette.fg3)
                Spacer()
                Text(t("time.today")).font(PP.Typo.label).foregroundStyle(PP.Palette.fg3)
            }
        }
    }

    // MARK: - 구성

    // MARK: - 태그

    /// 태그를 **facet으로 묶어** 보여준다.
    ///
    /// 원래 여기 위에 facet 비율 도넛이 있었는데 걷어냈다. `by_tag`는 전체 기간 누적이라
    /// 시간 축이 없어서 정작 궁금한 "요즘 관심이 어디로 옮겨갔나"를 계산할 수 없고,
    /// 남는 것은 거의 변하지 않는 비율을 내부 분류 용어로, 아무 데도 닿지 않게 보여주는
    /// 차트뿐이었다. 차트를 하나 더 만드는 대신 **구성이 목록의 구조로 보이게** 했다 —
    /// 묶음을 보면 비율이 읽히고, 모든 줄은 그 태그의 목록으로 간다.
    @ViewBuilder
    private func topTags(_ s: Components.Schemas.Stats) -> some View {
        let groups = groupedTags(s)
        if !groups.isEmpty {
            VStack(alignment: .leading, spacing: 16) {
                sectionTitle(t("rhythm.collected"), trailing: t("rhythm.collectedHint"))
                ForEach(groups, id: \.facet) { group in
                    VStack(alignment: .leading, spacing: 2) {
                        HStack(spacing: 7) {
                            Circle().fill(group.facet.ink).frame(width: 7, height: 7)
                            // 한글 라벨은 웹과 같은 단어다(§8.1).
                            Text(group.facet.label)
                                .font(PP.Typo.label)
                                .foregroundStyle(PP.Palette.fg2)
                            Text("\(group.total)")
                                .font(PP.Typo.metaMono)
                                .monospacedDigit()
                                .foregroundStyle(PP.Palette.fg3)
                        }
                        .padding(.bottom, 2)

                        ForEach(group.tags, id: \.name) { tag in
                            Button { onFilter(.tag(tag.name)) } label: {
                                tagRow(tag, facet: group.facet)
                            }
                            .buttonStyle(.plain)
                        }
                    }
                }
            }
        }
    }

    private struct TagGroup {
        let facet: PP.Facet
        let tags: [Components.Schemas.Stats.by_tagPayloadPayload]
        var total: Int { tags.reduce(0) { $0 + $1.count } }
    }

    /// facet 순서는 고정(craft → media → life → neutral)이다. 개수순으로 묶음을
    /// 재배치하면 화면을 열 때마다 같은 태그가 다른 자리에 있어 위치 기억이 무너진다.
    private func groupedTags(_ s: Components.Schemas.Stats) -> [TagGroup] {
        var byFacet: [PP.Facet: [Components.Schemas.Stats.by_tagPayloadPayload]] = [:]
        for tag in s.by_tag {
            byFacet[facetOf(tag.name), default: []].append(tag)
        }
        return PP.Facet.allCases.compactMap { facet in
            guard let tags = byFacet[facet], !tags.isEmpty else { return nil }
            return TagGroup(facet: facet, tags: tags.sorted { $0.count > $1.count })
        }
    }

    /// 개수는 오른쪽 정렬 고정폭이라 세로로 훑을 때 자릿수가 맞는다.
    private func tagRow(_ tag: Components.Schemas.Stats.by_tagPayloadPayload,
                        facet: PP.Facet) -> some View {
        HStack(spacing: 10) {
            Text(tag.name)
                .font(PP.Typo.body)
                .tracking(PP.Tracking.body)
                .foregroundStyle(PP.Palette.fg1)
                .lineLimit(1)
            Spacer(minLength: 8)
            Text("\(tag.count)")
                .font(PP.Typo.metaMono)
                .monospacedDigit()
                .foregroundStyle(PP.Palette.fg3)
            Image(systemName: "chevron.right")
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.fg3)
        }
        .padding(.leading, 14) // facet 점 아래로 들여써서 묶음이 보이게
        .padding(.vertical, 5)
        .contentShape(Rectangle())
    }

    // MARK: - 손이 필요한 것

    /// 실패한 링크는 통계가 아니라 **할 일**이다. 개수만 알려주고 끝내면 그 링크는
    /// 영원히 실패로 남는다 — 눌러서 그 목록으로 갈 수 있어야 한다.
    @ViewBuilder
    private func needsAttention() -> some View {
        if let failedCount, failedCount > 0 {
            Button { onFilter(.failed) } label: {
                HStack(spacing: 10) {
                    Image(systemName: "exclamationmark.triangle.fill")
                        .foregroundStyle(PP.Palette.danger)
                    // **"이상"이 사라졌다.** 계약이 정확한 수를 주므로 상한 표기가 필요 없다.
                    Text(t(failedCount == 1 ? "stats.failedLinksOne" : "stats.failedLinks",
                           ["count": failedCount]))
                        .font(PP.Typo.body)
                        .foregroundStyle(PP.Palette.fg1)
                    Spacer()
                    Image(systemName: "chevron.right")
                        .font(PP.Typo.label)
                        .foregroundStyle(PP.Palette.fg3)
                }
                .padding(14)
                .background(PP.Palette.dangerTint)
                .clipShape(RoundedRectangle(cornerRadius: PP.Radius.card, style: .continuous))
            }
            .buttonStyle(.plain)
        }
    }

    // MARK: -

    /// 시간 척추와 같은 serif 머리글 — 화면이 달라도 같은 목소리다.
    /// **serif가 아니다.** §2.2.5는 serif의 용처를 "시간 척추 머리글 한 곳"으로 한정하고,
    /// 그 규칙을 어기면 §1.3의 "본문 serif 금지"가 되살아난다고 못박는다. 이 화면은 그
    /// 예외를 문서에 등재하지 않은 채 `spine`을 써 왔고, 웹의 같은 자리는 안 써서 두
    /// 클라이언트도 갈라져 있었다. 문서를 고치는 대신 코드를 되돌린다 — 웹과 같은
    /// `label` 슬롯이다.
    private func sectionTitle(_ text: String, trailing: String?) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 9) {
            Text(text)
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.fg3)
            Rectangle().fill(PP.Palette.line1).frame(height: 1)
            if let trailing {
                Text(trailing)
                    .font(PP.Typo.metaMono)
                    .foregroundStyle(PP.Palette.fg3)
            }
        }
    }

    private func load() async {
        guard let client = backend.client else { return }
        do {
            let fresh = try await client.getStats(.init()).ok.body.json
            stats = fresh
            loadError = nil
            // 위젯이 읽을 스냅샷을 남긴다(10 §8.6). **계약 그대로의 JSON을 다시 인코드한다** —
            // 요약해 넘기면 모양이 둘이 되고, 위젯이 자기 디코더를 갖게 된다.
            if let data = try? JSONEncoder().encode(fresh) {
                StatsSnapshot.write(data)
                WidgetCenter.shared.reloadAllTimelines()
            }
        } catch {
            loadError = error.localizedDescription
            return
        }
    }
}
