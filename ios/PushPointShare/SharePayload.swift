import Foundation
import UniformTypeIdentifiers

/// 공유 시트가 넘긴 첨부를 저장 계약(`LinkInput`)의 필드로 바꾼다.
///
/// **뷰에서 떼어 낸 이유는 공유 원본이 하나가 아니기 때문이다.** 이 규칙은 Safari만
/// 상대하지 않는다 — Chrome·Firefox는 URL 하나만 주고, 메모·메시지·SNS 앱은 캡션과
/// URL이 섞인 plain text만 주며, 어떤 앱은 URL이 아예 텍스트 안에만 있다. 네 갈래가
/// 전부 저장으로 이어져야 하는데, 뷰 컨트롤러의 private 메서드로 두는 한 그중 무엇도
/// 테스트가 부를 수 없었다. 2026-08-03에 Safari 갈래 하나가 조용히 깨져 저장이
/// `URL을 찾을 수 없습니다`로 끝났고, 그것을 잡아낸 것은 시뮬레이터에서 손으로 공유해
/// 본 것뿐이었다. 나머지 세 갈래는 그렇게조차 확인된 적이 없다.
enum SharePayload {
    /// 어느 갈래로 들어왔는가. 계측에만 쓰지만 **저장 결과를 해석하는 열쇠**다 — 태그가
    /// 빈약한 저장을 봤을 때, 규칙 태거가 못 붙인 것인지 애초에 본문이 안 왔던 것인지를
    /// 이 값 없이는 구분할 수 없다.
    enum Source: String {
        /// 사파리 JS 전처리 — 본문까지 실려 온 유일한 갈래.
        case captured
        /// `public.url` 하나. Chrome·Firefox와 대부분의 네이티브 앱이 여기다.
        case url
        /// URL + 캡션. 인스타그램처럼 캡션이 유일한 내용인 경우가 있다.
        case urlWithText = "url_with_text"
        /// 텍스트 안에서 URL을 찾아냈다. 메모·메시지.
        case textOnly = "text_only"
    }
    /// 첨부를 훑어 저장할 필드를 만든다. 항목을 **끝까지 본다** — 첫 URL에서 멈추지
    /// 않는 이유는 같은 항목에 딸려 온 캡션을 버리게 되기 때문이다. 그게 특히 아픈 곳이
    /// 인스타그램이다: 실측(2026-07-25)으로 서버가 그 URL을 받아도 og 메타가 0이라,
    /// 캡션이 이 링크에 대해 우리가 가질 수 있는 **유일한 내용**이다.
    static func extract(from items: [NSExtensionItem]) async throws -> (fields: [String: String], source: Source) {
        var url: String?
        var text: String?

        for item in items {
            for provider in item.attachments ?? [] {
                // 사파리 JS 전처리 결과가 있으면 그것으로 끝이다 — 본문까지 들어 있는
                // 유일한 경로라 다른 항목을 더 봐도 나아질 게 없다.
                //
                // 실패하면 **계속 훑는다.** 여기서 던지면 안 된다: JS가 URL을 못 만든
                // 경우에도 같은 공유 안에 URL 첨부가 따로 올 수 있고, 그때 저장을
                // 포기하는 것은 전처리가 있다는 이유만으로 없느니만 못해지는 것이다.
                if provider.hasItemConformingToTypeIdentifier(UTType.propertyList.identifier),
                   let captured = try? await loadPropertyList(provider) {
                    return (captured, .captured)
                }
                if url == nil,
                   provider.hasItemConformingToTypeIdentifier(UTType.url.identifier),
                   let loaded = try? await loadURL(provider) {
                    url = loaded.absoluteString
                }
                if text == nil,
                   provider.hasItemConformingToTypeIdentifier(UTType.plainText.identifier),
                   let loaded = try? await loadText(provider) {
                    text = loaded
                }
                // public.image는 매핑하지 않는다. 계약(LinkInput)에 이미지를 받을 자리가
                // 없고, 저장의 단위는 **URL**이라 이미지만 온 공유는 저장할 대상 자체가 없다.
            }
        }

        // 텍스트만 온 경우 — 그 안에 URL이 있으면 그걸 쓴다. 앱에 따라
        // "캡션 https://..." 한 덩어리를 plain-text로만 주기 때문이다.
        var source = Source.url
        if url == nil, let text, let found = firstURL(in: text) {
            url = found
            source = .textOnly
        }
        guard let url else { throw ShareError.noURL }

        var payload = ["url": url]
        // 캡션은 description에 넣는다. title이 아닌 이유: 제목은 스크랩이 얻어 올 수 있고
        // 캡션을 제목 자리에 넣으면 나중에 온 진짜 제목과 경쟁한다. description은
        // 계약상 클라이언트 캡처 필드이므로 여기가 제자리다.
        if let text, !text.isEmpty, text != url {
            payload["description"] = text
            if source == .url { source = .urlWithText }
        }
        return (payload, source)
    }

    /// 텍스트에서 첫 http(s) URL. 정규식을 쓰지 않는 이유는 공유 텍스트가 짧고,
    /// `NSDataDetector`가 링크 판정을 시스템과 같은 규칙으로 하기 때문이다.
    static func firstURL(in text: String) -> String? {
        guard let detector = try? NSDataDetector(types: NSTextCheckingResult.CheckingType.link.rawValue)
        else { return nil }
        let range = NSRange(text.startIndex ..< text.endIndex, in: text)
        for match in detector.matches(in: text, range: range) {
            if let u = match.url, u.scheme == "http" || u.scheme == "https" {
                return u.absoluteString
            }
        }
        return nil
    }

    private static func loadText(_ provider: NSItemProvider) async throws -> String? {
        let raw = try await provider.loadItem(forTypeIdentifier: UTType.plainText.identifier)
        let value = (raw as? String) ?? (raw as? NSString).map(String.init)
        return value?.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private static func loadPropertyList(_ provider: NSItemProvider) async throws -> [String: String]? {
        let raw = try await provider.loadItem(forTypeIdentifier: UTType.propertyList.identifier)
        guard let dict = raw as? NSDictionary,
              let js = dict[NSExtensionJavaScriptPreprocessingResultsKey] as? [String: Any],
              let url = js["url"] as? String, !url.isEmpty
        else { return nil }
        // 계약 필드만 문자열로 추린다 — extract.js가 만드는 키와 같다.
        var out = ["url": url]
        for key in ["title", "description", "body_text", "keywords"] {
            if let value = js[key] as? String, !value.isEmpty { out[key] = value }
        }
        return out
    }

    private static func loadURL(_ provider: NSItemProvider) async throws -> URL? {
        try await provider.loadItem(forTypeIdentifier: UTType.url.identifier) as? URL
    }
}

enum ShareError: LocalizedError {
    case noInput, noURL, noAppGroup

    var errorDescription: String? {
        switch self {
        case .noInput: "공유된 항목이 없습니다"
        case .noURL: "URL을 찾을 수 없습니다"
        case .noAppGroup: "App Group 컨테이너를 열 수 없습니다"
        }
    }
}
