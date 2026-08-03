import CoreGraphics
import Foundation

/// 커버 무늬를 **도형 목록**으로 만든다. 웹 `frontend/src/lib/covers.ts`의
/// `coverGeometry()`와 같은 함수이고, 같은 입력에 같은 목록을 내야 한다.
///
/// **왜 그리기 코드에서 떼어 냈는가.** 두 클라이언트는 무늬 *파라미터*(kind/step/rotate/
/// variant)를 `testdata/cover-cases.json`으로 맞춰 두고도, 그 파라미터로 **완전히 다른
/// 그림**을 그리고 있었다 — 웹은 대각선으로 빗금 치는데 iOS는 수직선을 긋고, 웹은 점을
/// 찍어 격자를 만드는데 iOS는 선을 그어 모눈을 만들고, 웹은 프레임 아래에서 반원을
/// 올리는데 iOS는 화면 한가운데에 온전한 타원을 그렸다. 양쪽 테스트가 전부 통과했다.
/// **표시(mark)를 비교한 것이 아무것도 없었기 때문이다.**
///
/// `testdata/cover-ops.json`이 이제 그것을 비교한다. 이 파일은 그 픽스처가 겨냥할 수 있는
/// 순수 함수이고, 그리기(`GeneratedCover`)는 목록을 받아 칠하기만 한다.
enum CoverGeometry {
    enum Op: Equatable {
        case line(x1: CGFloat, y1: CGFloat, x2: CGFloat, y2: CGFloat)
        /// 채워진 점.
        case dot(cx: CGFloat, cy: CGFloat, r: CGFloat)
        /// **아래쪽 반원만**(π → 2π). contour가 그리는 것이 그것이다.
        case arc(cx: CGFloat, cy: CGFloat, r: CGFloat)
        case rect(x: CGFloat, y: CGFloat, w: CGFloat, h: CGFloat)
    }

    enum Mode: String { case stroke, fill }

    struct Result {
        let alpha: Double
        let lineWidth: CGFloat
        /// 도(degree). 그리기 전에 상자 중심을 기준으로 적용한다.
        let rotate: Int
        let mode: Mode
        let ops: [Op]
    }

    /// 웹의 `ALPHA`와 같은 값이어야 한다 — stack은 채우므로 더 낮게 앉는다.
    private static func alpha(_ kind: CoverPattern.Kind) -> Double {
        kind == .stack ? 0.13 : 0.16
    }

    static let lineWidth: CGFloat = 1.25

    static func make(_ pattern: CoverPattern, width w: CGFloat, height h: CGFloat) -> Result {
        let step = CGFloat(pattern.step)
        // 회전한 뒤에도 모서리가 비지 않도록 상자 밖까지 그린다.
        let reach = (w * w + h * h).squareRoot()
        var ops: [Op] = []

        switch pattern.kind {
        case .hatch:
            var x = -reach
            while x < reach * 2 {
                ops.append(.line(x1: x, y1: -reach, x2: x + h + reach, y2: reach * 2))
                x += step
            }
        case .lattice:
            let r = max(1.4, step / 8)
            var y = step / 2
            while y < h + step {
                // 한 줄 걸러 반 칸씩 밀린다 — 모눈이 아니라 격자다
                let offset = CGFloat(Int((y / step).rounded(.down)) % 2) * (step / 2)
                var x = step / 2
                while x < w + step {
                    ops.append(.dot(cx: x + offset, cy: y, r: r))
                    x += step
                }
                y += step
            }
        case .contour:
            let cx = w * (0.18 + CGFloat(pattern.variant) * 0.16)
            let cy = h * 1.02
            var r = step
            while r < reach {
                ops.append(.arc(cx: cx, cy: cy, r: r))
                r += step
            }
        case .stack:
            let s = step * 1.4
            var i: CGFloat = 0
            while i * s < w + h {
                ops.append(.rect(x: i * s - h, y: i * s * 0.55, w: s * 0.62, h: h * 2))
                i += 1
            }
        }

        return Result(
            alpha: alpha(pattern.kind),
            lineWidth: lineWidth,
            rotate: pattern.rotate,
            mode: (pattern.kind == .lattice || pattern.kind == .stack) ? .fill : .stroke,
            ops: ops
        )
    }
}
