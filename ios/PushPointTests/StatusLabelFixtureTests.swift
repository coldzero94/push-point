import XCTest

/// 상태 라벨을 **웹과 같은 파일로** 대조한다.
///
/// §8.1은 두 클라이언트가 같은 단어를 쓰라고 하는데, 그동안 그건 주장이었다 — 13 §3의
/// 판정표에도 "일치 검사: 없음"으로 적혀 있다. streak과 상대 시각을 픽스처로 묶었을 때
/// 곧바로 갈라짐이 드러났으니, 이것도 파일로 묶는다.
///
/// **라벨 규칙이 뷰 안에 있어 직접 못 부른다.** `LinkCard.statusLabel`은 `private var`이고
/// 그걸 꺼내는 것은 별건이라, 여기서는 픽스처의 문자열이 소스에 **실재하는지**를 본다.
/// 약한 검사지만 없는 것보다 낫다: 웹에서 단어를 고치고 iOS를 안 고치면 빨개진다.
final class StatusLabelFixtureTests: XCTestCase {
    private struct Fixture: Decodable {
        let labels: [String: String]
        let retryWaiting: String
    }

    func testLabelsExistInTheSource() throws {
        let url = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "status-labels", withExtension: "json"),
            "픽스처가 번들에 없다 — project.yml의 buildPhase: resources 확인"
        )
        let fx = try JSONDecoder().decode(Fixture.self, from: Data(contentsOf: url))

        // LinkCard.swift를 소스로 읽는다. 뷰의 private을 부를 수 없으므로 차선이다.
        let here = URL(fileURLWithPath: #filePath)
        let card = here.deletingLastPathComponent().deletingLastPathComponent()
            .appendingPathComponent("PushPoint/LinkCard.swift")
        let src = try String(contentsOf: card, encoding: .utf8)

        for (status, label) in fx.labels {
            XCTAssertTrue(src.contains("\"\(label)\""),
                          "\(status)의 라벨 \"\(label)\"이 LinkCard.swift에 없다 — "
                              + "웹에서 고치고 여기를 안 고쳤다(§8.1)")
        }
        XCTAssertTrue(src.contains("\"\(fx.retryWaiting)\""),
                      "백오프 문구 \"\(fx.retryWaiting)\"이 없다")
    }
}
