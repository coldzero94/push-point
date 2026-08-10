import SwiftUI
import WidgetKit

/// 되살림 위젯 — 잊고 있던 링크 하나를 홈 화면에 띄운다.
///
/// **왜 이게 필요한가.** 되살림은 `GET /links/resurfaced`로 진작 있었지만 **앱을 열어야만**
/// 왔다. 잊어버린 것에 대한 기능을, 앱을 열어야 한다는 걸 기억해야 쓸 수 있는 구조였다.
/// 홈 화면은 이미 하루에 수십 번 보는 자리다 — 여기 놓으면 링크가 사람을 찾아온다.
///
/// 연속 위젯과 **따로** 등록한다. 하나로 합치면 둘 중 하나는 반드시 밀려나는데, 연속은
/// 습관을 만드는 숫자이고 되살림은 읽을 거리라 서로를 대신하지 못한다. 사용자가 고른다.
struct ResurfaceEntry: TimelineEntry {
    let date: Date
    /// 스냅샷이 없거나 후보가 없으면 nil. 그때는 **아무것도 지어내지 않는다.**
    let link: Components.Schemas.Link?
}

struct ResurfaceProvider: TimelineProvider {
    func placeholder(in _: Context) -> ResurfaceEntry {
        ResurfaceEntry(date: .now, link: nil)
    }

    func getSnapshot(in _: Context, completion: @escaping (ResurfaceEntry) -> Void) {
        completion(load())
    }

    func getTimeline(in _: Context, completion: @escaping (Timeline<ResurfaceEntry>) -> Void) {
        // **다음 자정에 다시 그린다.** 서버가 하루 동안 같은 답을 주므로(계약 §되살림)
        // 그 안에서 다시 그릴 이유가 없다. 앱이 새 되살림을 받는 순간은
        // `reloadAllTimelines()`로 직접 깨우므로 폴링도 필요 없다 — 연속 위젯과 같은 판단.
        let next = Calendar.current.nextDate(after: .now, matching: DateComponents(hour: 0, minute: 1),
                                             matchingPolicy: .nextTime) ?? .now.addingTimeInterval(3600)
        completion(Timeline(entries: [load()], policy: .after(next)))
    }

    private func load() -> ResurfaceEntry {
        guard let data = ResurfaceSnapshot.read(),
              let link = try? JSONDecoder().decode(Components.Schemas.Link.self, from: data) else {
            return ResurfaceEntry(date: .now, link: nil)
        }
        return ResurfaceEntry(date: .now, link: link)
    }
}

struct ResurfaceWidgetView: View {
    @Environment(\.widgetFamily) private var family
    var entry: ResurfaceEntry

    var body: some View {
        Group {
            if let link = entry.link {
                filled(link)
            } else {
                empty
            }
        }
        .containerBackground(PP.Palette.canvas, for: .widget)
        // 링크 URL이 아니라 **앱으로** 보낸다. 사파리로 바로 열면 그 링크가 열렸다는 사실을
        // 아무도 기록하지 못해 되살림 후보에서 안 빠지고, 다음 날 또 같은 것이 뜬다.
        .widgetURL(URL(string: "pushpoint://resurfaced"))
    }

    /// 되살릴 것이 없다. **빈 카드를 그리지 저장 독려를 하지 않는다** — 후보가 없다는 것은
    /// 대개 아카이브가 아직 7일을 안 넘겼다는 뜻이고, 그건 사용자가 뭘 잘못해서가 아니다.
    private var empty: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(t("widget.resurface.title")).font(PP.Typo.metaMono).foregroundStyle(PP.Palette.fg3)
            Spacer(minLength: 0)
            Text(t("widget.resurface.empty")).font(PP.Typo.label).foregroundStyle(PP.Palette.fg2)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }

    private func displayTitle(_ link: Components.Schemas.Link) -> String {
        if !link.title.isEmpty { return link.title }
        return link.domain.isEmpty ? link.url : link.domain
    }

    private func filled(_ link: Components.Schemas.Link) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(t("widget.resurface.title")).font(PP.Typo.metaMono).foregroundStyle(PP.Palette.fg3)

            // 제목이 본문이다. small에서는 3줄, medium에서는 2줄 — medium은 가로가 넓어
            // 같은 글자가 더 적은 줄에 들어간다.
            //
            // 폴백은 제목 → 도메인 → URL. 서버는 og도 title도 없으면 빈 문자열을 그대로
            // 주므로(사실을 숨기지 않는다) 빈 칸을 막는 것은 클라이언트 몫이다
            // (.claude/rules/ios.md).
            Text(displayTitle(link))
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.fg1)
                .lineLimit(family == .systemMedium ? 2 : 3)
                .multilineTextAlignment(.leading)

            Spacer(minLength: 0)

            HStack(spacing: 6) {
                // 제목 자리가 이미 도메인을 쓰고 있으면 같은 글자를 두 번 적지 않는다.
                if !link.domain.isEmpty, displayTitle(link) != link.domain {
                    Text(link.domain).font(PP.Typo.meta).foregroundStyle(PP.Palette.fg2).lineLimit(1)
                }
                Spacer(minLength: 0)
                Text(RelativeTime.label(link.created_at))
                    .font(PP.Typo.meta)
                    .foregroundStyle(PP.Palette.fg3)
                    .layoutPriority(1)
            }
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .leading)
    }
}

struct ResurfaceWidget: Widget {
    var body: some WidgetConfiguration {
        StaticConfiguration(kind: "com.pushpoint.resurface", provider: ResurfaceProvider()) {
            ResurfaceWidgetView(entry: $0)
        }
        .configurationDisplayName(t("widget.resurface.name"))
        .description(t("widget.resurface.desc"))
        .supportedFamilies([.systemSmall, .systemMedium])
    }
}
