import Foundation

/// 상태 레일이 **색 말고 무엇으로 말하는가**.
///
/// §4.7과 §7.1이 색 단독 표현을 금지하므로 모든 비-done 상태에는 숨은 텍스트가 따라붙는다.
/// 그 텍스트를 정하는 규칙이 여기 있다.
///
/// ## 왜 뷰 밖인가 (2026-07-30)
///
/// 이건 웹 `frontend/src/lib/statusAnnounce.ts`와 **글자까지 같아야 하는 규칙**인데
/// (13 §3 · §8.1), 뷰의 `private var`로 두면 테스트가 부를 수 없다. 그래서 그 일치 검사는
/// **"소스 파일에 문자열이 실재하는가"** 라는 약한 형태였다 — 판정표의 네 규칙 중 유일하게
/// 함수를 안 부르는 것이었고, 그 표에 그렇게 적혀 있었다.
///
/// 그리고 이 값은 화면으로도 확인할 수 없다. `accessibilityLabel`은 스크린샷에 안 찍히고,
/// 레일은 색과 펄스를 그대로 두고 **말만** 바꾸기 때문이다. 볼 수도 없고 부를 수도 없으면
/// 검사할 방법이 없다.
enum StatusAnnounce {
    /// 웹 `STATUS_LABEL`과 같은 단어. `testdata/status-labels.json`이 둘을 묶는다.
    // 다섯 갈래를 편 채로 둔다. `t("status.\(status.rawValue)")`로 조립하면
    // `scripts/ios_i18n_check.py`가 호출을 못 보고 다섯 키를 전부 "아무도 안 쓰는 키"로
    // 잡는다 — 웹 `statusAnnounce.ts`가 같은 이유로 같은 모양이다.
    static func label(_ status: Components.Schemas.LinkStatus) -> String {
        switch status {
        case .pending: t("status.pending")
        case .scraping: t("status.scraping")
        case .tagging: t("status.tagging")
        case .done: t("status.done")
        case .failed: t("status.failed")
        }
    }

    /// 레일이 읽어 줄 문장.
    ///
    /// `retryWaiting`이 **status보다 우선한다**: 백오프로 누워 있는 링크는 `status`가 여전히
    /// `pending`이라 "대기"로 읽히는데, 그건 큐에서 순서를 기다리는 것과 구분되지 않는다.
    /// 실제로는 실패해서 30×attempts초를 세는 중이고, 그 둘은 사용자에게 다른 일이다.
    static func announcement(_ status: Components.Schemas.LinkStatus,
                             retryWaiting: Bool) -> String {
        retryWaiting ? t("status.retryWaiting") : label(status)
    }
}
