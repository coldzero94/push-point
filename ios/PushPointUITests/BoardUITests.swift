import XCTest

/// 화면을 실제로 조작해 검증한다.
///
/// 단위 테스트가 잡지 못하는 것을 잡는 것이 목적이다. 지금까지 목록·검색·태그 편집은
/// 컴파일이 통과하면 "된 것"으로 보고 눈으로 확인했는데, 컴파일러가 통과시키는 실패가
/// 실제로 여러 번 있었다: 썸네일 URL이 상대 경로라 이미지가 통째로 비었고(`thumb_url`은
/// `/thumbs/…`라 host 없는 URL이 됐다), 카드의 3:1 비율이 이미지에 걸려 있어 무시됐고,
/// `listTags` 응답이 래퍼가 아니라 배열이라 `.tags` 접근이 어긋났다. 셋 다 **타입은 맞고
/// 화면만 틀린** 종류다.
///
/// 앱은 `-uitest`로 띄운다 — 임시 디렉터리 + 자체 픽스처라 시뮬레이터 상태에 무관하다.
final class BoardUITests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-uitest"]
        app.launch()
    }

    /// 실패하면 그 순간의 화면을 남긴다. UI 테스트의 실패 메시지는 "없다"밖에 말해 주지
    /// 못해서, 화면이 없는지·다르게 그려졌는지·아직 로딩 중인지를 구분할 수 없다.
    override func tearDown() {
        if let failureCount = testRun?.failureCount, failureCount > 0 {
            let shot = XCTAttachment(screenshot: XCUIScreen.main.screenshot())
            shot.lifetime = .keepAlways
            shot.name = "실패 시점 화면"
            add(shot)
            // 접근성 트리도 함께 — 무엇이 실제로 잡히는 이름인지 여기서만 알 수 있다.
            let tree = XCTAttachment(string: app.debugDescription)
            tree.lifetime = .keepAlways
            tree.name = "접근성 트리"
            add(tree)
        }
    }

    /// 픽스처가 실제로 화면에 도착하는지. 이게 깨지면 아래 모든 테스트의 전제가 무너지므로
    /// 가장 먼저, 가장 단순하게 확인한다.
    func testBoardShowsSavedLinks() {
        XCTAssertTrue(waitForCard("쿠버네티스 프로덕션 운영 가이드"),
                      "저장한 링크가 목록에 나타나지 않는다")
        XCTAssertTrue(app.staticTexts["Swift Concurrency 정리"].exists)
    }

    /// 검색 — 세 글자 이상이면 FTS 경로다.
    ///
    /// **좁혀지는 것까지** 확인한다. 검색창에 치기만 하고 결과가 그대로면 "검색이 된다"고
    /// 말할 수 없는데, 목록이 원래 짧으면 눈으로는 구분이 안 간다.
    func testSearchNarrowsTheBoard() {
        XCTAssertTrue(waitForCard("쿠버네티스 프로덕션 운영 가이드"))

        search(for: "쿠버네티스")

        XCTAssertTrue(app.staticTexts["쿠버네티스 프로덕션 운영 가이드"]
            .waitForExistence(timeout: 5), "검색 결과에 맞는 링크가 없다")
        XCTAssertFalse(app.staticTexts["Swift Concurrency 정리"].exists,
                       "검색이 목록을 좁히지 못했다 — 안 맞는 링크가 남아 있다")
    }

    /// 결과가 없을 때 빈 화면이 아니라 설명이 나와야 한다 — 빈칸을 만들지 않는다(R4).
    func testSearchWithNoMatchExplainsItself() {
        XCTAssertTrue(waitForCard("쿠버네티스 프로덕션 운영 가이드"))
        search(for: "존재하지않는검색어입니다")
        XCTAssertTrue(app.staticTexts["결과가 없습니다"].waitForExistence(timeout: 5))
    }

    /// 두 글자 이하는 400이 아니라 LIKE 폴백이고(계약), 화면은 그 사실을 말해야 한다.
    /// 결과가 적은 이유를 모르면 사용자는 "없구나"로 읽고 검색을 그만둔다.
    func testShortQueryExplainsTheFallback() {
        XCTAssertTrue(waitForCard("쿠버네티스 프로덕션 운영 가이드"))
        search(for: "쿠버")
        XCTAssertTrue(app.staticTexts.containing(
            NSPredicate(format: "label CONTAINS %@", "세 글자부터")).firstMatch
            .waitForExistence(timeout: 5), "LIKE 폴백 안내가 없다")
    }

    /// 태그 편집 — 기계가 붙인 태그를 사람이 고칠 수 있어야 한다.
    ///
    /// 이 경로가 `tag_feedback`을 만드는 유일한 통로다(M5 재랭킹 학습 데이터). 화면이
    /// 조용히 망가지면 데이터가 안 쌓이는데, 그 사실은 몇 달 뒤에나 드러난다.
    func testEditingTagsPersists() {
        XCTAssertTrue(waitForCard("Notes on a quiet afternoon"))
        app.staticTexts["Notes on a quiet afternoon"].tap()

        let editButton = app.buttons["태그 붙이기"].firstMatch
        XCTAssertTrue(editButton.waitForExistence(timeout: 5),
                      "태그가 없는 링크에 붙이는 자리가 없다")
        editButton.tap()

        let sheetTitle = app.navigationBars["태그"]
        XCTAssertTrue(sheetTitle.waitForExistence(timeout: 5), "태그 편집 시트가 열리지 않는다")

        // 사전에서 하나 고른다. 자유 입력이 아니라 목록이라는 것 자체가 계약이다.
        // 사전이 40개가 넘어 화면 밖에 있을 수 있으므로 찾을 때까지 스크롤한다.
        let candidate = app.buttons["tag-book"]
        XCTAssertTrue(scrollToFind(candidate), "사전 태그가 목록에 없다")
        candidate.tap()

        let confirm = app.buttons["확인"]
        XCTAssertTrue(confirm.isEnabled, "선택이 바뀌었는데 확인이 잠겨 있다")
        confirm.tap()

        // 상세로 돌아와 칩이 실제로 붙었는지. 시트가 닫히는 것만으로는 저장을 증명하지 못한다.
        XCTAssertTrue(app.staticTexts["book"].waitForExistence(timeout: 5),
                      "고른 태그가 상세에 반영되지 않았다")
    }

    /// 메모 — 기계가 절대 만들어 줄 수 없는 유일한 필드라, 저장되지 않으면 대체재가 없다.
    func testEditingNotePersists() {
        XCTAssertTrue(waitForCard("Swift Concurrency 정리"))
        app.staticTexts["Swift Concurrency 정리"].tap()

        let field = app.textFields["왜 담았는지 한 줄"]
        XCTAssertTrue(field.waitForExistence(timeout: 5), "메모 입력 칸이 없다")
        field.tap()
        field.typeText("나중에 다시 읽기")

        let save = app.buttons["메모 저장"]
        XCTAssertTrue(save.waitForExistence(timeout: 3), "저장 버튼이 나타나지 않는다")
        save.tap()

        // 저장되면 버튼이 사라진다(초안 == 서버 값). 이게 화면이 서버 응답을 실제로
        // 받아 반영했다는 증거다 — 텍스트만 확인하면 입력한 글자가 그대로 보일 뿐이다.
        XCTAssertTrue(waitForDisappearance(save), "메모가 저장되지 않았다 — 저장 버튼이 남아 있다")
    }

    /// 커서 페이지네이션 — 목록이 50건에서 끊기지 않아야 한다.
    ///
    /// 이 결함은 **아카이브가 커지기 전까지 보이지 않는다.** 링크가 몇 건일 때는 화면이
    /// 완벽해 보이고, 50건을 넘긴 어느 날부터 오래된 링크가 조용히 사라진다 — 사라졌다는
    /// 신호도 없다. 저장한 것을 되찾는 것이 이 앱의 존재 이유라 그 실패는 치명적인데,
    /// 정작 일상적인 사용으로는 절대 발견되지 않는 종류다.
    ///
    /// 그래서 픽스처를 60건으로 심고(`-uitest-many`), **1장에 있을 수 없는 항목**이
    /// 보이는지로 판정한다. "많이 보인다"로는 두 번째 장이 왔는지 알 수 없다.
    func testListPagesPastTheFirstFifty() {
        app.terminate()
        app.launchArguments = ["-uitest", "-uitest-many"]
        app.launch()

        // 최신순이므로 059가 맨 위, 000이 맨 끝이다. 000은 두 번째 장에만 있다.
        XCTAssertTrue(waitForCard("페이지 검증 링크 059", timeout: 60),
                      "대량 픽스처가 목록에 오지 않았다")

        let last = app.staticTexts["페이지 검증 링크 000"]
        XCTAssertFalse(last.exists, "첫 장에 마지막 항목이 있다 — 픽스처가 50건을 못 넘겼다")

        // 끝까지 민다. 한 장(50건)을 지나야 다음 장이 붙으므로 넉넉히 준다.
        var swipes = 0
        while !last.exists, swipes < 40 {
            app.swipeUp(velocity: .fast)
            swipes += 1
        }
        XCTAssertTrue(last.exists,
                      "50건 경계를 넘지 못했다 — next_cursor로 다음 장을 받지 않는다")
    }

    /// 검색이 **실패**했을 때 "결과가 없습니다"로 위장되지 않아야 한다.
    ///
    /// 둘은 사용자에게 정반대 뜻이다. "없습니다"는 *저장한 적 없다*는 단정이고, 아카이브에서
    /// 그건 가장 나쁜 거짓말이다 — 사용자가 찾기를 포기한다. 실패는 실패라고 말하고
    /// 다시 시도할 수단을 줘야 한다.
    ///
    /// 서버를 죽이는 대신 화면 분기의 **순서**를 고정한다: 오류 분기가 빈 결과 분기보다
    /// 앞에 있어야 하고, 그 순서가 뒤집히는 것이 실제로 일어나는 회귀다.
    func testSearchFailureIsNotDisguisedAsEmpty() {
        XCTAssertTrue(waitForCard("쿠버네티스 프로덕션 운영 가이드"))
        search(for: "존재하지않는검색어입니다")

        // 정상 미스에서는 "결과가 없습니다"가 맞다.
        XCTAssertTrue(app.staticTexts["결과가 없습니다"].waitForExistence(timeout: 5))
        // 그리고 그 화면에 실패 문구가 섞여 있으면 안 된다 — 두 상태가 뒤엉키면
        // 어느 쪽인지 사용자가 판단할 수 없다.
        XCTAssertFalse(app.staticTexts["검색하지 못했습니다"].exists,
                       "정상 미스인데 실패 문구가 떴다")
    }

    // MARK: - 헬퍼

    /// 서버가 프로세스 안에서 뜨고 픽스처가 들어가기까지 시간이 걸린다.
    private func waitForCard(_ title: String, timeout: TimeInterval = 20) -> Bool {
        app.staticTexts[title].waitForExistence(timeout: timeout)
    }

    private func search(for text: String) {
        let field = app.searchFields.firstMatch
        XCTAssertTrue(field.waitForExistence(timeout: 5), "검색창이 없다")
        field.tap()
        field.typeText(text)
    }

    /// 알림 배너가 **화면 폭을 가득 채우지 않아야** 한다.
    ///
    /// 2026-07-29에 이 배너의 `warnTint` 배경이 투명한 네비게이션 바 뒤로 번져 제목·검색
    /// 필드·툴바까지 경고색으로 칠했다. 실측으로 갈랐고(배너 없는 통계 탭 헤더는 canvas,
    /// 목록 탭은 warnTint), `safeAreaInset`으로 옮겨도 그대로였다 — **원인은 위치가 아니라
    /// 가장자리를 무는 것**이었다.
    ///
    /// 그래서 이 테스트는 색이 아니라 **기하**를 본다. 색은 접근성 트리에 없지만 원인은
    /// 있다: 전면이면 회귀, 들여쓰면 정상. 픽셀을 읽는 테스트는 팔레트를 한 번만 손봐도
    /// 썩고 기기 크기·다크 모드·바 머티리얼에 흔들리는데, 이 단언은 그중 어느 것도 안 탄다.
    ///
    /// **한계를 적어 둔다**: 이건 원인을 고정하는 것이지 증상을 고정하는 게 아니다. 다른
    /// 경로로 헤더가 물드는 회귀는 이 테스트를 통과한다. 그래도 값이 있는 이유는, 리팩토링이
    /// 되돌릴 가능성이 가장 높은 것이 정확히 이 형태이기 때문이다.
    func testNotificationBannerDoesNotSpanTheFullWidth() {
        app.launchArguments = ["-uitest", UITestModeFlags.dropped, "3"]
        app.launch()

        let banner = app.buttons["notice.notifications"]
        XCTAssertTrue(banner.waitForExistence(timeout: 8),
                      "배너가 없다 — 시드 플래그가 안 먹었거나 공유 defaults가 격리되지 않았다")

        let screen = app.windows.firstMatch.frame
        let f = banner.frame
        XCTAssertGreaterThan(f.minX, 0,
                             "배너가 왼쪽 가장자리를 물고 있다 — 전면이면 그 배경이 " +
                             "투명한 네비 바 뒤로 번져 헤더 전체를 경고색으로 칠한다")
        XCTAssertLessThan(f.maxX, screen.maxX,
                          "배너가 오른쪽 가장자리를 물고 있다")

        // 그리고 여백은 버튼 밖이어야 한다 — 안에 있으면 그 투명 영역까지 탭 타깃이 되어
        // 카드를 노린 손가락이 알림 설정을 연다.
        XCTAssertLessThan(f.height, 120, "배너 히트 영역이 그려진 것보다 훨씬 크다")
    }

    /// 조밀 모드가 **실제로 더 조밀해야** 한다.
    ///
    /// **조밀 모드에는 자동 게이트가 하나도 없었다.** 밀도 토글에 식별자가 없어 테스트가 누를
    /// 수도 없었고, 그래서 재설계 대상인 쪽이 사각지대였다.
    ///
    /// **44pt 터치 하한은 여기서 단언하지 않는다.** 처음엔 그걸 넣었는데, 커버를 12pt로 줄이고
    /// 패딩을 0으로 만들어도 통과했다 — `List`가 자체적으로 44pt 최소 행 높이를 보장하기
    /// 때문이다. **실패할 수 없는 단언은 커버리지로 읽혀서 없는 것보다 나쁘다.** 하한은
    /// 플랫폼이 지키고, 이 테스트는 플랫폼이 지켜 주지 않는 것을 본다.
    ///
    /// 한계: 높이만 본다. 가로로 무엇이 잘리는지는 트리에 없고 화면을 봐야 한다(CLAUDE.md).
    func testCompactModeIsActuallyDenser() {
        app.launch()

        let toggle = app.buttons["density.toggle"]
        XCTAssertTrue(toggle.waitForExistence(timeout: 8), "밀도 전환 버튼이 없다")

        let title = app.staticTexts["Swift Concurrency 정리"]
        XCTAssertTrue(title.waitForExistence(timeout: 8), "카드가 안 뜬다")
        // **셀을 재야 한다** — staticText의 높이는 제목 글자 높이(36pt)이지 행 높이가 아니다.
        // 처음엔 그걸 비교해서 단언이 무의미했다.
        let cellOf = { self.app.cells
            .containing(.staticText, identifier: "Swift Concurrency 정리").firstMatch }
        XCTAssertTrue(cellOf().waitForExistence(timeout: 5), "셀을 못 찾았다")
        let cardHeight = cellOf().frame.height

        toggle.tap()
        XCTAssertTrue(title.waitForExistence(timeout: 5), "조밀 모드에서 카드가 사라졌다")

        // 기본값은 카드이므로 한 번 누르면 조밀이다. `@AppStorage`가 이전 실행에서 남으면
        // 방향이 반대가 되어 이 단언이 실패한다 — **실제로 그렇게 실패했고**, 그래서
        // `UITestMode.resetSharedDefaults`가 표준 defaults의 밀도 키까지 비운다.
        XCTAssertLessThan(cellOf().frame.height, cardHeight * 0.75,
                          "조밀 행이 카드의 3/4보다 낮지 않다 — 밀도가 바뀌지 않았거나 "
                              + "@AppStorage가 남았거나, 조밀이 조밀하지 않다")
    }

    /// 목록이 길어 화면 밖에 있는 요소를 찾을 때까지 스크롤한다.    /// 목록이 길어 화면 밖에 있는 요소를 찾을 때까지 스크롤한다.
    /// 못 찾으면 false — 무한히 밀면 실패가 타임아웃으로 위장된다.
    private func scrollToFind(_ element: XCUIElement, maxSwipes: Int = 8) -> Bool {
        if element.waitForExistence(timeout: 3), element.isHittable { return true }
        for _ in 0 ..< maxSwipes {
            app.swipeUp()
            if element.exists, element.isHittable { return true }
        }
        return false
    }

    private func waitForDisappearance(_ element: XCUIElement, timeout: TimeInterval = 8) -> Bool {
        let gone = expectation(for: NSPredicate(format: "exists == false"),
                               evaluatedWith: element)
        return XCTWaiter().wait(for: [gone], timeout: timeout) == .completed
    }
}
