import SwiftUI

@main
struct PushPointApp: App {
    @StateObject private var backend = Backend()
    @Environment(\.scenePhase) private var scenePhase

    var body: some Scene {
        WindowGroup {
            ContentView()
                .environmentObject(backend)
        }
        // 서버 수명을 앱의 포그라운드 수명에 묶는다. iOS는 백그라운드 앱의 CPU·네트워크를
        // 곧 정지시키므로 "항상 떠 있는 서버"라는 가정 자체가 성립하지 않는다 —
        // 저장은 확장이 SQLite에 직접 하므로 서버가 꺼져 있어도 유실되지 않는다.
        .onChange(of: scenePhase) { _, phase in
            switch phase {
            case .active: backend.start()
            case .background: backend.stop()
            default: break
            }
        }
    }
}
