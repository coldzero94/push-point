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

    /// 도형 목록을 받아 칠하기만 한다 — 모양 결정은 전부 `CoverGeometry`에 있고,
    /// 그래야 `testdata/cover-ops.json`이 웹과 대조할 수 있다.
    private func draw(_ pattern: CoverPattern, in context: inout GraphicsContext, size: CGSize) {
        let g = CoverGeometry.make(pattern, width: size.width, height: size.height)

        // **중심을 기준으로 회전한다** — 웹이 그렇게 한다(covers.ts는 캔버스 중앙으로
        // 옮겼다가 회전하고 되돌린다). 원점 기준으로 돌리면 같은 각도라도 무늬가 다른
        // 자리에 놓인다.
        context.translateBy(x: size.width / 2, y: size.height / 2)
        context.rotate(by: .degrees(Double(g.rotate)))
        context.translateBy(x: -size.width / 2, y: -size.height / 2)

        var path = Path()
        for op in g.ops {
            switch op {
            case let .line(x1, y1, x2, y2):
                path.move(to: CGPoint(x: x1, y: y1))
                path.addLine(to: CGPoint(x: x2, y: y2))
            case let .dot(cx, cy, r):
                path.addEllipse(in: CGRect(x: cx - r, y: cy - r, width: r * 2, height: r * 2))
            case let .arc(cx, cy, r):
                // 아래쪽 반원. SwiftUI의 y축은 아래로 커지므로 웹의 π→2π(캔버스 기준
                // 위쪽 절반이 아니라 **화면 위로 솟은 호**)와 같은 그림이 되려면
                // 180°→360° 구간을 그린다.
                path.addArc(center: CGPoint(x: cx, y: cy), radius: r,
                            startAngle: .degrees(180), endAngle: .degrees(360),
                            clockwise: false)
            case let .rect(x, y, w, h):
                path.addRect(CGRect(x: x, y: y, width: w, height: h))
            }
        }

        let ink = facet.ink.opacity(g.alpha)
        if g.mode == .fill {
            context.fill(path, with: .color(ink))
        } else {
            context.stroke(path, with: .color(ink), lineWidth: g.lineWidth)
        }
    }
}
