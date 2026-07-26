import SwiftUI

/// 태그 칩 — hue × fill 2축(§5.2).
///
/// **hue는 facet이, 채움은 "누가 손댔나"가 진다.** 이 분리가 색맹 조건에서도 정보를
/// 남기는 근거다 — 색을 못 봐도 형태(테두리 없음 / tint / solid)가 남는다.
struct Chip: View {
    /// 채움 3단. 0 = 기계가 붙임, 1 = 사람이 붙임, 2 = 지금 선택됨.
    enum Fill {
        case machine, manual, selected

        /// 계약의 `LinkTag.source`에서 만든다. 선택 여부는 화면이 안다.
        init(source: String, isActive: Bool) {
            if isActive { self = .selected } else { self = source == "manual" ? .manual : .machine }
        }
    }

    let name: String
    let facet: PP.Facet
    let fill: Fill

    var body: some View {
        Text(name)
            .font(PP.Typo.label)
            .tracking(PP.Tracking.label)
            .foregroundStyle(foreground)
            .padding(.horizontal, horizontalPadding)
            .padding(.vertical, 3)
            .background(background, in: Capsule())
    }

    private var foreground: Color {
        switch fill {
        case .machine, .manual: facet.ink
        // solid 위에서는 잉크가 뒤집힌다.
        case .selected: PP.Palette.onAccent
        }
    }

    private var background: Color {
        switch fill {
        // 기계 출력은 잉크만 — 테두리도 배경도 없다. 화면에 칩이 많아도 조용하다.
        case .machine: .clear
        case .manual: facet.tint
        case .selected: facet.ink
        }
    }

    /// 채움이 없으면 좌우 여백도 없앤다 — 배경이 없는데 여백만 있으면 정렬이 어긋나 보인다.
    private var horizontalPadding: CGFloat {
        fill == .machine ? 0 : 9
    }
}

extension Chip.Fill: Equatable {}
