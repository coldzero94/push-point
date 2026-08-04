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
    /// 한 페이지(50건)를 넘기는 분량을 심는다. 커서 페이지네이션은 **경계를 넘겨야만**
    /// 검증되는 종류라, 픽스처 3건으로는 무한 스크롤이 깨져 있어도 화면이 멀쩡해 보인다.
    /// 기본 픽스처와 분리한 이유는 속도다 — 60건 심기를 매 테스트가 치를 이유가 없다.
    static let manyFlag = "-uitest-many"

    static var isActive: Bool {
        ProcessInfo.processInfo.arguments.contains(flag)
    }

    static var wantsMany: Bool {
        ProcessInfo.processInfo.arguments.contains(manyFlag)
    }

    /// 페이지 경계를 넘기는 건수. limit=50이므로 60이면 두 번째 장이 반드시 생긴다.
    static let manyCount = 60

    /// 공유 defaults에서 **테스트가 읽는 값들**을 실행 상태로 되돌린다.
    ///
    /// `dataDirectory()`가 임시 디렉터리를 비우므로 링크는 격리되는데, **공유
    /// `UserDefaults`는 격리되지 않았다.** `-uitest`로 띄워도 App Group entitlement는
    /// 그대로라 `UserDefaults(suiteName:)`가 **진짜 스위트**를 돌려주고, 그 안의
    /// `droppedNotices`가 0보다 크면 화면 맨 위에 알림 배너가 뜬다 — 모든 테스트의
    /// 레이아웃이 그만큼 밀린다.
    ///
    /// 그래서 `BoardUITests`가 스스로 적어 둔 "임시 디렉터리 + 자체 픽스처라 시뮬레이터
    /// 상태에 무관하다"가 이 값 하나에서 거짓이었다. 2026-07-29에 `ios-uitest`가 한 번
    /// 실패하고 이후 연속 통과했는데, 그때 이 값이 3이었다 — 손으로 배너를 시험하며
    /// 남긴 값이다. 원인으로 확정하지는 못했지만, 확정할 수 없다는 것 자체가 문제다.
    ///
    /// `seedDroppedFlag`가 있으면 그 값을 심는다 — 배너 자체를 검증하려면 필요하다.
    static func resetSharedDefaults() {
        // **표준 defaults도 격리한다.** `@AppStorage`는 App Group이 아니라 표준 스위트를
        // 쓰므로 `AppGroup.defaults`만 비우면 남는다. 목록 밀도가 그 예다 — 손으로 조밀
        // 모드를 켜 두면 다음 UI 테스트가 그 상태로 시작하고, 밀도를 토글하는 테스트는
        // 방향이 반대가 되어 실패한다. 실제로 그렇게 실패했다(2026-07-30).
        //
        // `droppedNotices`와 같은 부류이고 키만 다르다. 격리를 키마다 따로 기억해야 하는
        // 구조 자체가 위험이라, 새 `@AppStorage`를 추가하면 여기도 함께 늘려야 한다.
        UserDefaults.standard.removeObject(forKey: "pushpoint.density")
        // **언어도 지운다.** `L.set`이 같은 suite에 쓰므로, 지우지 않으면 앱에서 마지막에
        // 고른 언어로 테스트가 뜬다 — 실행마다 화면 언어가 달라지는 스위트가 된다.
        // 지금은 어느 단정도 문구를 읽지 않아 결과가 흔들리진 않지만, 비결정성을 남겨 두면
        // 다음에 문구를 읽는 케이스가 생겼을 때 원인을 찾는 데 시간이 든다.
        UserDefaults.standard.removeObject(forKey: "pushpoint.lang")

        guard let d = AppGroup.defaults else { return }
        let seeded = ProcessInfo.processInfo.arguments
            .firstIndex(of: seedDroppedFlag)
            .flatMap { i -> Int? in
                let next = i + 1
                return next < ProcessInfo.processInfo.arguments.count
                    ? Int(ProcessInfo.processInfo.arguments[next]) : nil
            }
        d.set(seeded ?? 0, forKey: SaveNotifier.droppedKey)
    }

    /// `-uitest-dropped N` — 알림 배너를 띄운 상태를 만든다.
    static let seedDroppedFlag = "-uitest-dropped"

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
    ///
    /// **순서가 곧 계약이다.** `dataDirectory()`가 매 실행 DB를 지우고 아래 순서대로
    /// 한 건씩 POST하므로 id가 1부터 이 배열 순서로 붙는다. UI 테스트는 카드를 표시
    /// 문구가 아니라 `link.card.<id>`로 겨냥하므로(§ui-verification), 이 배열의 순서를
    /// 바꾸면 `BoardUITests.Fixture`도 함께 고쳐야 한다.
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
        if wantsMany {
            await seedMany(using: client)
            return
        }
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

    /// 페이지 경계 검증용 대량 픽스처.
    ///
    /// 제목에 **순번을 박는다.** "많이 보인다"로는 두 번째 장이 왔는지 알 수 없고,
    /// 1장에 절대 없는 번호가 보이는지로만 판정할 수 있다. 본문을 넣지 않아 태깅·요약이
    /// 돌지 않으므로 60건이 빠르게 들어간다.
    ///
    /// 저장 순서 = created_at 순서이고 목록은 최신순이므로, **먼저 넣은 것이 아래로 간다.**
    /// 그래서 000번이 목록의 맨 끝이고, 그게 곧 "끝까지 스크롤했다"의 표식이다.
    static func seedMany(using client: Client) async {
        for i in 0 ..< manyCount {
            let n = String(format: "%03d", i)
            _ = try? await client.createLink(.init(body: .json(.init(
                url: "https://uitest.example/many/\(n)",
                title: "페이지 검증 링크 \(n)"
            ))))
        }
    }
}
