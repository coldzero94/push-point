import SwiftUI

@main
struct PushPointApp: App {
    @StateObject private var backend = Backend()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(backend)
        }
        // 서버 수명을 앱의 포그라운드 수명에 묶는다. iOS는 백그라운드 앱의 CPU·네트워크를
        // 곧 정지시키므로 "항상 떠 있는 서버"라는 가정 자체가 성립하지 않는다 —
        // 저장은 확장이 SQLite에 직접 하므로 서버가 꺼져 있어도 유실되지 않는다.
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .active:
                backend.start()
                // 확장이 저장 결과를 배너로 알린다 — 권한은 본체 앱에서 받는다.
                // 확장에서 시스템 권한 창을 띄우면 공유 흐름 한복판에 모달이 끼어들어
                // "2초 저장"이 깨진다.
                Task { await SaveNotifier.requestAuthorization() }
            case .background: backend.stop()
            default: break
            }
        }
    }
}
