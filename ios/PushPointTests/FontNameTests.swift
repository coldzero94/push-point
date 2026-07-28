import CoreText
import XCTest

/// 서체가 **조용히 시스템 폰트로 떨어지는 것**을 막는다.
///
/// `Font.custom(_:size:)`는 이름을 못 찾으면 에러를 내지 않고 시스템 폰트를 쓴다.
/// 빌드도 통과하고 테스트도 통과하고 **화면만 달라진다** — `.claude/rules/ui-verification.md`가
/// 열거하는 실패 유형 그대로다.
///
/// 실제로 이 PR을 만들면서 두 번 밟았다:
///   1. 파일명(`WantedSans-Variable`)을 `Font.custom`에 넘겼다. PostScript 이름이어야 한다.
///   2. 가변 폰트에 `.weight(.semibold)`를 걸었다. SwiftUI는 커스텀 가변 폰트의 축을
///      움직여 주지 않으므로 굵기가 400 그대로 나올 수 있다 — 그래서 지금은 named
///      instance의 PostScript 이름을 직접 부른다.
///
/// 이 테스트는 **폰트 파일에 그 이름이 실재하는지**를 고정한다. `DesignSystem.swift`가
/// 앱 타깃에만 있어 여기서 `PP.Typo`를 참조할 수 없으므로 이름을 문자열로 다시 적는다 —
/// `CoverPatternTests`가 웹에서 계산한 값을 상수로 적어 두는 것과 같은 형태다.
/// **여기를 고칠 때는 `DesignSystem.swift`의 `Typo`도 같이 고친다.**
final class FontNameTests: XCTestCase {
    /// `PP.Typo`가 부르는 이름 전부. 여기 없는 이름을 Typo가 쓰면 이 테스트는 못 잡는다.
    private static let required = [
        "WantedSansVariable-Regular",   // body / meta / card
        "WantedSansVariable-Medium",    // label
        "WantedSansVariable-SemiBold",  // title / head / display
        "GeistMono-Regular",            // metaMono — R2의 기계 데이터
    ]

    private static var registered = false

    override class func setUp() {
        super.setUp()
        guard !registered else { return }
        registered = true
        // 이 번들은 앱이 아니라 단위 테스트 번들이라 UIAppFonts가 적용되지 않는다.
        // 파일을 직접 등록해서 이름을 조회한다.
        for name in ["WantedSans-Variable", "GeistMono-Variable"] {
            guard let url = Bundle(for: FontNameTests.self).url(forResource: name, withExtension: "ttf") else {
                XCTFail("폰트 파일이 테스트 번들에 없습니다: \(name).ttf")
                continue
            }
            var error: Unmanaged<CFError>?
            CTFontManagerRegisterFontsForURL(url as CFURL, .process, &error)
            if let error { XCTFail("폰트 등록 실패 \(name): \(error.takeRetainedValue())") }
        }
    }

    func testEveryNameDesignSystemUsesResolves() {
        for name in Self.required {
            let font = CTFontCreateWithName(name as CFString, 15, nil)
            let resolved = CTFontCopyPostScriptName(font) as String
            XCTAssertEqual(
                resolved, name,
                """
                '\(name)' 을(를) 찾지 못해 '\(resolved)' 로 떨어졌습니다.
                DesignSystem.swift의 Typo가 이 이름을 부르는데 폰트 파일에는 없습니다 —
                화면은 시스템 폰트로 그려지고 빌드는 통과합니다.
                """
            )
        }
    }

    /// 굵기가 실제로 다른지. 이름만 맞고 전부 같은 굵기로 그려지면 위계가 사라진다.
    func testWeightsAreActuallyDifferent() {
        func stemWidth(_ name: String) -> CGFloat {
            let f = CTFontCreateWithName(name as CFString, 100, nil)
            var glyph = CGGlyph(0)
            var ch = UniChar(72) // 'H' — 세로획이 굵기를 가장 잘 드러낸다
            CTFontGetGlyphsForCharacters(f, &ch, &glyph, 1)
            var rect = CGRect.zero
            withUnsafePointer(to: glyph) { p in
                rect = CTFontGetBoundingRectsForGlyphs(f, .horizontal, p, nil, 1)
            }
            return rect.width
        }
        let regular = stemWidth("WantedSansVariable-Regular")
        let semibold = stemWidth("WantedSansVariable-SemiBold")
        XCTAssertGreaterThan(
            semibold, regular,
            "SemiBold가 Regular보다 굵지 않습니다 — 가변 축이 안 움직였을 수 있습니다"
        )
    }
}
