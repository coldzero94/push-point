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
    static func thumbFailed(_ url: URL, linkID: Int) {
        ui.error("thumb load failed link=\(linkID, privacy: .public) url=\(url.absoluteString, privacy: .public)")
    }
}
