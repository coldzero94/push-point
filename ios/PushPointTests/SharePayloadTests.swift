import Foundation
import Testing
import UniformTypeIdentifiers

/// 공유 시트가 넘길 수 있는 **네 가지 모양**을 전부 고정한다.
///
/// 이 파일이 생긴 이유: 2026-08-03까지 이 규칙에는 테스트가 하나도 없었고, 검증은
/// "시뮬레이터에서 사파리로 공유해 본다" 뿐이었다. 그래서 Safari 갈래가 깨졌을 때는
/// 손으로 잡았지만, Chrome·메모·SNS에서 오는 나머지 세 갈래는 **한 번도 확인된 적이
/// 없었다.** 여기서 쓰는 `NSItemProvider`는 진짜 객체라, 테스트가 부르는 것은 확장이
/// 실제로 부르는 코드와 같다(스텁이 아니다).
struct SharePayloadTests {
    private func item(_ providers: NSItemProvider...) -> NSExtensionItem {
        let it = NSExtensionItem()
        it.attachments = providers
        return it
    }

    private func urlProvider(_ s: String) -> NSItemProvider {
        NSItemProvider(item: URL(string: s)! as NSSecureCoding, typeIdentifier: UTType.url.identifier)
    }

    private func textProvider(_ s: String) -> NSItemProvider {
        NSItemProvider(item: s as NSString, typeIdentifier: UTType.plainText.identifier)
    }

    private func jsProvider(_ results: [String: Any]) -> NSItemProvider {
        let dict: NSDictionary = [NSExtensionJavaScriptPreprocessingResultsKey: results]
        return NSItemProvider(item: dict, typeIdentifier: UTType.propertyList.identifier)
    }

    // MARK: - 갈래 1: 사파리 (JS 전처리)

    @Test("사파리는 본문까지 실려 온다")
    func safariCarriesBody() async throws {
        let (out, src) = try await SharePayload.extract(from: [item(jsProvider([
            "url": "https://example.com/a",
            "title": "제목",
            "body_text": "본문",
            "keywords": "k",
        ]))])
        #expect(out["url"] == "https://example.com/a")
        #expect(out["body_text"] == "본문")
        #expect(out["title"] == "제목")
        #expect(src == .captured)
    }

    /// **이것이 2026-08-03에 실제로 터진 저장 실패다.** `Info.plist`가 전처리 파일을
    /// 선언하면 사파리는 propertyList **하나만** 준다. 그때 JS가 url을 못 만들면 예전
    /// 코드는 곧장 `noURL`로 끝났다 — 화면에는 "URL을 찾을 수 없습니다"만 남았다.
    /// 지금은 `extract.js`가 url을 반드시 채우지만, 규칙 쪽도 빈손을 삼키지 않고
    /// 나머지 첨부를 계속 훑어야 한다.
    @Test("JS가 url을 못 만들어도 같은 공유의 URL 첨부로 저장된다")
    func safariFallsBackWhenJSYieldsNoURL() async throws {
        let (out, src) = try await SharePayload.extract(from: [item(
            jsProvider(["url": "", "title": "제목만 있고 url이 비었다"]),
            urlProvider("https://example.com/b")
        )])
        #expect(out["url"] == "https://example.com/b")
        #expect(src == .url)
    }

    @Test("첨부가 propertyList 하나뿐이고 url이 비면 저장할 대상이 없다")
    func safariWithOnlyEmptyPropertyListThrows() async {
        await #expect(throws: ShareError.self) {
            _ = try await SharePayload.extract(from: [item(jsProvider(["url": ""]))])
        }
    }

    // MARK: - 갈래 2: Chrome·Firefox 등 URL만 주는 앱

    /// Chrome·Firefox는 사파리의 JS 전처리를 태우지 않는다 — `public.url` 하나가 전부다.
    /// 그래서 **본문 캡처가 없고**, 제목·설명·태그는 서버 스크랩에 달린다. 저장 자체는
    /// 반드시 성공해야 한다.
    @Test("URL 하나만 와도 저장된다")
    func urlOnly() async throws {
        let (out, src) = try await SharePayload.extract(from: [item(urlProvider("https://example.com/c"))])
        #expect(out["url"] == "https://example.com/c")
        #expect(out["body_text"] == nil)
        #expect(src == .url)
    }

    // MARK: - 갈래 3: 캡션이 함께 오는 앱 (인스타그램 등)

    @Test("URL과 캡션이 같이 오면 캡션을 description으로 남긴다")
    func urlWithCaption() async throws {
        let (out, src) = try await SharePayload.extract(from: [item(
            urlProvider("https://example.com/d"),
            textProvider("사진 설명")
        )])
        #expect(out["url"] == "https://example.com/d")
        #expect(out["description"] == "사진 설명")
        #expect(src == .urlWithText)
    }

    @Test("텍스트가 URL과 같은 문자열이면 description을 만들지 않는다")
    func textEqualToURLIsNotACaption() async throws {
        let (out, src) = try await SharePayload.extract(from: [item(
            urlProvider("https://example.com/e"),
            textProvider("https://example.com/e")
        )])
        #expect(out["description"] == nil)
        #expect(src == .url)
    }

    // MARK: - 갈래 4: 메모·메시지 — URL이 텍스트 안에만 있다

    @Test("텍스트 안의 URL을 찾아 저장한다")
    func urlInsideTextOnly() async throws {
        let (out, src) = try await SharePayload.extract(from: [item(
            textProvider("이거 봐 https://example.com/f 재밌음")
        )])
        #expect(out["url"] == "https://example.com/f")
        #expect(out["description"] == "이거 봐 https://example.com/f 재밌음")
        #expect(src == .textOnly)
    }

    @Test("URL이 없는 텍스트는 저장할 대상이 없다")
    func plainTextWithoutURLThrows() async {
        await #expect(throws: ShareError.self) {
            _ = try await SharePayload.extract(from: [item(textProvider("그냥 메모"))])
        }
    }

    /// 저장 대상은 http(s)뿐이다 — `mailto:`는 URL이어도 저장할 링크가 아니다.
    @Test("http(s)가 아닌 스킴은 URL로 치지 않는다")
    func nonHTTPSchemesRejected() {
        #expect(SharePayload.firstURL(in: "mailto:a@b.com 로 연락") == nil)
    }

    /// 스킴 없이 도메인만 적어 보내는 공유가 실제로 있다. `NSDataDetector`가 그것을
    /// `http://`로 승격해 주므로 저장된다 — 처음엔 이걸 거절하는 줄 알고 그렇게 단정하는
    /// 테스트를 썼는데, 실측(2026-08-03) 결과 코드 쪽이 맞았다. 숫자·금액은 링크로
    /// 잡히지 않으니 아무 텍스트나 링크가 되는 것은 아니다.
    @Test("스킴 없는 도메인은 http로 승격되고, 숫자는 링크가 아니다")
    func bareDomainIsPromoted() {
        #expect(SharePayload.firstURL(in: "example.com 참고") == "http://example.com")
        #expect(SharePayload.firstURL(in: "3.5초 걸림") == nil)
        #expect(SharePayload.firstURL(in: "가격은 1,200원") == nil)
    }

    // MARK: - 경계

    @Test("첨부가 없으면 저장할 대상이 없다")
    func noAttachments() async {
        await #expect(throws: ShareError.self) {
            _ = try await SharePayload.extract(from: [NSExtensionItem()])
        }
    }

    /// 항목이 여러 개일 때 본문이 실린 쪽을 고른다 — 첫 항목에서 멈추면 사파리 캡처를
    /// 버리고 URL만 저장하게 된다.
    @Test("URL 항목이 먼저 와도 본문이 실린 항목을 쓴다")
    func propertyListWinsOverBareURL() async throws {
        let (out, src) = try await SharePayload.extract(from: [
            item(urlProvider("https://example.com/g")),
            item(jsProvider(["url": "https://example.com/g", "body_text": "본문"])),
        ])
        #expect(out["body_text"] == "본문")
        #expect(src == .captured)
    }
}
