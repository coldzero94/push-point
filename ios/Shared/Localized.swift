import Foundation

/// 화면 문자열. 앱·공유 확장·테스트가 **같은 사전**을 쓴다(`Shared/`에 있는 이유다).
///
/// **왜 `.xcstrings`(String Catalog)가 아닌가.** 그쪽이 iOS의 정석이지만 이 저장소에서는
/// 두 가지가 걸린다. 첫째, 카탈로그는 Xcode가 관리하는 파일이라 손으로 편집하면 어긋나기
/// 쉽고 XcodeGen이 리소스를 조용히 빠뜨린 전례가 있다(project.yml의 폰트 주석). 둘째, 웹이
/// `frontend/src/lib/strings.ts`로 같은 일을 하고 **두 클라이언트를 같은 검사기로 묶고
/// 싶다** — `just ios-i18n-check`는 `web-i18n-check`와 같은 규칙을 본다. 시스템 언어를
/// 자동으로 따라가는 것과 복수형 규칙을 잃지만, 이 앱에는 복수 변화가 필요한 문장이 없고
/// 언어는 `Locale.preferredLanguages`로 충분히 고른다.
enum L {
    enum Lang: String { case ko, en }

    private static let key = "pushpoint.lang"

    /// 저장된 선택 → 시스템 선호 언어 → 한국어. 웹의 `detect()`와 같은 순서다.
    static var current: Lang = {
        if let raw = UserDefaults.standard.string(forKey: key), let l = Lang(rawValue: raw) {
            return l
        }
        let pref = Locale.preferredLanguages.first ?? "ko"
        return pref.hasPrefix("ko") ? .ko : .en
    }()

    static func set(_ lang: Lang) {
        current = lang
        UserDefaults.standard.set(lang.rawValue, forKey: key)
        // `set`은 어디서든 불릴 수 있지만(테스트가 setUp에서 부른다) Store는 화면용이라
        // MainActor에 있다. 뷰가 없는 곳에서 부른 경우에도 안전하게 건너뛰도록 감싼다.
        Task { @MainActor in Store.shared.lang = lang }
    }

    /// 화면이 언어 변경을 알아채는 통로.
    ///
    /// `current`는 `static var`라 바뀌어도 SwiftUI가 다시 그리지 않는다. 뷰가 이걸
    /// 구독하고 루트에 `.id(lang)`을 걸어야 화면 전체가 새 언어로 다시 그려진다 —
    /// 문자열은 뷰 본문에서 `t()`로 읽히므로 부분 갱신으로는 섞인 화면이 남는다.
    @MainActor
    final class Store: ObservableObject {
        static let shared = Store()
        @Published var lang: Lang = L.current
    }

    /// 문자열을 찾는다. `{이름}` 자리를 `params`로 채운다.
    ///
    /// 없는 키는 **키 자체를 돌려준다.** 화면에 `list.empty`가 보이면 버그 신고지만 빈 줄은
    /// 아무 말도 하지 않는다 — 웹의 `t()`와 같은 판단이다.
    static func t(_ key: String, _ params: [String: CustomStringConvertible] = [:]) -> String {
        guard var value: String = Strings.table[current.rawValue]?[key] else {
            assertionFailure("없는 문자열 키: \(key)")
            return key
        }
        // 영어 복수는 **값** 쪽에 `단수|복수`로 들어 있다 — 웹의 `t()`와 같은 규약이다.
        // 한국어는 복수 일치가 없어 대개 `|`가 없고, 그래서 복수를 키가 아니라 값의 성질로
        // 두었다. 이걸 iOS에 안 넣었더니 웹이 "1 day saved"를 낼 자리에서 iOS는
        // "1 days saved"를 냈다 — 같은 사전을 쓰기로 한 이상 가르는 규칙도 같아야 한다.
        if value.contains("|") {
            let parts = value.split(separator: "|", maxSplits: 1, omittingEmptySubsequences: false)
            let isOne = (params["count"]?.description == "1")
            value = String(isOne ? parts[0] : parts[1])
        }
        guard !params.isEmpty else { return value }
        var out = value
        for (name, v) in params {
            out = out.replacingOccurrences(of: "{\(name)}", with: v.description)
        }
        return out
    }
}

/// 짧게 쓰기 위한 자유 함수. 뷰 코드가 `L.t(...)`로 도배되는 것보다 읽기 쉽다.
func t(_ key: String, _ params: [String: CustomStringConvertible] = [:]) -> String {
    L.t(key, params)
}
