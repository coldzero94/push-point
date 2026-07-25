# 개발 계획

> Push-Point v2.1 — 마지막 업데이트: 2026-07-21

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
| M4 iOS | 5주 | Share Extension(로컬 큐) + 목록 + Tailscale 실기기 (검색·상세 편집은 컷 후보) | 서버 오프라인에도 공유 저장 2초 내 성공·유실 0건, 연속 7일 하루 1건+ 저장 |
| M5 태깅 B | 4주 | Go 토크나이저 + ONNX 베이크오프 + 앙상블 + tag_feedback 반영 + 추출식 요약(LLM 없이) | 진입: Phase A 베이스라인+15pp. 종료: 앙상블 Phase A+10pp (참고 80%) |
| M6 다듬기 | 4주 | 위젯 + 성능 튜닝 + 공개 글 (Live Activity는 이후 후보) | `scripts/streak.sh` 4주 연속 일일 사용, 기술 글 1편 |
| M-Web 웹 앱 | 병렬 트랙 | Vite+React+TS SPA, `api/openapi.yaml` 계약 소비(openapi-typescript), 6개 화면, Go embed 서빙 | `just web-gen-check` 드리프트 0 + `just web-build` 성공(단일 바이너리 embed), iOS와 대등한 기능 |

**M-Web (웹 앱)** — iOS(M4)와 **대등한 정식 클라이언트**다. iOS를 밀지 않는 병렬 트랙으로, 실사용 열람·검색·관리 수요를 채운다. 두 클라이언트는 같은 `api/openapi.yaml` 계약을 소비하므로 기능이 동일하고, **저장의 "iOS 공유 시트 2초 진입"만 iOS 고유**다(웹은 URL 입력창 + 선택적 북마클릿). 스택·계약 파이프라인·embed 배포 상세는 [02-TECH-SPEC.md](02-TECH-SPEC.md)·`.claude/rules/frontend.md`. M4의 검색·상세 편집 화면이 컷돼도 웹이 백필하므로 그 컷이 안전장치가 된다.

순서의 의미: **실사용 시작이 M2 종료(5주차)로 앞당겨졌다.** 단축어 캡처(M1)와 임포트+매일 저장(M2)으로 실데이터가 먼저 쌓이고, M3~M6은 그 데이터 위에서 돈다. M4는 실사용의 시작점이 아니라 저장 경로를 단축어에서 Share Extension으로 바꿔 마찰을 줄이는 단계다.

### 일정 운영 원칙

- 캐파 가정: 주당 투입 약 10시간 가정 (본인 상황에 맞춰 조정) — 마일스톤 기간은 이 가정 기준이다.
- 합계 22주 + **명시적 버퍼 4주** = 6개월. 버퍼는 계획에 포함된 예산이며, **버퍼 2주 소진 시 컷 발동**.
- 컷 순서 (지연 시 이 순서대로 잘라낸다):
  1. Live Activity (이미 M6 이후 후보로 강등)
  2. 위젯
  3. M5 전체 (Phase A로 운영)
  4. M4 검색 화면 (태그 필터로 대체, M5 병행 이월)
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
- golden set 구축: M2 임포트+실사용 축적분에서 층화 샘플링(도메인·content_type 비율 유지)한 100건을 `nlu/golden/` JSONL로 — **dev 50 / test 50 분할, test는 동결**. 스키마 `{url, snapshot: {title, description, meta_keywords, body_text}, expected_tags: []}` (eval은 네트워크 접근 0, snapshot만 입력)
- `just eval` 구현: top-3 Recall(hit = 예측 top-3 ∩ expected_tags ≥ 1) + 태그별 precision/recall·부착 빈도 표 출력. "도메인 휴리스틱만" 베이스라인을 항상 함께 측정
- 규칙 튜닝(dev만 사용) → **베이스라인 대비 측정치 기록. 정확도 게이트는 M4 진입을 차단하지 않는다 — 판정은 M5 진입 조건으로 이동** (Phase A ≥ 베이스라인+15pp)

### M4 iOS (5주)

**Week 1**
- Apple Developer Program 가입 ($99/년 — 무료 프로비저닝은 7일 만료라 매일 사용과 양립 불가)
- ATS 결정: 서버 주소는 IP 형식(`http://100.x.y.z:8420`)만 사용 (IP는 ATS 면제). MagicDNS 이름을 쓰려면 `tailscale cert` HTTPS 필수
- `ios/` 워크스페이스에 Xcode 프로젝트 세팅, SwiftUI 앱 골격, API 클라이언트, API 키 Keychain 저장 (앱 그룹 공유)
- API 클라이언트는 swift-openapi-generator로 `api/openapi.yaml`에서 생성 — API 타입 수작성 금지. SPM 플러그인 대신 CLI 실행 + 생성물 커밋(클린 빌드 페널티 회피). Swift allOf 생성물(value1/value2) 실측 후 스펙 allOf 해체 여부 결정

**Week 2**
- Share Extension 최소 구현: 공유 시트에서 한 탭 저장
- 시뮬레이터 + localhost 서버로 저장 경로 검증

**Week 3**
- App Group 로컬 큐: 공유 시 큐에 **우선 기록** → timeoutInterval 2~3s로 POST → 성공 시 큐 제거, 실패/타임아웃 시 큐 잔류하고 시트 닫힘. "요청 발사 후 즉시 닫기" 금지 (in-flight 유실)
- 본앱 실행 시 + BGTaskScheduler로 큐 드레인. 재시도 안전성 근거: `POST /api/v1/links`는 url_hash 멱등 (중복 시 200 duplicate:true)

**Week 4**
- 목록 화면 (커서 페이지네이션 무한 스크롤) + 태그 필터 + 썸네일 이미지 로더

**Week 5**
- Tailscale 실기기 구성: VPN On Demand를 Wi-Fi/Cellular 모두 Always로 (필수 단계)
- 검증 시나리오: 공유 탭 → 응답 2초 미만 (클라이언트 계측 로그), **서버 오프라인 상태에서도 공유 저장 2초 내 성공(로컬 큐 적재)·서버 복구 후 자동 업로드 유실 0건** (M4 DoD), 연속 7일 하루 1건+ 저장

검색 화면·상세 편집 화면은 M4 범위에서 **컷 후보**다 — 지연 시 태그 필터로 대체하고 M5와 병행 이월한다.

### M5 태깅 B (4주)

진입 조건: **Phase A가 베이스라인 대비 +15pp 이상** (동결 test 셋 기준, 절대 60%는 참고치). M5 시작 시 실사용 축적분으로 두 번째 golden set을 추가한다.

**Week 1**
- 모델 후보 3종 베이크오프: (a) multilingual-e5-small-ko 계열 (ONNX 기제공, 384-dim, 1순위 검토 — "query:/passage:" 프리픽스 규약 주의), (b) jhgan/ko-sroberta-multitask int8 (110M), (c) BM-K/KoSimCSE int8 (110M, ONNX 직접 변환 필요). 크기·지연·정확도 실측으로 선정 (변환 스크립트·아티팩트는 `nlu/models/`)
- Go 토크나이저 선정: sugarme/tokenizer 또는 knights-analytics/hugot (onnxruntime_go는 텐서 입출력만 제공)

**Week 2**
- 진입 게이트: **Python HF 토크나이저와 토큰 ID 시퀀스 100% 일치 골든 테스트** (한글 NFC 정규화 포함) 통과 후 추론 구현 착수
- yalue/onnxruntime_go로 Go 내 추론, `tag_embeddings` 캐시 생성·갱신
- 배포 형태 3택 결정: (1) libonnxruntime.dylib을 embed 후 시작 시 data/로 추출 (cgo 빌드 감수), (2) hugot 순수 Go 백엔드 (~8배 느리지만 태깅은 비동기 3s 예산 내), (3) Phase A 유지

**Week 3**
- 문서 임베딩 vs 태그 임베딩 코사인 유사도 분류 → Phase A와 점수 앙상블
- **추출식 요약 Phase A (구현 완료, LLM 없이)**: 본문에서 핵심 문장 2~3개를 고르는 extractive 요약 — 생성이 아니라 원문 문장 선택이라 환각 0, 순수 Go(`backend/internal/summarizer`). **TextRank**(어휘 겹침 유사도 기반 문장 그래프 PageRank) + **description-aware MMR**로 중심 문장을 뽑되 설명과 겹치는 문장을 눌러, 인스펙터에서 같은 말을 두 번 하지 않게 한다. M3의 `tagger.Tokenize`(조사 정규화)를 재사용하고, tag 잡이 이미 본문을 읽으므로 **한 번의 본문 처리로 태깅+요약**을 함께 한다(추가 I/O 0).
  - 저장·노출: `links.summary`(마이그레이션 0005) + 계약 **`LinkDetail.summary`만** — 목록(`Link`)·검색(`SearchResult`)에는 싣지 않는다. 웹은 **인스펙터의 「요약」 섹션**(설명 바로 위)에만 그리고 **카드는 바꾸지 않는다**: 요약은 원문 대체재가 아니라 "열까 말까"의 판단 보조이고, 목록 응답을 가볍게 유지한다. 계약이 좁은 덕에 목록·검색 store 경로(`linkCols`/`scanLink`/`sqlite_search.go`)를 한 글자도 건드리지 않는다. `links_fts` 미색인(가상 테이블 재생성 위험 — 재검토는 아래 stage 2).
  - 품질 가드 5겹: 본문 200룬 미만 / 산문 3문장 미만 / 문장별 산문 게이트(목차·코드·이메일 목록 제거) / 총 450룬 캡 / description과 0.8 이상 겹치면 통째로 폐기. 불통과면 빈 문자열이고 UI는 아무것도 그리지 않는다.
  - 측정(`just eval-summary`, golden dev/test 각 50건): **정답 요약이 없어 ROUGE는 불가**하므로 lead-3 베이스라인 대비 상대 게이트만 건다(desc 중복도 · 태그 신호 보존 · 결정성). 실측은 [nlu/golden/README.md](../../nlu/golden/README.md)에 기록한다.
  - **stage 2 후보**(이번 범위 밖): Phase B 임베딩으로 문장 유사도 교체(골격은 그대로 — `similarity()` 하나만 바뀐다), `links_fts`에 summary 색인(desc-aware MMR 덕에 요약 토큰의 대부분이 description 밖이라 진짜 새 검색 표면이다), 카드 본문 슬롯 백필(`description || summary`) — 재검토 트리거는 "description 빈 링크 25% 초과 또는 요약 커버리지 85% 초과".

**Week 4**
- tag_feedback 데이터로 재랭킹 가중치 보정
- `just eval` (동결 test) → **앙상블이 Phase A 대비 +10pp 이상 통과** (절대 80%는 참고치)
- 기술 글 메모 축적 시작 (베이크오프·토크나이저 실측 기록 — 이후 마일스톤마다 축적, M6 Week 4는 퇴고만)

### M6 다듬기 (4주)

**Week 1~2**
- iOS 위젯 (`GET /api/v1/stats` 기반). Live Activity는 M6 범위에서 제외 — "M6 이후 후보"로 이동

**Week 3**
- 성능 튜닝: `/debug/pprof` 프로파일링, `just bench-http` 회귀 확인

**Week 4**
- `scripts/streak.sh` (`GET /api/v1/stats`의 by_day 기반) — **최근 28일 연속 count > 0** 확인
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
| M4 | 시뮬레이터 공유 절차 + 클라이언트 계측 로그 | 공유 탭→응답 2초 미만, 서버 오프라인 시에도 큐 적재 성공·유실 0 |
| M5 | `just eval` (동결 test) | Phase A 베이스라인+15pp(진입), 앙상블 Phase A+10pp(종료) |
| M6 | `scripts/streak.sh` (GET /api/v1/stats by_day) | 최근 28일 연속 count > 0 |

bench-http / test-crash / seed 레시피는 justfile에 기존 가드 패턴("M1/M2에서 활성화" 안내)으로 포함돼 있다. `just bench`(go test 마이크로벤치)는 유지하되, **go test 벤치는 평균만 내므로 p99 판정 수단이 아니다 — p99 판정은 bench-http가 담당한다.**

---

## 5. 품질 게이트 원칙

측정 없는 "잘 되는 것 같다"는 금지한다. 통과 여부는 수치로 판정한다.

- 성능 게이트: `just bench-http` 저장 p99 < 50ms, `scripts/coldstart.sh` 콜드 스타트 < 1s, 검색(1만 링크) < 30ms, 10만 건 목록 < 50ms. 해당 마일스톤의 통과 조건이다
- 태깅 게이트는 **상대 조건**이다. 절대 수치(60% / 80%)는 참고치일 뿐 판정 기준이 아니다:
  - M5 진입 = Phase A가 "도메인 휴리스틱만" 베이스라인 대비 **+15pp 이상**
  - M5 종료 = 앙상블이 Phase A 대비 **+10pp 이상**
- dev/test 분리·동결: golden set은 dev 50 / test 50으로 분할한다. 규칙 튜닝은 dev만 사용하고, 게이트 판정은 동결된 test로만 한다
- M3의 태깅 측정은 기록이며 M4 진입을 차단하지 않는다 — 판정 시점은 M5 진입이다

---

## 6. 위험 관리

| 위험 | 대응 |
|---|---|
| 한국어 태깅 품질 미달 (Phase A가 베이스라인+15pp 미달) | 태그 사전을 축소해 분류 난도를 낮추고, 수동 보정 데이터(tag_feedback)를 축적해 M5 재랭킹 재료로 쓴다 |
| Go 토크나이저 토큰 불일치 (Python HF와 ID 시퀀스 상이) | 토큰 ID 100% 일치 골든 테스트(한글 NFC 정규화 포함)를 M5 Week 2 진입 게이트로 둔다. 불일치가 해소되지 않으면 후보 토크나이저(sugarme/tokenizer ↔ hugot) 교체 |
| ONNX 배포 복잡화 (dylib 동적 링크로 단일 정적 바이너리 붕괴) | M5 Week 2에 3택 결정: dylib embed 후 시작 시 추출(cgo 감수) / hugot 순수 Go 백엔드(~8배 느리지만 비동기 3s 예산 내) / Phase A 유지 |
| ONNX Go 바인딩 난항 (onnxruntime_go 빌드·호환 문제) | Phase A(규칙 기반)로 버틴다. Phase A 품질로도 실사용은 가능하며 M5 전체가 컷 순서 3번이다 |
| 스크랩 차단 (봇 차단, 빈 응답) | User-Agent 정비, 도메인별 rate limit(1 req/s), 재시도·백오프. 그래도 실패하면 failed + retry API로 수동 재시도 |
| iOS 개발 경험 부족 | M4 기간을 5주로 여유 있게 잡았다. ADP·ATS 결정 → Share Extension → 로컬 큐 → 목록 순으로 핵심부터. 검색·상세 편집은 컷 후보 |
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

---

관련 문서: [02-TECH-SPEC.md](02-TECH-SPEC.md), [03-SYSTEM-ARCHITECTURE.md](03-SYSTEM-ARCHITECTURE.md), [07-DEPLOYMENT.md](07-DEPLOYMENT.md)
