import Foundation

/// App Group 컨테이너 — 본체 앱과 Share Extension이 **같은 SQLite**를 보게 하는 유일한 통로다.
/// 두 프로세스가 서로 다른 경로를 열면 확장이 저장한 링크가 앱에서 영원히 보이지 않으므로,
/// 경로를 만드는 자리는 여기 하나뿐이다.
enum AppGroup {
    /// Info.plist가 아니라 코드 상수로 두는 이유: 앱과 확장이 같은 값을 써야 하는데
    /// 두 타깃의 plist에 각각 적으면 갈라져도 빌드가 통과한다.
    static let identifier = "group.com.pushpoint.shared"

    /// 공유 컨테이너 안의 데이터 디렉터리. `store.Open`이 여기에 pushpoint.db를 만든다.
    ///
    /// 컨테이너를 못 얻으면 nil이다 — App Group entitlement가 없거나(무료 개인 팀에서
    /// 발생할 수 있다) 프로비저닝이 어긋난 경우다. 그 상황을 앱 전용 디렉터리로 조용히
    /// 대체하면 확장과 앱이 서로 다른 DB를 쓰면서 "저장은 되는데 목록에 없다"는
    /// 진단하기 어려운 상태가 되므로, 폴백하지 않고 nil로 드러낸다.
    static func dataDirectory() -> URL? {
        guard let container = FileManager.default
            .containerURL(forSecurityApplicationGroupIdentifier: identifier)
        else { return nil }
        return container.appendingPathComponent("data", isDirectory: true)
    }
}
