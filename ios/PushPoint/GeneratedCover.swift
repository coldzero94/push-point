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

    var body: some View {
        let pattern = CoverPattern(domain: domain)
        Canvas { context, size in
            draw(pattern, in: &context, size: size)
        }
        // Canvas가 그리기 전에도 회색이 보이지 않도록 tint를 바닥에 깐다 —
        // R4의 "빈칸을 만들지 않는다"는 첫 프레임에도 적용된다.
        .background(facet.tint)
        .overlay(alignment: .bottomLeading) {
            // 도메인 워드마크. 커버가 출처의 표식이 되려면 무늬만으로는 부족하다.
            Text(domain)
                .font(PP.Typo.metaMono)
                .foregroundStyle(facet.ink)
                .padding(.leading, 12)
                .padding(.bottom, 9)
        }
    }

    private func draw(_ pattern: CoverPattern, in context: inout GraphicsContext, size: CGSize) {
        let step = CGFloat(pattern.step)
        let stroke = facet.ink
        // 무늬마다 획 알파가 다르다 — stack은 채우므로 더 낮게 앉는다.
        let alpha: Double = pattern.kind == .stack ? 0.10 : 0.16

        context.rotate(by: .degrees(Double(pattern.rotate)))
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
            context.stroke(path, with: .color(stroke.opacity(alpha)), lineWidth: 1.5)
        }
    }
}
