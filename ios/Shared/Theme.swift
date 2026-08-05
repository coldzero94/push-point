import SwiftUI

/// 외관 선택 — 라이트 / 다크 / 시스템.
///
/// **기본값이 `system`인 것이 이 기능의 근거 전체다.** 10 §8.5는 원래 iOS에 이 손잡이를
/// 금지했고 근거는 "HIG가 앱별 외관 설정을 지양한다"였다. 그런데 그 지양의 이유는 **시스템
/// 설정이 이미 그 일을 하기 때문**이므로, 기본값이 시스템을 따르는 한 취지는 그대로 지켜진다 —
/// 선택은 그 위에 얹히는 것이지 대체하는 것이 아니다. 2026-08-05에 그렇게 풀렸다(10 §1.3).
///
/// 허용 범위는 **이 3-state 하나뿐**이다. 프리셋 갤러리는 계속 금지인데, 둘은 다른 것이다:
/// 3-state는 OS가 이미 아는 축 위의 선택이고 프리셋은 새 축을 만드는 일이다.
enum Theme {
    /// 웹과 **같은 키**여야 한다. 같은 이름을 쓰는 것에 기능적 이유는 없지만(저장소가
    /// 다르다), 두 클라이언트가 같은 개념을 다른 이름으로 부르기 시작하면 그 다음 사람이
    /// 둘을 같은 것으로 못 읽는다.
    private static let key = "pushpoint.theme"

    enum Pref: String, CaseIterable { case light, dark, system }

    /// 저장된 선택 → `system`. **`system`은 값으로 저장하지 않고 키를 지운다** —
    /// 웹의 `setThemePref`가 그렇게 하고(`localStorage.removeItem`), 그래야 "한 번도 고른
    /// 적 없음"과 "시스템을 골랐음"이 같은 상태가 된다. 둘을 구분해 봐야 쓸 데가 없는데
    /// 구분해 두면 기본값이 두 군데에 생긴다.
    static var pref: Pref {
        get {
            guard let raw = UserDefaults.standard.string(forKey: key),
                  let p = Pref(rawValue: raw), p != .system else { return .system }
            return p
        }
        set {
            if newValue == .system {
                UserDefaults.standard.removeObject(forKey: key)
            } else {
                UserDefaults.standard.set(newValue.rawValue, forKey: key)
            }
            Task { @MainActor in Store.shared.pref = newValue }
        }
    }

    /// SwiftUI에 넘길 값. **`system`은 `nil`이다** — `.preferredColorScheme(nil)`이
    /// "관여하지 않는다"는 뜻이라, OS를 따라가는 동작을 우리가 흉내 낼 필요가 없다.
    /// 웹은 `<html>`에 항상 해석된 클래스를 찍어야 해서 직접 계산하지만(CSS가 그렇게
    /// 동작한다), 여기서는 플랫폼이 그 일을 한다.
    static func colorScheme(for pref: Pref) -> ColorScheme? {
        switch pref {
        case .light: .light
        case .dark: .dark
        case .system: nil
        }
    }

    /// 화면이 선택 변경을 알아채는 통로. `L.Store`와 같은 형태다 — `static var`가 바뀌는
    /// 것만으로는 SwiftUI가 다시 그리지 않는다.
    @MainActor
    final class Store: ObservableObject {
        static let shared = Store()
        @Published var pref: Pref = Theme.pref
    }

    /// 선택한 외관을 이 계층에 적용한다.
    ///
    /// **시트마다 따로 걸어야 한다.** 루트에만 걸면 시트가 뜬 채로 값을 바꿨을 때 시트가
    /// 따라오지 않는다 — `system → dark`는 따라오는데 `dark → light`는 안 따라온다.
    /// 화면에서 그대로 나왔다: 세그먼트는 `Light`로 옮겨졌고, 닫고 나온 목록은 라이트였고,
    /// **떠 있는 시트만 어두운 채로 남았다.** `.sheet`는 별도 표현 컨텍스트라 루트의
    /// 선언이 갱신 시점에 다시 내려가지 않는다.
    ///
    /// 그래서 수식자로 만들었다. 시트가 셋이고 앞으로 늘 텐데, 한 곳을 빠뜨리면 그
    /// 화면만 다른 테마로 뜨고 그건 조용한 결함이다.
    struct Applying: ViewModifier {
        @ObservedObject private var store = Store.shared
        func body(content: Content) -> some View {
            content.preferredColorScheme(Theme.colorScheme(for: store.pref))
        }
    }

    /// 라벨. 세그먼트에만 쓴다.
    static func label(_ pref: Pref) -> String {
        switch pref {
        case .light: t("settings.themeLight")
        case .dark: t("settings.themeDark")
        case .system: t("settings.themeSystem")
        }
    }
}

extension View {
    /// 선택한 외관을 적용한다. **루트와 모든 시트에 건다** — 이유는 `Theme.Applying`.
    func pushPointTheme() -> some View { modifier(Theme.Applying()) }
}
