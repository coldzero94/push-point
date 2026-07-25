import PPShare
import UIKit
import UniformTypeIdentifiers

/// Share Extension — 공유 시트에서 한 번에 저장한다.
///
/// 서버에 붙지 않는다. App Group의 공유 SQLite에 **직접 쓰고**, 같은 프로세스에서 태그·요약까지
/// 끝낸다(`PpshareSave`). 그래서 비행기 모드에서도, 홈서버가 꺼져 있어도, 앱을 한 번도 열지
/// 않아도 저장이 완결된다.
///
/// scraper가 빠진 최소 프레임워크(PPShare)만 링크한다 — 확장의 메모리 예산이 이유다
/// (docs/v2/08-DEVELOPMENT-PLAN.md M4 선행 검증).
final class ShareViewController: UIViewController {
    private let label = UILabel()

    override func viewDidLoad() {
        super.viewDidLoad()
        setUpMinimalUI()
        Task { await run() }
    }

    private func run() async {
        do {
            let payload = try await extractPayload()
            let result = try save(payload)
            show(result.duplicate ? "이미 저장된 링크입니다" : "저장했습니다\(result.tagSuffix)")
        } catch {
            show("저장 실패: \(error.localizedDescription)")
        }
        // 사용자가 결과를 읽을 최소 시간만 두고 닫는다 — 공유는 2초 안에 끝나야 한다.
        try? await Task.sleep(for: .milliseconds(700))
        extensionContext?.completeRequest(returningItems: nil)
    }

    // MARK: - 입력

    /// 공유 출처에 따라 받는 것이 다르다(docs/v2/04-DATA-FLOW.md §7.3.1):
    ///   - 사파리: JS 전처리기(extract.js)가 DOM에서 본문까지 뽑아 딕셔너리로 넘긴다.
    ///   - 네이티브 앱: 대개 URL 하나뿐이다. 본문 없이 저장되고 나중에 스크랩이 채운다.
    private func extractPayload() async throws -> [String: String] {
        guard let items = extensionContext?.inputItems as? [NSExtensionItem] else {
            throw ShareError.noInput
        }
        for item in items {
            for provider in item.attachments ?? [] {
                // 1순위: 사파리 JS 전처리 결과 — 본문이 들어 있는 유일한 경로다.
                if provider.hasItemConformingToTypeIdentifier(UTType.propertyList.identifier),
                   let captured = try? await loadPropertyList(provider) {
                    return captured
                }
                // 2순위: URL만.
                if provider.hasItemConformingToTypeIdentifier(UTType.url.identifier),
                   let url = try? await loadURL(provider) {
                    return ["url": url.absoluteString]
                }
            }
        }
        throw ShareError.noURL
    }

    private func loadPropertyList(_ provider: NSItemProvider) async throws -> [String: String]? {
        let raw = try await provider.loadItem(forTypeIdentifier: UTType.propertyList.identifier)
        guard let dict = raw as? NSDictionary,
              let js = dict[NSExtensionJavaScriptPreprocessingResultsKey] as? [String: Any],
              let url = js["url"] as? String, !url.isEmpty
        else { return nil }
        // 계약 필드만 문자열로 추린다 — extract.js가 만드는 키와 같다.
        var out = ["url": url]
        for key in ["title", "description", "body_text"] {
            if let value = js[key] as? String, !value.isEmpty { out[key] = value }
        }
        return out
    }

    private func loadURL(_ provider: NSItemProvider) async throws -> URL? {
        try await provider.loadItem(forTypeIdentifier: UTType.url.identifier) as? URL
    }

    // MARK: - 저장

    private func save(_ payload: [String: String]) throws -> SaveResult {
        guard let dir = AppGroup.dataDirectory() else { throw ShareError.noAppGroup }
        try FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)

        var err: NSError?
        PpshareOpen(dir.path, &err)
        if let err { throw err }
        // 확장은 언제든 서스펜드될 수 있고, 공유 컨테이너 파일 락을 쥔 채 서스펜드되면
        // iOS가 0xdead10cc로 강제 종료한다. 연결을 짧게 가져가는 것이 그 위험을 줄이는
        // 유일한 수단이라 반드시 닫는다.
        defer { var e: NSError?; PpshareClose(&e) }

        let json = String(data: try JSONSerialization.data(withJSONObject: payload), encoding: .utf8) ?? "{}"
        let raw = PpshareSave(json, &err)
        if let err { throw err }
        return try JSONDecoder().decode(SaveResult.self, from: Data(raw.utf8))
    }

    // MARK: - UI

    private func setUpMinimalUI() {
        view.backgroundColor = .systemBackground
        label.textAlignment = .center
        label.numberOfLines = 0
        label.text = "저장 중…"
        label.translatesAutoresizingMaskIntoConstraints = false
        view.addSubview(label)
        NSLayoutConstraint.activate([
            label.centerXAnchor.constraint(equalTo: view.centerXAnchor),
            label.centerYAnchor.constraint(equalTo: view.centerYAnchor),
            label.leadingAnchor.constraint(greaterThanOrEqualTo: view.leadingAnchor, constant: 24),
        ])
    }

    private func show(_ text: String) { label.text = text }
}

/// PpshareSave가 돌려주는 JSON.
private struct SaveResult: Decodable {
    let id: Int64
    let duplicate: Bool
    let tags: Int
    let summaryLen: Int
    let tagError: String?

    enum CodingKeys: String, CodingKey {
        case id, duplicate, tags
        case summaryLen = "summary_len"
        case tagError = "tag_error"
    }

    /// 태그가 붙었으면 그 사실을 보여준다 — 확장이 오프라인에서 태깅까지 했다는 증거다.
    var tagSuffix: String { tags > 0 ? " · 태그 \(tags)개" : "" }
}

private enum ShareError: LocalizedError {
    case noInput, noURL, noAppGroup

    var errorDescription: String? {
        switch self {
        case .noInput: "공유된 항목이 없습니다"
        case .noURL: "URL을 찾을 수 없습니다"
        case .noAppGroup: "App Group 컨테이너를 열 수 없습니다"
        }
    }
}
