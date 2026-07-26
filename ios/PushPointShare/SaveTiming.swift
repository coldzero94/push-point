import Foundation
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
    static func end(_ start: Date, outcome: String, tags: Int = 0) {
        let ms = Date().timeIntervalSince(start) * 1000
        let over = ms > budget * 1000
        log.info("저장 \(outcome) \(ms, format: .fixed(precision: 1))ms over=\(over)")
        append([
            "at": ISO8601DateFormatter().string(from: start),
            "ms": (ms * 10).rounded() / 10,
            "outcome": outcome,
            "tags": tags,
            "over": over,
        ])
    }

    /// App Group의 `data/save-timing.jsonl`에 한 줄 덧붙인다.
    ///
    /// 실패해도 조용히 넘어간다. 계측이 저장을 방해하면 본말이 전도된다 — 이 파일이
    /// 없어서 아쉬운 것과 공유가 실패하는 것은 비교 대상이 아니다.
    private static func append(_ record: [String: Any]) {
        guard let dir = AppGroup.dataDirectory(),
              let data = try? JSONSerialization.data(withJSONObject: record),
              var line = String(data: data, encoding: .utf8)
        else { return }
        line += "\n"

        let url = dir.appendingPathComponent("save-timing.jsonl")
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
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
