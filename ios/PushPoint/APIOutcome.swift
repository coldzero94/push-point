import Foundation

/// 생성된 API Output을 "성공했는가 / 왜 실패했는가"로 접는다.
///
/// ## 왜 필요한가 (2026-07-29)
///
/// swift-openapi-generator는 **계약에 문서화된 응답을 `Output` 케이스로** 만든다. 그래서
/// `try await client.createLink(...)`는 **400·401·500에서 던지지 않는다** — 전송 실패나
/// 디코드 실패에서만 던진다. `try` 하나로 끝내면 서버가 거절한 요청이 성공으로 읽힌다.
///
/// 실제로 그렇게 되어 있었다. `SaveSheet`는 "실패하면 시트를 열어 둔 채 문구를 띄운다"고
/// 공들여 만들어 놨는데, 그 경로가 **한 번도 실행되지 않았다**: 잘못된 주소를 붙여넣으면
/// 백엔드가 400 `url must be absolute http(s)`를 주고, 그게 `.badRequest` 케이스로 와서
/// 조용히 성공 처리되고 시트가 닫혔다. `retry`와 `undo`도 같았다.
/// (`delete`만 `.noContent`를 요구해서 우연히 옳았다.)
///
/// ## 서버 문구를 그대로 보여준다
///
/// 계약의 `Error`는 `code`와 `message` 둘뿐이고(§06), `message`는 사람이 읽을 한국어다.
/// 그걸 그대로 쓰는 것이 우리가 지어내는 것보다 항상 정확하다 — "저장하지 못했습니다"보다
/// "url must be absolute http(s)"가 사용자가 고칠 것을 말해 준다.
enum APIOutcome {
    /// 실패 사유 문구. 성공이면 nil.
    typealias Failure = String?

    /// 계약의 `Error` 본문에서 message를 꺼낸다. 못 꺼내면 상태 이름으로 대체한다 —
    /// **nil을 돌려주지 않는다**: 여기서 nil은 곧 "성공"이라 실패가 성공으로 둔갑한다.
    static func message(_ body: Components.Schemas._Error?, fallback: String) -> String {
        guard let m = body?.error.message, !m.isEmpty else { return fallback }
        return m
    }
}
