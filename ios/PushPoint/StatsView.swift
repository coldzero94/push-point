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
                    streakBlock(stats)
                    rhythm(stats)
                    topTags(stats)
                    needsAttention()
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

    private func streakBlock(_ s: Components.Schemas.Stats) -> some View {
        let days = streak(s.by_day)
        return VStack(alignment: .leading, spacing: 6) {
            HStack(alignment: .firstTextBaseline, spacing: 6) {
                Text("\(days)")
                    .font(PP.Typo.display)
                    .tracking(PP.Tracking.display)
                    .monospacedDigit()
                    .foregroundStyle(days > 0 ? PP.Palette.accent : PP.Palette.fg3)
                Text("일 연속")
                    .font(PP.Typo.head)
                    .foregroundStyle(PP.Palette.fg1)
            }
            // 목표를 숫자 옆에 두면 그 숫자가 무엇을 향하는지가 화면에서 읽힌다.
            Text(days >= 28
                 ? "4주 연속 — 목표를 넘겼습니다"
                 : "4주(28일) 연속이 목표입니다")
                .font(PP.Typo.meta)
                .tracking(PP.Tracking.meta)
                .foregroundStyle(PP.Palette.fg2)
        }
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
            Chart {
                ForEach(s.by_day, id: \.date) { day in
                    BarMark(
                        x: .value("날짜", day.date),
                        // 0인 날도 최소 높이를 그려 **공백이 공백으로 보이게** 한다.
                        // 막대가 아예 없으면 끊긴 날인지 데이터가 없는 날인지 구분되지 않는다.
                        y: .value("건수", max(day.count, 0))
                    )
                    .foregroundStyle(day.count > 0 ? PP.Palette.accent : PP.Palette.line2)
                    .cornerRadius(2)
                }
            }
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

    // MARK: - 태그

    @ViewBuilder
    private func topTags(_ s: Components.Schemas.Stats) -> some View {
        if !s.by_tag.isEmpty {
            let top = Array(s.by_tag.prefix(6))
            let maxCount = top.map(\.count).max() ?? 1
            VStack(alignment: .leading, spacing: 10) {
                sectionTitle("많이 모은 주제", trailing: nil)
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

    private func tagRow(_ tag: Components.Schemas.Stats.by_tagPayloadPayload, maxCount: Int) -> some View {
        HStack(spacing: 10) {
            Text(tag.name)
                .font(PP.Typo.label)
                .foregroundStyle(facetOf(tag.name).ink)
                .frame(width: 80, alignment: .leading)
                .lineLimit(1)
            GeometryReader { geo in
                // 막대 색이 칩 색과 같아서, 목록에서 본 색과 여기서 본 색이 이어진다.
                RoundedRectangle(cornerRadius: 3)
                    .fill(facetOf(tag.name).tint)
                    .frame(width: max(geo.size.width * ratio(tag.count, maxCount), 3))
            }
            .frame(height: 16)
            Text("\(tag.count)")
                .font(PP.Typo.metaMono)
                .monospacedDigit()
                .foregroundStyle(PP.Palette.fg3)
                .frame(width: 28, alignment: .trailing)
            Image(systemName: "chevron.right")
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.fg3)
        }
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
