# 배포·운영

> Push-Point v2.1 — 마지막 업데이트: 2026-07-21

v2의 배포 단위는 단일 Go 바이너리 하나다. v1이 요구하던 Docker Desktop, kubectl, Minikube, Helm은 전부 필요 없다. 이 문서는 로컬 실행부터 집 Mac 상시 구동, iPhone에서의 접근, 단축어 캡처, 백업, 관측까지 운영에 필요한 전부를 다룬다. v1의 k8s 매니페스트는 삭제하지 않고 `deploy/k8s-future/`에 보존했다 (마지막 섹션 참고).

---

## 1. 로컬 실행

### 요구사항

- Go 1.25+
- just (`brew install just`)
- iOS 실기기 매일 사용(M4+): **Apple Developer Program ($99/년)** — 무료 계정의 프로비저닝 프로파일은 7일마다 만료되어 매일 쓰는 앱과 양립할 수 없다.

  **무료 프로비저닝(Personal Team)으로 먼저 가는 경로 (2026-07-26 채택).** 7일 만료는 매일
  사용을 막을 뿐 **1회성 실측은 막지 않는다.** 실기기가 필요한 판정 두 가지 — 확장 메모리
  실측과 `0xdead10cc`(App Group 파일 락을 쥔 채 서스펜드) — 는 폰 한 대만 있으면 $99 없이
  끝난다. $99가 실제로 필요해지는 것은 M4 DoD의 "연속 7일"과 M6의 28일 스트릭뿐이다.

  절차: Xcode → Settings → Accounts 에 Apple ID 로그인(자격증명 입력이라 자동화 불가) →
  폰 USB 연결 후 신뢰 승인 → `just ios-teams`로 팀 ID 확인 → `just ios-device <TEAMID>`.

  **검증되지 않은 전제 하나: 무료 팀에서 App Groups가 되는가.** [09-PLAN-REVIEW.md](09-PLAN-REVIEW.md)는
  "공식 표상 가능"으로 반박 처리했지만, 그 표는 App Groups를 따로 적지 않고 "Advanced app
  capabilities"로 뭉뚱그릴 뿐이라 근거가 약하다. 막히면 앱과 확장이 다른 DB를 보게 되어
  저장 경로 전체가 죽는다. 그래서 확장의 계측(`SaveTiming`)은 App Group이 안 열리면 자기
  컨테이너로 떨어지도록 해 뒀다 — **저장이 실패해도 메모리 수치와 실패 사실은 남는다.**
  `just save-timing`이 그 결과(`app_group: false`)를 명시적으로 알린다.

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
curl http://localhost:8420/healthz
# {"status":"ok"}

# 링크 저장
curl -X POST http://localhost:8420/api/v1/links \
  -H "Authorization: Bearer dev-key" \
  -H "Content-Type: application/json" \
  -d '{"url": "https://go.dev/blog/wal"}'
# 201 {"id":1,"status":"pending","created_at":...}

# 몇 초 후 목록 조회 — 스크랩·태깅이 끝났으면 status가 done
curl -H "Authorization: Bearer dev-key" \
  "http://localhost:8420/api/v1/links?limit=5"
# keyset 커서 페이지네이션 — 응답의 next_cursor를 ?cursor= 로 넘긴다

# 검색 — FTS5 trigram 전문 검색, bm25 랭킹
curl -H "Authorization: Bearer dev-key" \
  "http://localhost:8420/api/v1/search?q=kubernetes"
```

`q`가 3자 이상이면 FTS5 trigram MATCH + bm25 랭킹(`"mode":"fts"`), 3자 미만이면 400이 아니라 LIKE 폴백으로 동작한다(`"mode":"like"`) — 상세는 [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md).

---

## 2. 환경 변수

접두어 `PUSHPOINT_`, 표준 `os.Getenv`로 읽는다 (viper 없음).

| 변수 | 기본값 | 설명 |
|---|---|---|
| `PUSHPOINT_ADDR` | `:8420` | HTTP 리슨 주소 |
| `PUSHPOINT_DATA_DIR` | `./data` | SQLite DB·썸네일 저장 디렉터리 |
| `PUSHPOINT_API_KEY` | (없음, 필수) | Bearer 인증 키. `just dev`는 `dev-key`로 설정 |
| `PUSHPOINT_SCRAPE_CONCURRENCY` | `8` | 스크래퍼 워커 동시 실행 상한 |
| `PUSHPOINT_LOG_LEVEL` | `info` | slog 로그 레벨 (`debug`/`info`/`warn`/`error`) |
| `PUSHPOINT_LOG_FORMAT` | `auto` | 로그 출력 형식. `text`(사람이 읽는 컬러)·`json`(구조화·`jq` 파싱)·`auto`(stderr가 터미널이면 text, 아니면 json). `just dev`는 `text`를 강제한다 |
| `PUSHPOINT_ALLOW_PRIVATE_HOSTS` | `false` | `true`면 스크랩·썸네일의 사설 대역 차단(SSRF 가드)을 해제 — 로컬 fixture 테스트 전용 |

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
3. iOS 클라이언트(단축어·앱·Share Extension)의 서버 주소는 **IP 형식만** 사용: `http://100.x.y.z:8420`
4. iOS 앱(M4+)에서 Tailscale **VPN On Demand**를 Wi-Fi/Cellular 모두 **Always**로 설정 — 필수 단계. 이걸 켜지 않으면 VPN이 내려간 상태에서 Share Extension의 POST가 실패한다.

**ATS 노트 (서버 주소가 IP 형식이어야 하는 이유)**: iOS App Transport Security는 호스트네임에 대한 평문 HTTP를 차단하지만 IP 리터럴은 면제다. MagicDNS 이름(`mac.tailnet-xxx.ts.net`)을 쓰려면 `tailscale cert`로 HTTPS를 구성해야 한다 — **평문 HTTP + 호스트네임 조합은 금지**. ATS 예외를 넣는 경우에는 앱과 Share Extension **양쪽** Info.plist에 모두 넣어야 한다 (Extension은 별도 타깃이라 앱의 설정을 상속하지 않는다).

공유기 포트 포워딩이 전혀 없고, 모든 트래픽이 WireGuard로 암호화되며, 셀룰러에서도 집 Mac에 도달한다. 서버를 공인 인터넷에 노출하지 않으므로 API 키 단일 인증([02-TECH-SPEC.md](02-TECH-SPEC.md))으로 충분한 근거가 된다.

### 대안: LAN 고정 IP

같은 Wi-Fi에서만 쓴다면 공유기에서 Mac에 DHCP 고정 IP(예: `192.168.0.10`)를 할당하고 iOS 앱에 `http://192.168.0.10:8420`을 설정한다. 셀룰러·외부 네트워크에서는 저장이 안 되는 한계가 있으므로 실사용에는 Tailscale을 권한다.

---

## 5. iOS 단축어로 캡처 (M1부터)

M4 앱이 나오기 전까지의 **공식 캡처 경로**다. 단축어 앱으로 공유 시트에서 바로 저장하며, M1 DoD에 "폰 단축어로 실제 저장 1건"이 포함된다 ([08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)).

단축어 앱에서 새 단축어를 만든다:

1. 단축어 세부사항에서 **공유 시트에 표시** 활성화, 입력 유형은 URL
2. **"URL 콘텐츠 가져오기"** 액션을 추가하고 다음처럼 설정:
   - URL: `http://100.x.y.z:8420/api/v1/links` (Mac의 Tailscale IP)
   - 방법: POST
   - 헤더: `Authorization` = `Bearer {PUSHPOINT_API_KEY 값}`
   - 본문 요청: JSON, 필드 `url` = (매직 변수) **단축어 입력**
3. Safari·YouTube 등에서 공유 시트 → 이 단축어 선택 → 저장 완료

서버가 `201 {"id":...}`를 반환하면 성공이다. `POST /api/v1/links`는 url_hash 기준 멱등이므로 실수로 두 번 실행해도 중복 저장되지 않는다 (`200 {duplicate:true}`).

---

## 6. 북마크·Takeout 임포트 (M2)

M2의 목표 중 하나는 **실관심사 링크 300건 이상을 한 번에 적재**하는 것이다 ([08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)). 매일 하나씩 저장하기 전에, 이미 브라우저 북마크와 YouTube 시청기록에 쌓여 있는 관심사를 밀어넣으면 M3 태거의 `corpus_df`(TF-IDF 코퍼스)가 실데이터로 워밍되고, golden set 층화 샘플링의 모수가 확보된다. 임포트는 `pushpoint import` 서브커맨드가 담당한다 — 추출한 URL을 `POST /api/v1/links`로 순차 전송(초당 약 10건)하며, url_hash 멱등이라 몇 번을 재실행해도 중복 저장은 `200 duplicate`로 안전하게 정리된다.

전제: 서버가 이미 떠 있어야 한다(§1 로컬 실행). 임포트는 별도 프로세스로, 아래처럼 실행한다.

### 브라우저 북마크 (Netscape HTML export)

각 브라우저의 북마크 관리자에서 HTML로 내보낸다 (Netscape bookmark 형식 — `<A HREF="...">`).

- Chrome/Edge: 북마크 관리자(`chrome://bookmarks`) → 우상단 ⋮ → **북마크 내보내기**
- Firefox: 북마크 라이브러리(`Ctrl+Shift+O`) → 가져오기 및 백업 → **HTML로 북마크 내보내기**
- Safari: 파일 → **북마크 내보내기**

```bash
cd backend
go run ./cmd/pushpoint import \
  -type bookmarks \
  -file ~/Downloads/bookmarks.html \
  -addr http://localhost:8420 \
  -key dev-key
```

HTML 안의 모든 http(s) URL을 추출한다 (`javascript:` 북마클릿 등 비-HTTP 스킴은 건너뛴다).

### YouTube Takeout (시청기록·좋아요)

[takeout.google.com](https://takeout.google.com)에서 **YouTube 및 YouTube Music**만 선택 → 콘텐츠에서 "기록"(watch-history)·"재생목록"(좋아요 포함)을 포함해 내보낸다. 시청기록의 **기본 export 형식은 HTML**(`watch-history.html`)이며, Takeout의 "여러 형식" 설정에서 기록을 **JSON**으로 바꾸면 `watch-history.json`으로 받을 수 있다. 임포터는 HTML·JSON·CSV 셋 다 지원하고 파일 내용으로 자동 감지한다 (`-format auto` 기본, 필요 시 `html`/`csv`/`json` 강제). 즉 형식을 바꾸지 않고 기본 HTML을 그대로 넣어도 되고, JSON을 선택했다면 그것도 그대로 동작한다.

```bash
# watch-history.html — Takeout 기본 export (형식 자동 감지)
go run ./cmd/pushpoint import \
  -type takeout \
  -file ~/Takeout/YouTube\ and\ YouTube\ Music/history/watch-history.html \
  -addr http://localhost:8420 -key dev-key

# watch-history.json — Takeout에서 JSON 형식을 선택한 경우 (자동 감지)
go run ./cmd/pushpoint import -type takeout \
  -file ~/Takeout/.../watch-history.json -addr http://localhost:8420 -key dev-key

# 형식 강제 (html | csv | json)
go run ./cmd/pushpoint import -type takeout -format csv \
  -file ~/Takeout/.../watch-history.csv -addr http://localhost:8420 -key dev-key
```

형식과 무관하게 영상 watch URL(`youtube.com/watch?v=...`, `youtu.be/...`)만 추출하고 채널·검색 등 나머지 항목은 무시한다.

### 출력

진행률은 100건마다 로그로 나오고, 끝나면 요약 한 줄을 stdout에 출력한다:

```
저장 312 / 중복 18 / 실패 2
```

`저장`은 신규(201), `중복`은 이미 있던 URL(200), `실패`는 네트워크 오류·예상 밖 상태 코드다. 실패는 로그를 남기고 계속 진행하므로 한 건이 막혀도 전체가 중단되지 않는다. 저장된 링크는 워커 풀이 백그라운드에서 스크랩·태깅하므로, 임포트 직후 목록은 `status=pending`이었다가 몇 초 안에 제목·썸네일이 채워진다 ([04-DATA-FLOW.md](04-DATA-FLOW.md)).

---

## 7. 백업·복원

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

## 7.1 Google 스프레드시트로 내보내기 (`just sheets-sync`)

**단방향이다. SQLite가 원본이고 시트는 파생물이다.**

반대 방향(시트를 DB로)은 검토했고 성립하지 않는다:

- 저장 API의 **p99 < 50ms 게이트**에 Sheets 왕복(수백 ms + 분당 write 한도)이 안 들어간다
- **FTS5 전문 검색**이 없다 — 10만 행 30ms는 불가능하다
- **트랜잭션이 없다** — `links` + `jobs` + FTS 동기화를 한 트랜잭션으로 묶는 것이 크래시 내구성의 근거인데 그게 사라진다
- 무엇보다 **확장이 비행기 모드에서도 저장을 끝내는 성질**(M4 DoD)이 네트워크 전제 위에서는 성립하지 않는다

그래서 저장 경로를 전혀 건드리지 않고, 다 끝난 데이터를 나중에 밀어 넣는 별도 명령으로
둔다. **이 명령이 실패해도 아카이브는 멀쩡하다.**

**백업 용도가 아니다.** 백업은 §7이 이미 푼다 — 데이터가 SQLite 파일 하나라 `cp`나
`VACUUM INTO`가 시트 왕복보다 더 완전하다(FTS·corpus_df·tag_feedback·잡 이력까지 포함).
시트가 값을 하는 자리는 **SQLite가 못 하는 일** — 시트에서 필터·피벗으로 훑고, 남에게
보여주고, 다른 도구에 붙이는 것이다.

### 준비 — 실질적으로 한 단계다

**시트는 우리가 만들어 공유해 준다.** 사용자가 시트를 만들고 URL에서 ID를 잘라 오는 단계는
틀리기 쉽고(ID와 URL 전체를 헷갈리는 것이 흔하다) 없앨 수 있는 단계라 없앴다.

1. Google Cloud 콘솔에서 **서비스 계정**을 만들고 JSON 키를 내려받는다.
   같은 프로젝트에서 **Sheets API와 Drive API**를 켠다.
2. 끝.

```bash
export PUSHPOINT_SHEETS_KEY=data/sheets-key.json
PUSHPOINT_SHEETS_SHARE=you@example.com just sheets-sync   # 첫 실행에만
just sheets-sync                                          # 그다음부터는 이것만
```

첫 실행이 시트를 만들어 `PUSHPOINT_SHEETS_SHARE` 계정에 편집자로 공유하고, 만든 ID를
`data/sheets.json`에 기억한다. 두 번째부터는 환경변수 하나면 된다. 이미 있는 시트를 쓰려면
`PUSHPOINT_SHEETS_ID`를 주면 그쪽이 우선한다.

**1번은 없앨 수 없다** — 구글이 자격증명 없이는 아무것도 내주지 않는다. 사용자 OAuth로
바꿔도 클라이언트 ID를 만드는 같은 콘솔 작업이 남고, 거기에 리다이렉트·토큰 갱신·동의
화면이 더해진다. 단일 사용자 셀프호스트에는 서비스 계정이 더 싸다. **키는 기기 밖으로
나가지 않는다.**

요청하는 스코프는 `spreadsheets` + **`drive.file`**이다. `drive.file`은 전체 드라이브가
아니라 **이 도구가 만든 파일에만** 권한을 준다 — 시트를 만들고 공유하는 데 그것으로
충분하고, 넓은 `auth/drive`는 사용자의 나머지 드라이브까지 이 키의 사정권에 넣는다.

주기 실행은 launchd/cron에 `just sheets-sync`를 걸면 된다.

### 알아 둘 것

- **덧붙이기가 아니라 교체다.** 태그를 고치거나 링크를 지운 것이 시트에 반영돼야 하므로
  매번 탭을 비우고 다시 쓴다. 그 대가로 **시트에 손으로 적은 것은 지워진다** — 버그가
  아니라 계약이다. 메모를 남기려면 별도 탭에 하고, 그 탭은 건드리지 않는다.
- **`body_text`와 `summary`는 내보내지 않는다.** 본문은 링크당 최대 32KB인데 시트 셀 상한이
  50,000자, 시트 전체가 1,000만 셀이라 몇백 건에서 한도에 부딪힌다. 그리고 시트에서 본문을
  읽을 일이 없다 — 읽으려면 원문을 연다.
- 열 순서는 계약이다. 시트에서 만든 필터·수식이 열 위치에 의존하므로 **열은 중간에 끼워
  넣지 말고 뒤에 붙인다**.
- 구현은 표준 라이브러리만 쓴다. 공식 클라이언트(`google.golang.org/api/sheets/v4`)는
  실측하면 grpc·protobuf·opentelemetry까지 **모듈 75개**를 끌고 오는데, 직접 의존성이 11개인
  백엔드가 스프레드시트 행 쓰기 하나에 치를 값이 아니다(CGO-free sqlite와 같은 판단).
  서비스 계정 JWT(RFC 7523)는 `crypto/rsa` + `encoding/json`으로 충분하다.

## 8. 관측

### 헬스체크

```bash
curl http://localhost:8420/healthz
# {"status":"ok"}
```

인증이 없으므로 launchd/systemd 외부의 간단한 모니터링 스크립트나 Uptime 체크에 바로 쓸 수 있다.

### 프로파일링 (/debug/pprof)

`net/http/pprof`가 기본 탑재돼 있다.

```bash
# CPU 프로파일 30초 수집 후 인터랙티브 분석
go tool pprof http://localhost:8420/debug/pprof/profile?seconds=30

# 힙 스냅샷
go tool pprof http://localhost:8420/debug/pprof/heap

# goroutine 덤프 (워커 풀 상태 확인에 유용)
curl http://localhost:8420/debug/pprof/goroutine?debug=1
```

### 로그

`log/slog`로 출력하며 형식은 `PUSHPOINT_LOG_FORMAT`가 정한다 (기본 `auto`). 운영 배포는 stderr가
터미널이 아니므로(systemd/journal·파일 리다이렉트) `auto`가 JSON으로 떨어져 `jq`로 바로 필터링된다.

```bash
tail -f /tmp/pushpoint.log | jq 'select(.level == "ERROR")'
```

레벨은 `PUSHPOINT_LOG_LEVEL`로 조절한다 (기본 `info`). 개발(`just dev`)은 `text`를 강제해 사람이
읽는 컬러 로그로 나오고, 접근 로그와 잡(scrape/thumb) 처리 로그는 `debug` 레벨이라 dev에서만 보인다.

### 성능·복구 게이트

```bash
just bench            # 마이크로벤치: cd backend && go test -bench=. -benchmem ./...
just bench-http       # HTTP 경로 저장 p99 측정 — p99 < 50ms 미달 시 exit 1 (M1)
scripts/coldstart.sh  # 실행 → /healthz 200 응답까지 1초 미만 검증 (M1)
just test-crash       # 빌드 → fixture 서버 → 저장 → kill -9 → 재기동 → 전량 done 단언 (M2)
```

p99 판정은 `just bench-http`가 담당한다 — go test 벤치는 평균만 내므로 p99 판정 수단이 아니다. 검색(1만 링크) < 30ms, 10만 건 목록 < 50ms 등 [02-TECH-SPEC.md](02-TECH-SPEC.md)의 목표치를 매 마일스톤마다 검증한다 (마일스톤별 검증 매트릭스는 [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)). 수치 없는 "빨라진 것 같다"는 인정하지 않는다.

### 실측 기록

측정 환경: Apple Silicon 로컬 개발 머신, 2026-07-20. 저장 p99는 `just bench-http`, 콜드 스타트는 `scripts/coldstart.sh` 출력이다. 목표 열은 [00-README.md](00-README.md)의 성능 목표 표와 같은 값이며, 실측 열은 마일스톤이 진행되면서 채워진다.

| 지표 | 목표 | 실측 (2026-07-20) |
|---|---|---|
| 저장 API p99 | < 50ms | p50 0.244ms / p95 0.35ms / p99 0.981ms |
| 저장 → 태그 완료 (비동기) | < 3s | — (태거는 M3) |
| 검색 (FTS5, 1만 링크) | < 30ms | — |
| 링크 10만 건에서 목록 스크롤 API | < 50ms | — |
| 콜드 스타트 (바이너리 실행 → 서빙) | < 1s | 314~684ms |

수치를 갱신할 때는 측정 일자와 측정에 쓴 커맨드를 함께 바꾼다 — 언제 어떤 경로로 잰 값인지가 빠지면 비교가 불가능해진다.

---

## 9. deploy/k8s-future/ — 보존된 v1 매니페스트

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
