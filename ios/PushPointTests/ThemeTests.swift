import SwiftUI
import XCTest

/// 외관 선택의 저장 규칙.
///
/// 웹(`frontend/src/lib/theme.ts`)과 **같은 규칙이어야 한다**: 같은 키, 같은 기본값,
/// 그리고 `system`은 값으로 저장하지 않고 키를 지운다. 세 번째가 이 파일의 이유다 —
/// 나머지 둘은 틀리면 곧바로 눈에 띄지만, 이건 틀려도 화면이 똑같아 보인다.
@MainActor
final class ThemeTests: XCTestCase {
    private let key = "pushpoint.theme"

    override func setUp() {
        super.setUp()
        UserDefaults.standard.removeObject(forKey: key)
    }

    override func tearDown() {
        UserDefaults.standard.removeObject(forKey: key)
        super.tearDown()
    }

    /// 한 번도 고른 적 없으면 시스템이다. **이게 이 기능이 허용된 조건 자체다**(10 §1.3) —
    /// 기본값이 시스템이 아니면 HIG의 지양 취지를 어기는 것이 되고, 금지를 푼 근거가 사라진다.
    func testDefaultIsSystem() {
        XCTAssertEqual(Theme.pref, .system)
        XCTAssertNil(Theme.colorScheme(for: .system),
                     "system이 nil이 아니면 OS 설정을 앱이 덮어쓴다")
    }

    /// `system`은 **키를 지운다.** 값으로 저장하면 "한 번도 안 고름"과 "시스템을 고름"이
    /// 갈라지고, 기본값이 두 군데에 생긴다. 웹의 `setThemePref`가 `removeItem`을 부르는
    /// 것과 같은 규칙이다.
    func testChoosingSystemRemovesTheKey() {
        Theme.pref = .dark
        XCTAssertEqual(UserDefaults.standard.string(forKey: key), "dark")

        Theme.pref = .system

        XCTAssertNil(UserDefaults.standard.string(forKey: key),
                     "system을 값으로 저장했다 — 웹과 규칙이 갈라진다")
        XCTAssertEqual(Theme.pref, .system)
    }

    func testLightAndDarkRoundTrip() {
        for p in [Theme.Pref.light, .dark] {
            Theme.pref = p
            XCTAssertEqual(Theme.pref, p)
        }
        XCTAssertEqual(Theme.colorScheme(for: .light), .light)
        XCTAssertEqual(Theme.colorScheme(for: .dark), .dark)
    }

    /// 알 수 없는 값이 들어 있어도 시스템으로 떨어져야 한다 — 웹의 `getThemePref`가
    /// `light`/`dark`가 아닌 것을 전부 `system`으로 읽는 것과 같다. 키를 손으로 넣거나
    /// 예전 버전이 다른 값을 썼을 때 앱이 이상한 상태에 갇히지 않는다.
    func testUnknownStoredValueFallsBackToSystem() {
        UserDefaults.standard.set("sepia", forKey: key)
        XCTAssertEqual(Theme.pref, .system)
    }

    /// 세 상태 전부 라벨이 있어야 한다. 없으면 `t()`가 키를 그대로 돌려주므로
    /// 세그먼트에 `settings.themeLight`가 그려진다.
    func testEveryPrefHasALabel() {
        for p in Theme.Pref.allCases {
            XCTAssertFalse(Theme.label(p).contains("settings."),
                           "\(p) 라벨이 없다 — 화면에 키가 그대로 나온다")
        }
    }
}
