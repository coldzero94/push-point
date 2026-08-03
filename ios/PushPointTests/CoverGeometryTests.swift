import XCTest

/// 커버가 웹과 **같은 그림**을 그리는지.
///
/// `CoverPatternTests`는 무늬 *파라미터*가 같은지 본다. 그것만으로는 부족했다 — 두
/// 클라이언트가 같은 kind·step·rotate를 받고도 한 달 넘게 서로 다른 그림을 그렸고
/// (웹은 대각 빗금, iOS는 수직선 / 웹은 점 격자, iOS는 모눈 / 웹은 프레임 아래에서 올라온
/// 반원, iOS는 화면 중앙의 온전한 타원), **양쪽 테스트가 전부 통과했다.** 표시 자체를
/// 비교한 것이 없었기 때문이다.
///
/// 기준값은 `testdata/cover-ops.json`이고 웹이 생성한다
/// (`scripts/gen_cover_ops.ts`). 무늬를 고치려면 웹을 고치고 픽스처를 다시 만들어야 하며,
/// 그러면 이 테스트가 빨개져 iOS도 같이 고치게 된다 — 그 강제가 요점이다.
final class CoverGeometryTests: XCTestCase {
    private struct Case: Decodable {
        let domain: String
        let w: Double
        let h: Double
        let kind: String
        let alpha: Double
        let lineWidth: Double
        let rotate: Int
        let mode: String
        let ops: [Op]
    }

    /// 픽스처의 도형은 필드 구성이 종류마다 다르다 — 전부 옵셔널로 받고 `op`로 가른다.
    private struct Op: Decodable {
        let op: String
        let x1: Double?, y1: Double?, x2: Double?, y2: Double?
        let cx: Double?, cy: Double?, r: Double?
        let x: Double?, y: Double?, w: Double?, h: Double?
    }

    private struct File: Decodable { let cases: [Case] }

    private func fixture() throws -> [Case] {
        let url = try XCTUnwrap(
            Bundle(for: Self.self).url(forResource: "cover-ops", withExtension: "json"),
            "cover-ops.json이 테스트 번들에 없다 — project.yml의 resources를 확인할 것")
        return try JSONDecoder().decode(File.self, from: Data(contentsOf: url)).cases
    }

    /// 부동소수 비교 여유. 픽스처는 소수 셋째 자리에서 반올림돼 있다.
    private let eps = 0.002

    func testMarksMatchTheWeb() throws {
        let cases = try fixture()
        XCTAssertEqual(Set(cases.map(\.kind)), ["hatch", "lattice", "contour", "stack"],
                       "네 무늬를 다 덮지 않으면 갈라진 것을 놓친다")

        for c in cases {
            let p = CoverPattern(domain: c.domain)
            XCTAssertEqual(p.kind.rawValue, c.kind, "\(c.domain): 무늬 종류")
            let g = CoverGeometry.make(p, width: CGFloat(c.w), height: CGFloat(c.h))

            XCTAssertEqual(g.alpha, c.alpha, accuracy: eps, "\(c.domain): 알파")
            XCTAssertEqual(Double(g.lineWidth), c.lineWidth, accuracy: eps, "\(c.domain): 획 두께")
            XCTAssertEqual(g.rotate, c.rotate, "\(c.domain): 회전")
            XCTAssertEqual(g.mode.rawValue, c.mode, "\(c.domain): 칠하기/긋기")
            XCTAssertEqual(g.ops.count, c.ops.count,
                           "\(c.domain): 도형 개수가 다르다 — 같은 파라미터로 다른 그림을 그리고 있다")

            for (i, (got, want)) in zip(g.ops, c.ops).enumerated() {
                let where_ = "\(c.domain) #\(i)"
                switch got {
                case let .line(x1, y1, x2, y2):
                    XCTAssertEqual(want.op, "line", where_)
                    XCTAssertEqual(Double(x1), want.x1 ?? .nan, accuracy: eps, "\(where_) x1")
                    XCTAssertEqual(Double(y1), want.y1 ?? .nan, accuracy: eps, "\(where_) y1")
                    XCTAssertEqual(Double(x2), want.x2 ?? .nan, accuracy: eps, "\(where_) x2")
                    XCTAssertEqual(Double(y2), want.y2 ?? .nan, accuracy: eps, "\(where_) y2")
                case let .dot(cx, cy, r):
                    XCTAssertEqual(want.op, "dot", where_)
                    XCTAssertEqual(Double(cx), want.cx ?? .nan, accuracy: eps, "\(where_) cx")
                    XCTAssertEqual(Double(cy), want.cy ?? .nan, accuracy: eps, "\(where_) cy")
                    XCTAssertEqual(Double(r), want.r ?? .nan, accuracy: eps, "\(where_) r")
                case let .arc(cx, cy, r):
                    XCTAssertEqual(want.op, "arc", where_)
                    XCTAssertEqual(Double(cx), want.cx ?? .nan, accuracy: eps, "\(where_) cx")
                    XCTAssertEqual(Double(cy), want.cy ?? .nan, accuracy: eps, "\(where_) cy")
                    XCTAssertEqual(Double(r), want.r ?? .nan, accuracy: eps, "\(where_) r")
                case let .rect(x, y, w, h):
                    XCTAssertEqual(want.op, "rect", where_)
                    XCTAssertEqual(Double(x), want.x ?? .nan, accuracy: eps, "\(where_) x")
                    XCTAssertEqual(Double(y), want.y ?? .nan, accuracy: eps, "\(where_) y")
                    XCTAssertEqual(Double(w), want.w ?? .nan, accuracy: eps, "\(where_) w")
                    XCTAssertEqual(Double(h), want.h ?? .nan, accuracy: eps, "\(where_) h")
                }
            }
        }
    }
}
