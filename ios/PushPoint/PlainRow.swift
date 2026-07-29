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

/// 구간 머리글용. `PlainRow`와 같되 **행 배경이 불투명하다.**
///
/// `List`의 `Section` 헤더는 스크롤하는 동안 상단에 **고정된다.** 그래서 카드가 그 밑으로
/// 흘러가는데, 헤더 배경이 투명하면 카드의 제목이 머리글 글자 뒤로 비쳐 **두 글자가 같은
/// 자리에 겹친다.** 2026-07-29에 사용자가 페이지네이션 UI 테스트가 도는 화면에서
/// 잡았다 — "오늘"과 카드 제목이 포개져 있었다.
///
/// 카드 행은 투명이 맞다(캔버스 위에 그림자와 모서리가 살아야 한다). 갈리는 것은 고정
/// 여부이고, 고정되는 것만 불투명하면 된다.
struct PlainHeaderRow: ViewModifier {
    let top: CGFloat
    let bottom: CGFloat

    func body(content: Content) -> some View {
        // **여백을 뷰 안으로 넣고 배경을 직접 칠한다.** `listRowBackground`로는 안 된다 —
        // 고정된 헤더는 별도 레이어로 떠오르고 행 배경은 따라오지 않아서, 칠했는데도
        // 카드가 그대로 비쳤다(실측). `listRowInsets`를 0으로 두어야 배경이 행 폭 전체를
        // 덮는다. 인셋을 남겨 두면 좌우 16pt가 안 칠해져 그 틈으로 계속 비친다.
        content
            .padding(EdgeInsets(top: top, leading: 16, bottom: bottom, trailing: 16))
            .frame(maxWidth: .infinity, alignment: .leading)
            .background(PP.Palette.canvas)
            .listRowSeparator(.hidden)
            .listRowBackground(Color.clear)
            .listRowInsets(EdgeInsets())
    }
}

extension View {
    func plainHeaderRow(top: CGFloat, bottom: CGFloat) -> some View {
        modifier(PlainHeaderRow(top: top, bottom: bottom))
    }
    func plainRow(top: CGFloat, bottom: CGFloat) -> some View {
        modifier(PlainRow(top: top, bottom: bottom))
    }
}
