# 개발 계획

> Push-Point v2.1 — 마지막 업데이트: 2026-07-26

이 문서는 사용자가 확정한 v2 계획의 공식 버전이다. v1의 8~10주 계획(k8s 배포·OpenAI 태깅·동기화 중심)은 폐기하고, "내가 매일 쓰는 앱"을 목표로 6개월 계획을 새로 세운다.

---

## 1. v1 → v2 변경 요약과 전환 이유

핵심 원칙: **인프라가 아니라 제품부터.** k8s/HPA/MinIO/Redis는 유저가 생긴 뒤의 문제다. 성능은 분산이 아니라 단일 프로세스 설계의 질로 확보한다.

| 영역 | v1 | v2 | 이유 |
|---|---|---|---|
| 배포 | Minikube + k8s + HPA | 단일 Go 바이너리 (`just dev` 한 번) | 유저 0명에 오토스케일링은 역설계. 로컬 테스트 마찰 제거 |
| DB | PostgreSQL (k8s pod) | SQLite (WAL 모드) + FTS5 | 개인 앱 규모에서 충분히 고성능. 백업 = 파일 복사 |
| 메시지 큐 | Redis Streams | 인프로세스 워커 풀 (goroutine + SQLite jobs 테이블) | 프로세스 하나면 네트워크 큐 불필요. 재시작 내구성은 jobs 테이블이 보장 |
| 오브젝트 스토리지 | MinIO | 로컬 디스크 (`data/thumbs/`) | 썸네일 몇 GB에 S3 API는 과함 |
| AI 태깅 | OpenAI API | 경량 NLU (규칙 기반 → ONNX 임베딩 2단계) | 비용 0, 수백 ms 응답, 프라이버시. 이 프로젝트의 기술적 차별점 |
| 클라이언트 | React Native (미정) | iOS Share Extension 최우선 (SwiftUI) | 저장 마찰이 2초를 넘으면 매일 쓰는 앱이 못 됨 |
| 인증 | JWT + 회원가입 | 단일 사용자, 정적 API 키 1개 | 멀티유저는 명시적 비목표 |

k8s 매니페스트는 삭제하지 않고 `deploy/k8s-future/`로 이동해 보존한다. **지금 접는 것이지 버리는 것이 아니다.** Store/Queue/Tagger가 인터페이스 뒤에 있으므로 유저가 생기면 구현체만 교체하면 된다.

---

## 2. 전체 일정: 6개월

| 단계 | 기간 | 내용 | 완료 기준 (DoD) |
|---|---|---|---|
| M1 코어 | 2주 | 스키마 + Store/Queue + 저장/목록 API + 벤치 하네스 + iOS 단축어 캡처 | `just bench-http` p99 < 50ms·콜드 스타트 < 1s 통과, 폰 단축어로 실제 저장 1건 |
| M2 스크래퍼 | 3주 | 워커 풀 + 파싱(사이트 어댑터) + 썸네일 + 재시도/복구 + 북마크·Takeout 임포트 | `just test-crash` 통과, 대표 도메인 세트에서 3s 내 제목·썸네일, 실링크 300건+ 적재, **매일 실사용 시작** |
| M3 태깅 A + 검색 | 4주 | 규칙 태거(한국어 정규화) + 태그 사전 + FTS5/LIKE 검색 + eval 하네스 | `just eval` 동작, golden set(dev/test 분할) 구축, 베이스라인 대비 측정치 기록 |
| M4 iOS | 5주 | Share Extension + 목록 + 검색 + 상세 편집 + Tailscale 실기기 | 서버 오프라인에도 공유 저장 2초 내 성공·유실 0건, 연속 7일 하루 1건+ 저장 |
| M5 태깅 B | **재범위** | ~~4주 통짜 ONNX 앙상블~~ → Phase 0 계측(1주) → Phase 1 오프라인 스파이크(1주, 폐기 전제) → Phase 2 조건부 통합(2주) | 종료: 앙상블이 Phase A 대비 **회귀 0 + 개선 5건 이상**(동결 test). 원안 취소 경위는 §M5 |
| M6 다듬기 | 4주 | 위젯 + 성능 튜닝 + 공개 글 (Live Activity는 이후 후보) | `scripts/streak.sh` 4주 연속 일일 사용, 기술 글 1편 |
| M-Web 웹 앱 | 병렬 트랙 | Vite+React+TS SPA, `api/openapi.yaml` 계약 소비(openapi-typescript), 6개 화면, Go embed 서빙 | `just web-gen-check` 드리프트 0 + `just web-build` 성공(단일 바이너리 embed), iOS와 대등한 기능 |

**M-Web (웹 앱)** — iOS(M4)와 **대등한 정식 클라이언트**다. iOS를 밀지 않는 병렬 트랙으로, 실사용 열람·검색·관리 수요를 채운다. 두 클라이언트는 같은 `api/openapi.yaml` 계약을 소비하므로 기능이 동일하고, **저장의 "iOS 공유 시트 2초 진입"만 iOS 고유**다(웹은 URL 입력창 + 선택적 북마클릿). 스택·계약 파이프라인·embed 배포 상세는 [02-TECH-SPEC.md](02-TECH-SPEC.md)·`.claude/rules/frontend.md`. M4의 검색·상세 편집 화면이 컷돼도 웹이 백필하므로 그 컷이 안전장치가 됐다 — 실제로는 컷 없이 iOS에도 구현됐다(2026-07-26).

순서의 의미: **실사용 시작이 M2 종료(5주차)로 앞당겨졌다.** 단축어 캡처(M1)와 임포트+매일 저장(M2)으로 실데이터가 먼저 쌓이고, M3~M6은 그 데이터 위에서 돈다. M4는 실사용의 시작점이 아니라 저장 경로를 단축어에서 Share Extension으로 바꿔 마찰을 줄이는 단계다.

### 일정 운영 원칙

- 캐파 가정: 주당 투입 약 10시간 가정 (본인 상황에 맞춰 조정) — 마일스톤 기간은 이 가정 기준이다.
- 합계 22주 + **명시적 버퍼 4주** = 6개월. 버퍼는 계획에 포함된 예산이며, **버퍼 2주 소진 시 컷 발동**.
- 컷 순서 (지연 시 이 순서대로 잘라낸다):
  1. Live Activity (이미 M6 이후 후보로 강등)
  2. 위젯
  3. M5 전체 (Phase A로 운영)
  4. ~~M4 검색 화면~~ — **발동하지 않았다**(2026-07-26 구현 완료). 다음 컷 대상은 그 다음 순번으로 내려간다
- **어떤 경우에도 지키는 앵커 = 실사용 시작(M2 종료).** 컷은 앵커를 지키기 위한 수단이다.

---

## 3. 마일스톤별 상세 분해

### M1 코어 (2주)

`backend/go.mod`는 이미 v2용으로 초기화됐다 (2026-07-20, 의존성 0). M1은 재편이 아니라 `backend/` 아래 **신규 작성**이다 — v1 백엔드 코드는 존재한 적 없고 go.mod 선언만 있었다.

**Week 1**
- `api/openapi.yaml` 확정 (OpenAPI 3.1 — API의 기계 원본, [06-API-SPECIFICATION.md](06-API-SPECIFICATION.md)는 사람용 해설) + oapi-codegen 파이프라인: `just gen`으로 chi 서버 인터페이스·타입을 `backend/internal/api/gen/`에 생성(생성물 커밋), `just gen-check`로 드리프트 검사. oapi-codegen은 **v2.8.0 핀** — OpenAPI 3.1 생성 실측 통과(2026-07-20), generate 세트에 strict-server 포함
- enum 정합 lint 스크립트: `api/openapi.yaml`의 enum ↔ [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md) DDL CHECK 제약 대조
- `backend/cmd/pushpoint/main.go` 단일 진입점 + `backend/internal/...` 골격 신규 작성
- SQLite 스키마 작성 + golang-migrate 마이그레이션 (`embed.FS`로 바이너리 내장, 시작 시 자동 적용)
- SQLite PRAGMA 설정 (WAL, synchronous NORMAL, busy_timeout 5000, foreign_keys ON, cache_size 64MB)
- Store 인터페이스 + sqlite 구현 (writer 1개 + reader 풀 N=4)
- Queue 인터페이스 + sqlite jobs 구현 (enqueue만, 워커는 M2)

**Week 2**
- `POST /api/v1/links` — links + scrape 잡 INSERT를 한 트랜잭션으로, 즉시 201. 중복 url_hash면 `200 {duplicate:true}`
- `GET /api/v1/links` (keyset 커서 페이지네이션), `GET /api/v1/links/{id}`
- Bearer API 키 인증 미들웨어, `GET /healthz`
- 벤치 하네스 구축: `just bench-http` (실 HTTP 경로 저장 p99 측정, 50ms 초과 시 exit 1), `scripts/coldstart.sh` (실행 → `/healthz` 200 < 1s), `just seed 100000` (벤치용 한영 혼합 시드 DB, 고정 난수). `just bench`(go test 마이크로벤치)는 유지하되, go test 벤치는 평균만 내므로 p99 판정은 bench-http가 담당한다
- iOS 단축어 캡처: 공유 시트 → "URL 콘텐츠 가져오기" POST (Authorization 헤더 포함) 단축어 등록 → **폰에서 실제 저장 1건 성공** (M1 DoD)

### M2 스크래퍼 (3주)

**Week 1**
- dispatcher goroutine (notify 채널 + 1초 폴링 티커) + 워커 풀
- 원자적 claim 쿼리 (`UPDATE ... RETURNING`) 및 상태 전이 구현

**Week 2**
- goquery 스크래퍼: title / og:* / meta keywords / published_time / author / lang 파싱
- 사이트 어댑터: youtube.com → oEmbed + watch 페이지 og:description 병합, 채널명(author)을 태거 입력 피처에 포함 / x.com·twitter.com → publish.twitter.com/oembed 분기 / blog.naver.com → m.blog.naver.com 재작성 후 파싱 / instagram.com → 메타 부재 허용 (domain+URL만으로 done)
- content_type 휴리스틱 판정
- singleflight (url_hash 기준 동일 URL 중복 스크랩 제거), 도메인별 rate limit (1 req/s), semaphore 동시성 제한 (기본 8), context timeout 10s, 본문 5MB 제한
- 썸네일 워커: og:image → 최대 폭 640px 리사이즈 → JPEG q80, `data/thumbs/{hash[:2]}/{url_hash}.jpg` + `GET /thumbs/{path}` 서빙

**Week 3**
- 재시도·선형 백오프 (`run_after = unixepoch() + 30 * attempts`, max_attempts 3), `POST /api/v1/links/{id}/retry`
- 시작 시 `running → pending` 복구 로직 + `just test-crash` (빌드 → fixture 서버 → 저장 → kill -9 → 재기동 → 전량 done 단언, M2 DoD)
- 북마크·Takeout 임포트: 브라우저 북마크(HTML export)·YouTube Takeout를 `POST /api/v1/links`로 밀어넣는 일회성 스크립트 → 실관심사 링크 300건+ 적재 (corpus_df 워밍 겸용)
- 대표 도메인 세트(YouTube/일반 아티클/네이버 블로그/X)에서 저장 → 3s 내 제목·썸네일 검증
- **M2 종료 = 단축어로 매일 실사용 시작** (전체 일정의 앵커)

### M3 태깅 A + 검색 (4주)

**Week 1**
- 태그 사전 시드 (초기 30~50개, name + aliases — 정의는 `nlu/dictionary/`에 커밋) + 태그 CRUD API (`/api/v1/tags`)
- 한국어 어절 정규화: 대표 조사 접미 목록(을/를/이/가/은/는/의/에/에서/으로/와/과/도/만/보다/부터/까지/처럼/에게/한테/께서 등 20~30개)을 벗기는 normalize 함수 — corpus_df 누적과 사전 매칭 **양쪽에 동일 적용**
- `corpus_df` 누적 파이프라인 (normalize 적용)

**Week 2**
- 규칙 태거: 도메인-태그 맵 → 키워드 추출 (구분자를 공백+조사 접미로 확장한 후보구 추출 + TF-IDF 스코어링, 형태소 분석기 없이) → 사전 name/aliases 매칭 → 점수 병합 → 상위 k(≤5) + threshold 컷
- 매칭 규칙: 한글 항목 = 정규화 후 전방일치. 라틴 항목 3자 미만(ai, ml, ui 등) = 단어 경계(\b) 매칭 필수 (부분 문자열 금지)
- 단위 테스트: "쿠버네티스를 처음 배우는 사람" → kubernetes 매칭, "he said hello" → ai 미매칭
- 결과를 `link_tags(source='rules', confidence)`로 저장, tag 잡을 파이프라인에 연결

**Week 3**
- FTS5(trigram) 인덱스 동기화 (링크/태그 쓰기와 같은 트랜잭션에서 DELETE 후 INSERT)
- `GET /api/v1/search` — q 3자 이상은 FTS5 MATCH + bm25 (`"mode":"fts"`), 3자 미만은 LIKE 폴백 (`"mode":"like"`), 태그·기간 필터
- `PATCH /api/v1/links/{id}` 태그 전체 교체 + tag_feedback 기록

**Week 4**
- golden set 구축: M2 임포트+실사용 축적분에서 층화 샘플링(도메인·content_type 비율 유지)한 100건을 `nlu/golden/` JSONL로 — **dev / test 분할, test는 동결**(실제 결과: dev 62 / test 61 → 2026-07-27에 84로 증설, 여기에 실제 웹 세트 wild 28 추가). 스키마 `{url, snapshot: {title, description, body_text, keywords}, expected_tags: []}` (eval은 네트워크 접근 0, snapshot만 입력)
- `just eval` 구현: top-3 Recall(hit = 예측 top-3 ∩ expected_tags ≥ 1) + 태그별 precision/recall·부착 빈도 표 출력. "도메인 휴리스틱만" 베이스라인을 항상 함께 측정
- 규칙 튜닝(dev만 사용) → **베이스라인 대비 측정치 기록. 정확도 게이트는 M4 진입을 차단하지 않는다 — 판정은 M5 진입 조건으로 이동** (재정의된 M5 진입 게이트 — 02-TECH-SPEC.md)

### M4 iOS (5주)

**선행 검증 (2026-07-25 완료)** — 착수 전에 가장 깨지기 쉬운 가정 두 개를 실측으로 걷어냈다.

1. **Go 백엔드가 iOS로 묶이는가** — `gomobile bind -target=ios`로 `.xcframework` 생성 확인
   (기기 arm64 + 시뮬레이터 슬라이스). modernc sqlite·go-trafilatura가 모두 `ios/arm64`로
   빌드된다. CGO-free 스택을 고집한 선택이 여기서 값을 했다. 참고로 `wazero`는 링크되지
   않는다(go.mod indirect 항목일 뿐 — trafilatura의 re2go는 WASM이 아니라 생성된 Go 코드다).
2. **확장의 메모리 예산에 들어가는가** — 아래 표.

**바인드 표면을 둘로 나눈다** (`backend/mobile/`). 근거는 측정값이다 — 20,000건·97MB DB에
콜드 프로세스로 1건 저장했을 때의 최대 RSS:

| 링크한 패키지 | 최대 RSS |
|---|---|
| store + queue만 | 13.4 MB |
| **+ scraper** | **64.2 MB** |
| + summarizer | 13.2 MB |
| + tagger | 13.4 MB |

`scraper`는 **호출하지 않고 링크만 해도** +51MB다(trafilatura·readability·domdistiller·
htmldate의 패키지 `init()`이 정규식·셀렉터 테이블을 미리 만든다). Share Extension 예산
(~120MB)에서 시작부터 64MB를 깔면 위험하므로:

- **`mobile/ppshare`** (확장용, 기기 슬라이스 19MB) — `Open`/`Save`/`Close`. scraper를
  import하지 않는다. 본문은 `extract.js`가 DOM에서 뽑아 주므로 애초에 필요 없다. tagger·
  summarizer는 RSS에 잡히지 않아 포함했고, 그래서 **공유 시점에 오프라인으로 태그·요약까지
  붙어** 저장된다. 이 불변식은 의존성 그래프를 보는 테스트로 강제한다.
- **`mobile/ppcore`** (본체용, 49MB) — `Start`/`Addr`/`Stop`. 개별 CRUD 함수를 노출하지 않고
  **인프로세스 chi 서버를 `127.0.0.1:0`에 띄우고 주소만 돌려준다.** 함수로 내보내면 자립
  모드의 JSON이 `api/openapi.yaml`과 갈라져 Swift가 디코더를 두 벌 갖게 되기 때문이다.
  이 방식이면 앱은 생성된 OpenAPI 클라이언트 한 벌로 base URL만 바꾼다. 배선은
  `internal/app`에 있어 서버 바이너리와 **같은 코드**다.
  API 키는 앱이 실행마다 난수로 만들어 넘긴다 — iOS 루프백은 앱 샌드박스를 넘어 공유된다.

SQLite `cache_size(-64000)`은 예약이 아니라 상한이라, DB가 커져도 저장 경로 RSS가 오르지
않는다 — 별도 실행에서 97MB DB 13.8MB vs 빈 DB 14.4MB로, 차이가 방향조차 일정하지 않다.
즉 **측정 잡음은 약 ±1MB**이고(위 표의 13.2~13.4 편차도 같은 폭), 그래서 tagger·summarizer는
"오차 내"라고 말할 수 있고 scraper의 +51MB는 잡음과 혼동할 수 없다. 커넥션 5개×64MB=320MB
우려는 기우였다.

측정은 macOS arm64 기준이다. Go 런타임·순수 Go 의존성이 동일해 이전될 것으로 보지만,
**Xcode 프로젝트가 생기면 실기기에서 재확인한다**(Week 2 항목).

**Week 1**
- Apple Developer Program 가입 ($99/년 — 무료 프로비저닝은 7일 만료라 매일 사용과 양립 불가)
- ATS 결정: 서버 주소는 IP 형식(`http://100.x.y.z:8420`)만 사용 (IP는 ATS 면제). MagicDNS 이름을 쓰려면 `tailscale cert` HTTPS 필수
- `ios/` 워크스페이스에 Xcode 프로젝트 세팅, SwiftUI 앱 골격, API 클라이언트. **API 키 Keychain 저장은 하지 않았다** — 자립 모드는 실행마다 난수 키라 보관 대상이 없다(:154). Keychain은 홈서버 모드가 생길 때 필요해진다
- API 클라이언트는 swift-openapi-generator로 `api/openapi.yaml`에서 생성 — API 타입 수작성 금지. SPM 플러그인 대신 CLI 실행 + 생성물 커밋(클린 빌드 페널티 회피). Swift allOf 생성물(value1/value2) 실측 후 스펙 allOf 해체 여부 결정

**Week 2**
- Share Extension 최소 구현: 공유 시트에서 한 탭 저장
- **공유 출처별 입력 처리**([04-DATA-FLOW.md](04-DATA-FLOW.md) §7.3.1): Safari 공유는
  `NSExtensionJavaScriptPreprocessingFile`에 `extension/src/extract.js`를 지정해 본문까지 받고,
  네이티브 앱 공유는 `NSItemProvider`의 `public.url`·`public.plain-text`를 전부(`public.image`는
  매핑하지 않는다 — 계약(`LinkInput`)에 이미지를 받을 자리가 없고 저장 단위가 URL이다)
  확인해 계약 필드에 채운다. 어느 쪽이든 App Group의 **공유 SQLite에 저장 계약 그대로** 넘긴다.
- 로그인 벽 사이트(인스타그램 등)를 네이티브 공유로 받을 때의 처리는 앱 내 `WKWebView` 세션
  재사용으로 풀 수 있는지 실기기에서 판정한다(§7.3.1 규칙 3). 서버는 자격증명을 갖지 않는다.
- **확장 메모리 실기기 재측정**: 선행 검증의 13.4MB는 macOS arm64 값이다. 확장이 실제 jetsam
  한도 안에서 도는지 `os_proc_available_memory()`로 확인하고, 예상을 벗어나면 ppshare에서
  tagger·summarizer를 떼는 것이 첫 대응이다(scraper는 이미 빠져 있다)
- 시뮬레이터 + localhost 서버로 저장 경로 검증
  — **2026-07-26 통과**: 사파리 공유 → 확장 → App Group SQLite 직접 저장까지 실기 확인.
  `body_source='client'`에 본문 11.8KB가 들어왔고(JS 전처리가 DOM을 실제로 뚫었다는 뜻),
  같은 프로세스에서 태그·요약까지 붙었다. 시뮬레이터는 ad-hoc 서명(`CODE_SIGN_IDENTITY="-"`)
  이면 entitlement가 살아 있어 Apple 계정 없이 이 경로 전체를 검증할 수 있다 —
  서명을 끄면(`CODE_SIGNING_ALLOWED=NO`) App Group이 `client is not entitled`로 죽는다.

**Week 3**
- ~~App Group 로컬 큐: 공유 시 큐에 **우선 기록** → timeoutInterval 2~3s로 POST → 성공 시 큐 제거, 실패/타임아웃 시 큐 잔류하고 시트 닫힘~~
- ~~본앱 실행 시 + BGTaskScheduler로 큐 드레인~~

**이 두 항목은 구현하지 않았다 — 문제가 사라졌기 때문이다.** 큐는 "POST가 실패해도
유실되지 않게" 하려는 장치인데, 확장이 `ppshare`로 App Group의 공유 SQLite에 **직접
쓰면서** 보낼 POST 자체가 없어졌다. 저장은 확장 프로세스 안에서 태그·요약까지 끝나고
커밋되므로 비행기 모드에서도 완결된다. 큐도, 드레인도, BGTaskScheduler도 대상이 없다.

divergence는 [04-DATA-FLOW.md](04-DATA-FLOW.md) §7.4에 기록돼 있다. **되돌리는 조건**도
거기 있다: 실기기에서 확장이 App Group 파일 락을 쥔 채 서스펜드돼 `0xdead10cc`가 나면,
원래의 큐 설계로 복귀하는 것이 대응이다.

**Week 4 — 화면 3종 (2026-07-26 개정)**

Week 1~3의 저장 경로가 시뮬레이터에서 실제로 돌아간 뒤, 목록 화면 하나만 놓고 써 보니
**저장은 되는데 쓸 수가 없다**는 것이 드러났다. 아래 셋은 그때 나온 구체적 결함이고,
"화면을 예쁘게"가 아니라 각각 없으면 못 쓰는 기능이다.

- **목록에 시간 축이 없다.** 제목·URL·태그만 있어 어제 저장한 것과 지난달 것이 구분되지
  않는다. 개인 아카이브에서 회상의 단서는 대개 "언제"다. 날짜 섹션 헤더(오늘·어제·이번
  주·이전)로 끊는다 — keyset 커서가 이미 `(created_at, id)` 정렬이라 페이지 경계와
  섹션 경계가 자연스럽게 맞는다. 행 안에는 상대 시간("3시간 전")을 둔다.
- **눌러도 아무 일이 없다.** 링크를 열려면 원문으로 나가야 하고, 그러면 "무엇을 저장해
  뒀는지 훑는" 동작 자체가 불가능하다. 상세는 **카드뉴스 형태**로 만든다: 추출식 요약
  2~3문장을 각각 카드로 세우고 썸네일·태그·원문 링크를 함께 둔다. LLM 없이 만든 요약이
  제품 표면에 처음 드러나는 자리이므로, 요약 품질 회귀가 여기서 바로 보인다.
- **모아둔 것에 대한 조망이 없다.** `GET /api/v1/stats`가 이미 계약에 있고
  `total_links`·`links_this_week`·`by_tag`·`by_day`(30일)를 준다 — 서버 작업 없이
  화면만 만들면 된다. 일별 막대(Swift Charts), 상위 태그, 헤드라인 수치.

화면이 늘어나므로 `NavigationStack` 하나에서 **TabView(목록·통계)** 로 바꾼다. 검색은 탭이
아니라 목록 안의 `.searchable`이다 — 찾는 대상이 그 목록이라 탭을 나누면 "필터 걸린 목록"과
"검색 결과"라는 거의 같은 두 화면이 생긴다.

여기서 `allOf` 판정이 실제 문제가 된다. 상세 화면은 `LinkDetail`을 쓰는데
swift-openapi-generator가 이를 `value1`/`value2`로 감싸 `detail.value1.title` /
`detail.value2.summary`가 된다(선행 검증에서 실측). **결정: `api/openapi.yaml`은
건드리지 않는다** — Go·TS 생성기는 `allOf`를 정상 처리하므로, 한 생성기의 인체공학을
위해 3개 소비자가 공유하는 계약을 납작하게 만들 이유가 없다. iOS 쪽에 얇은 전달
extension(`var title: String { value1.title }`)을 두어 문제를 그 자리에 가둔다.

**Week 5**
- Tailscale 실기기 구성: VPN On Demand를 Wi-Fi/Cellular 모두 Always로 (필수 단계)
- 검증 시나리오: 공유 탭 → 응답 2초 미만 (`just save-timing`), **서버가 없어도 공유 저장이 그 자리에서 완결·유실 0건** (M4 DoD), 연속 7일 하루 1건+ 저장

**컷 후보 개정.** 원래 "검색 화면·상세 편집 화면"이 컷 후보였다. 실사용 결과 **상세
보기(카드뉴스)는 컷 대상이 아니다** — 저장한 것을 다시 보는 수단이 없으면 아카이브가
아니라 쓰레기통이 된다. 컷 후보는 **검색 화면**(태그 필터로 대체)과 **상세에서의 태그
편집**(보기만 남기고 편집은 M5 이월)으로 좁혔다. 통계 탭은 `by_day` 막대만 남기고
`by_tag`를 접는 식으로 부분 컷이 가능하다.

**결과(2026-07-26): 컷은 발동하지 않았다.** 검색(`.searchable` + `GET /api/v1/search`)과
상세에서의 태그·메모 편집(`PATCH /api/v1/links/{id}`)이 둘 다 들어갔다. 태그 편집을 M5로
미루지 않은 이유는 화면 욕심이 아니라 **`tag_feedback`이 거기서만 만들어지기 때문**이다 —
그게 M5 재랭킹의 학습 데이터인데, 편집이 웹에만 있으면 정작 저장이 일어나는 기기에서
데이터가 안 쌓인다.

**공유 결과 표시 방식 (2026-07-26 확정).** 확장이 자체 화면을 그리지 않는다. iOS는 공유
확장을 시트로 띄우므로 무엇을 그리든 보고 있던 페이지를 덮고, 커스텀 detent로 높이를
줄이려는 시도는 시스템이 시트 크기를 쥐고 있어 통하지 않았다(실측). 대신 **저장 직후
즉시 닫고 로컬 알림 배너로 결과를 알린다** — 화면을 전혀 가리지 않으면서 무엇이 저장됐고
어떤 태그가 붙었는지 전달된다. 배너에 태그를 노출하는 것은 장식이 아니라, 서버·네트워크
없이 태깅이 끝났다는 자립 모드의 주장을 사용자가 매번 확인하는 지점이다.

### M5 태깅 B — **원안 취소, 재범위 (2026-07-26)**

> **원안(4주 통짜 ONNX 앙상블)은 착수 취소했다.** 난이도가 아니라 **판정 불가**가 이유다.
> 아래는 실측으로 다시 잡은 범위이고, 근거는 전부 2026-07-26 조사에서 재현한 값이다.

#### 왜 원안을 취소했는가 — 산술 셋

**(가) 종료 게이트가 만점을 요구한다.** 동결 test Phase A = 54/61 = 0.885. "+10pp 이상"은
≥ 0.985인데 60/61 = 0.9836이 미달이므로 **통과 가능한 결과가 61/61 하나뿐**이다.
통과 조합은 `(개선 7, 회귀 0)` 단 하나. 이 게이트는 Phase A ≤ 0.90에서만 성립하는데,
Phase A가 예상(참고치 80%)을 크게 넘으면서 **잘해서 벌받는 구조**가 됐다.

**(나) 미스의 대부분은 재랭킹으로 못 고친다.** test 미스 7건을 해부하면 **6건은 정답 태그
점수가 정확히 0.000**이다 — threshold 미달이 아니라 아예 매칭이 없다. 순위만 바꿔 되는 건
1건이고, **재랭킹 상한은 55/61 = +1.6pp**다.
>
> **[2026-07-27 추가] 이 +1.6pp는 이미 소진됐다.** 결함 E 수정(한국어 매칭을 어절 앞뒤 모두
> 보게)이 그 순위 밀림 1건을 hit으로 바꿨다. 동결 test는 54/61 = 0.885 → 55/61 = 0.902가
> 됐고 **남은 미스 6건은 전부 정답 0점, 재랭킹 상한 +0.000**이다. 즉 위 (나)의 결론
> — "재랭킹으로는 게이트에 못 닿는다" — 은 더 강해졌다. 근거는 `nlu/golden/README.md`
> 「결함 E 수정」 절. 원안의 "점수 앙상블"이 자연히 사는 모드가
재랭킹인데, 게이트에 닿으려면 0점 태그를 threshold 위로 **승격**시켜야 하고 그건 오탐
대량 유입이라는 다른 위험이다. 원안은 두 모드를 구분하지 않았다.

**(다) 튜닝 셋이 포화됐다.** dev Phase A = 59/62, **미스 3건**. Week 3~4의 "가중치 보정"이
관측할 수 있는 신호가 3항목이다. 게다가 dev 링크의 **73%가 3위와 4위 점수가 동일**해
태그 이름 알파벳순으로 갈리는데, 가중치를 건드리면 그 덩어리가 통째로 재배열되고
hit@3는 그걸 거의 못 본다. **조타 장치 없이 4주를 시작하는 상태였다.**

#### 실측으로 깨진 원안의 전제

| 원안이 적은 것 | 실측 |
|---|---|
| (a) multilingual-e5-small-ko — **ONNX 기제공**, 1순위 | `.onnx` **0개**(safetensors뿐). 기제공 int8을 가진 건 **(b) ko-sroberta**(`onnx/model_qint8_avx512_vnni.onnx`) — 1순위 근거가 뒤집힌다 |
| (b)(c)만 "110M" | e5-small이 **117.65M으로 셋 중 가장 크다**(나머지 110.62M). int8 파일도 112.9 vs 106.2 MiB로 **크기 축이 세 후보를 못 가른다** |
| 배포 (2) hugot 순수 Go "~8배 느리지만 3s 예산 내" | 토크나이저가 1순위 모델의 normalizer를 **조용히 무시** → golden 토큰 일치 **0/123**. 지연은 상수가 아니라 길이 함수(15토큰 24.5배 … 481토큰 43.6배 = **1.35초**), 512토큰 초과는 실패. **후보가 아니다** |
| 배포 (1) cgo — 리스크가 iOS | iOS는 원래 cgo 링킹을 요구해 오히려 된다. 실제로 깨지는 건 **서버 `GOOS=linux` 크로스컴파일**이고, 그때 02 §2의 "GOOS/GOARCH만으로 크로스컴파일"이 무너진다 |
| 진입 게이트 "베이스라인 대비 +15pp" (현재 +54.1pp) | 진짜 바닥은 0.344가 아니다. **내용을 안 보는 상수 예측기 `{article, tutorial, dev}`가 test 0.721**. Phase A의 대응표본 실제 우위는 **+16.4pp**(McNemar p=0.0063) — 마진 표기가 크게 부풀려져 있었다 |
| "토큰 ID 100% 일치" 진입 게이트 | 어느 후보로도 도달 불가(최고 97.6%). 그리고 **"한글 NFC 포함"은 방향이 뒤집혀 있다** — NFD 입력에서 Go와 Python은 *똑같이 망가진 채* 100% 일치한다. 게이트는 초록이고 시스템은 깨진다 |
| **확장(ppshare)에서도 Phase B** | 언급 자체가 없었다. int8 가중치만 106~113MiB인데 확장 여유는 ~106MB이고, 실측 로드 후 RSS는 **367MB(ORT) / 603MB(순수 Go)** — 예산의 3~5배. **물리적으로 불가능하다** |

부수로, 원안과 무관한 **현재 결함**을 하나 찾았다: 유니코드 정규화가 백엔드에 없어 NFD
입력에서 dev 0.952 → 0.710, test 0.885 → 0.689였다. 2026-07-26 수정 완료
([nlu/golden/README.md](../../nlu/golden/README.md)).

#### 재범위 — 세 단계, 각 단계에 폐기 기준

**Phase 0 — 보이지 않는 것을 보이게 하기 (약 1주).**

**이건 기술 부채 상환이 아니다.** 부채는 빌린 것이 있어야 성립한다 — 속도를 얻고 나중에
이자를 내는 것. 여기 있는 것들은 **애초에 만든 적 없는 계기**다. 빌린 게 없으니 갚을 것도
없고, 그래서 "나중에 정리하자"가 성립하지 않는다.

하나만 부채에 가깝다: `just eval`을 **너무 거친 단위로 만들었다.** hit@3 하나만 내는 지표는
아래 두 가지를 구조적으로 못 본다. 그건 만들 때 진 빚이 맞다.

**M5의 선행 작업이 아니라, M5를 할지 말지 판단할 근거다.** 셋 다 M5를 취소해도 값을 한다:

- **동점 가시화** — `just eval`이 3위/4위 동점 비율을 함께 낸다. 지금 dev 링크의 73%가
  3위와 4위 점수가 같아 **태그 이름 알파벳순으로 갈리는데 지표가 그걸 못 본다.**
  E1(사전 확장이 동결 test를 1.7pp 떨어뜨린 것)을 사흘 동안 못 본 이유가 정확히 이거고,
  사전을 다시 건드리면 다시 못 본다.
- **미스 해부** — 미스마다 "정답 태그 점수가 0인가, 순위에서 밀렸는가"를 낸다.
  재랭킹 상한이 +1.6pp라는 것을 알려준 것이 이 구분이었다. **어떤 개선을 하든 조준에
  필요하고**, 없으면 무엇을 고쳤는지 모른 채 숫자만 본다.
- **golden 2차 — 본문 없는 실제 링크 30건.** 현재 golden에 `body_text`가 빈 항목이
  **0건**이다. 즉 **이미 출시된 클라이언트 캡처 경로(SPA·봇 차단·로그인 벽)가 한 번도
  측정된 적이 없다.** M5와 무관하게 구멍이다.

- **폐기 기준: 없음.** 위 셋은 M5를 취소해도 그대로 값을 한다 — 그게 이 단계를 먼저 두는
  이유이자, 여기에 기간을 쓰는 것이 M5에 대한 베팅이 아닌 이유다.

> 게이트 재정의는 이 단계에 있었으나 **2026-07-26에 먼저 끝냈다**(위 "실측으로 깨진 원안의
> 전제"와 [02-TECH-SPEC.md](02-TECH-SPEC.md) §품질 게이트). 통과 불가능한 게이트를 남겨 두면
> 그 아래 계획이 전부 의미를 잃어서 순서를 당겼다.

> ### ⛔ **[2026-07-27] Phase 1 실행 → 폐기 기준 발동 → M5 중단. 컷 순서 3번(M5 전체를 Phase A로 운영)이 발동했다.**
>
> **판정 축이 태깅에서 검색으로 바뀐 뒤** 스파이크를 돌렸다. 태깅은 재랭킹 여지가 세 세트
> 모두 0으로 확인돼(남은 미스가 전부 정답 태그 진짜 0점) 임베딩이 값을 하려면 승격밖에
> 없는데 그건 오탐 위험이고, 검색은 미발견의 8할이 언어 경계라 임베딩이 원래 잘하는 문제였다.
>
> **결과**: 1.0GB 모델(`paraphrase-multilingual-mpnet-base-v2`)은 언어 경계 7건 중 3건을
> 살려 기준을 **정확히 문턱값에서** 통과했다. 그런데 출하 가능한 크기
> (`multilingual-e5-small`, fp32 470MB — 실제 후보인 int8 118MB는 더 나쁠 것)로 내리자
> **3건 → 1건**으로 떨어져 기준 미달이다. 작은 모델은 같은 언어 매칭은 오히려 낫고
> (14/15 vs 11/15) **언어를 건널 때만 무너진다.**
>
> 크기와 별개로 **CGO-free 관문**이 남아 있었다 — Go에서 ONNX를 쓰려면 CGO가 필요하고,
> 그 제약은 SQLite 드라이버 선택까지 결정한 것이다.
>
> **비용·이득**: 색인 수정 두 개가 이미 +12pp를 냈고 모델도 디스크도 안 썼다. 임베딩의
> 상한이 같은 +12pp인데 출하 가능한 크기에서는 +4pp다.
>
> 전체 수치·재현 방법·다시 열 조건은 [`nlu/models/README.md`](../../nlu/models/README.md).
> 아래 원안은 **실행된 그대로** 남긴다 — 무엇을 걸고 무엇으로 판정했는지가 기록이다.

**Phase 1 — 오프라인 타당성 스파이크 (1주). 코드 통합 0, 폐기 전제.**
- `nlu/models/`에서 Python으로만 돌린다. Go 통합·계약·마이그레이션 **전부 없음**
- 후보 모델로 golden 123건의 문서 임베딩과 태그 임베딩을 만들어 **코사인 유사도만으로** Recall@3를 잰다
- **폐기 기준(하나라도 걸리면 M5 중단, 컷 순서 3번 발동)**:
  - 임베딩 단독 Recall@3가 상수 예측기(test 0.721)를 못 넘으면 — 앙상블할 신호가 아니다
  - Phase A가 0점을 준 6건 중 **3건 이상을 살리지 못하면** — 재랭킹 상한 +1.6pp에 갇힌다
  - 태그 42개에 임베딩할 문장이 없다는 문제가 안 풀리면(현재 `tags.json`은 `{name, facet, aliases}`뿐이고 name은 전부 한 단어 영어다)

**Phase 2 — 서버 축 통합 (2주). Phase 1 통과 시에만.**
- 배포는 **서버 축만**. 확장은 Phase A 유지가 확정이다(위 표)
- 결합 규칙을 먼저 정한다 — 현재 score map은 **정수 격자**(가중치 1/2/3 × 정수 매칭 수)에 threshold 1.0이고, 거기에 [-1,1] 코사인을 더할 배율·정규화가 원안에 없다. 그 배율은 Phase 1의 코사인 분포에서 나오므로 **Week 3 항목이 아니라 Phase 1 산출물**이다
- **폐기 기준**: 결합 후 동결 test가 재정의된 게이트를 못 넘거나, 서버 CGO-free 성질을 잃는 대가가 얻는 것보다 크다고 판단되면

#### 명시적 비-범위

- **확장(ppshare)의 Phase B** — 위 표의 실측으로 불가능. 대신 "같은 링크가 저장 경로에 따라 다른 태그를 받는다"를 검증하는 테스트가 필요하다(현재 0개)
- **hugot 순수 Go 백엔드** — 토큰 일치 0/123, 481토큰 1.35초
- **tag_feedback 재랭킹** — `tag_feedback`에 `removed`가 0건이다. 학습할 데이터가 없다. 백로그의 `feedback-golden`(실사용 오분류를 golden 후보로) 쪽이 먼저다

#### 원안에서 살아남은 것 — 추출식 요약

요약은 원안의 Week 3 항목이었지만 **M3 단계에서 이미 구현됐고 Phase B와 무관하다.**
설계 기록이 여기밖에 없으므로 그대로 남긴다.

- **추출식 요약 Phase A (구현 완료, LLM 없이)**: 본문에서 핵심 문장 2~3개를 고르는 extractive 요약 — 생성이 아니라 원문 문장 선택이라 환각 0, 순수 Go(`backend/internal/summarizer`). **TextRank**(어휘 겹침 유사도 기반 문장 그래프 PageRank) + **description-aware MMR**로 중심 문장을 뽑되 설명과 겹치는 문장을 눌러, 인스펙터에서 같은 말을 두 번 하지 않게 한다. M3의 `tagger.Tokenize`(조사 정규화)를 재사용하고, tag 잡이 이미 본문을 읽으므로 **한 번의 본문 처리로 태깅+요약**을 함께 한다(추가 I/O 0).
  - 저장·노출: `links.summary`(마이그레이션 0005) + 계약 **`LinkDetail.summary`만** — 목록(`Link`)·검색(`SearchResult`)에는 싣지 않는다. 웹은 **인스펙터의 「요약」 섹션**(설명 바로 위)에만 그리고 **카드는 바꾸지 않는다**: 요약은 원문 대체재가 아니라 "열까 말까"의 판단 보조이고, 목록 응답을 가볍게 유지한다. 계약이 좁은 덕에 목록·검색 store 경로(`linkCols`/`scanLink`/`sqlite_search.go`)를 한 글자도 건드리지 않는다. `links_fts` 미색인(가상 테이블 재생성 위험 — 재검토는 아래 stage 2).
  - 품질 가드 5겹: 본문 200룬 미만 / 산문 3문장 미만 / 문장별 산문 게이트(목차·코드·이메일 목록 제거) / 총 450룬 캡 / description과 0.8 이상 겹치면 통째로 폐기. 불통과면 빈 문자열이고 UI는 아무것도 그리지 않는다.
  - 측정(`just eval-summary`, golden dev/test 각 50건): **정답 요약이 없어 ROUGE는 불가**하므로 lead-3 베이스라인 대비 상대 게이트만 건다(desc 중복도 · 태그 신호 보존 · 결정성). 실측은 [nlu/golden/README.md](../../nlu/golden/README.md)에 기록한다.
  - **stage 2 후보**(이번 범위 밖): Phase B 임베딩으로 문장 유사도 교체(골격은 그대로 — `similarity()` 하나만 바뀐다), `links_fts`에 summary 색인(desc-aware MMR 덕에 요약 토큰의 대부분이 description 밖이라 진짜 새 검색 표면이다), 카드 본문 슬롯 백필(`description || summary`) — 재검토 트리거는 "description 빈 링크 25% 초과 또는 요약 커버리지 85% 초과".

  - **stage 2 진행 상황(2026-07-26)**: `links_fts`에 summary 색인은 **완료**했다(백로그 B1,
    게이트 87% 통과 — golden 123건 중 107건이 요약에만 있는 3-gram을 얻는다). Phase B로
    문장 유사도를 교체하는 것은 위 재범위의 Phase 2 이후 문제다.

### M6 다듬기 (4주)

**Week 1~2**
- iOS 위젯 (`GET /api/v1/stats` 기반). Live Activity는 M6 범위에서 제외 — "M6 이후 후보"로 이동

**Week 3**
- 성능 튜닝: `/debug/pprof` 프로파일링, `just bench-http` 회귀 확인

**Week 4**
- ~~`scripts/streak.sh`~~ — **2026-07-26 구현 완료**(`just streak`). 연속 정의는 iOS `StatsView`와 같다(오늘 아직 저장 안 했으면 어제부터 센다). M6에는 판정만 남는다
- "LLM 없이 만든 자동 태깅" 기술 글 1편 공개 — M5부터 축적한 메모의 퇴고만

---

## 4. 마일스톤 × 검증 커맨드

| 마일스톤 | 검증 커맨드 | 통과 조건 |
|---|---|---|
| M1 | `just bench-http` / `scripts/coldstart.sh` | HTTP 경로 저장 p99 < 50ms (초과 시 exit 1) / 실행→/healthz 200 < 1s |
| M1 | `just seed 100000` | 벤치용 한영 혼합 시드 DB 생성 (고정 난수) |
| M1 | `just gen-check` | 생성물 드리프트 0 (`just gen` 재실행 후 git diff 없음) |
| M2 | `just test-crash` | 빌드→fixture 서버→저장→kill -9→재기동→전량 done 단언 |
| M3 | `just eval` | 베이스라인 대비 리포트 기록 (게이트 판정은 M5 진입 시) |
| M4 | 시뮬레이터 공유 절차 + `just save-timing` | 공유 탭→응답 2초 미만 (초과 시 exit 1), 서버 오프라인 시에도 저장 성공·유실 0 |
| M5 | `just eval` (동결 test) | 진입: 상수 예측기(0.721) 대비 McNemar p<0.05 — **충족**. 종료: 회귀 0 + 개선 5건 이상 |
| M6 | `scripts/streak.sh` (GET /api/v1/stats by_day) | 최근 28일 연속 count > 0 |

`just save-timing`은 Share Extension이 App Group에 쌓는 `save-timing.jsonl`을 읽는다
(`ios/PushPointShare/SaveTiming.swift`). 공유 자체는 사람이 밟는 절차라 자동화하지 않았고,
**판정만** 스크립트가 한다 — 성공과 실패를 나눠 세는 이유는 실패가 느린 것(대개 타임아웃)이
성공이 느린 것과 다른 문제인데 섞으면 평균만 좋아 보이기 때문이다.

bench-http / test-crash / seed 레시피는 justfile에 기존 가드 패턴("M1/M2에서 활성화" 안내)으로 포함돼 있다. `just bench`(go test 마이크로벤치)는 유지하되, **go test 벤치는 평균만 내므로 p99 판정 수단이 아니다 — p99 판정은 bench-http가 담당한다.**

---

## 5. 품질 게이트 원칙

측정 없는 "잘 되는 것 같다"는 금지한다. 통과 여부는 수치로 판정한다.

- 성능 게이트: `just bench-http` 저장 p99 < 50ms, `scripts/coldstart.sh` 콜드 스타트 < 1s, 검색(1만 링크) < 30ms, 10만 건 목록 < 50ms. 해당 마일스톤의 통과 조건이다
- 태깅 게이트는 **상대 조건**이다. 절대 수치(60% / 80%)는 참고치일 뿐 판정 기준이 아니다:
  - M5 진입 = Phase A가 **상수 예측기**(내용 미참조, test 0.721) 대비 대응표본 유의 — **충족**
  - M5 종료 = 앙상블이 Phase A 대비 **회귀 0 + 개선 5건 이상**(2026-07-26 재정의)
- dev/test 분리·동결: golden set은 dev / test로 분할한다(실제: dev 62 / test 61). 규칙 튜닝은 dev만 사용하고, 게이트 판정은 동결된 test로만 한다
- M3의 태깅 측정은 기록이며 M4 진입을 차단하지 않는다 — 판정 시점은 M5 진입이다

---

## 6. 위험 관리

| 위험 | 대응 |
|---|---|
| 한국어 태깅 품질 미달 (Phase A가 상수 예측기 대비 유의하지 않음) | 태그 사전을 축소해 분류 난도를 낮추고, 수동 보정 데이터(tag_feedback)를 축적해 M5 재랭킹 재료로 쓴다 |
| Go 토크나이저 토큰 불일치 (Python HF와 ID 시퀀스 상이) | 토큰 ID 100% 일치 골든 테스트(한글 NFC 정규화 포함)를 M5 Week 2 진입 게이트로 둔다. 불일치가 해소되지 않으면 후보 토크나이저(sugarme/tokenizer ↔ hugot) 교체 |
| ONNX 배포 복잡화 (dylib 동적 링크로 단일 정적 바이너리 붕괴) | M5 Week 2에 3택 결정: dylib embed 후 시작 시 추출(cgo 감수) / hugot 순수 Go 백엔드(~8배 느리지만 비동기 3s 예산 내) / Phase A 유지 |
| ONNX Go 바인딩 난항 (onnxruntime_go 빌드·호환 문제) | Phase A(규칙 기반)로 버틴다. Phase A 품질로도 실사용은 가능하며 M5 전체가 컷 순서 3번이다 |
| 스크랩 차단 (봇 차단, 빈 응답) | User-Agent 정비, 도메인별 rate limit(1 req/s), 재시도·백오프. 그래도 실패하면 failed + retry API로 수동 재시도 |
| iOS 개발 경험 부족 | M4 기간을 5주로 여유 있게 잡았다. ADP·ATS 결정 → Share Extension → 목록 순으로 핵심부터. 검색·상세 편집은 컷 후보였으나 **컷 없이 구현됐다**(2026-07-26) |
| 흥미 소실 (사이드 프로젝트 최대 리스크) | **실사용 시작 = 5주차(M2 종료)**로 앞당겼다. 매일 쓰는 앱이 되는 순간 동기가 유지된다 |

---

## 7. 명시적 비목표 (v2에서 하지 않는 것)

- k8s / HPA / 멀티 노드 — 유저가 생기면 `deploy/k8s-future/` 부활
- 회원가입 / 멀티유저 — 단일 사용자, API 키 하나로 인증
- OpenAI 등 외부 LLM API 의존 — NLU 파이프라인이 이 프로젝트의 정체성
- Android — iOS 실사용 검증 후 판단
- (2026-07-21 갱신: 웹 프론트엔드는 비목표에서 제외됨 — iOS와 대등한 정식 클라이언트로 승격, 아래 M-Web 참고. 배경은 [09-PLAN-REVIEW.md](09-PLAN-REVIEW.md))

---

## 8. M6 이후 후보

전부 실사용 검증(4주 연속 일일 사용)이 끝난 뒤에만 검토한다.

1. Live Activity — M6 범위에서 제외됨 (컷 순서 1번). 위젯 실사용 후 필요가 확인되면
2. Android — iOS에서의 저장 습관이 자리 잡은 뒤
3. 멀티유저 — 남에게 권할 만한 물건이 됐을 때. Store/Queue/Tagger 인터페이스 뒤 구현체 교체 + `deploy/k8s-future/` 부활로 대응

기능 후보는 [12-BACKLOG.md](12-BACKLOG.md)가 따로 관리한다 (2026-07-26 신설). 위 세 항목이
"방향"이라면 그쪽은 **착수·폐기 조건이 붙은 구체적 후보**다. 지금 살아 있는 넷은:

| 후보 | 요지 | 착수 조건 |
|---|---|---|
| `scripts/streak.sh` | M6 Week 4가 이미 이름까지 지정했는데 파일이 없다 — 연속 저장일 판정기 | 없음 (M6 산출물) |
| 검색 계측 하네스 | 검색 p99 < 30ms 게이트를 **재는 명령이 리포에 없다**. `nlu/golden/search.jsonl` + `just eval-search` + `just bench-search` | 검색을 실제로 건드릴 마음이 있을 때 |
| 요약을 FTS 색인에 | `summary`가 desc와 안 겹치는 문장을 고른다는 실측(overlap 0.10~0.13)이 있어 새 검색 표면이 실재한다 | golden 123건에서 "요약에만 있는 3-gram"을 얻는 링크 비율 **30% 이상** |
| `links.opened_at` | 코어 루프 5단계 중 마지막(재열람)만 계측이 0이다 | 없음 |

**그 문서에서 더 값진 절은 3절이 아니라 4절이다** — 검토했으나 자른 20건이 이유와 함께 있고,
같은 아이디어가 다시 올라올 때 재논의를 막는 것이 그 문서의 주된 존재 이유다. 새 기능이
떠오르면 3절보다 4절을 먼저 찾아보라.

---

관련 문서: [02-TECH-SPEC.md](02-TECH-SPEC.md), [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md), [07-DEPLOYMENT.md](07-DEPLOYMENT.md), [12-BACKLOG.md](12-BACKLOG.md)
