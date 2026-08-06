# Deployment and operations

> Push-Point v2.1 — last updated: 2026-07-21

The deployment unit in v2 is one Go binary. Docker Desktop, kubectl, Minikube and Helm — everything v1 demanded — are all unnecessary. This document covers everything operations needs: running locally, keeping the home Mac serving around the clock, reaching it from the iPhone, Shortcuts capture, backup, and observability. v1's k8s manifests were not deleted; they are preserved under `deploy/k8s-future/` (see the last section).

---

## 1. Running locally

### Requirements

- Go 1.25+
- just (`brew install just`)
- A physical iOS device: **the Apple Developer Program ($99/yr) is needed for M6, not for M4** (re-examined 2026-07-26).

  **Expiring after 7 days does not block the M4 DoD.** A free provisioning profile expires 7 days after it is issued, and the M4 DoD is "1+ save a day, 7 consecutive days". Start saving on the day you install: day 0 and the six days after it are a 7-day run, and the profile dies the day after that — **it fits exactly, with zero reinstalls.** Tight, but the arithmetic holds. This entry originally read "incompatible with an app you use every day"; that judgement was made against M6's 28-day window and does not carry over to M4.

  What $99-a-year actually buys is **M6's streak of 28 days** (on the free path, three reinstalls at 7-day intervals) and a daily life without reinstalls. Convenience, not capability.

  **The genuinely unverified premise is not expiry, it is App Groups** — see below. If that is refused, the app and the extension end up looking at different databases, the whole save path dies, and the arithmetic about 7 days stops meaning anything.

  **Taking the free-provisioning (Personal Team) route first (adopted 2026-07-26).** Expiry at 7 days blocks daily use; it **does not block a one-off measurement.** The two verdicts that need real hardware — extension memory measured for real, and `0xdead10cc` (suspended while holding an App Group file lock) — are finished with a single phone and no $99. The only things that actually come to need the $99-tier are the M4 DoD's "7 consecutive days" and M6's streak of 28 days.

  Procedure: sign in with your Apple ID under Xcode → Settings → Accounts (credential entry, so it cannot be automated) → connect the phone over USB and approve the trust prompt → `just ios-teams` to read the team ID → `just ios-device <TEAMID>`. If a device is attached it **builds and installs**, then tells you what to do next (trust the developer app → share once → `just save-timing`). If nothing is attached it builds only and says so.

  `just save-timing` also reads the device's record first when a phone is plugged in — extension memory only produces a number on real hardware, so the most confusing state of all is reading the simulator's record with the phone attached and seeing "not measured". **This device path has not been verified on hardware yet** (no device as of 2026-07-26) — if it fails, it says so and falls back to the simulator path.

  **One unverified premise: whether App Groups works on a free team.** [09-PLAN-REVIEW.md](09-PLAN-REVIEW.md) rebutted this with "the official table says it is possible", but that table never lists App Groups separately — it lumps everything under "Advanced app capabilities", which is thin evidence. If it is blocked, the app and the extension see different databases and the entire save path dies. So the extension's instrumentation (`SaveTiming`) falls back to its own container when the App Group will not open — **even when the save fails, the memory number and the fact of the failure survive.** `just save-timing` reports that outcome (`app_group: false`) explicitly.

That is all there is on the server side. The DB driver is CGO-free (`modernc.org/sqlite`), so there is no separate C toolchain and no container runtime.

### Running

```bash
just dev
# = cd backend && PUSHPOINT_API_KEY=dev-key go run ./cmd/pushpoint
```

The Go recipes in the root justfile run from the `backend/` directory (`cd backend && ...`). The monorepo split is a repository layout, nothing more; the deployment unit is still a single binary.

Handled automatically on first run:

- creating the `data/` directory (`data/pushpoint.db`, `data/thumbs/`)
- applying migrations — `backend/migrations/` is embedded in the binary via golang-migrate + `embed.FS` and runs at startup. There is no separate migrate command.
- crash recovery — jobs left in `running` at startup are put back to `pending` and resumed

The cold-start target (launch → serving) is under one second.

### Smoke test

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

When `q` is three characters or longer it runs FTS5 trigram MATCH with bm25 ranking (`"mode":"fts"`); shorter than three, it falls back to LIKE (`"mode":"like"`) instead of a 400-error — details in [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md).

---

## 2. Environment variables

Prefix `PUSHPOINT_`, read with plain `os.Getenv` (no viper).

| Variable | Default | Description |
|---|---|---|
| `PUSHPOINT_ADDR` | `:8420` | HTTP listen address |
| `PUSHPOINT_DATA_DIR` | `./data` | Directory holding the SQLite DB and thumbnails |
| `PUSHPOINT_API_KEY` | (none, required) | Bearer auth key. `just dev` sets it to `dev-key` |
| `PUSHPOINT_SCRAPE_CONCURRENCY` | `8` | Upper bound on concurrent scraper workers |
| `PUSHPOINT_LOG_LEVEL` | `info` | slog log level (`debug`/`info`/`warn`/`error`) |
| `PUSHPOINT_LOG_FORMAT` | `auto` | Log output format. `text` (human-readable colour), `json` (structured, `jq`-parsable), `auto` (text when stderr is a terminal, json otherwise). `just dev` forces `text` |
| `PUSHPOINT_ALLOW_PRIVATE_HOSTS` | `false` | `true` lifts the private-range block (the SSRF guard) on scraping and thumbnails — local fixture tests only |

For real use, replace `PUSHPOINT_API_KEY` with a sufficiently long random string (`openssl rand -hex 32` or similar).

---

## 3. Running around the clock

Real use (from the end of M2 — the moment Shortcuts capture begins) means leaving the server up on the home Mac. Build it first.

```bash
just build   # = cd backend && go build -o bin/pushpoint ./cmd/pushpoint → backend/bin/pushpoint
```

M5 footnote: depending on the shape ONNX adoption takes, the artifact may become binary + `libonnxruntime.dylib` rather than a single binary (the M5 three-way decision in [02-TECH-SPEC.md](02-TECH-SPEC.md) — dylib embed / hugot pure Go / stay on Phase A). That decision affects the launchd procedure and is made in the first week of M5. Through M1–M4 it is a single static binary exactly as written here.

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

`KeepAlive` takes care of restarting after a crash. Thanks to the jobs table, unprocessed jobs are not lost across a restart (see [04-DATA-FLOW.md](04-DATA-FLOW.md)).

### Power and sleep (required)

Registering with launchd is not enough. When the Mac sleeps the server stops with it, and **a sleeping Mac cannot be woken over Tailscale** — Wake-on-LAN magic packets are not carried through a WireGuard tunnel. Configure the server Mac never to sleep at all.

```bash
sudo pmset -a sleep 0        # 시스템 잠자기 비활성화
sudo pmset -a autorestart 1  # 정전 복구 시 자동 재부팅
```

- Enable automatic login (System Settings → Users & Groups) and confirm `RunAtLoad`: a LaunchAgent runs inside a login session, so unattended recovery after a reboot only completes when automatic login and `RunAtLoad` mesh.
- External uptime check: have external monitoring call `/healthz` periodically so downtime reaches you as an alert.

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

## 4. Reaching the server from the iPhone (needed from M1)

For the iOS Shortcut (from M1) and the Share Extension and app (from M4) to `POST /api/v1/links` to the server on the home Mac, there has to be a route in from outside.

### Recommended: Tailscale

1. Install Tailscale on the Mac and the iPhone and sign in with the same account
2. Find the Mac's Tailscale IP: `tailscale ip -4` → `100.x.y.z`
3. The server address in the iOS clients (Shortcut, app, Share Extension) must be **in IP form only**: `http://100.x.y.z:8420`
4. In the iOS app (M4+), set Tailscale **VPN On Demand** to **Always** on both Wi-Fi and Cellular — a required step. Leave it off and the Share Extension's POST fails whenever the VPN is down.

**ATS note (why the server address has to be in IP form)**: iOS App Transport Security blocks plaintext HTTP to a hostname but exempts IP literals. Using a MagicDNS name (`mac.tailnet-xxx.ts.net`) means configuring HTTPS with `tailscale cert` — **plaintext HTTP plus a hostname is forbidden**. If you do add an ATS exception, it has to go into the Info.plist of **both** the app and the Share Extension (the extension is a separate target and does not inherit the app's settings).

There is no router port forwarding at all, every packet is WireGuard-encrypted, and it reaches the home Mac from cellular too. Because the server is never exposed to the public internet, single-API-key authentication ([02-TECH-SPEC.md](02-TECH-SPEC.md)) has a defensible basis.

### Alternative: a fixed LAN IP

If you only use it on the same Wi-Fi, assign the Mac a DHCP reservation on the router (e.g. `192.168.0.10`) and set the iOS app to `http://192.168.0.10:8420`. Saving does not work from cellular or any outside network, so Tailscale is the recommendation for real use.

---

## 5. Capturing with an iOS Shortcut (from M1)

This is the **official capture path** until the M4 app arrives. It saves straight from the share sheet through the Shortcuts app, and the M1 DoD includes "1 real save from the phone shortcut" ([08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)).

Create a new shortcut in the Shortcuts app:

1. In the shortcut details, enable **Show in Share Sheet** with URL as the input type
2. Add a **"Get Contents of URL"** action and configure it like this:
   - URL: `http://100.x.y.z:8420/api/v1/links` (the Mac's Tailscale IP)
   - Method: POST
   - Header: `Authorization` = `Bearer {PUSHPOINT_API_KEY value}`
   - Request Body: JSON, field `url` = (magic variable) **Shortcut Input**
3. From Safari, YouTube, wherever: share sheet → pick this shortcut → saved

A `201 {"id":...}` from the server means it worked. `POST /api/v1/links` is idempotent on url_hash, so running it twice by accident does not store a duplicate (`200 {duplicate:true}`).

---

## 6. Bookmark and Takeout import (M2)

One of M2's goals is **loading 300+ real-interest links in one go** ([08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)). Before starting to save one link a day, pushing in the interests already piled up in browser bookmarks and YouTube watch history warms M3's tagger `corpus_df` (the TF-IDF corpus) with real data and secures a population for the golden set's stratified sampling. Import is the job of the `pushpoint import` subcommand — it sends the extracted URLs to `POST /api/v1/links` one after another (about 10 per second), and because url_hash is idempotent, re-running it any number of times settles duplicates safely as `200 duplicate`.

Precondition: the server has to be up already (§1 Running locally). Import is a separate process, run like this.

### Browser bookmarks (Netscape HTML export)

Export to HTML from each browser's bookmark manager (Netscape bookmark format — `<A HREF="...">`).

- Chrome/Edge: bookmark manager (`chrome://bookmarks`) → ⋮ top right → **Export bookmarks**
- Firefox: bookmark library (`Ctrl+Shift+O`) → Import and Backup → **Export Bookmarks to HTML**
- Safari: File → **Export Bookmarks**

```bash
cd backend
go run ./cmd/pushpoint import \
  -type bookmarks \
  -file ~/Downloads/bookmarks.html \
  -addr http://localhost:8420 \
  -key dev-key
```

Every http(s) URL in the HTML is extracted (non-HTTP schemes such as `javascript:` bookmarklets are skipped).

### YouTube Takeout (watch history and likes)

At [takeout.google.com](https://takeout.google.com) select **YouTube and YouTube Music** only → under content, include "history" (watch-history) and "playlists" (which covers likes), then export. The **default export format for watch history is HTML** (`watch-history.html`); switching history to **JSON** under Takeout's "multiple formats" setting gets you `watch-history.json` instead. The importer supports HTML, JSON and CSV alike and detects the format from the file contents (`-format auto` by default, forced with `html`/`csv`/`json` when needed). So you can feed it the default HTML without changing anything, and if you did pick JSON that works as-is too.

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

Whatever the format, only video watch URLs (`youtube.com/watch?v=...`, `youtu.be/...`) are extracted; channels, searches and the rest are ignored.

### Output

Progress is logged every hundred links, and at the end a one-line summary goes to stdout:

```
저장 312 / 중복 18 / 실패 2
```

`저장` is new (201), `중복` is a URL that already existed (200), `실패` is a network error or an unexpected status code. Failures are logged and the run carries on, so one blocked link does not stop the whole import. Saved links are scraped and tagged by the worker pool in the background, so right after an import the list reads `status=pending` and then fills in titles and thumbnails within seconds ([04-DATA-FLOW.md](04-DATA-FLOW.md)).

---

## 7. Backup and restore

The data is all under `data/`: `pushpoint.db` (+`-wal`, `-shm`) and `thumbs/`. At 100k links that is roughly 150MB of DB plus about 3GB of thumbnails.

### Backup (no need to stop the process)

```bash
# 방법 1: SQLite 온라인 백업 — 서버 구동 중에도 일관된 스냅샷
sqlite3 data/pushpoint.db ".backup 'backup/pushpoint-$(date +%Y%m%d).db'"

# 방법 2: 디렉터리 통째 복사 (썸네일 포함)
rsync -a data/ backup/data-$(date +%Y%m%d)/
```

WAL caution: when you plain-copy only the DB file, you must copy `-wal` and `-shm` with it or the most recent writes are not preserved (`.backup` merges them for you).

### Restore

Stop the server, put the backup file back in place, start it again. That is all.

```bash
launchctl unload ~/Library/LaunchAgents/ai.pushpoint.server.plist
cp backup/pushpoint-20260720.db data/pushpoint.db
rm -f data/pushpoint.db-wal data/pushpoint.db-shm   # 이전 WAL 잔여물 제거
launchctl load ~/Library/LaunchAgents/ai.pushpoint.server.plist
```

This is the operational simplification you feel most against v1: a file copy instead of a PostgreSQL dump and a PVC snapshot.

---

## 7.1 Exporting to Google Sheets (`just sheets-sync`)

**It is one-way. SQLite is the original and the sheet is a derivative.**

The other direction (sheet into DB) was examined and does not hold:

- a Sheets round trip (hundreds of ms, plus a per-minute write cap) does not fit inside the save API's **p99 < 50ms gate**
- there is no **FTS5 full-text search** — 30ms-over-100k-rows is impossible
- there are **no transactions** — binding `links` + `jobs` + FTS synchronisation into one transaction is the basis of crash durability, and it disappears
- above all, **the extension finishing a save even in airplane mode** (the M4 DoD) does not hold on top of a network premise

So it stays a separate command that never touches the save path and pushes already-finished data in later. **If this command fails, the archive is untouched.**

**It is not for backup.** Backup is already solved one section up — the data is a single SQLite file, so `cp` or `VACUUM INTO` is more complete than a sheet round trip (it carries FTS, corpus_df, tag_feedback and job history too). Where the sheet earns its keep is **what SQLite cannot do** — sweeping through it with filters and pivots, showing it to someone else, wiring it into another tool.

### Setup — from the settings screen, one paste

**The "Start connecting" button in the web settings** hands over the script and takes the
deployment URL back (2026-08-06). No terminal — before this there was only `just sheets-setup`,
which made knowing how to open a terminal a precondition for getting started.

The CLI is still there, and now takes the URL as an argument too (for environments without stdin):

```
just sheets-setup
pushpoint sheets-setup -url <URL>
```

The first line is the guided form; the second is for environments without stdin.

No cloud console, no JSON key, no API to switch on. The command puts the script on your clipboard and opens a new sheet in the browser; you paste it under **Extensions → Apps Script**, deploy it, and hand back the single URL that comes out. It verifies the connection and carries straight on into the first sync.

After that it is `just sheets-sync`, or **the "Sync now" button on the web settings screen**.

**Why the user pastes the script.** Doing it that way inverts the trust relationship. The service-account approach is a structure where *we reach into your Drive*; this way the script touches **only its own sheet, inside your account**. All the script can open is `getActiveSpreadsheet()`, and deleting the deployment cuts the access off — the basis for trust is that scope, not the line count. A random token is baked into the script, so even a leaked deployment URL is useless without it.

**The one Google authorisation step cannot be removed** — Google built it that way. Everything else that could be removed (cloud project, enabling APIs, service account, JSON key, copying a sheet ID) is gone.

<details>
<summary>Service account approach (optional)</summary>

If you already have a cloud project and would rather not paste a script, this way works too.
Create a service-account JSON key, enable the Sheets API and Drive API, then:

```bash
export PUSHPOINT_SHEETS_KEY=data/sheets-key.json
PUSHPOINT_SHEETS_SHARE=you@example.com just sheets-sync   # 첫 실행에만
```

It creates the sheet, shares it with that account, and remembers the ID in `data/sheets.json`. The scopes are
`spreadsheets` + **`drive.file`** — `drive.file` grants access **only to the files this tool created**, not the
whole Drive, so unlike the broad `auth/drive` it never brings the rest of your Drive into range.

</details>

For periodic runs, hang `just sheets-sync` off launchd/cron.

**Editing the script means pasting it again.** The script lives as a copy inside the user's sheet, so a fix in the repo does not propagate on its own. Running `just sheets-setup` again refreshes it through the same procedure (and mints a new token).

### Things to know

- **Columns A–I are ours and are regenerated on every sync. From column J on it is yours and we never touch it.** That is the boundary. It is replacement rather than append because tag edits and deletions have to reach the sheet, and the only way to do that without losing what you wrote by hand is to narrow what gets erased — **detecting** what is a human's work is impossible in the first place (Sheets gives you neither a per-row/per-cell edit time nor a revision id). Instead of detecting loss, we made loss impossible. Note though that **row order is regenerated every time**, so a formula in column J is only safe in the form that references column A of the same row.
- **A broken sheet is not data loss.** Upend the sort, delete rows — one more sync rebuilds A–I from SQLite. The sheet is a derivative, so damage is closer to a cache miss. (Google Sheets' own version history can undo it too.)
- **`body_text` and `summary` are not exported.** A body is up to 32KB-per-link, while a sheet cell caps at 50,000-characters and 1 whole sheet holds 10-million cells, so a few hundred links run into the limit. And there is no occasion to read body text in a sheet — to read it, you open the original.
- Column order is a contract. Filters and formulas built in the sheet depend on column positions, so **do not insert a column in the middle; append at the end**.
- The implementation uses the standard library only. The official client (`google.golang.org/api/sheets/v4`), measured, drags in **75 modules** including grpc, protobuf and opentelemetry — not a price an 11-dependency backend should pay for writing spreadsheet rows (the same judgement as CGO-free sqlite). A service-account JWT (RFC 7523) is `crypto/rsa` + `encoding/json` and nothing more.

## 8. Observability

### Health check

```bash
curl http://localhost:8420/healthz
# {"status":"ok"}
```

There is no authentication, so it drops straight into a simple monitoring script or uptime check outside launchd/systemd.

### Profiling (/debug/pprof)

`net/http/pprof` is built in by default.

```bash
# CPU 프로파일 30초 수집 후 인터랙티브 분석
go tool pprof http://localhost:8420/debug/pprof/profile?seconds=30

# 힙 스냅샷
go tool pprof http://localhost:8420/debug/pprof/heap

# goroutine 덤프 (워커 풀 상태 확인에 유용)
curl http://localhost:8420/debug/pprof/goroutine?debug=1
```

### Logs

Output goes through `log/slog` and the format is decided by `PUSHPOINT_LOG_FORMAT` (default `auto`). In a production
deployment stderr is not a terminal (systemd/journal, file redirection), so `auto` lands on JSON and filters directly with `jq`.

```bash
tail -f /tmp/pushpoint.log | jq 'select(.level == "ERROR")'
```

The level is set with `PUSHPOINT_LOG_LEVEL` (default `info`). Development (`just dev`) forces `text` so the logs come out
human-readable and coloured, and access logs plus job (scrape/thumb) processing logs sit at `debug`, so they show up only in dev.

### Performance and recovery gates

```bash
just bench            # 마이크로벤치: cd backend && go test -bench=. -benchmem ./...
just bench-http       # HTTP 경로 저장 p99 측정 — p99 < 50ms 미달 시 exit 1 (M1)
scripts/coldstart.sh  # 실행 → /healthz 200 응답까지 1초 미만 검증 (M1)
just test-crash       # 빌드 → fixture 서버 → 저장 → kill -9 → 재기동 → 전량 done 단언 (M2)
```

The p99 verdict belongs to `just bench-http` — a go test benchmark only produces a mean, so it is not an instrument for judging p99. Search (10k links) < 30ms, list at 100k links < 50ms and the rest of [02-TECH-SPEC.md](02-TECH-SPEC.md)'s targets are verified at every milestone (the per-milestone verification matrix is in [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md)). "It seems faster" with no number behind it is not accepted.

### Measured record

Measurement environment: Apple Silicon local development machine, 2026-07-20. Save p99 comes from `just bench-http`, cold start from the output of `scripts/coldstart.sh`. The target column carries the same values as the performance target table in [00-README.md](00-README.md); the measured column fills in as the milestones progress.

| Metric | Target | Measured (2026-07-27) |
|---|---|---|
| Save API p99 | < 50ms | p50 0.265ms / p95 0.355ms / **p99 1.22ms** (n=2000) |
| Save API p99 — client-capture path | < 50ms | p50 0.733ms / p95 1.248ms / **p99 4.408ms** (n=500) |
| Save → tagging complete (async) | < 3s | — |
| Search (FTS5, 10k links) | < 30ms | — |
| List-scroll API at 100k links | < 50ms | — |
| Cold start (binary launch → serving) | < 1s | **405ms** |

> The 2026-07-20 measurement was a 0.981ms-p99. Since then the client-capture path (a save that carries the body
> with it) came in and added fields to the save path, and the value above is that state, measured. Both are inside the gate (50ms).

When you update these numbers, change the measurement date and the command used to measure along with them — without knowing when and by which path a value was taken, comparison becomes impossible.

---

## 9. deploy/k8s-future/ — the preserved v1 manifests

v1's Kubernetes manifests were not deleted; they were moved to `deploy/k8s-future/` and preserved. **This is folding it up, not throwing it away.**

Preserved files:

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

### Why we folded it

v1 stood up Minikube + k8s + HPA + PostgreSQL + Redis + MinIO with zero users. Building the autoscaler first when there was no traffic to autoscale was backwards engineering, and the friction of needing a cluster up for a single local test ate into development speed. v2 spends all of that cost on the product instead — scraper, NLU tagging, iOS.

### Revival conditions and the swap points

The revival condition is clear: **external users appearing.** By that point the code structure is already prepared — Store/Queue/Tagger sit behind interfaces ([03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md)), so only the implementations get swapped.

| Swap point | v2 (now) | On revival |
|---|---|---|
| `Store` | SQLite (WAL) | PostgreSQL — `postgresql.yaml` |
| `Queue` | SQLite jobs table + goroutine | Redis — `redis.yaml` |
| Thumbnail storage | Local disk `data/thumbs/` | S3-compatible storage — `minio.yaml` |
| Process layout | Single binary (API + worker) | `api-server.yaml` / `worker.yaml` deployed separately + `hpa.yaml` |

Until then this directory takes no part in any build or run path.

---

## Related documents

- [02-TECH-SPEC.md](02-TECH-SPEC.md) — tech stack and performance targets
- [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md) — single-process structure and interface boundaries
- [04-DATA-FLOW.md](04-DATA-FLOW.md) — job queue behaviour and crash recovery
- [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md) — API specification
- [08-DEVELOPMENT-PLAN.md](08-DEVELOPMENT-PLAN.md) — milestones
