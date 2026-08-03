import SwiftUI

/// 디자인 토큰 — **값의 출처는 `docs/v2/10-DESIGN-SYSTEM.md`이고 이 파일은 이름만 갖는다.**
/// 값이 어긋나면 10번 문서가 옳다(`ios/design/README.md`의 규칙).
///
/// 색은 여기에 hex로 적지 않고 **Asset Catalog**(`ios/PushPoint/Assets.xcassets`)에서 온다
/// (§8.2). 웹이 CSS 변수로 값을 갖고 이 파일이 이름만 갖는 구조가 같아서, 값을 고칠 때
/// 손댈 곳이 두 클라이언트 각각 한 군데씩으로 유지된다. 에셋은 문서 값에서 스크립트로
/// 생성했다 — 27개를 손으로 옮기면 오타가 조용히 들어간다.
///
/// **`.primary`/`.secondary` 같은 시스템 색을 쓰지 않는다.** 값이 웹과 갈라지는 순간
/// "두 클라이언트가 같은 제품"이라는 주장이 깨진다(§8.2).
///
/// 웹과 공유하는 것: 의미 색 토큰 전체, facet 팔레트 6개, radius 의미 이름, R1~R3,
/// 한글 라벨 사전. iOS에만 있는 것은 공유 시트 하나다(§8.1).
enum PP {}

// MARK: - 색 (§2.1.2, §8.2)

extension PP {
    enum Palette {
        static let canvas = Color("canvas")
        static let surface = Color("surface")
        /// iOS에는 hover가 없다 — **pressed 상태와 폴백 배경에만** 쓴다.
        static let hover = Color("hover")
        static let elevated = Color("elevated")
        static let selected = Color("selected")

        static let fg1 = Color("fg1")
        static let fg2 = Color("fg2")
        /// 3차 텍스트 — 도메인·시각 같은 **보조 메타 전용**이다. 라이트 대비가 2.66으로
        /// 4.5:1에 못 미치는데, 3단 색 위계를 지키려고 사용 범위를 좁히는 것으로 갚기로
        /// 한 의도된 예외다(§2.1.3). 본문·라벨에 쓰면 그 합의가 깨진다.
        static let fg3 = Color("fg3")
        static let fgInverse = Color("fgInverse")

        /// 장식 헤어라인 전용 — 척추 하한선, 카드 링. 대비 게이트 대상이 아니다(§7.1).
        static let line1 = Color("line1")
        /// 장식 헤어라인 전용 — 섹션 구분, 패널 외곽.
        static let line2 = Color("line2")
        /// 컨트롤 경계 전용 — 입력·필터 칩·secondary 버튼 보더.
        /// **3:1 대상이므로 시스템 기본 보더로 대체하지 않는다**(§8.2).
        static let lineControl = Color("lineControl")

        /// 진행 중(pending/scraping/tagging) 상태 레일.
        static let railProgress = Color("railProgress")

        static let accent = Color("accent")
        static let accentHover = Color("accentHover")
        /// 선택된 카드 배경 전용. **manual 칩에는 쓰지 않는다** — manual은 그 태그 자신의
        /// facet tint다(§5.2).
        static let accentTint = Color("accentTint")
        static let onAccent = Color("onAccent")

        static let danger = Color("danger")
        static let dangerTint = Color("dangerTint")
        static let warn = Color("warn")
        static let warnTint = Color("warnTint")
    }
}

// MARK: - facet (§5)

extension PP {
    /// 색은 개별 태그가 아니라 **facet 3개 + 무채색 1개**가 갖는다.
    ///
    /// hue를 태그 이름으로 해시하지 않는 것이 핵심 제약이다 — 사전이 바뀌면 전량
    /// 재배치되고 의미와 색이 무관해진다(§5.4 금지). L을 facet마다 다르게 둔 것도
    /// 의도다: 등명도로 맞추면 색각 이상에서 분리가 붕괴한다. L은 protan/deutan에서
    /// 살아남는 유일한 채널이다.
    ///
    /// hex를 복제하지 않고 **계약의 `Tag.facet`으로 asset을 고르는 것**이 §8.1이 말하는
    /// "같은 원본에서 나온다"의 실제 이행이다.
    enum Facet: String, CaseIterable {
        case craft, media, life, neutral

        /// 계약(`api/openapi.yaml`)의 facet 문자열에서 만든다. 모르는 값은 neutral이다 —
        /// 사전이 확장돼도 화면이 깨지지 않아야 한다.
        init(apiValue: String) {
            self = Facet(rawValue: apiValue.lowercased()) ?? .neutral
        }

        var ink: Color {
            switch self {
            case .craft: Color("tagCraftInk")
            case .media: Color("tagMediaInk")
            case .life: Color("tagLifeInk")
            // neutral은 새 토큰을 만들지 않고 기존 토큰을 재사용한다 — "색이 없는
            // 상태"라는 사실을 토큰 이름이 그대로 말한다(§5.2).
            case .neutral: PP.Palette.fg2
            }
        }

        var tint: Color {
            switch self {
            case .craft: Color("tagCraftTint")
            case .media: Color("tagMediaTint")
            case .life: Color("tagLifeTint")
            case .neutral: PP.Palette.hover
            }
        }

        /// 생성 커버의 바탕. **칩 tint와 다른 값이다**(§4.5.2) — 칩 tint는 12px ink
        /// 텍스트가 읽히도록 잡은 밝은 값이고, 커버는 카드의 40%를 차지하는 면이라
        /// 같은 값을 쓰면 흰 사각형이 된다. hue는 같고 L만 내렸다.
        var cover: Color {
            switch self {
            case .craft: Color("coverCraft")
            case .media: Color("coverMedia")
            case .life: Color("coverLife")
            case .neutral: Color("coverNeutral")
            }
        }

        /// 한글 라벨은 두 클라이언트가 **같은 단어**를 써야 한다(§8.1).
        var label: String {
            switch self {
            case .craft: t("tags.facetCraft")
            case .media: t("tags.facetMedia")
            case .life: t("tags.facetLife")
            case .neutral: t("tags.facetNeutral")
            }
        }
    }
}

// MARK: - 타이포그래피 (§2.2, §8.3)

extension PP {
    /// 8단 고정 스케일. 굵기는 400/500/600 셋뿐이다 — 700 이상·300 이하·가변 폰트
    /// 실수 weight는 플랫폼별 스냅이 일어나 예측 불가라 금지다.
    ///
    /// **웹과 같은 폰트 파일을 쓴다**(§2.2.1) — Wanted Sans / Geist Mono, 둘 다 OFL.
    /// `relativeTo:`가 붙어 있으므로 커스텀 폰트여도 Dynamic Type에 맞춰 스케일한다.
    /// (문서가 오래 "커스텀 폰트를 쓰면 Dynamic Type을 잃는다"고 적어 뒀는데 사실이
    /// 아니었다. iOS 14부터 이 이니셜라이저가 그 일을 한다.)
    ///
    /// 등록은 `project.yml`의 `UIAppFonts`가 한다. 빠뜨리면 `Font.custom`이 **조용히
    /// 시스템 폰트로 떨어진다** — 빌드는 통과하고 화면만 달라진다.
    enum Typo {
        // **가변 폰트의 named instance PostScript 이름을 직접 부른다.**
        // `Font.custom(...).weight(.semibold)`는 가변 폰트에서 축을 움직여 주지 않아
        // 400 그대로 그려질 수 있다 — 빌드도 통과하고 굵기만 틀리는 종류다.
        // 파일명이 아니라 이 이름이어야 한다(파일명을 주면 시스템 폰트로 조용히 떨어진다).
        private static let sans400 = "WantedSansVariable-Regular"
        private static let sans500 = "WantedSansVariable-Medium"
        private static let sans600 = "WantedSansVariable-SemiBold"
        private static let mono400 = "GeistMono-Regular"

        /// 태그 칩, 상태 텍스트, 카운트.
        static let label = Font.custom(sans500, size: 12, relativeTo: .caption)
        /// 도메인, 저장 시각, 보조 설명.
        static let meta = Font.custom(sans400, size: 13, relativeTo: .footnote)
        /// 기계 데이터는 고정폭(R2) — 도메인·시각·카운트·신뢰도.
        static let metaMono = Font.custom(mono400, size: 13, relativeTo: .footnote)
        /// 카드의 description 2줄. body를 쓰면 카드가 8px 높아지고 meta를 쓰면
        /// 한글 두 줄이 붙는다 — 그 사이.
        static let card = Font.custom(sans400, size: 13, relativeTo: .footnote)
        /// 설명, 메모, 입력 필드.
        static let body = Font.custom(sans400, size: 15, relativeTo: .subheadline)
        /// 링크 제목(카드 2줄 클램프).
        static let title = Font.custom(sans600, size: 15, relativeTo: .subheadline)
        /// 화면 제목, 인스펙터 제목.
        static let head = Font.custom(sans600, size: 20, relativeTo: .title3)
        /// 시간 척추 머리글 — serif의 유일한 용처(§2.2.5).
        static let spine = Font.system(size: 21, weight: .semibold, design: .serif)
        /// 통계 숫자, 상세 화면 제목.
        static let display = Font.custom(sans600, size: 32, relativeTo: .largeTitle)
    }

    /// 자간. SwiftUI는 em이 아니라 pt로 받으므로 크기를 곱해 둔다.
    ///
    /// **기본값이 0이다**(§2.2.2, 2026-07-28). 예전 값들은 SF의 광학 곡선을 옮긴 것이었는데,
    /// Wanted Sans는 광학 크기 패밀리가 아니라 단일 마스터라 자체 스페이싱 위에 남의
    /// 보정을 얹으면 두 번 보정된다. 예외는 display 하나뿐이다.
    enum Tracking {
        static let label: CGFloat = 0
        static let meta: CGFloat = 0
        static let card: CGFloat = 0
        static let body: CGFloat = 0
        static let title: CGFloat = 0
        static let head: CGFloat = 0
        static let spine: CGFloat = 0
        /// 32px 통계 숫자에서만. 단일 마스터는 큰 크기에서 자간이 느슨해 보이고
        /// 이 자리는 고정폭 숫자라 그게 특히 드러난다 — 띄워서 보고 정했다.
        static let display: CGFloat = 32 * -0.012
    }
}

// MARK: - 레이아웃 상수 (§2.3 / §4.7)

extension PP {
    enum Size {
        /// 상태 레일 두께. **이 값 하나뿐이다** — 카드든 인스펙터든 같다(§4.7).
        /// R3가 "2px 세로 획"으로 고정했고, §2.1.4 채도 예산표가 유채 레일의 크기 상한을
        /// 2px으로 못 박았다. 3px·4px는 예산 위반이다. (2026-07-28까지 카드가 3px이었다.)
        static let rail: CGFloat = 2
    }
}

// MARK: - 반경 (§2.4)

extension PP {
    /// 단일 radius를 두지 않고 요소별 semantic 이름으로 고정한다.
    enum Radius {
        static let chip: CGFloat = 999
        static let control: CGFloat = 10
        static let thumb: CGFloat = 8
        static let card: CGFloat = 16
        /// 인스펙터는 카드를 확대한 것이지 다른 종류의 면이 아니라서 card와 같다.
        static let panel: CGFloat = 16
        static let sheet: CGFloat = 20
    }
}
