import Foundation

/// 연속 저장 규칙.
///
/// **구현이 셋이고 iOS만 검사 밖이었다.** 웹 `frontend/src/lib/rhythm.ts`와 터미널
/// `scripts/streak.sh`는 `testdata/streak-cases.json`을 같이 읽는데, iOS는 이 함수가
/// `StatsView` 안의 `private func`이라 테스트가 부를 수가 없었다 — 13 §3이 "iOS는 아직"이라고
/// 적어 둔 자리가 여기다. 위젯이 **네 번째 구현이 되기 전에** 꺼냈다.
///
/// 규칙 자체는 옮기면서 한 글자도 바꾸지 않았다.
enum Streak {
    /// 마지막 칸(=오늘)부터 거슬러 올라가며 저장이 있는 날을 센다.
    ///
    /// 오늘을 건너뛰는 이유: 오늘 아직 저장하지 않았다고 어제까지의 연속이 끊긴 것은
    /// 아니다. 자정 직후에 연속이 0으로 보이면 그 지표는 아무도 안 믿는다.
    ///
    /// **날짜 연산이 없다.** 계약이 마지막 칸을 서버 로컬타임 기준 오늘로 보장하므로
    /// (api/openapi.yaml Stats.by_day) 위치로 세면 된다. 예전에는 `DateFormatter`로
    /// 오늘 문자열을 만들어 맞춰 봤는데, 그 포맷터에 로케일이 안 박혀 있어서 비그레고리력
    /// 지역(일본력·불기·민국)에서는 `yyyy`가 연호 연도로 나와 **어떤 날짜도 매칭되지 않고
    /// 연속이 항상 0**이었다. 기기 설정 하나로 조용히 0이 되는 지표였다.
    static func days(_ counts: [Int]) -> Int {
        var i = counts.count - 1
        guard i >= 0 else { return 0 }
        if counts[i] == 0 { i -= 1 }

        var n = 0
        while i >= 0, counts[i] > 0 {
            n += 1
            i -= 1
        }
        return n
    }

    /// 연속이 창 끝까지 닿아 실제 길이를 모르는 경우. `scripts/streak.sh`가 이미 밝히던
    /// 사실이고, 화면만 모르는 척하고 있었다.
    static func isCapped(_ counts: [Int], days: Int) -> Bool {
        days > 0 && days >= counts.count
    }

    /// 오늘 저장이 있었는가. 위젯이 목적지를 정할 때 쓴다(10 §8.6) — 오늘이 비어 있으면
    /// 저장 시트로, 아니면 통계로 보낸다.
    static func savedToday(_ counts: [Int]) -> Bool {
        (counts.last ?? 0) > 0
    }
}
