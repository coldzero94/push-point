import Foundation
import os
import OSLog

/// 저장 계측 — M4 DoD("공유 탭 → 응답 2초 미만")를 **주장이 아니라 수치로** 만든다.
///
/// 계획(08 §4)이 M4 검증 커맨드로 "시뮬레이터 공유 절차 + 클라이언트 계측 로그"를 지정했는데
/// 그 로그가 없었다. 이 프로젝트의 규칙은 측정 없는 "잘 되는 것 같다"를 금지하므로, 재는
/// 수단이 없으면 2초를 지켰다고 말할 수 없다 — 지키고 있더라도 그렇다.
///
/// **왜 App Group 파일에 쓰는가.** os_log만으로는 부족하다. 확장은 시트가 닫히면서 프로세스가
/// 곧 정리되고, 실기기의 stderr는 버려진다(`ppcore/logger.go`가 같은 이유로 파일에 쓴다).
/// 콘솔.app을 띄워 놓고 공유한 순간에만 볼 수 있는 수치는 "연속 7일" 같은 누적 판정에 쓸 수
/// 없다. 파일이면 나중에 세어 볼 수 있다.
///
/// 한 줄이 JSON 한 건이다(JSON Lines) — `ppcore`의 로그 형식과 같아서 같은 도구로 읽힌다.
enum SaveTiming {
    private static let log = Logger(subsystem: "com.pushpoint.app", category: "timing")

    /// DoD의 경계값. 넘으면 로그에 `over: true`가 박혀 세기만 하면 되도록 한다 —
    /// 판정을 읽는 쪽이 임계값을 다시 알고 있어야 하면 그 판정은 잘 안 쓰인다.
    static let budget: TimeInterval = 2.0

    /// 시작 시각. 확장이 뜬 순간이 곧 "공유 탭"이다 — 사용자가 공유 시트에서 이 앱을
    /// 고른 시점이고, 그 앞은 우리가 어쩌지 못하는 시스템 구간이다.
    static func begin() -> Date { Date() }

    /// 끝. `outcome`은 `saved` / `duplicate` / `failed`.
    ///
    /// 실패도 잰다. 실패가 느린 것은 성공이 느린 것과 **다른 문제**인데(대개 타임아웃),
    /// 성공만 재면 그 구분이 사라지고 평균만 좋아 보인다.
    ///
    /// `source`는 어느 갈래로 들어왔는지다(`SharePayload.Source`). 이걸 남기는 이유는
    /// 본문 캡처가 **사파리에서만** 실려 오기 때문이다 — Chrome·Firefox·SNS 앱은 URL만
    /// 주므로 제목도 태그도 서버 스크랩에 달린다. 저장된 링크만 봐서는 어느 쪽이었는지
    /// 구분할 수 없어서(그런 열이 없다), 캡처 경로가 실사용에서 얼마나 자주 걸리는지
    /// 물었을 때 답할 수가 없었다.
    static func end(_ start: Date, outcome: String, tags: Int = 0, source: String = "unknown") {
        let ms = Date().timeIntervalSince(start) * 1000
        let over = ms > budget * 1000
        let memMB = availableMemoryMB()
        log.info("저장 \(outcome) \(ms, format: .fixed(precision: 1))ms over=\(over) mem=\(memMB)MB src=\(source)")
        append([
            "at": ISO8601DateFormatter().string(from: start),
            "ms": (ms * 10).rounded() / 10,
            "outcome": outcome,
            "tags": tags,
            "over": over,
            "mem_avail_mb": memMB,
            "source": source,
        ])
    }

    /// 저장이 끝난 시점의 **남은** 확장 메모리(MB).
    ///
    /// 계획(08 M4 Week 2)이 확장 메모리를 실기기에서 재확인하라고 요구한다. 선행 검증
    /// 수치(13.4MB 등)는 macOS arm64에서 잰 것이라 iOS 확장의 실제 예산과 같다는 보장이
    /// 없고, 확장은 예산을 넘기면 경고 없이 죽는다 — 사용자에게는 "가끔 저장이 안 된다"로
    /// 보인다. 여기서 재는 이유는 **저장 직후가 최대 사용 시점**이기 때문이다(태그·요약까지
    /// 끝난 뒤다).
    ///
    /// `os_proc_available_memory()`는 확장에서만 의미가 있고 **시뮬레이터에서는 0을 준다.**
    /// 그 0을 숨기지 않는다 — 0이 찍혀 있다는 사실 자체가 "이 값은 아직 실기기에서 재지
    /// 않았다"는 표식이고, 실기기에서 처음 돌리는 날 숫자가 나타나는 것으로 판정이 끝난다.
    private static func availableMemoryMB() -> Int {
        Int(os_proc_available_memory() / (1024 * 1024))
    }

    /// 계측 기록을 남길 디렉터리.
    ///
    /// App Group이 1순위지만, **없으면 확장 자신의 컨테이너로 떨어진다.** 저장 경로는
    /// 그렇게 하지 않는다(`AppGroup.dataDirectory()`가 nil을 그대로 드러내 앱과 확장이
    /// 다른 DB를 보는 상태를 막는다) — 여기만 다르게 구는 이유가 있다.
    ///
    /// 무료 프로비저닝(Personal Team)에서 App Group entitlement가 거부될 수 있다. Apple의
    /// 공식 비교 표는 "Advanced app capabilities"를 유료 전용으로 뭉뚱그릴 뿐 App Groups를
    /// 따로 적지 않아 판정이 갈린다(09-PLAN-REVIEW는 가능하다고 봤으나 근거가 그 모호한
    /// 표다). 그런데 **이 로그가 재려는 것은 App Group과 무관하다** — 확장 프로세스의
    /// 메모리 예산과 걸린 시간이고, 둘 다 저장이 실패해도 유효한 수치다.
    ///
    /// 그래서 저장이 못 되는 기기에서도 "왜 못 되는지"와 "메모리는 얼마였는지"는 남는다.
    /// 실패했다는 사실까지 잃으면 실기기 판정 자체가 불가능해진다.
    private static func recordDirectory() -> (url: URL, shared: Bool)? {
        if let dir = AppGroup.dataDirectory() { return (dir, true) }
        guard let fallback = FileManager.default
            .urls(for: .applicationSupportDirectory, in: .userDomainMask).first
        else { return nil }
        return (fallback.appendingPathComponent("pushpoint", isDirectory: true), false)
    }

    /// 계측 한 줄을 덧붙인다.
    ///
    /// 실패해도 조용히 넘어간다. 계측이 저장을 방해하면 본말이 전도된다 — 이 파일이
    /// 없어서 아쉬운 것과 공유가 실패하는 것은 비교 대상이 아니다.
    private static func append(_ record: [String: Any]) {
        guard let (dir, shared) = recordDirectory() else { return }
        var record = record
        // 어느 컨테이너에 썼는지 남긴다. false면 App Group이 안 열렸다는 뜻이고,
        // 그 자체가 무료 프로비저닝에서 가장 알고 싶은 한 가지다.
        record["app_group"] = shared
        guard let data = try? JSONSerialization.data(withJSONObject: record),
              var line = String(data: data, encoding: .utf8)
        else { return }
        line += "\n"

        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
        let url = dir.appendingPathComponent("save-timing.jsonl")
        // 이어쓰기. 확장은 여러 번 뜨므로 덮어쓰면 마지막 한 건만 남는다.
        if let handle = try? FileHandle(forWritingTo: url) {
            defer { try? handle.close() }
            _ = try? handle.seekToEnd()
            try? handle.write(contentsOf: Data(line.utf8))
        } else {
            try? Data(line.utf8).write(to: url)
        }
    }
}
