import SwiftUI

/// 하단 중앙 토스트 — 동시에 하나만(§4.10).
///
/// 삭제를 확인창 없이 즉시 실행하는 대신 두는 안전망이다. 확인창은 **모든** 삭제를
/// 느리게 만들어 흔한 경우(정말 지우려는 것)에 세금을 매기고, 드문 경우(실수)만 막는다.
/// 토스트는 반대다 — 흔한 경우는 방해받지 않고, 실수했을 때만 손을 뻗으면 된다.
struct UndoToast: View {
    let message: String
    /// 액션 라벨. 되돌리기가 아닌 용도(변경 실패 알림)로도 쓰므로 고정하지 않는다.
    var actionLabel: String = "실행 취소"
    /// 실패 문구일 때 danger 색을 쓴다 — §4.10이 토스트에 상태 색을 허용하는 유일한 경우.
    var isError: Bool = false
    let onUndo: () -> Void

    var body: some View {
        HStack(spacing: 14) {
            if isError {
                Image(systemName: "exclamationmark.triangle.fill")
                    .font(PP.Typo.label)
                    .foregroundStyle(PP.Palette.danger)
            }
            Text(message)
                .font(PP.Typo.body)
                .foregroundStyle(PP.Palette.fg1)
                .lineLimit(2)
            Button(actionLabel, action: onUndo)
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.fg2)
                // 44×44pt는 예외 없다(§7.5·§8.5). 텍스트 상자만으로는 12pt짜리 표적이다.
                .frame(minWidth: 44, minHeight: 44)
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
