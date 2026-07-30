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
    /// 이 카드가 **날짜 머리글 아래**에 있는가. 보드는 true, 검색 결과는 false —
    /// 구간이 없는 화면에서는 "어제"가 유일한 날짜 정보다.
    var dayStated: Bool = false

    /// 펄스를 감소가 아니라 **제거**로 처리하기 위한 것(§7.4).
    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @Environment(\.dynamicTypeSize) private var dynamicTypeSize
    /// 커버 한 변(§4.4.1). `@ScaledMetric`인 이유는 §8.3이 행 높이 고정을 금지하기 때문이다 —
    /// 글자만 커지고 앵커가 그대로면 큰 글자에서 균형이 무너진다.
    @ScaledMetric(relativeTo: .body) private var scaledCover: CGFloat = 44
    @State private var pulsing = false

    var body: some View {
        Group {
            switch density {
            case .card:
                VStack(alignment: .leading, spacing: 0) {
                    cover
                    content
                }
            case .compact:
                compactRow
            }
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

    // MARK: - 조밀 행

    /// 제목·도메인·시각 + trailing 44pt 커버. **본문과 칩은 없다**(§4.4.1).
    ///
    /// 첫 판의 조밀은 반대였다 — 커버를 빼고 본문·칩을 남겼다. 2026-07-30에 뒤집었고 근거는
    /// §4.4.1에 있다. 요약하면: 이 목록의 목적은 **되찾기**이고 그 조건에서 이미지를 빼는 것이
    /// 가장 비싼 제거(Teevan 2009)인데, 반대로 본문 텍스트는 줄이는 편이 위치 정보에 시선을
    /// 남긴다(Cutrell & Guan 2007). 지표는 화면당 4행에서 11행이 된다 — 그런데 첫 판이 노린
    /// 것도 그 지표였고 **주석이 "6~7행"이라 적어 둔 값은 실측 4행이었다.**
    ///
    /// 접근성 크기에서는 세로로 갈라진다. 클램프가 아니라 **다른 레이아웃**이다 — 본문이
    /// AX5에서 3배가 되므로 44pt 한 줄 행은 그 크기에 존재할 수 없다.
    @ViewBuilder
    private var compactRow: some View {
        if isAccessibilitySize {
            VStack(alignment: .leading, spacing: 8) {
                compactCover
                compactText
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
        } else {
            HStack(alignment: .top, spacing: 12) {
                compactText
                compactCover
            }
            .padding(.horizontal, 12)
            .padding(.vertical, 8)
        }
    }

    private var compactText: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(displayTitle)
                .font(PP.Typo.title)
                .tracking(PP.Tracking.title)
                .foregroundStyle(PP.Palette.fg1)
                // **줄 예산.** 제목이 먼저 가져간다(§4.4.1). 조밀에는 본문이 없으므로 지금은
                // 상한으로만 작동하는데, 예산 형태로 두는 이유는 행 높이가 거의 일정해야
                // 훑기가 되기 때문이다 — 가변 높이 셀은 스캔성을 이유로 기각된 형태다.
                .lineLimit(Self.compactLineBudget)
            metaLine
            failureRow
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    /// 조밀 행의 커버. 없는 링크도 생성 커버를 받는다 — R4는 밀도와 무관하다.
    private var compactCover: some View {
        Group {
            if let thumb = link.thumb_url, let url = resolveThumb(thumb) {
                AsyncImage(url: url) { phase in
                    if case let .success(image) = phase {
                        image.resizable().scaledToFill()
                    } else {
                        GeneratedCover(domain: link.domain, facet: dominantFacet,
                                       showsWordmark: false)
                    }
                }
            } else {
                GeneratedCover(domain: link.domain, facet: dominantFacet,
                               showsWordmark: false)
            }
        }
        .frame(width: coverSide, height: coverSide)
        .clipShape(RoundedRectangle(cornerRadius: PP.Radius.thumb, style: .continuous))
    }

    static let compactLineBudget = 2

    /// 도메인 · 시각. **두 밀도가 같은 줄을 쓴다** — 따로 두면 갈라진다.
    ///
    /// 기계 데이터는 고정폭(R2) — 사람이 쓴 줄과 나란히 놓여 대비가 산다.
    private var metaLine: some View {
        HStack(spacing: 6) {
            Text(link.domain)
            Text("·")
            Text(relativeTime)
        }
        .font(PP.Typo.metaMono)
        .foregroundStyle(PP.Palette.fg3)
    }

    /// 큰 글자에서 커버도 같이 커진다(§8.2) — 고정 pt로 두면 글자만 커지고 앵커는 남는다.
    private var coverSide: CGFloat { scaledCover }

    private var isAccessibilitySize: Bool { dynamicTypeSize.isAccessibilitySize }

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
                        case let .failure(error):
                            // **오류를 버리지 않는다.** 404(서버가 없는 파일을 광고),
                            // 연결 거부(내장 서버가 아직 바인딩 중), 디코드 실패(0바이트
                            // JPEG), 타임아웃은 서로 다른 대응을 요구하는데, URL만 남기면
                            // 넷 다 같은 줄이 된다.
                            GeneratedCover(domain: link.domain, facet: dominantFacet)
                                .task { PPLog.thumbFailed(url, linkID: link.id, error: error) }
                        case .empty:
                            GeneratedCover(domain: link.domain, facet: dominantFacet)
                        @unknown default:
                            GeneratedCover(domain: link.domain, facet: dominantFacet)
                        }
                    }
                } else if let thumb = link.thumb_url {
                    // **서버는 주소를 줬는데 우리가 URL을 못 만든 갈래다.**
                    //
                    // 이 프로젝트가 실제로 출하한 썸네일 사고가 바로 여기였다 —
                    // `Backend.absoluteURL` 주석이 그대로 적어 두고 있다: "그대로
                    // `URL(string:)`에 넣으면 host 없는 URL이 되어 **조용히 아무것도 안
                    // 그린다** — 실제로 그렇게 썸네일이 전부 비어 보였다."
                    //
                    // 그런데 계측을 붙일 때 이 갈래를 빼먹었다. `.failure`는 어차피
                    // 네트워크 오류를 내는 쪽이고, **역사적으로 조용했던 쪽은 이쪽이다.**
                    GeneratedCover(domain: link.domain, facet: dominantFacet)
                        .task { PPLog.thumbUnresolvable(thumb, linkID: link.id) }
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

            metaLine.padding(.top, 4)
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
                // **reduced-motion은 정적 `1`이다** — 감소가 아니라 제거다(§4.7 · §7.4 ·
                // §8.2가 세 곳에서 같은 말을 한다). 예전에는 `pulsing ? 1 : 0.7`이라
                // 애니메이션만 끄고 값은 `0.7`에 남았는데, 그건 **펄스의 하한**이고
                // 명세가 피하려던 값이다. 대비 수치 5.94/5.57도 `1` 기준으로 계산돼 있다.
                .opacity(reduceMotion ? 1 : (pulsing ? 1 : 0.7))
                .animation(reduceMotion ? nil
                           : .easeInOut(duration: 1.2).repeatForever(autoreverses: true),
                           value: pulsing)
                // **`.task(id: link.status)`로 돌린다.**
                //
                // `onAppear`는 `pulsing`을 한 번만 뒤집는다. 그래서 **뷰가 다시 나타나지
                // 않는 전이**에서는 애니메이션이 재시작되지 않는다 — 실패한 링크를
                // 재시도하면 카드는 화면에 그대로 있고 `status`만 failed → pending으로
                // 바뀌는데, `onAppear`는 안 불리고 `pulsing`은 이미 true라 값도 안 변한다.
                // §1.4 S2가 "폴러가 갱신하면 슬롯이 켜진다"고 규정한 그 순간이다.
                //
                // **정직하게 적어 둔다**: 원래 이 변경은 "셀 재사용 시 펄스가 멎는다"를
                // 근거로 제안됐는데, 2026-07-30에 스크롤로 재현을 시도했더니 옛 코드도
                // 계속 돌았다(진폭 43.9). 그 기전은 확인하지 못했다. 남는 근거는 위의
                // 상태 전이 하나이고, 그건 `onAppear`의 정의상 확실하다.
                .task(id: link.status) {
                    guard !reduceMotion else { return }
                    pulsing = false
                    pulsing = true
                }
                .accessibilityLabel(statusLabel)
        case .done:
            EmptyView() // 완료에는 획이 없다
        }
    }

    /// 웹 `StatusRail`의 STATUS_LABEL과 **같은 단어**를 쓴다(§8.1).
    ///
    /// `waiting`이 여기서 갈린다. **백오프로 누워 있는 링크는 `status`가 여전히 `pending`**
    /// 이라 진행 레일이 돌고, 화면은 일하는 중이라고 말한다 — 실제로는 최대 30×attempts초를
    /// 기다리는 중이다. 12 §4.3이 "이 제안이 발견한 유일하게 참인 관찰"이라고 적은 것이
    /// 그것이고, 계약의 `retry_state`가 이제 그 사실을 싣는다.
    private var statusLabel: String {
        if link.retry_state == .waiting { return "재시도 대기 중" }
        switch link.status {
        case .pending: return "대기"
        case .scraping: return "수집 중"
        case .tagging: return "태깅 중"
        case .done: return "완료"
        case .failed: return "실패"
        }
    }

    /// 실패 문구. 사유가 있으면 사유를, 없으면 예전 문장을 쓴다.
    ///
    /// **`waiting`은 여기 오지 않는다** — 그 링크의 `status`는 아직 `failed`가 아니라
    /// `pending`이고, 진행 레일이 도는 쪽(`statusLabel`)에서 다뤄야 한다.
    private var failureLabel: String {
        link.error.isEmpty ? "수집에 실패했습니다" : link.error
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
                // **사유를 보여준다.** 예전에는 `Link`에 `error`가 없어서 모든 실패가
                // "수집에 실패했습니다" 한 문장이었고, 무엇이 잘못됐는지 보려면 링크마다
                // 상세를 열어야 했다. 계약이 사유를 목록으로 올렸다.
                Text(failureLabel)
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
        RelativeTime.label(link.created_at, dayStated: dayStated)
    }
}
