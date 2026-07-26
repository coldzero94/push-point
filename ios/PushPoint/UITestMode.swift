import Foundation

/// UI 테스트 전용 하네스.
///
/// **왜 앱 안에 테스트용 코드를 두는가.** XCUITest는 앱을 바깥에서 조작할 뿐 앱의 데이터에
/// 손을 댈 수 없다. 그런데 이 앱의 데이터는 App Group 안의 SQLite이고, 넣는 경로는 공유
/// 시트뿐이다 — 즉 테스트가 스스로 상태를 만들 방법이 없다. 그러면 테스트는 "시뮬레이터에
/// 마침 들어 있던 링크"에 기대게 되는데, 그건 어제는 통과하고 오늘은 실패하는 테스트다.
///
/// 그래서 실행 인자 하나로만 열리는 문을 둔다. 두 가지를 지킨다:
///
/// 1. **사용자 데이터를 절대 건드리지 않는다.** App Group이 아니라 임시 디렉터리를 쓰고,
///    시작할 때 지운다. 테스트가 실제 아카이브를 지우는 일은 구조적으로 불가능하다.
/// 2. **시딩도 실제 경로로 한다.** 픽스처를 SQLite에 직접 쓰지 않고 앱이 자기 서버에
///    `POST /api/v1/links`로 넣는다 — 저장 계약이 깨지면 테스트 준비 단계에서 먼저 터진다.
///
/// 인자는 XCUITest의 `launchArguments`로만 들어온다. 사용자가 앱 아이콘을 눌러 이 상태에
/// 들어갈 방법은 없다.
enum UITestMode {
    static let flag = "-uitest"

    static var isActive: Bool {
        ProcessInfo.processInfo.arguments.contains(flag)
    }

    /// 테스트용 데이터 디렉터리. 매 실행 비운다 — 이전 실행이 남긴 링크가 보이면
    /// "N건이 보여야 한다"는 단언이 실행 순서에 따라 흔들린다.
    static func dataDirectory() -> URL {
        let dir = URL(fileURLWithPath: NSTemporaryDirectory())
            .appendingPathComponent("pushpoint-uitest", isDirectory: true)
        try? FileManager.default.removeItem(at: dir)
        return dir
    }

    /// 화면을 검증하기에 충분한 최소 픽스처.
    ///
    /// 일부러 서로 다르게 만든다: 태그가 붙는 것과 안 붙는 것, 한국어와 영어, 메모가
    /// 있는 것과 없는 것. 전부 같은 모양이면 화면이 무엇을 잘못 그려도 테스트가 못 잡는다.
    static let fixtures: [(url: String, title: String, description: String, body: String, keywords: String)] = [
        ("https://uitest.example/kube",
         "쿠버네티스 프로덕션 운영 가이드",
         "클러스터를 실제로 굴리며 배운 것들",
         "쿠버네티스 클러스터를 프로덕션에서 운영하는 일은 설치와 전혀 다르다. 노드가 죽고 파드가 밀린다. 도커 이미지 크기부터 줄여야 한다.",
         "kubernetes, devops"),
        ("https://uitest.example/swift",
         "Swift Concurrency 정리",
         "async/await와 액터를 처음부터",
         "Swift의 동시성 모델은 async await와 actor 두 축이다. 개발자는 스레드를 직접 다루지 않는다.",
         "swift, 개발"),
        ("https://uitest.example/plain",
         "Notes on a quiet afternoon",
         "",
         "There was nothing much to say about it, and that was rather the point of the whole thing.",
         ""),
    ]

    /// 픽스처를 앱 자신의 서버에 넣는다. 태깅은 비동기라 여기서 기다리지 않는다 —
    /// 태그가 붙었는지 보는 테스트는 자기 단언에서 기다리는 편이 정확하다.
    static func seed(using client: Client) async {
        for f in fixtures {
            _ = try? await client.createLink(.init(body: .json(.init(
                url: f.url,
                title: f.title,
                description: f.description,
                body_text: f.body,
                keywords: f.keywords
            ))))
        }
    }
}
