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
                    composition(stats).reveal(3)
                    topTags(stats).reveal(4)
                    needsAttention().reveal(5)
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
        let parts = facetShares(s)
        if let top = parts.max(by: { $0.count < $1.count }), top.facet != .neutral {
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

    /// facet 구성 — "요즘 나는 무엇을 모으고 있나".
    ///
    /// 태그를 개수순으로 늘어놓는 것은 목록이지 통찰이 아니다. 이 제품에서 색은 개별
    /// 태그가 아니라 **facet 4개**가 갖고(§5.1), 그 넷은 각각 "만드는 것 / 형식 /
    /// 일 바깥 / 분류 없음"이라는 서로 다른 삶의 영역이다. 그 비율이야말로 목록을
    /// 아무리 스크롤해도 보이지 않는 값이고, 비율은 원형이 가장 정직하게 읽힌다.
    ///
    /// **분류 없음이 크면 그것도 신호다** — 사전이 내 관심사를 못 따라가고 있다는 뜻이라
    /// 숨기지 않고 같이 보여준다.
    @ViewBuilder
    private func composition(_ s: Components.Schemas.Stats) -> some View {
        let parts = facetShares(s)
        let total = parts.reduce(0) { $0 + $1.count }
        if total > 0 {
            VStack(alignment: .leading, spacing: 12) {
                sectionTitle("무엇을 모으고 있나", trailing: nil)
                HStack(spacing: 20) {
                    Chart(parts, id: \.facet) { part in
                        SectorMark(
                            angle: .value("건수", part.count),
                            // 도넛으로 비우는 이유: 가운데가 비면 조각 크기를 각도로
                            // 비교하게 되고(원판은 면적으로 착시가 생긴다), 빈 공간에
                            // 합계를 둘 수 있다.
                            innerRadius: .ratio(0.62),
                            angularInset: 1.5
                        )
                        .foregroundStyle(part.facet.ink)
                        .cornerRadius(2)
                    }
                    .frame(width: 132, height: 132)
                    .chartLegend(.hidden)
                    .overlay {
                        VStack(spacing: 0) {
                            Text("\(total)")
                                .font(PP.Typo.head)
                                .monospacedDigit()
                                .foregroundStyle(PP.Palette.fg1)
                            Text("태그").font(PP.Typo.label).foregroundStyle(PP.Palette.fg3)
                        }
                    }

                    VStack(alignment: .leading, spacing: 7) {
                        ForEach(parts, id: \.facet) { part in
                            HStack(spacing: 8) {
                                Circle().fill(part.facet.ink).frame(width: 8, height: 8)
                                // 한글 라벨은 웹과 같은 단어를 쓴다(§8.1).
                                Text(part.facet.label)
                                    .font(PP.Typo.label)
                                    .foregroundStyle(PP.Palette.fg1)
                                Spacer(minLength: 8)
                                Text("\(percent(part.count, total))%")
                                    .font(PP.Typo.metaMono)
                                    .monospacedDigit()
                                    .foregroundStyle(PP.Palette.fg3)
                            }
                        }
                    }
                }
            }
        }
    }

    private struct FacetShare { let facet: PP.Facet; let count: Int }

    /// 태그별 개수를 facet으로 접는다. 0인 facet은 빼서 범례가 비어 있지 않게 한다.
    private func facetShares(_ s: Components.Schemas.Stats) -> [FacetShare] {
        var sum: [PP.Facet: Int] = [:]
        for tag in s.by_tag {
            sum[facetOf(tag.name), default: 0] += tag.count
        }
        return PP.Facet.allCases
            .compactMap { f in sum[f].map { FacetShare(facet: f, count: $0) } }
            .filter { $0.count > 0 }
    }

    private func percent(_ value: Int, _ total: Int) -> Int {
        total > 0 ? Int((Double(value) / Double(total) * 100).rounded()) : 0
    }

    // MARK: - 태그

    @ViewBuilder
    private func topTags(_ s: Components.Schemas.Stats) -> some View {
        if !s.by_tag.isEmpty {
            let top = Array(s.by_tag.prefix(6))
            let maxCount = top.map(\.count).max() ?? 1
            VStack(alignment: .leading, spacing: 10) {
                sectionTitle("자주 붙은 태그", trailing: "누르면 그 목록으로")
                VStack(spacing: 6) {
                    ForEach(top, id: \.name) { tag in
                        // 누르면 그 태그의 목록으로 — 통계가 막다른 길이 되지 않게 한다.
                        Button { onFilter(.tag(tag.name)) } label: {
                            tagRow(tag, maxCount: maxCount)
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
        }
    }

    /// 막대를 쓰지 않는다. facet **tint**는 칩 배경용이라 그 위에 잉크 글씨가 얹힐 때만
    /// 구분되고, 단독 채움으로 쓰면 셋 다 "연한 무언가"로 뭉개진다 — 실제로 그렇게 보였다.
    /// 비율은 위 도넛이 이미 말하므로 여기서는 **소속(점)과 개수**만 보여주면 된다.
    private func tagRow(_ tag: Components.Schemas.Stats.by_tagPayloadPayload, maxCount: Int) -> some View {
        HStack(spacing: 10) {
            Circle()
                .fill(facetOf(tag.name).ink)
                .frame(width: 8, height: 8)
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
        .padding(.vertical, 5)
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
