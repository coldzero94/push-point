import SwiftUI

/// List가 기본으로 얹는 것들(구분선·행 배경·좌우 여백)을 걷어낸다.
///
/// 카드 보드를 List로 만든 이유는 스와이프 액션과 셀 재활용이지, 시스템 테이블 모양을
/// 원해서가 아니다. 이 수정자가 없으면 카드마다 회색 구분선이 그어지고 배경이 흰색으로
/// 덮여, 카드의 모서리와 그림자가 무의미해진다.
struct PlainRow: ViewModifier {
    let top: CGFloat
    let bottom: CGFloat

    func body(content: Content) -> some View {
        content
            .listRowSeparator(.hidden)
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets(top: top, leading: 16, bottom: bottom, trailing: 16))
    }
}

extension View {
    func plainRow(top: CGFloat, bottom: CGFloat) -> some View {
        modifier(PlainRow(top: top, bottom: bottom))
    }
}
