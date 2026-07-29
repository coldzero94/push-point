import Foundation

/// UI 테스트가 앱에 넘기는 실행 인자. **앱 타깃과 테스트 타깃이 같은 문자열을 봐야 한다.**
///
/// 문자열을 양쪽에 따로 적어 두면 한쪽만 고쳤을 때 플래그가 조용히 안 먹고, 테스트는
/// "기능이 없다"가 아니라 "시드가 안 됐다"로 실패한다 — 둘은 화면에서 구분되지 않는다.
enum UITestModeFlags {
    /// `-uitest-dropped N` — 알림 배너를 띄운 상태로 시작한다.
    static let dropped = "-uitest-dropped"
}
