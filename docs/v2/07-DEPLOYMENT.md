# 배포·운영

> Push-Point v2.1 — 마지막 업데이트: 2026-07-20

v2의 배포 단위는 단일 Go 바이너리 하나다. v1이 요구하던 Docker Desktop, kubectl, Minikube, Helm은 전부 필요 없다. 이 문서는 로컬 실행부터 집 Mac 상시 구동, iPhone에서의 접근, 단축어 캡처, 백업, 관측까지 운영에 필요한 전부를 다룬다. v1의 k8s 매니페스트는 삭제하지 않고 `deploy/k8s-future/`에 보존했다 (마지막 섹션 참고).

---

## 1. 로컬 실행

### 요구사항

- Go 1.25+
- just (`brew install just`)
- iOS 실기기 매일 사용(M4+): **Apple Developer Program ($99/년)** — App Groups·Keychain 공유 자체는 무료 계정으로도 가능하지만, 무료 계정의 프로비저닝 프로파일은 7일마다 만료되어 매일 쓰는 앱과 양립할 수 없다. 가입은 M4 Week 1 태스크다.

서버 측은 이게 전부다. DB 드라이버가 CGO-free(`modernc.org/sqlite`)이므로 별도 C 툴체인도, 컨테이너 런타임도 필요 없다.

### 실행

```bash
just dev
# = cd backend && PUSHPOINT_API_KEY=dev-key go run ./cmd/pushpoint
```

루트 justfile의 Go 레시피는 `backend/` 디렉터리에서 실행된다 (`cd backend && ...`). 모노레포 분리는 저장소 배치일 뿐, 배포 단위는 여전히 단일 바이너리다.

첫 실행 시 자동으로 처리되는 것:

- `data/` 디렉터리 생성 (`data/pushpoint.db`, `data/thumbs/`)
- 마이그레이션 적용 — `backend/migrations/`가 golang-migrate + `embed.FS`로 바이너리에 내장돼 있어 시작 시 자동 실행. 별도 마이그레이션 커맨드가 없다.
- 크래시 복구 — 시작 시 `running` 상태로 남은 잡을 `pending`으로 되돌려 재개

콜드 스타트(실행 → 서빙) 목표는 1초 미만이다.

### 스모크 테스트

```bash
# 헬스체크 (인증 없음)
curl http://localhost:8080/healthz
# {"status":"ok"}

# 링크 저장
curl -X POST http://localhost:8080/api/v1/links \
  -H "Authorization: Bearer dev-key" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://go.dev/blog/wal"}'
# 201 {"id":1,"status":"pending","created_at":...}

# 몇 초 후 목록 조회 — 스크랩·태깅이 끝났으면 status가 done
curl -H "Authorization: Bearer dev-key" \
  "http://localhost:8080/api/v1/links?limit=5"
```

---

## 2. 환경 변수

접두어 `PUSHPOINT_`, 표준 `os.Getenv`로 읽는다 (viper 없음).

| 변수 | 기본값 | 설명 |
|---|---|---|
| `PUSHPOINT_ADDR` | `:8080` | HTTP 리슨 주소 |
| `PUSHPOINT_DATA_DIR` | `./data` | SQLite DB·썸네일 저장 디렉터리 |
| `PUSHPOINT_API_KEY` | (없음, 필수) | Bearer 인증 키. `just dev`는 `dev-key`로 설정 |
| `PUSHPOINT_SCRAPE_CONCURRENCY` | `8` | 스크래퍼 워커 동시 실행 상한 |
| `PUSHPOINT_LOG_LEVEL` | `info` | slog 로그 레벨 (`debug`/`info`/`warn`/`error`) |

실사용 구동 시에는 `PUSHPOINT_API_KEY`를 충분히 긴 랜덤 문자열로 교체할 것 (`openssl rand -hex 32` 등).

---

## 3. 상시 구동

실사용(M2 종료 시점부터 — 단축어 캡처가 시작되는 순간)은 집 Mac에서 서버를 항상 켜 두는 형태다. 먼저 빌드한다.

```bash
just build   # = cd backend && go build -o bin/pushpoint ./cmd/pushpoint → backend/bin/pushpoint
```

M5 각주: ONNX 채택 형태에 따라 배포물이 단일 바이너리가 아니라 바이너리 + `libonnxruntime.dylib` 구성이 될 수 있다 ([02-TECH-SPEC.md](02-TECH-SPEC.md)의 M5 3택 결정 — dylib embed / hugot 순수 Go / Phase A 유지). launchd 절차에 영향을 주는 이 결정은 M5 Week 1에 내린다. M1~M4 기준으로는 이 문서 그대로 단일 정적 바이너리다.

### macOS: launchd

`~/Library/LaunchAgents/ai.pushpoint.server.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"
  "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>ai.pushpoint.server</string>
  <key>ProgramArguments</key>
  <array>
    <string>/Users/coby/chanyoung/push-point/backend/bin/pushpoint</string>
  </array>
  <key>WorkingDirectory</key>
  <string>/Users/coby/chanyoung/push-point</string>
  <key>EnvironmentVariables</key>
  <dict>
    <key>PUSHPOINT_API_KEY</key>
    <string>CHANGE-ME-long-random-string</string>
    <key>PUSHPOINT_DATA_DIR</key>
    <string>/Users/coby/chanyoung/push-point/data</string>
  </dict>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>/tmp/pushpoint.log</string>
  <key>StandardErrorPath</key>
  <string>/tmp/pushpoint.err.log</string>
</dict>
</plist>
```

```bash
launchctl load ~/Library/LaunchAgents/ai.pushpoint.server.plist   # 등록·시작
launchctl unload ~/Library/LaunchAgents/ai.pushpoint.server.plist # 중지
```

`KeepAlive`가 크래시 시 자동 재시작을 맡는다. 재시작해도 jobs 테이블 덕에 미처리 잡은 유실되지 않는다 ([04-DATA-FLOW.md](04-DATA-FLOW.md) 참고).

### 전원·절전 (필수)

launchd 등록만으로는 부족하다. 맥이 잠들면 서버도 함께 멈추고, **잠든 맥은 Tailscale로 깨울 수 없다** — Wake-on-LAN 매직 패킷은 WireGuard 터널로 전달되지 않는다. 서버 맥은 아예 잠들지 않게 설정한다.

```bash
sudo pmset -a sleep 0        # 시스템 잠자기 비활성화
sudo pmset -a autorestart 1  # 정전 복구 시 자동 재부팅
```

- 자동 로그인 활성화 (시스템 설정 → 사용자 및 그룹) + `RunAtLoad` 확인: LaunchAgent는 로그인 세션에서 돌므로, 재부팅 후 자동 로그인과 `RunAtLoad`가 맞물려야 무인 복구가 완성된다.
- 외부 업타임 체크: 외부 모니터링에서 주기적으로 `/healthz`를 호출해 다운을 알림으로 받는다.

### Linux: systemd

`/etc/systemd/system/pushpoint.service`:

```ini
[Unit]
Description=Push-Point server
After=network.target

[Service]
User=pushpoint
WorkingDirectory=/opt/push-point
ExecStart=/opt/push-point/backend/bin/pushpoint
Environment=PUSHPOINT_API_KEY=CHANGE-ME-long-random-string
Environment=PUSHPOINT_DATA_DIR=/opt/push-point/data
Restart=always
RestartSec=2

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now pushpoint
journalctl -u pushpoint -f   # 로그 확인
```

---

## 4. iPhone에서 서버 접근 (M1부터 필요)

iOS 단축어(M1부터)와 Share Extension·앱(M4부터)이 집 Mac의 서버로 `POST /api/v1/links`를 보내려면 외부에서 접근할 경로가 필요하다.

### 권장: Tailscale

1. Mac과 iPhone에 Tailscale 설치 후 같은 계정으로 로그인
2. Mac의 Tailscale IP 확인: `tailscale ip -4` → `100.x.y.z`
3. iOS 클라이언트(단축어·앱·Share Extension)의 서버 주소는 **IP 형식만** 사용: `http://100.x.y.z:8080`
4. iOS 앱(M4+)에서 Tailscale **VPN On Demand**를 Wi-Fi/Cellular 모두 **Always**로 설정 — 필수 단계. 이걸 켜지 않으면 VPN이 내려간 상태에서 Share Extension의 POST가 실패한다.

**ATS 노트 (서버 주소가 IP 형식이어야 하는 이유)**: iOS App Transport Security는 호스트네임에 대한 평문 HTTP를 차단하지만 IP 리터럴은 면제다. MagicDNS 이름(`mac.tailnet-xxx.ts.net`)을 쓰려면 `tailscale cert`로 HTTPS를 구성해야 한다 — **평문 HTTP + 호스트네임 조합은 금지**. ATS 예외를 넣는 경우에는 앱과 Share Extension **양쪽** Info.plist에 모두 넣어야 한다 (Extension은 별도 타깃이라 앱의 설정을 상속하지 않는다).

공유기 포트 포워딩이 전혀 없고, 모든 트래픽이 WireGuard로 암호화되며, 셀룰러에서도 집 Mac에 도달한다. 서버를 공인 인터넷에 노출하지 않으므로 API 키 단일 인증([02-TECH-SPEC.md](02-TECH-SPEC.md))으로 충분한 근거가 된다.

### 대안: LAN 고정 IP

같은 Wi-Fi에서만 쓴다면 공유기에서 Mac에 DHCP 고정 IP(예: `192.168.0.10`)를 할당하고 iOS 앱에 `http://192.168.0.10:8080`을 설정한다. 셀룰러·외부 네트워크에서는 저장이 안 되는 한계가 있으므로 실사용에는 Tailscale을 권한다.

---

## 5. iOS 단축어로 캡처 (M1부터)

M4 앱이 나오기 전까지의 **공식 캡처 경로**다. 단축어 앱으로 공유 시트에서 바로 저장하며, M1 DoD에 "폰 단축어로 실제 저장 1건"이 포함된다 ([08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)).

단축어 앱에서 새 단축어를 만든다:

1. 단축어 세부사항에서 **공유 시트에 표시** 활성화, 입력 유형은 URL
2. **"URL 콘텐츠 가져오기"** 액션을 추가하고 다음처럼 설정:
   - URL: `http://100.x.y.z:8080/api/v1/links` (Mac의 Tailscale IP)
   - 방법: POST
   - 헤더: `Authorization` = `Bearer {PUSHPOINT_API_KEY 값}`
   - 본문 요청: JSON, 필드 `url` = (매직 변수) **단축어 입력**
3. Safari·YouTube 등에서 공유 시트 → 이 단축어 선택 → 저장 완료

서버가 `201 {"id":...}`를 반환하면 성공이다. `POST /api/v1/links`는 url_hash 기준 멱등이므로 실수로 두 번 실행해도 중복 저장되지 않는다 (`200 {duplicate:true}`).

---

## 6. 백업·복원

데이터는 전부 `data/` 아래에 있다: `pushpoint.db`(+`-wal`, `-shm`)와 `thumbs/`. 링크 10만 건 기준 DB 약 150MB + 썸네일 약 3GB 규모다.

### 백업 (프로세스 중지 불필요)

```bash
# 방법 1: SQLite 온라인 백업 — 서버 구동 중에도 일관된 스냅샷
sqlite3 data/pushpoint.db ".backup 'backup/pushpoint-$(date +%Y%m%d).db'"

# 방법 2: 디렉터리 통째 복사 (썸네일 포함)
rsync -a data/ backup/data-$(date +%Y%m%d)/
```

WAL 주의: DB 파일만 단순 복사할 때는 `-wal`, `-shm` 파일을 반드시 함께 복사해야 최신 쓰기가 보존된다 (`.backup`은 알아서 병합해 준다).

### 복원

서버를 중지하고 백업 파일을 제자리로 되돌린 뒤 다시 시작하면 끝이다.

```bash
launchctl unload ~/Library/LaunchAgents/ai.pushpoint.server.plist
cp backup/pushpoint-20260720.db data/pushpoint.db
rm -f data/pushpoint.db-wal data/pushpoint.db-shm   # 이전 WAL 잔여물 제거
launchctl load ~/Library/LaunchAgents/ai.pushpoint.server.plist
```

이것이 v1 대비 가장 체감되는 운영 단순화다: PostgreSQL 덤프·PVC 스냅샷 대신 파일 복사.

---

## 7. 관측

### 헬스체크

```bash
curl http://localhost:8080/healthz
# {"status":"ok"}
```

인증이 없으므로 launchd/systemd 외부의 간단한 모니터링 스크립트나 Uptime 체크에 바로 쓸 수 있다.

### 프로파일링 (/debug/pprof)

`net/http/pprof`가 기본 탑재돼 있다.

```bash
# CPU 프로파일 30초 수집 후 인터랙티브 분석
go tool pprof http://localhost:8080/debug/pprof/profile?seconds=30

# 힙 스냅샷
go tool pprof http://localhost:8080/debug/pprof/heap

# goroutine 덤프 (워커 풀 상태 확인에 유용)
curl http://localhost:8080/debug/pprof/goroutine?debug=1
```

### 로그

표준 `log/slog` JSON 핸들러로 출력한다. `jq`로 바로 필터링 가능하다.

```bash
tail -f /tmp/pushpoint.log | jq 'select(.level == "ERROR")'
```

레벨은 `PUSHPOINT_LOG_LEVEL`로 조절한다 (기본 `info`).

### 성능·복구 게이트

```bash
just bench            # 마이크로벤치: cd backend && go test -bench=. -benchmem ./...
just bench-http       # HTTP 경로 저장 p99 측정 — p99 < 50ms 미달 시 exit 1 (M1)
scripts/coldstart.sh  # 실행 → /healthz 200 응답까지 1초 미만 검증 (M1)
just test-crash       # 빌드 → fixture 서버 → 저장 → kill -9 → 재기동 → 전량 done 단언 (M2)
```

p99 판정은 `just bench-http`가 담당한다 — go test 벤치는 평균만 내므로 p99 판정 수단이 아니다. 검색(1만 링크) < 30ms, 10만 건 목록 < 50ms 등 [02-TECH-SPEC.md](02-TECH-SPEC.md)의 목표치를 매 마일스톤마다 검증한다 (마일스톤별 검증 매트릭스는 [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)). 수치 없는 "빨라진 것 같다"는 인정하지 않는다.

---

## 8. deploy/k8s-future/ — 보존된 v1 매니페스트

v1의 Kubernetes 매니페스트는 삭제하지 않고 `deploy/k8s-future/`로 옮겨 보존했다. **지금 접는 것이지 버리는 것이 아니다.**

보존 파일:

```
deploy/k8s-future/
├── namespace.yaml
├── configmap.yaml
├── secret.yaml
├── postgresql.yaml
├── redis.yaml
├── minio.yaml
├── api-server.yaml
├── worker.yaml
└── hpa.yaml
```

### 왜 접었나

v1은 유저 0명 상태에서 Minikube + k8s + HPA + PostgreSQL + Redis + MinIO를 세웠다. 오토스케일링할 트래픽이 없는데 오토스케일러부터 만드는 역설계였고, 로컬 테스트 한 번에 클러스터 기동이 필요한 마찰이 개발 속도를 갉아먹었다. v2는 그 비용을 전부 제품(스크래퍼·NLU 태깅·iOS)에 쓴다.

### 부활 조건과 교체 지점

부활 조건은 명확하다: **외부 유저가 생기는 것.** 그 시점에 코드 구조는 이미 준비돼 있다 — Store/Queue/Tagger가 인터페이스 뒤에 있으므로 ([03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md)) 구현체만 교체하면 된다.

| 교체 지점 | v2 (현재) | 부활 시 |
|---|---|---|
| `Store` | SQLite (WAL) | PostgreSQL — `postgresql.yaml` |
| `Queue` | SQLite jobs 테이블 + goroutine | Redis — `redis.yaml` |
| 썸네일 저장 | 로컬 디스크 `data/thumbs/` | S3 호환 스토리지 — `minio.yaml` |
| 프로세스 구성 | 단일 바이너리 (API + worker) | `api-server.yaml` / `worker.yaml` 분리 배포 + `hpa.yaml` |

그때까지 이 디렉터리는 어떤 빌드·실행 경로에도 관여하지 않는다.

---

## 관련 문서

- [02-TECH-SPEC.md](02-TECH-SPEC.md) — 기술 스택·성능 목표
- [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md) — 단일 프로세스 구조와 인터페이스 경계
- [04-DATA-FLOW.md](04-DATA-FLOW.md) — 잡 큐 동작·크래시 복구
- [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) — API 명세
- [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md) — 마일스톤
