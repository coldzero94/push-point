import XCTest

/// 생성 커버가 웹과 **같은 무늬**를 내야 한다.
///
/// 커버는 자주 저장하는 출처의 표식이 되라고 만든 것이다(§4.5). 같은 도메인이 웹과 iOS에서
/// 다른 무늬로 나오면 그 표식이 무의미해지는데, 두 구현이 갈라져도 **양쪽 다 정상 동작하는
/// 것처럼 보이므로** 눈으로는 잡히지 않는다. 그래서 기준값을 박아 둔다.
///
/// 아래 값은 `frontend/src/lib/covers.ts`의 알고리즘으로 계산한 것이다. 이 테스트가 깨지면
/// 둘 중 하나가 움직인 것이므로, 고치기 전에 **어느 쪽이 옳은지부터** 정해야 한다.
final class CoverPatternTests: XCTestCase {
    private struct Expected {
        let seed: UInt32
        let kind: CoverPattern.Kind
        let rotate: Int
        let step: Int
        let variant: Int
    }

    private let golden: [String: Expected] = [
        "go.dev": .init(seed: 1_169_986_798, kind: .contour, rotate: 2, step: 12, variant: 1),
        "m.blog.naver.com": .init(seed: 1_580_799_633, kind: .lattice, rotate: 0, step: 24, variant: 2),
        "n.news.naver.com": .init(seed: 3_890_673_925, kind: .lattice, rotate: -2, step: 12, variant: 1),
        "github.com": .init(seed: 1_228_281_881, kind: .lattice, rotate: 0, step: 16, variant: 3),
        "example.com": .init(seed: 1_125_968_678, kind: .contour, rotate: 0, step: 12, variant: 4),
    ]

    func testMatchesWebImplementation() {
        for (domain, want) in golden {
            XCTAssertEqual(CoverPattern.hash(domain), want.seed,
                           "\(domain): FNV-1a 해시가 웹과 다르다")
            let got = CoverPattern(domain: domain)
            XCTAssertEqual(got.kind, want.kind, "\(domain): 무늬 종류")
            XCTAssertEqual(got.rotate, want.rotate, "\(domain): 회전")
            XCTAssertEqual(got.step, want.step, "\(domain): 밀도")
            XCTAssertEqual(got.variant, want.variant, "\(domain): 변형")
        }
    }

    /// 같은 도메인은 항상 같은 커버여야 한다 — 표식이 되려면 안정성이 전제다.
    func testIsDeterministic() {
        let first = CoverPattern(domain: "go.dev")
        for _ in 0 ..< 50 {
            XCTAssertEqual(CoverPattern(domain: "go.dev"), first)
        }
    }

    /// 값의 범위는 그리기 코드가 의존하는 계약이다.
    func testValuesStayInRange() {
        for domain in ["a", "", "very.long.subdomain.example.co.kr", "한글.테스트", "x.io"] {
            let p = CoverPattern(domain: domain)
            XCTAssertTrue((-2 ... 2).contains(p.rotate), "\(domain): 회전 범위 벗어남 \(p.rotate)")
            XCTAssertTrue((12 ... 28).contains(p.step), "\(domain): 밀도 범위 벗어남 \(p.step)")
            XCTAssertTrue((0 ... 4).contains(p.variant), "\(domain): 변형 범위 벗어남 \(p.variant)")
        }
    }

    /// JS의 charCodeAt은 UTF-16 코드 유닛이다. 스칼라 단위로 해시하면 ASCII에서는
    /// 같지만 BMP 밖 문자에서 갈라지므로, 그 차이가 실제로 있는지 확인해 둔다.
    func testUsesUTF16CodeUnits() {
        // "가"(U+AC00)는 UTF-16 한 유닛, UTF-8 세 바이트다. 바이트로 해시했다면 다른 값이 된다.
        XCTAssertEqual(CoverPattern.hash("가"), {
            var h: UInt32 = 2_166_136_261
            h ^= 0xAC00
            h = h &* 16_777_619
            return h
        }(), "UTF-16 코드 유닛으로 해시해야 웹과 일치한다")
    }
}
