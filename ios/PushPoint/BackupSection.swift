import OpenAPIRuntime
import SwiftUI
import UIKit
import UniformTypeIdentifiers

/// 설정의 백업 구역 — 내보내기와 되돌리기.
///
/// **자립형 iOS에는 이것 말고 백업 경로가 없다.** 아카이브는 App Group 안 SQLite 파일
/// 하나이고, 폰에는 그것을 꺼낼 `cp`가 없다(07-DEPLOYMENT §7의 데스크톱 방법이 여기서는
/// 성립하지 않는다). 폰을 잃으면 전부 사라진다 — 개인 아카이브에서 **회복 불가능한 유일한
/// 실패**라 다른 어떤 기능보다 먼저 있어야 한다.
///
/// 내보낸 파일은 공유 시트로 나간다. iCloud Drive든 AirDrop이든 사용자가 이미 쓰는 곳에
/// 놓게 하는 편이, 우리가 동기화 대상을 고르는 것보다 낫다.
struct BackupSection: View {
    let client: Client?

    @State private var exporting = false
    @State private var importing = false
    @State private var restoring = false
    @State private var message: Message?

    /// 성공과 실패를 **한 자리에** 모은다. 둘을 따로 두면 실패가 조용히 사라지는 분기가
    /// 반드시 하나 생긴다.
    private struct Message: Identifiable {
        let id = UUID()
        let text: String
        let ok: Bool
    }

    var body: some View {
        Section {
            Button {
                Task { await exportArchive() }
            } label: {
                HStack {
                    Text(t("settings.backupExport"))
                    Spacer()
                    if exporting { ProgressView() }
                }
            }
            .disabled(client == nil || exporting)

            Button {
                importing = true
            } label: {
                HStack {
                    Text(t("settings.backupRestore"))
                    Spacer()
                    if restoring { ProgressView() }
                }
            }
            .disabled(client == nil || restoring)
        } header: {
            Text(t("settings.backup"))
        } footer: {
            Text(t("settings.backupFooter"))
        }
        .fileImporter(isPresented: $importing, allowedContentTypes: [.data]) { result in
            switch result {
            case let .success(url): Task { await restoreArchive(from: url) }
            case let .failure(err): message = Message(text: err.localizedDescription, ok: false)
            }
        }
        .alert(item: $message) { m in
            Alert(title: Text(m.ok ? t("settings.backupDone") : t("settings.backupFailed")),
                  message: Text(m.text),
                  dismissButton: .default(Text(t("common.done"))))
        }
    }

    private func exportArchive() async {
        guard let client else { return }
        exporting = true
        defer { exporting = false }
        do {
            let out = try await client.downloadBackup(.init())
            let body = try out.ok.body.binary
            // 날짜를 파일 이름에 넣는다 — 백업을 여러 번 내보내면 Files 안에서 같은 이름이
            // 겹치고, 그러면 어느 것이 최신인지 사용자가 알 방법이 없다.
            let stamp = ISO8601DateFormatter.backupStamp.string(from: Date())
            let url = FileManager.default.temporaryDirectory
                .appendingPathComponent("pushpoint-\(stamp).db")
            try? FileManager.default.removeItem(at: url)
            // HTTPBody는 스트림이다. 통째로 메모리에 올리지 않고 흘려 쓴다 — 10만 건 아카이브가
            // 수백 MB라 폰에서 한 번에 올리면 그대로 죽는다.
            FileManager.default.createFile(atPath: url.path, contents: nil)
            let handle = try FileHandle(forWritingTo: url)
            defer { try? handle.close() }
            for try await chunk in body {
                try handle.write(contentsOf: chunk)
            }
            presentShare(url)
        } catch {
            message = Message(text: error.localizedDescription, ok: false)
        }
    }

    private func restoreArchive(from url: URL) async {
        guard let client else { return }
        restoring = true
        defer { restoring = false }
        // 파일 앱이 준 URL은 **보안 스코프 안에 있다.** 이 짝을 빼먹으면 읽기가 권한 오류로
        // 실패하는데, 시뮬레이터에서는 대개 성공해서 실기기에서만 드러난다.
        let scoped = url.startAccessingSecurityScopedResource()
        defer { if scoped { url.stopAccessingSecurityScopedResource() } }
        do {
            let data = try Data(contentsOf: url)
            let out = try await client.restoreBackup(.init(body: .binary(.init(data))))
            switch out {
            case .ok:
                message = Message(text: t("settings.backupRestartNeeded"), ok: true)
            case let .badRequest(bad):
                let detail = (try? bad.body.json.error.message) ?? t("settings.backupFailed")
                message = Message(text: detail, ok: false)
            default:
                message = Message(text: t("settings.backupFailed"), ok: false)
            }
        } catch {
            message = Message(text: error.localizedDescription, ok: false)
        }
    }
}

private extension ISO8601DateFormatter {
    /// 파일 이름에 콜론이 들어가면 곤란한 곳이 있어 날짜만 쓴다.
    static let backupStamp: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        f.formatOptions = [.withYear, .withMonth, .withDay, .withDashSeparatorInDate]
        return f
    }()
}

/// 공유 시트를 **가장 위에 떠 있는 컨트롤러에서** 직접 띄운다.
///
/// **SwiftUI의 `.sheet`으로는 안 된다.** 이 구역은 이미 시트(설정) 안에 있고, 시트 안에서
/// 또 시트를 요청하면 새로 뜨는 대신 **바깥 시트가 닫힌다.** 실측으로 그랬다 — 파일은
/// 정상적으로 만들어졌는데(163KB) 설정이 닫히고 목록으로 돌아가서, 사용자 눈에는
/// "눌렀더니 아무 일도 안 일어남"이었다. 빌드도 통과했고 오류도 없었다.
private func presentShare(_ url: URL) {
    guard let scene = UIApplication.shared.connectedScenes
        .compactMap({ $0 as? UIWindowScene }).first,
        let root = scene.windows.first(where: \.isKeyWindow)?.rootViewController
    else { return }
    var top = root
    while let presented = top.presentedViewController { top = presented }
    let vc = UIActivityViewController(activityItems: [url], applicationActivities: nil)
    // iPad에서는 popover 앵커가 없으면 크래시한다.
    vc.popoverPresentationController?.sourceView = top.view
    vc.popoverPresentationController?.sourceRect = CGRect(
        x: top.view.bounds.midX, y: top.view.bounds.midY, width: 0, height: 0)
    top.present(vc, animated: true)
}
