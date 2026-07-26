import SwiftUI

/// 섹션이 처음 보일 때 한 번 올라오며 나타난다.
///
/// **스크롤에 묶지 않는 것이 이 구현의 요점이다.** 디자인 시스템은 시그니처 모션 예산을
/// 정확히 하나로 두고("여전히 앱 전체에서 연출은 이것 하나뿐이다"), 그 하나는 이미
/// 저장 직후 카드가 채워지는 연출(S2)에 쓰였다. 스크롤 위치에 반응하는 패럴랙스·핀
/// 고정은 두 번째 시그니처가 되므로 하지 않는다.
///
/// 대신 여기서 하는 것은 **진입 전환 한 번**이다: 화면에 처음 들어올 때 살짝 올라오며
/// 나타나고, 그 뒤로는 아무 일도 하지 않는다. 스크롤을 되돌려도 다시 재생되지 않는다 —
/// 반복되는 순간 그것은 전환이 아니라 연출이 된다.
///
/// `reduceMotion`이 켜져 있으면 이동 없이 즉시 나타난다. 모션은 장식이므로 접근성
/// 설정보다 우선할 이유가 없다.
struct Reveal: ViewModifier {
    /// 위에서부터 순서대로 아주 조금씩 늦춘다 — 한꺼번에 나타나면 개별 섹션이 아니라
    /// 화면 전체가 깜빡인 것처럼 보인다.
    let index: Int

    @Environment(\.accessibilityReduceMotion) private var reduceMotion
    @State private var shown = false

    func body(content: Content) -> some View {
        content
            .opacity(shown ? 1 : 0)
            .offset(y: shown || reduceMotion ? 0 : 10)
            .onAppear {
                guard !shown else { return }
                if reduceMotion {
                    shown = true
                    return
                }
                withAnimation(.smooth(duration: 0.45).delay(Double(index) * 0.06)) {
                    shown = true
                }
            }
    }
}

extension View {
    func reveal(_ index: Int) -> some View { modifier(Reveal(index: index)) }
}
