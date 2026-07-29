import SwiftUI

/// 보드의 기본 단위(§4.4 — 2026-07-25에 행에서 승격).
///
/// 행이 아니라 카드인 이유: 계약이 이미 주고 있는 `description`을 행이 한 글자도 쓰지
/// 않아서 화면이 "내가 모은 것"이 아니라 "레코드 목록"으로 읽혔다. 밀도를 조금 내주고
/// 읽을 수 있는 본문과 절대 비지 않는 커버를 얻는다.
/// 목록 밀도(§1.3, 2026-07-29 iOS 한정 해제).
///
/// **웹에는 없다.** 창 폭이 이미 그 일을 하기 때문이고, 폰은 뷰포트가 하나뿐이라
/// "밀도는 뷰포트가 결정한다"는 원래 규칙이 폰에서만 성립하지 않았다(13 §1 ② 축).
enum ListDensity: String, CaseIterable {
    case card, compact

    var label: String { self == .card ? "카드" : "조밀" }
    var symbol: String { self == .card ? "rectangle.grid.1x2" : "list.bullet" }
    var next: ListDensity { self == .card ? .compact : .card }
}

struct LinkCard: View {
    let link: Components.Schemas.Link
    /// 태그 이름 → facet. 계약의 `LinkTag`에는 facet이 없어서 사전에서 해석한다.
    let facetOf: (String) -> PP.Facet
    /// 현재 켜져 있는 태그 필터 — 일치하는 칩이 solid(채움 2)가 된다.
    let activeTag: String?
    /// 상대 `thumb_url`을 절대 URL로 푼다(Backend가 서버 주소를 안다).
    let resolveThumb: (String) -> URL?
    /// 실패한 링크의 잡을 다시 넣는다. nil이면 재시도 줄을 그리지 않는다.
    var onRetry: (() -> Void)? = nil
    /// 조밀 모드는 **커버만 뺀다** — 제목·태그·상태·메타는 그대로다. 커버가 카드
    /// 높이의 절반이라 그것만 빼도 한 화면에 들어가는 수가 2장에서 6~7장이 된다.
    /// 글자 크기는 건드리지 않는다: 타입 스케일은 밀도의 손잡이가 아니다(§2.2.2).
    var density: ListDensity = .card

    /// 펄스를 감소가 아니라 **제거**로 처리하기 위한 것(§7.4).
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var pulsing = false

    var body: some View {
        VStack(alignment: .leading, spacing: 0) {
            if density == .card { cover }
            content
        }
        .background(PP.Palette.surface)
        .clipShape(RoundedRectangle(cornerRadius: PP.Radius.card, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: PP.Radius.card, style: .continuous)
                .strokeBorder(PP.Palette.line1, lineWidth: 1)
        )
        // S1 — 상태는 배지가 아니라 획. 카드의 leading edge에 `PP.Size.rail`(2px).
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
            // 3:1 — **iOS만의 의도적 편차**이고 §8.5에 등재돼 있다. 웹의 16:9(§4.4)는
            // 보드가 2~3열이라 카드 하나가 화면의 1/3이지만, 아이폰은 1열이라 같은
            // 종횡비면 화면당 2장이 된다. 커버는 "무엇이었는지 알아보는" 역할이면
            // 충분하고, 그 역할에 화면의 절반을 쓸 이유가 없다.
            .aspectRatio(3, contentMode: .fit)
            .overlay {
                if let thumb = link.thumb_url, let url = resolveThumb(thumb) {
                    // **`phase`를 쓰는 이유는 `.failure`를 보기 위해서다.** 두 갈래
                    // `AsyncImage(url:content:placeholder:)`는 "아직 안 왔다"와 "못 받았다"를
                    // 같은 placeholder로 접는다. 그래서 썸네일이 깨져도 화면은 생성 커버를
                    // 그리고, 생성 커버는 **썸네일이 원래 없는 링크의 정상 표시**이기도 하다
                    // — 두 경우가 완전히 같아진다.
                    //
                    // 이 프로젝트는 그 부류를 이미 두 번 겪었다: 상대 `thumb_url`로 전부
                    // 비었던 것, 그리고 2026-07-29에 **사용자가 먼저 알아챈** 죽은 경로.
                    // 서버가 없는 파일을 광고하지 않게 고쳤지만(§thumbURL), 전송 실패는 여전히
                    // 남는다. 보이는 것은 그대로 두고 — 회색을 보이지 않는 R4는 유효하다 —
                    // **로그에는 남긴다.** 다음 번에도 우연에 기대지 않기 위해서다.
                    AsyncImage(url: url) { phase in
                        switch phase {
                        case let .success(image):
                            image.resizable().scaledToFill()
                        case .failure:
                            GeneratedCover(domain: link.domain, facet: dominantFacet)
                                .task { PPLog.thumbFailed(url, linkID: link.id) }
                        default:
                            GeneratedCover(domain: link.domain, facet: dominantFacet)
                        }
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

            failureRow

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

    /// 상태 레일(§4.7). 두께는 `PP.Size.rail` 하나이고 위계는 굵기가 아니라 위치로 만든다.
    ///
    /// 진행 중에는 `.7↔1`로 펄스한다 — 앱에서 **유일한 무한 루프**이고, 워커가 살아 있다는
    /// 실제 시스템 상태다. 하한이 `.35`가 아니라 `.7`인 이유는 대비다: 배지를 폐기했으므로
    /// 이 획이 진행 상태의 유일한 시각 신호이고 WCAG 1.4.11(비텍스트 3:1) 대상이 된다.
    ///
    /// 색만으로 뜻을 지지 않도록 상태 문구를 접근성 라벨로 항상 동반한다.
    @ViewBuilder
    private var rail: some View {
        switch link.status {
        case .failed:
            Rectangle().fill(PP.Palette.danger)
                .frame(width: PP.Size.rail)
                .accessibilityLabel("실패")
        case .pending, .scraping, .tagging:
            Rectangle().fill(PP.Palette.railProgress)
                .frame(width: PP.Size.rail)
                .opacity(pulsing ? 1 : 0.7)
                .animation(reduceMotion ? nil
                           : .easeInOut(duration: 1.2).repeatForever(autoreverses: true),
                           value: pulsing)
                .onAppear { if !reduceMotion { pulsing = true } }
                .accessibilityLabel(statusLabel)
        case .done:
            EmptyView() // 완료에는 획이 없다
        }
    }

    /// 웹 `StatusRail`의 STATUS_LABEL과 **같은 단어**를 쓴다(§8.1).
    private var statusLabel: String {
        switch link.status {
        case .pending: "대기"
        case .scraping: "수집 중"
        case .tagging: "태깅 중"
        case .done: "완료"
        case .failed: "실패"
        }
    }

    /// 실패는 레일만으로 끝내지 않는다 — §4.7이 "레일 + 텍스트 + 아이콘" 3중을 요구한다.
    ///
    /// 재시도가 **스와이프에만** 있던 것이 문제였다. 이 파일의 스와이프 주석이 스스로
    /// "빠르지만 발견되지 않는다"고 적어 두었는데, 실패 복구는 발견되지 않으면 안 되는
    /// 동작이다 — 그 링크는 영원히 실패로 남는다.
    @ViewBuilder
    private var failureRow: some View {
        if link.status == .failed {
            HStack(spacing: 6) {
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(PP.Typo.label)
                    .foregroundStyle(PP.Palette.danger)
                // **사유를 못 보여준다.** 목록 항목 `Link`에는 `error`가 없다(상세에만 있다).
                // 계약을 넓히는 건 별건이라, 여기서는 "실패했고 여기를 누르면 다시 한다"까지만
                // 말한다 — 그것만으로도 스와이프에 숨어 있던 것보다 낫다.
                Text("수집에 실패했습니다")
                    .font(PP.Typo.label)
                    .tracking(PP.Tracking.label)
                    .foregroundStyle(PP.Palette.fg2)
                    .lineLimit(1)
                Spacer(minLength: 4)
                if let onRetry {
                    Button("재시도", action: onRetry)
                        .font(PP.Typo.label)
                        .foregroundStyle(PP.Palette.accent)
                        .buttonStyle(.plain)
                }
            }
            .padding(.top, 4)
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

    /// 상대 시각. 규칙은 Shared/RelativeTime.swift에 있다 — 웹과 값이 맞아야 하고
    /// 갈라져도 양쪽 다 정상으로 보이므로 기준값을 테스트로 박아 둔 자리다.
    private var relativeTime: String {
        RelativeTime.label(link.created_at)
    }
}
