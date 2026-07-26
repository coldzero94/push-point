import Charts
import SwiftUI

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
    @State private var failedCount: Int?
    @State private var loadError: String?

    var body: some View {
        NavigationStack {
            ScrollView {
                content.padding(.horizontal, 16).padding(.vertical, 16)
            }
            .background(PP.Palette.canvas)
            .navigationTitle("통계")
            .refreshable { await load() }
        }
        .task { await load() }
    }

    @ViewBuilder
    private var content: some View {
        if let stats {
            if stats.total_links == 0 {
                // 빈 상태에서 0을 세 개 보여주는 것은 정보가 아니라 소음이다.
                ContentUnavailableView("아직 볼 통계가 없습니다", systemImage: "chart.bar",
                                       description: Text("링크를 저장하면 여기에 리듬이 쌓입니다."))
                    .padding(.top, 40)
            } else {
                VStack(alignment: .leading, spacing: 26) {
                    streakBlock(stats).reveal(0)
                    rhythm(stats).reveal(1)
                    weekly(stats).reveal(2)
                    topTags(stats).reveal(3)
                    needsAttention().reveal(4)
                }
            }
        } else if let loadError {
            ContentUnavailableView("불러오지 못했습니다", systemImage: "exclamationmark.triangle",
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
        VStack(alignment: .leading, spacing: 10) {
            Text(narrative(s))
                .font(PP.Typo.head)
                .tracking(PP.Tracking.head)
                .foregroundStyle(PP.Palette.fg1)
                .lineSpacing(4)
                .fixedSize(horizontal: false, vertical: true)
            Text(goalLine(streak(s.by_day)))
                .font(PP.Typo.meta)
                .tracking(PP.Tracking.meta)
                .foregroundStyle(PP.Palette.fg2)
        }
    }

    /// 데이터에서 문장을 만든다. 지난주 대비·지배 관심사·주 활동 요일까지 한 문단에 담아
    /// "무엇이 어떻게 바뀌었나"를 읽고 끝낼 수 있게 한다.
    private func narrative(_ s: Components.Schemas.Stats) -> String {
        let week = s.links_this_week
        if s.total_links == 0 { return "아직 저장한 링크가 없어요" }
        if week == 0 {
            return "이번 주에는 아직 저장한 게 없네요. 지금까지 \(s.total_links)개를 모았어요."
        }

        var sentences = ["이번 주에 \(week)개를 저장했어요."]

        // 지난주 대비 — "바뀌었다"는 비교가 있어야 성립한다.
        if let delta = weekOverWeek(s.by_day) {
            switch delta {
            case let d where d > 0: sentences.append("지난주보다 \(d)개 많아요.")
            case let d where d < 0: sentences.append("지난주보다 \(-d)개 적어요.")
            default: sentences.append("지난주와 같은 수예요.")
            }
        }

        // 무엇에 관심이 갔나 — facet 라벨은 웹과 같은 단어를 쓴다(§8.1).
        // 아래 목록의 묶음과 같은 계산을 써서 문장과 화면이 어긋나지 않게 한다.
        if let top = groupedTags(s).max(by: { $0.total < $1.total }), top.facet != .neutral {
            sentences.append("주로 '\(top.facet.label)'에 관심이 갔고,")
        }

        // 언제 — 요일은 습관에 대한 정보다.
        let counts = weekdayCounts(s.by_day)
        if let peak = counts.max(), peak > 0, let index = counts.firstIndex(of: peak) {
            sentences.append("\(Self.weekdayNames[index])요일에 가장 많이 저장했어요.")
        }
        return sentences.joined(separator: " ")
    }

    private func goalLine(_ days: Int) -> String {
        switch days {
        case 0: "오늘 하나 저장하면 연속이 시작돼요"
        case 1 ..< 28: "\(days)일 연속 — 4주(28일)까지 \(28 - days)일 남았어요"
        default: "\(days)일 연속 — 4주 목표를 넘겼습니다"
        }
    }

    /// 최근 7일과 그 앞 7일을 비교한다. by_day는 오래된 날짜가 앞이므로 뒤에서 센다.
    private func weekOverWeek(_ byDay: Components.Schemas.Stats.by_dayPayload) -> Int? {
        guard byDay.count >= 14 else { return nil }
        let recent = byDay.suffix(7).reduce(0) { $0 + $1.count }
        let prior = byDay.suffix(14).prefix(7).reduce(0) { $0 + $1.count }
        return recent - prior
    }

    /// 오늘(또는 어제)부터 거슬러 올라가며 저장이 있는 날을 센다.
    ///
    /// 오늘을 허용하는 이유: 오늘 아직 저장하지 않았다고 어제까지의 연속이 끊긴 것은
    /// 아니다. 자정 직후에 연속이 0으로 보이면 그 지표는 아무도 안 믿는다.
    private func streak(_ byDay: Components.Schemas.Stats.by_dayPayload) -> Int {
        let saved = Set(byDay.filter { $0.count > 0 }.map(\.date))
        guard !saved.isEmpty else { return 0 }
        let cal = Calendar.current
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        f.calendar = cal

        var cursor = Date()
        if !saved.contains(f.string(from: cursor)) {
            // 오늘 아직 안 했으면 어제부터 센다.
            guard let yesterday = cal.date(byAdding: .day, value: -1, to: cursor) else { return 0 }
            cursor = yesterday
        }
        var count = 0
        while saved.contains(f.string(from: cursor)) {
            count += 1
            guard let prev = cal.date(byAdding: .day, value: -1, to: cursor) else { break }
            cursor = prev
        }
        return count
    }

    // MARK: - 30일 리듬

    @ViewBuilder
    private func rhythm(_ s: Components.Schemas.Stats) -> some View {
        let active = s.by_day.filter { $0.count > 0 }.count
        VStack(alignment: .leading, spacing: 10) {
            sectionTitle("최근 30일", trailing: "\(active)일 저장")
            // x를 **날짜 문자열이 아니라 30일 전부터의 일수**로 둔다. 문자열 축은
            // 데이터가 있는 날만 칸을 만들어서, 하루치만 있으면 그 막대가 화면을 가득
            // 채운다 — "저장이 폭발했다"로 읽히는 거짓말이 된다. 축을 0...29로 고정하면
            // 하루는 30칸 중 한 칸으로 그려지고, 빈 구간이 그대로 빈 구간으로 보인다.
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
                Text("30일 전").font(PP.Typo.label).foregroundStyle(PP.Palette.fg3)
                Spacer()
                Text("오늘").font(PP.Typo.label).foregroundStyle(PP.Palette.fg3)
            }
        }
    }

    // MARK: - 언제

    /// 요일별 패턴 — "나는 언제 저장하나".
    ///
    /// 30일 막대는 "얼마나 꾸준한가"에는 답하지만 "언제"에는 답하지 못한다. 저장이
    /// 평일 업무 중에 몰리는지 주말에 몰리는지는 자기 습관에 대한 정보이고,
    /// `by_day`의 날짜에서 요일을 뽑으면 서버 변경 없이 알 수 있다.
    @ViewBuilder
    private func weekly(_ s: Components.Schemas.Stats) -> some View {
        let counts = weekdayCounts(s.by_day)
        let peak = counts.max() ?? 0
        if peak > 0 {
            VStack(alignment: .leading, spacing: 10) {
                sectionTitle("언제 저장하나", trailing: busiestLabel(counts))
                HStack(alignment: .bottom, spacing: 6) {
                    ForEach(0 ..< 7, id: \.self) { index in
                        VStack(spacing: 5) {
                            // 높이로만 말한다 — 라벨 7개면 축이 따로 필요 없다.
                            RoundedRectangle(cornerRadius: 3)
                                .fill(counts[index] > 0 ? PP.Palette.accent : PP.Palette.line2)
                                .frame(height: barHeight(counts[index], peak))
                            Text(Self.weekdayNames[index])
                                .font(PP.Typo.label)
                                .foregroundStyle(counts[index] == peak && peak > 0
                                                 ? PP.Palette.fg1 : PP.Palette.fg3)
                        }
                    }
                }
                .frame(height: 76, alignment: .bottom)
            }
        }
    }

    private static let weekdayNames = ["일", "월", "화", "수", "목", "금", "토"]

    private func weekdayCounts(_ byDay: Components.Schemas.Stats.by_dayPayload) -> [Int] {
        var counts = [Int](repeating: 0, count: 7)
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        let cal = Calendar.current
        for day in byDay where day.count > 0 {
            guard let date = f.date(from: day.date) else { continue }
            counts[cal.component(.weekday, from: date) - 1] += day.count
        }
        return counts
    }

    /// 가장 많이 저장한 요일을 문장으로 — 막대를 눈으로 비교하게 두지 않는다.
    private func busiestLabel(_ counts: [Int]) -> String? {
        guard let peak = counts.max(), peak > 0,
              let index = counts.firstIndex(of: peak) else { return nil }
        return "\(Self.weekdayNames[index])요일에 가장 많이"
    }

    private func barHeight(_ value: Int, _ peak: Int) -> CGFloat {
        // 0도 흔적을 남긴다 — 막대가 없으면 "저장 안 한 요일"인지 알 수 없다.
        peak > 0 ? Swift.max(CGFloat(value) / CGFloat(peak) * 52, 3) : 3
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
                sectionTitle("무엇을 모았나", trailing: "누르면 그 목록으로")
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
                    Text("수집에 실패한 링크 \(failedCount)개")
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

    private func ratio(_ value: Int, _ maxCount: Int) -> CGFloat {
        maxCount > 0 ? CGFloat(value) / CGFloat(maxCount) : 0
    }

    /// 시간 척추와 같은 serif 머리글 — 화면이 달라도 같은 목소리다.
    private func sectionTitle(_ text: String, trailing: String?) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 9) {
            Text(text)
                .font(PP.Typo.spine)
                .tracking(PP.Tracking.spine)
                .foregroundStyle(PP.Palette.fg1)
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
            stats = try await client.getStats(.init()).ok.body.json
            // 실패 목록은 통계 응답에 없다 — 개수만 필요하므로 최소로 받는다.
            let failed = try await client.listLinks(.init(query: .init(limit: 100, status: .failed)))
            failedCount = try failed.ok.body.json.links.count
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }
}

/// 통계에서 목록으로 넘기는 필터.
enum ListFilter: Equatable {
    case tag(String)
    case failed
}
