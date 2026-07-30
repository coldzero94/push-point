import SwiftUI

/// 생성 커버(R4) — 썸네일이 없는 링크의 커버를 그린다.
///
/// `thumb: failed` + `status: done`은 계약상 정상 조합이라, og:image에 기대는 화면은
/// 회색 박스 밭이 된다. 대신 확실히 아는 것으로 그린다: 지배 태그의 facet(색)과
/// 도메인(기하). **색은 해시하지 않는다** — 두 링크가 닮아 보이는 이유는 같은 주제라서지
/// 해시가 색을 지어냈기 때문이 아니어야 한다(§5.4).
///
/// 무늬 규칙은 웹 `frontend/src/lib/covers.ts`와 같고, 해시 일치는
/// `CoverPatternTests`가 고정한다.
struct GeneratedCover: View {
    let domain: String
    let facet: PP.Facet
    /// 도메인 워드마크를 그릴지. **조밀 행(44pt)에서는 끈다.**
    ///
    /// 그 크기에서 글자는 정보가 아니라 소음이다 — 실제로 `uitest.example`이
    /// "uite st…"로 두 줄에 깨져 나왔다. 측정으로도 그렇다: 64px 이하 썸네일에서는
    /// **색과 배치만** 정보를 나른다(Kaasten 2002, §4.4.1). 워드마크는 그 두 채널
    /// 어디에도 없고 자리만 먹는다.
    var showsWordmark: Bool = true

    var body: some View {
        let pattern = CoverPattern(domain: domain)
        Canvas { context, size in
            draw(pattern, in: &context, size: size)
        }
        // Canvas가 그리기 전에도 회색이 보이지 않도록 tint를 바닥에 깐다 —
        // R4의 "빈칸을 만들지 않는다"는 첫 프레임에도 적용된다.
        .background(facet.cover)
        .overlay(alignment: .bottomLeading) {
            if showsWordmark {
            // 도메인 워드마크. 커버가 출처의 표식이 되려면 무늬만으로는 부족하다.
                Text(domain)
                    .font(PP.Typo.metaMono)
                    .foregroundStyle(facet.ink)
                    .padding(.leading, 12)
                    .padding(.bottom, 9)
            }
        }
    }

    private func draw(_ pattern: CoverPattern, in context: inout GraphicsContext, size: CGSize) {
        let step = CGFloat(pattern.step)
        let stroke = facet.ink
        // 무늬마다 획 알파가 다르다 — stack은 채우므로 더 낮게 앉는다.
        // **웹과 같은 값이어야 한다**(§4.5): 같은 도메인은 두 클라이언트에서 같은 그림이
        // 나온다는 것이 R4의 약속이고, 해시가 같아도 렌더 상수가 다르면 그 약속이 깨진다.
        // 2026-07-29까지 stack이 0.10(웹 0.13), 획이 1.5(웹 1.25)였다.
        let alpha: Double = pattern.kind == .stack ? 0.13 : 0.16

        // **중심을 기준으로 회전한다** — 웹이 그렇게 한다(covers.ts는 캔버스 중앙으로
        // 옮겼다가 회전하고 되돌린다). 원점 기준으로 돌리면 같은 각도라도 무늬가 다른
        // 자리에 놓인다.
        context.translateBy(x: size.width / 2, y: size.height / 2)
        context.rotate(by: .degrees(Double(pattern.rotate)))
        context.translateBy(x: -size.width / 2, y: -size.height / 2)
        let bounds = CGRect(origin: .zero, size: size).insetBy(dx: -size.width, dy: -size.height)

        var path = Path()
        switch pattern.kind {
        case .hatch:
            var x = bounds.minX
            while x < bounds.maxX {
                path.move(to: CGPoint(x: x, y: bounds.minY))
                path.addLine(to: CGPoint(x: x, y: bounds.maxY))
                x += step
            }
        case .lattice:
            var x = bounds.minX
            while x < bounds.maxX {
                path.move(to: CGPoint(x: x, y: bounds.minY))
                path.addLine(to: CGPoint(x: x, y: bounds.maxY))
                x += step
            }
            var y = bounds.minY
            while y < bounds.maxY {
                path.move(to: CGPoint(x: bounds.minX, y: y))
                path.addLine(to: CGPoint(x: bounds.maxX, y: y))
                y += step
            }
        case .contour:
            // 동심 곡선 — variant가 중심을 옮긴다.
            let cx = size.width * (0.2 + 0.15 * CGFloat(pattern.variant))
            let cy = size.height * 0.5
            var r = step
            while r < size.width * 1.6 {
                path.addEllipse(in: CGRect(x: cx - r, y: cy - r, width: r * 2, height: r * 2))
                r += step
            }
        case .stack:
            // 가로 띠 — 채우므로 알파가 낮다.
            var y = bounds.minY
            var odd = false
            while y < bounds.maxY {
                if odd {
                    path.addRect(CGRect(x: bounds.minX, y: y, width: bounds.width, height: step))
                }
                y += step
                odd.toggle()
            }
        }

        if pattern.kind == .stack {
            context.fill(path, with: .color(stroke.opacity(alpha)))
        } else {
            context.stroke(path, with: .color(stroke.opacity(alpha)), lineWidth: 1.25)
        }
    }
}
