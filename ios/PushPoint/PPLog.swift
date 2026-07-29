import OSLog

/// 화면이 조용히 틀렸을 때 남는 자리.
///
/// 이 프로젝트가 반복해 온 실패는 하나다 — **타입이 맞고 빌드가 되는데 화면이 틀린 것.**
/// 그중에서도 나쁜 부류는 잘못된 화면이 **정상 화면과 구별되지 않는** 경우다. 썸네일이
/// 그 대표다: 깨지면 생성 커버로 떨어지는데, 생성 커버는 썸네일 없는 링크의 정상 표시다.
///
/// 그래서 여기 남기는 것은 "오류"가 아니라 **구별 불가능해진 사건**이다. 사용자에게는
/// 아무것도 바꾸지 않고(그게 옳은 화면이다), 대신 다음 사람이 우연이 아니라 명령 한 줄로
/// 확인할 수 있게 한다:
///
/// ```
/// xcrun simctl spawn booted log stream --predicate 'subsystem == "com.pushpoint.app"'
/// ```
enum PPLog {
    private static let ui = Logger(subsystem: "com.pushpoint.app", category: "ui")

    /// 서버가 준 썸네일 URL을 못 받았다. 화면은 생성 커버라 겉으로는 정상이다.
    ///
    /// **스크롤할 때마다 다시 찍힌다.** 보드는 셀을 재활용하는 `List`라 실패한 카드가
    /// 화면 밖으로 나갔다 돌아오면 `.task`가 다시 돈다. 그래서 **줄 수는 실패 횟수가
    /// 아니라 스크롤 횟수다** — 심각도를 세는 데 쓰지 말 것.
    static func thumbFailed(_ url: URL, linkID: Int, error: Error) {
        ui.error("""
            thumb load failed link=\(linkID, privacy: .public)             url=\(url.absoluteString, privacy: .public)             err=\(String(describing: error), privacy: .public)
            """)
    }

    /// 서버는 `thumb_url`을 줬는데 클라이언트가 절대 URL을 만들지 못했다.
    ///
    /// **이 프로젝트가 실제로 출하한 썸네일 사고의 갈래다.** 네트워크 요청이 아예 나가지
    /// 않으므로 `.failure`도 안 뜨고, 화면은 생성 커버라 정상으로 보인다 — 이 줄이 없으면
    /// 어디에도 흔적이 남지 않는다.
    static func thumbUnresolvable(_ raw: String, linkID: Int) {
        ui.error("thumb url unresolvable link=\(linkID, privacy: .public) raw=\(raw, privacy: .public)")
    }
}
