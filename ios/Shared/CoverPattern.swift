import Foundation

/// 생성 커버(R4) — "빈칸을 만들지 않는다, 없으면 생성한다".
///
/// `thumb: failed` + `status: done`은 계약상 **정상** 조합이다. 그래서 og:image에 기대는
/// 화면은 회색 박스 밭으로 무너지고, 기대지 않으면 이니셜 하나로 끝난다. 대신 썸네일이
/// 없는 링크는 우리가 확실히 아는 것 — 지배 태그의 facet과 도메인 — 으로 커버를 그린다.
///
/// **넘지 말아야 할 경계: hue는 해시에서 오지 않는다.** 바탕과 획 색은 그 태그의 칩이
/// 쓰는 것과 정확히 같은 facet tint/ink이고, 해시가 고르는 것은 **기하뿐**이다(무늬 4종,
/// 회전, 밀도). 그래야 §5.4의 "태그 색상 해시 금지"가 유지된다 — 두 링크가 닮아 보이는
/// 이유는 같은 주제라서지, 해시가 색을 지어냈기 때문이 아니어야 한다.
///
/// 웹(`frontend/src/lib/covers.ts`)과 **같은 FNV-1a 해시를 쓴다.** 같은 도메인이 두
/// 클라이언트에서 같은 무늬로 나와야 커버가 그 출처의 표식이 된다. 값이 갈라지면 그
/// 표식이 무의미해지므로 `CoverPatternTests`가 웹에서 계산한 기준값으로 고정한다.
struct CoverPattern: Equatable {
    enum Kind: String, CaseIterable {
        case hatch, lattice, contour, stack
    }

    let kind: Kind
    /// -2...2도. 무늬가 기계적으로 반복돼 보이지 않을 만큼만.
    let rotate: Int
    /// 12...28px — 무늬 밀도.
    let step: Int
    /// 0...4 — 무늬 종류 안의 변형.
    let variant: Int

    /// 도메인 → 기하. 순수 함수이고 의도적으로 무채색이다.
    init(domain: String) {
        let seed = Self.hash(domain)
        let kinds = Kind.allCases
        kind = kinds[Int(seed % UInt32(kinds.count))]
        rotate = Int((seed >> 4) % 5) - 2
        step = 12 + Int((seed >> 8) % 5) * 4
        variant = Int((seed >> 12) % 5)
    }

    /// FNV-1a 32비트. 웹의 `hashDomain`과 **바이트 단위까지 같아야 한다.**
    ///
    /// JS는 `charCodeAt`로 UTF-16 코드 유닛을 먹이므로 여기서도 `unicodeScalars`가 아니라
    /// `utf16`을 쓴다 — ASCII 도메인에서는 같지만 IDN(퓨니코드로 변환되지 않은 한글
    /// 도메인 등)에서 갈라진다.
    static func hash(_ domain: String) -> UInt32 {
        var h: UInt32 = 2_166_136_261
        for unit in domain.utf16 {
            h ^= UInt32(unit)
            h = h &* 16_777_619
        }
        return h
    }
}
