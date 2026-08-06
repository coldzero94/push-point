import OSLog
import PPShare
import WidgetKit
import UIKit

/// Share Extension — 공유 시트에서 한 번에 저장한다.
///
/// 서버에 붙지 않는다. App Group의 공유 SQLite에 **직접 쓰고**, 같은 프로세스에서 태그·요약까지
/// 끝낸다(`PpshareSave`). 그래서 비행기 모드에서도, 홈서버가 꺼져 있어도, 앱을 한 번도 열지
/// 않아도 저장이 완결된다.
///
/// scraper가 빠진 최소 프레임워크(PPShare)만 링크한다 — 확장의 메모리 예산이 이유다
/// (docs/v2/ko/08-DEVELOPMENT-PLAN.md M4 선행 검증).
final class ShareViewController: UIViewController {
    /// 확장은 화면이 곧 닫히고 Go 쪽은 반환 JSON 말고 통로가 없다 — os_log가 사후에
    /// 무슨 일이 있었는지 알 수 있는 유일한 수단이다(콘솔.app에서 조회).
    private static let log = Logger(subsystem: "com.pushpoint.app", category: "share")

    override func viewDidLoad() {
        super.viewDidLoad()
        // 화면을 그리지 않는다. 저장은 밀리초 단위이고, 결과는 알림 배너가 알린다 —
        // 확장 시트는 무엇을 그리든 보고 있던 페이지를 가리므로 즉시 닫는 편이 낫다.
        view.backgroundColor = .clear
        Task { await run() }
    }

    private func run() async {
        // 계측 시작은 run()의 첫 줄이다. extractPayload는 Safari에서 JS 전처리 결과를
        // 기다리므로 **저장 시간의 일부**이고, 그 뒤부터 재면 사용자가 겪는 시간이 아니라
        // 우리가 보고 싶은 시간을 재게 된다.
        let started = SaveTiming.begin()
        // do 바깥에 둔다 — **실패한 저장에도 출처가 필요하다.** 2026-08-03에 터진 실패는
        // 추출 자체가 URL을 못 찾은 것이었고, 그때 계측에 남은 것은 "failed 2122ms"뿐이라
        // 어느 갈래가 깨졌는지 기록만 봐서는 알 수 없었다.
        var source = SharePayload.Source.url
        do {
            let (payload, extracted) = try await extractPayload()
            source = extracted
            let result = try save(payload)
            let host = URL(string: payload["url"] ?? "")?.host ?? ""
            let title = payload["title"] ?? ""

            Self.log.info("저장 id=\(result.id) dup=\(result.duplicate) tags=\(result.tags) sum=\(result.summaryLen)")
            if let summaryError = result.summaryError, !summaryError.isEmpty {
                // 요약은 부가물이라 배너를 차지할 값어치가 없다 — 로그로만 남긴다.
                Self.log.error("요약 기록 실패 id=\(result.id): \(summaryError)")
            }
            if let tagError = result.tagError, !tagError.isEmpty {
                // 본문 없이 URL만 온 저장에는 재시도 잡이 없어 이 실패는 **영구적**이다.
                Self.log.error("태깅 실패 id=\(result.id): \(tagError)")
                await SaveNotifier.notifySaved(title: title, host: host, tags: [t("notify.tagFailed")],
                                               duplicate: result.duplicate)
            } else {
                await SaveNotifier.notifySaved(title: title, host: host,
                                               tags: result.tagNames, duplicate: result.duplicate,
                                               linkID: result.id)
            }
            // 위젯의 스냅샷을 올린다. **중복 저장은 올리지 않는다** — 이미 있던 링크를
            // 다시 공유한 것은 새 저장이 아니고, 연속을 그걸로 이어 주면 위젯이 하루를
            // 지어내는 셈이다.
            //
            // 앱이 아니라 여기서도 하는 이유(10 §8.6): 공유 시트로만 쓰는 사용자에게는
            // 앱이 스냅샷을 갱신할 기회가 없어서, 저장해도 위젯이 "오늘 아직"이라고
            // 말한다 — 방금 한 일을 부정하는 화면이 된다.
            if !result.duplicate {
                StatsSnapshot.bumpToday()
                WidgetCenter.shared.reloadAllTimelines()
            }
            // 배너까지 띄운 뒤에 잰다 — 사용자에게 "됐다"가 보이는 시점이 곧 응답이고,
            // 저장 함수가 반환한 시점이 아니다.
            SaveTiming.end(started, outcome: result.duplicate ? "duplicate" : "saved",
                           tags: result.tags, source: source.rawValue)
        } catch {
            Self.log.error("저장 실패: \(error.localizedDescription)")
            await SaveNotifier.notifyFailed(message: error.localizedDescription)
            SaveTiming.end(started, outcome: "failed", source: source.rawValue)
        }
        finish()
    }

    private func finish() {
        extensionContext?.completeRequest(returningItems: nil)
    }

    // MARK: - 입력

    /// 공유 출처에 따라 받는 것이 다르다(docs/v2/ko/04-DATA-FLOW.md §7.3.1). 어느 갈래로
    /// 오든 계약 필드로 바꾸는 규칙은 `SharePayload`에 있다 — 뷰에 두면 테스트가 부를 수
    /// 없어서 떼어 냈다.
    private func extractPayload() async throws -> (fields: [String: String], source: SharePayload.Source) {
        guard let items = extensionContext?.inputItems as? [NSExtensionItem] else {
            throw ShareError.noInput
        }
        return try await SharePayload.extract(from: items)
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
}

/// PpshareSave가 돌려주는 JSON.
///
/// 키 이름은 Go 쪽 `ppshare.result`와 맞춰야 한다 — Swift는 키로 디코드하므로 한쪽만
/// 바뀌면 조용히 0/false를 읽는다. Go 쪽에 그 드리프트를 잡는 테스트가 있다.
private struct SaveResult: Decodable {
    let id: Int64
    let duplicate: Bool
    let tags: Int
    let tagNames: [String]
    let summaryLen: Int
    /// 태깅 자체가 실패했을 때만 채워진다 — 이 경우 tags는 0이고 링크는 태그 없이 저장됐다.
    let tagError: String?
    /// 태깅은 됐지만 요약 기록만 실패했을 때 채워진다.
    let summaryError: String?

    enum CodingKeys: String, CodingKey {
        case id, duplicate, tags
        case tagNames = "tag_names"
        case summaryLen = "summary_len"
        case tagError = "tag_error"
        case summaryError = "summary_error"
    }
}
