import SwiftUI
import WidgetKit

/// 위젯 — 연속 저장을 잠금화면 밖에서 보여준다. 명세는 10 §8.6.
///
/// **서버를 부르지 않는다.** 인프로세스 서버는 앱 프로세스 안에서 돌고 위젯이 그려질 때
/// 앱은 대개 떠 있지 않다. 대신 App Group에 놓인 **계약 그대로의 `Stats` JSON**을 읽는다
/// (`StatsSnapshot`) — 같은 생성 타입으로 디코드하므로 모양도 디코더도 하나다.
struct Entry: TimelineEntry {
    let date: Date
    /// 스냅샷이 아직 없으면 nil. 그때는 **숫자를 지어내지 않고** 빈 상태를 그린다.
    let stats: Components.Schemas.Stats?

    var counts: [Int] { stats?.by_day.map(\.count) ?? [] }
    var streak: Int { Streak.days(counts) }
    var savedToday: Bool { Streak.savedToday(counts) }
    var capped: Bool { Streak.isCapped(counts, days: streak) }
}

struct Provider: TimelineProvider {
    func placeholder(in _: Context) -> Entry {
        Entry(date: .now, stats: nil)
    }

    func getSnapshot(in _: Context, completion: @escaping (Entry) -> Void) {
        completion(load())
    }

    func getTimeline(in _: Context, completion: @escaping (Timeline<Entry>) -> Void) {
        // **다음 자정에 다시 그린다.** 연속은 날이 바뀌면 뜻이 달라지는 값이라(어제까지
        // 3일이던 것이 오늘 저장이 없으면 여전히 3일이지만, 내일이면 끊긴다) 시간 단위
        // 갱신은 의미가 없고 자정만이 의미가 있다. 저장이 일어나는 순간은 앱과 확장이
        // `reloadAllTimelines()`로 직접 깨우므로 폴링이 필요 없다.
        let next = Calendar.current.nextDate(after: .now, matching: DateComponents(hour: 0, minute: 1),
                                             matchingPolicy: .nextTime) ?? .now.addingTimeInterval(3600)
        completion(Timeline(entries: [load()], policy: .after(next)))
    }

    private func load() -> Entry {
        guard let data = StatsSnapshot.read(),
              let stats = try? JSONDecoder().decode(Components.Schemas.Stats.self, from: data) else {
            return Entry(date: .now, stats: nil)
        }
        return Entry(date: .now, stats: stats)
    }
}

// MARK: - 화면

struct WidgetView: View {
    @Environment(\.widgetFamily) private var family
    var entry: Entry

    var body: some View {
        Group {
            if entry.stats == nil {
                empty
            } else if family == .systemMedium {
                medium
            } else {
                small
            }
        }
        .containerBackground(PP.Palette.canvas, for: .widget)
        // 오늘이 비어 있으면 저장 시트로, 아니면 통계로. 위젯이 "오늘 아직"이라고 말한
        // 다음 눌렀을 때 목록 맨 위로 데려가는 것은 그 말에 대한 답이 아니다(10 §8.6).
        .widgetURL(URL(string: entry.savedToday ? "pushpoint://stats" : "pushpoint://save"))
    }

    /// 스냅샷이 없다. **0일이라고 쓰지 않는다** — 그건 "아직 모른다"와 다른 말이고,
    /// 설치 직후 사용자에게 연속이 끊겼다고 말하는 셈이 된다.
    private var empty: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(t("widget.title")).font(PP.Typo.metaMono).foregroundStyle(PP.Palette.fg3)
            Spacer(minLength: 0)
            Text(t("widget.empty")).font(PP.Typo.label).foregroundStyle(PP.Palette.fg2)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }

    private var small: some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(t("widget.title")).font(PP.Typo.metaMono).foregroundStyle(PP.Palette.fg3)
            Spacer(minLength: 0)
            HStack(alignment: .firstTextBaseline, spacing: 3) {
                Text("\(entry.streak)")
                    .font(PP.Typo.display)
                    .foregroundStyle(PP.Palette.fg1)
                    .contentTransition(.numericText())
                Text(t("widget.daysUnit")).font(PP.Typo.label).foregroundStyle(PP.Palette.fg2)
                if entry.capped { Text("+").font(PP.Typo.label).foregroundStyle(PP.Palette.fg3) }
            }
            Text(entry.savedToday ? t("widget.savedToday") : t("widget.notYetToday"))
                .font(PP.Typo.meta)
                .foregroundStyle(entry.savedToday ? PP.Palette.fg2 : PP.Palette.accent)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }

    private var medium: some View {
        HStack(alignment: .top, spacing: 14) {
            small
            VStack(alignment: .leading, spacing: 6) {
                Text(t("widget.rhythm")).font(PP.Typo.metaMono).foregroundStyle(PP.Palette.fg3)
                Sparkline(counts: entry.counts)
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}

/// 30일 리듬. 막대 하나가 하루이고, **높이는 그날의 저장 수를 창 최댓값에 견준 값**이다.
///
/// 0인 날도 자리를 차지한다 — 빈 날이 사라지면 30일이 아니라 "저장한 날 목록"이 되고,
/// 리듬이라는 말이 뜻을 잃는다.
struct Sparkline: View {
    let counts: [Int]

    var body: some View {
        GeometryReader { geo in
            let peak = max(counts.max() ?? 1, 1)
            let gap: CGFloat = 1.5
            let w = max((geo.size.width - gap * CGFloat(max(counts.count - 1, 0))) / CGFloat(max(counts.count, 1)), 1)
            HStack(alignment: .bottom, spacing: gap) {
                ForEach(Array(counts.enumerated()), id: \.offset) { _, c in
                    RoundedRectangle(cornerRadius: 1)
                        .fill(c > 0 ? PP.Palette.accent : PP.Palette.line1)
                        // 저장이 있는 날은 최소 높이를 준다 — 창에 20건짜리 날이 있으면
                        // 1건인 날의 막대가 1px 미만이 되어 **있는데 없어 보인다.**
                        .frame(width: w,
                               height: c > 0 ? max(geo.size.height * CGFloat(c) / CGFloat(peak), 3) : 2)
                }
            }
            .frame(maxHeight: .infinity, alignment: .bottom)
        }
    }
}

// MARK: - 등록

@main
struct PushPointWidgetBundle: WidgetBundle {
    var body: some Widget { StreakWidget() }
}

struct StreakWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "com.pushpoint.streak", provider: Provider()) {
            WidgetView(entry: $0)
        }
        .configurationDisplayName(t("widget.title"))
        .description(t("widget.description"))
        // systemLarge를 넣지 않는다 — 통계가 다섯뿐이라 큰 면이 medium과 같은 말을 한다.
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}
