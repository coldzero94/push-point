import SwiftUI

@main
struct PushPointApp: App {
    /// **앱이 뜨는 즉시 건다.** iOS는 알림 탭으로 앱을 깨우면서 `didReceive`를 곧장
    /// 부르는데, 그때 델리게이트가 없으면 그 탭이 조용히 버려진다 — `.onAppear`는 늦다.
    init() { NotificationRouter.shared.install() }

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
