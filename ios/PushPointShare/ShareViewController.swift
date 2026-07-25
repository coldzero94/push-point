import OSLog
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
    /// 확장은 화면이 곧 닫히고 Go 쪽은 반환 JSON 말고 통로가 없다 — os_log가 사후에
    /// 무슨 일이 있었는지 알 수 있는 유일한 수단이다(콘솔.app에서 조회).
    private static let log = Logger(subsystem: "com.pushpoint.app", category: "share")
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
            show(result.message)
            // 확장의 실패는 이 로그가 아니면 어디에도 남지 않는다 — 화면은 곧 닫히고,
            // Go 쪽은 반환 JSON 말고 다른 통로가 없다. os_log는 콘솔.app으로 꺼내 볼 수 있다.
            Self.log.info("저장 완료 id=\(result.id) dup=\(result.duplicate) tags=\(result.tags) sum=\(result.summaryLen)")
            if let tagError = result.tagError, !tagError.isEmpty {
                Self.log.error("태깅 실패 id=\(result.id): \(tagError)")
            }
            if let summaryError = result.summaryError, !summaryError.isEmpty {
                Self.log.error("요약 기록 실패 id=\(result.id): \(summaryError)")
            }
        } catch {
            show("저장 실패: \(error.localizedDescription)")
            Self.log.error("저장 실패: \(error.localizedDescription)")
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
///
/// 키 이름은 Go 쪽 `ppshare.result`와 맞춰야 한다 — Swift는 키로 디코드하므로 한쪽만
/// 바뀌면 조용히 0/false를 읽는다. Go 쪽에 그 드리프트를 잡는 테스트가 있다.
private struct SaveResult: Decodable {
    let id: Int64
    let duplicate: Bool
    let tags: Int
    let summaryLen: Int
    /// 태깅 자체가 실패했을 때만 채워진다 — 이 경우 tags는 0이고 링크는 태그 없이 저장됐다.
    let tagError: String?
    /// 태깅은 됐지만 요약 기록만 실패했을 때 채워진다.
    let summaryError: String?

    enum CodingKeys: String, CodingKey {
        case id, duplicate, tags
        case summaryLen = "summary_len"
        case tagError = "tag_error"
        case summaryError = "summary_error"
    }

    /// 사용자에게 보여줄 한 줄.
    ///
    /// 태깅 실패를 굳이 드러내는 이유: 본문 없이 URL만 공유된 링크에는 재시도 잡이 없어서
    /// **그 실패가 영구적**이다(ppshare.Save 주석의 두 번째·세 번째 경로). "저장했습니다"만
    /// 보여주면 사용자는 태그가 영영 안 붙는 줄 모른 채 넘어간다.
    /// 반대로 요약 실패는 부가물이 하나 빠진 것뿐이라 화면을 차지할 값어치가 없다 —
    /// 대신 로그로 남긴다.
    var message: String {
        if let tagError, !tagError.isEmpty {
            return duplicate ? "이미 저장됨 · 태그 실패" : "저장했습니다 · 태그 실패"
        }
        if duplicate { return "이미 저장된 링크입니다" }
        return tags > 0 ? "저장했습니다 · 태그 \(tags)개" : "저장했습니다"
    }
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
