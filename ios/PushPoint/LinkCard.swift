import SwiftUI

/// 보드의 기본 단위(§4.4 — 2026-07-25에 행에서 승격).
///
/// 행이 아니라 카드인 이유: 계약이 이미 주고 있는 `description`을 행이 한 글자도 쓰지
/// 않아서 화면이 "내가 모은 것"이 아니라 "레코드 목록"으로 읽혔다. 밀도를 조금 내주고
/// 읽을 수 있는 본문과 절대 비지 않는 커버를 얻는다.
struct LinkCard: View {
    let link: Components.Schemas.Link
    /// 태그 이름 → facet. 계약의 `LinkTag`에는 facet이 없어서 사전에서 해석한다.
    let facetOf: (String) -> PP.Facet
    /// 현재 켜져 있는 태그 필터 — 일치하는 칩이 solid(채움 2)가 된다.
    let activeTag: String?
    /// 상대 `thumb_url`을 절대 URL로 푼다(Backend가 서버 주소를 안다).
    let resolveThumb: (String) -> URL?

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            cover
            content
        }
        .background(PP.Palette.surface)
        .clipShape(RoundedRectangle(cornerRadius: PP.Radius.card, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: PP.Radius.card, style: .continuous)
                .strokeBorder(PP.Palette.line1, lineWidth: 1)
        )
        // S1 — 상태는 배지가 아니라 획. 카드의 leading edge에 3px.
        // **완료 상태는 아무 표시도 없다** — 화면에 남은 획은 전부 "지금 뭔가 일어나고
        // 있거나 잘못됐다"는 뜻이어야 한다.
        .overlay(alignment: .leading) { rail }
        .clipShape(RoundedRectangle(cornerRadius: PP.Radius.card, style: .continuous))
    }

    // MARK: - 커버

    private var cover: some View {
        // 슬롯이 먼저 크기를 정하고 내용이 그 안을 채운다. 이미지에 종횡비를 걸면
        // AsyncImage가 자기 비율로 컨테이너를 밀어서 사진마다 카드 높이가 달라진다 —
        // 실제로 썸네일 있는 카드만 3:1이 안 먹었다. 빈 슬롯이 치수를 확정하므로
        // 커버가 늦게 와도 레이아웃이 흔들리지 않는다(CLS 0).
        Color.clear
            // 3:1 — **iOS만의 의도적 편차**다. 웹의 16:9(§4.4)는 보드가 2~3열이라
            // 카드 하나가 화면의 1/3이지만, 아이폰은 1열이라 같은 종횡비가 화면당
            // 2장으로 이어진다. 커버는 "무엇이었는지 알아보는" 역할이면 충분하고,
            // 그 역할에 화면의 절반을 쓸 이유가 없다.
            .aspectRatio(3, contentMode: .fit)
            .overlay {
                if let thumb = link.thumb_url, let url = resolveThumb(thumb) {
                    AsyncImage(url: url) { image in
                        image.resizable().scaledToFill()
                    } placeholder: {
                        // 로드 전에도 회색을 보이지 않는다 — R4.
                        GeneratedCover(domain: link.domain, facet: dominantFacet)
                    }
                } else {
                    GeneratedCover(domain: link.domain, facet: dominantFacet)
                }
            }
            .clipped()
    }

    // MARK: - 본문

    private var content: some View {
        VStack(alignment: .leading, spacing: 5) {
            // 폴백: title 빈 문자열 → domain → url (§8.1). 서버는 빈 제목을 숨기지 않고
            // 그대로 주므로, 빈 셀을 막는 것은 클라이언트 책임이다.
            Text(displayTitle)
                .font(PP.Typo.title)
                .tracking(PP.Tracking.title)
                .foregroundStyle(PP.Palette.fg1)
                .lineLimit(2)

            if !link.description.isEmpty {
                Text(link.description)
                    .font(PP.Typo.card)
                    .tracking(PP.Tracking.card)
                    .foregroundStyle(PP.Palette.fg2)
                    .lineLimit(2)
            }

            if !link.tags.isEmpty {
                HStack(spacing: 4) {
                    ForEach(sortedTags, id: \.id) { tag in
                        Chip(name: tag.name,
                             facet: facetOf(tag.name),
                             fill: .init(source: tag.source.rawValue, isActive: tag.name == activeTag))
                    }
                }
                .padding(.top, 2)
            }

            // 기계 데이터는 고정폭(R2) — 사람이 쓴 줄(제목·설명)과 나란히 놓여 대비가 산다.
            HStack(spacing: 6) {
                Text(link.domain)
                Text("·")
                Text(relativeTime)
            }
            .font(PP.Typo.metaMono)
            .foregroundStyle(PP.Palette.fg3)
            .padding(.top, 3)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, 14)
        .padding(.top, 12)
        .padding(.bottom, 14)
    }

    @ViewBuilder
    private var rail: some View {
        switch link.status {
        case .failed:
            Rectangle().fill(PP.Palette.danger).frame(width: 3)
        case .pending, .scraping, .tagging:
            Rectangle().fill(PP.Palette.railProgress).frame(width: 3)
        case .done:
            EmptyView() // 완료에는 획이 없다
        }
    }

    // MARK: -

    private var displayTitle: String {
        if !link.title.isEmpty { return link.title }
        return link.domain.isEmpty ? link.url : link.domain
    }

    /// 칩 순서 = manual 먼저, 그다음 신뢰도 내림차순. 웹의 `sortLinkTags`와 같은 규칙이라
    /// 두 클라이언트에서 같은 태그가 지배 태그가 된다.
    private var sortedTags: [Components.Schemas.LinkTag] {
        link.tags.sorted { a, b in
            let am = a.source == .manual, bm = b.source == .manual
            if am != bm { return am }
            return (a.confidence ?? 1) > (b.confidence ?? 1)
        }
    }

    /// 커버 색은 **지배 태그**(칩 순서의 첫 번째)의 facet에서 온다. 태그가 없으면 neutral —
    /// 커버는 새 색 채널이 아니라 태그 색의 연장이다(R1).
    private var dominantFacet: PP.Facet {
        sortedTags.first.map { facetOf($0.name) } ?? .neutral
    }

    private var relativeTime: String {
        let date = Date(timeIntervalSince1970: TimeInterval(link.created_at))
        let f = RelativeDateTimeFormatter()
        f.locale = Locale(identifier: "ko_KR")
        f.unitsStyle = .short
        return f.localizedString(for: date, relativeTo: Date())
    }
}
