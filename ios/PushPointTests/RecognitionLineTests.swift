import XCTest
@testable import PushPoint

/// 중복 저장 배너의 본문 조립.
///
/// **이 한 줄이 0단 전부다.** 저장 경로는 원본 시각과 메모를 이미 들고 있었고 배너까지
/// 오지 않았을 뿐이라, 검증할 로직은 "무엇을 말하고 무엇을 말하지 않는가" 하나다.
final class RecognitionLineTests: XCTestCase {
    // 메모가 있으면 메모가 이긴다 — 태그는 기계가 붙였고 메모는 그때의 자기 말이다.
    func testNoteWinsOverTags() {
        let line = SaveNotifier.recognitionLine(
            savedAt: 1_750_000_000, note: "실무 적용 전에 다시 읽기",
            tags: ["devops", "backend"], host: "example.com")
        XCTAssertTrue(line.contains("실무 적용 전에 다시 읽기"), line)
        XCTAssertFalse(line.contains("devops"), line)
    }

    // 메모가 없으면 태그로 물러난다.
    func testFallsBackToTags() {
        let line = SaveNotifier.recognitionLine(
            savedAt: 1_750_000_000, note: nil, tags: ["devops", "backend"], host: "example.com")
        XCTAssertTrue(line.contains("devops"), line)
    }

    // 공백뿐인 메모는 메모가 아니다 — 따옴표만 남은 줄을 보여주면 안 된다.
    func testBlankNoteIsNotANote() {
        let line = SaveNotifier.recognitionLine(
            savedAt: 1_750_000_000, note: "   \n ", tags: ["devops"], host: "example.com")
        XCTAssertTrue(line.contains("devops"), line)
        XCTAssertFalse(line.contains("\u{201C}"), line)
    }

    // **시각이 0이면 날짜를 지어내지 않는다.** 옛 확장 사본이 그 필드를 안 보내는 경우가
    // 있고, 그때는 예전 배너로 물러나는 것이 틀린 날짜를 보여주는 것보다 낫다.
    func testNoDateWhenUnknown() {
        let line = SaveNotifier.recognitionLine(
            savedAt: 0, note: nil, tags: ["devops"], host: "example.com")
        XCTAssertEqual(line, "devops")
    }

    // 아무것도 없으면 도메인이라도 — 빈 줄은 배너에서 "무언가 잘못됐다"로 읽힌다.
    func testHostIsTheFloor() {
        let line = SaveNotifier.recognitionLine(savedAt: 0, note: nil, tags: [], host: "example.com")
        XCTAssertEqual(line, "example.com")
    }
}
