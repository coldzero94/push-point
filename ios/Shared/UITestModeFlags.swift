import Foundation

/// UI 테스트가 앱에 넘기는 실행 인자. **앱 타깃과 테스트 타깃이 같은 문자열을 봐야 한다.**
///
/// 문자열을 양쪽에 따로 적어 두면 한쪽만 고쳤을 때 플래그가 조용히 안 먹고, 테스트는
/// "기능이 없다"가 아니라 "시드가 안 됐다"로 실패한다 — 둘은 화면에서 구분되지 않는다.
enum UITestModeFlags {
    /// `-uitest` — 하네스 자체를 켠다.
    static let uitest = "-uitest"

    /// 지금 UI 테스트로 떠 있는가. **`Shared/`에 두는 이유는 타깃 때문이다** —
    /// `FeedModel`이 앱과 단위 테스트 양쪽에 컴파일되는데 `UITestMode`는 앱 타깃에만
    /// 있어서, 모델이 그걸 물으면 단위 테스트가 컴파일되지 않는다.
    static var isActive: Bool {
        ProcessInfo.processInfo.arguments.contains(uitest)
    }

    /// `-uitest-dropped N` — 알림 배너를 띄운 상태로 시작한다.
    static let dropped = "-uitest-dropped"

    /// `-uitest-resurface N` — N번째 픽스처를 "오늘의 한 건"으로 세운다.
    ///
    /// 실제 시간으로는 못 만드는 상태다(7일 임계 + 서버가 `created_at`을 박는다).
    /// 자세한 이유와 이 경로가 **덮지 않는 것**은 `UITestMode.seedResurfaceFlag`.
    static let resurface = "-uitest-resurface"
}
