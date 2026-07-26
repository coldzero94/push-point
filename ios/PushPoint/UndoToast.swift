import SwiftUI

/// 하단 중앙 토스트 — 동시에 하나만(§4.10).
///
/// 삭제를 확인창 없이 즉시 실행하는 대신 두는 안전망이다. 확인창은 **모든** 삭제를
/// 느리게 만들어 흔한 경우(정말 지우려는 것)에 세금을 매기고, 드문 경우(실수)만 막는다.
/// 토스트는 반대다 — 흔한 경우는 방해받지 않고, 실수했을 때만 손을 뻗으면 된다.
struct UndoToast: View {
    let message: String
    let onUndo: () -> Void

    var body: some View {
        HStack(spacing: 14) {
            Text(message)
                .font(PP.Typo.body)
                .foregroundStyle(PP.Palette.fg1)
            Button("실행 취소", action: onUndo)
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.accent)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 12)
        .background(PP.Palette.elevated)
        .clipShape(RoundedRectangle(cornerRadius: PP.Radius.panel, style: .continuous))
        .overlay(
            RoundedRectangle(cornerRadius: PP.Radius.panel, style: .continuous)
                .strokeBorder(PP.Palette.line2, lineWidth: 1)
        )
        .shadow(color: .black.opacity(0.16), radius: 18, y: 6)
        .padding(.horizontal, 16)
        .padding(.bottom, 8)
        .transition(.move(edge: .bottom).combined(with: .opacity))
    }
}
