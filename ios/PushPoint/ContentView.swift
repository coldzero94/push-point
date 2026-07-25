import SwiftUI

/// M4 골격 화면 — 저장된 링크 목록.
///
/// 목적은 예쁜 UI가 아니라 **자립 모드가 실제로 성립하는지 눈으로 확인**하는 것이다:
/// 공유 시트로 넣은 링크가 서버 없이 폰 안에서 태그까지 붙어 나타나는가.
struct ContentView: View {
    @EnvironmentObject private var backend: Backend
    @State private var links: [Components.Schemas.Link] = []
    @State private var loadError: String?

    var body: some View {
        NavigationStack {
            Group {
                switch backend.state {
                case .idle, .starting:
                    ProgressView("서버 시작 중…")
                case let .failed(message):
                    ContentUnavailableView("서버를 시작하지 못했습니다",
                                           systemImage: "exclamationmark.triangle",
                                           description: Text(message))
                case .running:
                    listBody
                }
            }
            .navigationTitle("Push-Point")
            .refreshable { await load() }
        }
        .task(id: backend.state) { await load() }
    }

    @ViewBuilder
    private var listBody: some View {
        if let loadError {
            ContentUnavailableView("목록을 불러오지 못했습니다",
                                   systemImage: "wifi.exclamationmark",
                                   description: Text(loadError))
        } else if links.isEmpty {
            ContentUnavailableView("아직 저장한 링크가 없습니다", systemImage: "tray",
                                   description: Text("사파리나 앱에서 공유 시트로 보내 보세요."))
        } else {
            List(links, id: \.id) { link in
                VStack(alignment: .leading, spacing: 4) {
                    // 계약상 title은 비어 있을 수 있다(og·title 둘 다 없으면 서버가 빈 문자열을
                    // 그대로 준다 — 사실을 숨기지 않는다). 빈 셀을 막는 것은 클라이언트 책임이라
                    // domain으로 대체한다(.claude/rules/ios.md).
                    Text(link.title.isEmpty ? link.domain : link.title)
                        .font(.body).lineLimit(2)
                    Text(link.url).font(.caption).foregroundStyle(.secondary).lineLimit(1)
                    if !link.tags.isEmpty {
                        Text(link.tags.map(\.name).joined(separator: " · "))
                            .font(.caption2).foregroundStyle(.tint)
                    }
                }
                .padding(.vertical, 2)
            }
        }
    }

    private func load() async {
        guard case .running = backend.state, let client = backend.client else { return }
        do {
            let output = try await client.listLinks(.init(query: .init(limit: 50)))
            links = try output.ok.body.json.links
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }
}
