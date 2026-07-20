# 계획 점검 결과 (Plan Review)

> Push-Point v2.1 — 마지막 업데이트: 2026-07-20
> 상태: **반영 완료 (2026-07-20)** — 권고 8건 전부 docs/v2·justfile(구 Makefile)에 반영됨.

## 1. 점검 방법

v2 계획 전체(docs/v2)를 5개 렌즈(NLU 접근법 타당성, Go 스택 팩트체크, 로컬 검증 가능성, 모바일 실사용 신뢰성, 일정·순서)로 적대적 리뷰했다. finding 34건 중 사실 주장 21건은 1차 출처(공식 문서, GitHub README, 실측 실험)로 교차 검증했다.

## 2. 총평

**아키텍처 코어는 통과.** 단일 바이너리 + SQLite(WAL)+FTS5 + jobs 테이블 큐 + 통제 사전 분류 방향은 검증에서 살아남았다. modernc.org/sqlite 실측(CGO_ENABLED=0): 저장 트랜잭션 p99 129µs, 10만 건 keyset 목록 p99 92µs, 1만 건 trigram 검색 ~13ms — FTS5 trigram과 `UPDATE...RETURNING` 모두 동작 확인. 성능 게이트는 수십 배 여유.

**최대 결함은 기술이 아니라 순서.** 실사용 시작이 M4 종료(14주차)인데 M3 golden set은 "실제 저장 링크 100개"를 요구한다 — M3 시점에 실데이터가 존재할 수 없다. "흥미 소실 대응 = 실사용 최우선"이라는 리스크 대응책과도 자기모순.

## 3. 권고 8건 (우선순위순)

### ① 실사용 조기화 — 최우선

iOS 단축어(공유 시트 → Get Contents of URL POST, 커스텀 헤더 지원 — Apple 공식 확인)로 **앱 없이 M1 직후부터 폰 캡처 가능**. M1 DoD에 "단축어로 폰에서 저장 1건 성공" 추가, M2에 브라우저 북마크/YouTube Takeout 일괄 임포트(300~500건) 추가. 실사용 앵커가 5주차로 당겨지고 golden set·corpus_df 콜드 스타트 문제가 함께 풀린다. M3 60% 게이트는 "M4 진입 차단"에서 "M5 진입 조건"으로 이동.

→ 반영: 08-DEVELOPMENT-PLAN.md · 07-DEPLOYMENT.md (2026-07-20)

### ② Phase A 한국어 명세화

현재 "TF-IDF + RAKE 변형"은 매칭 단위 미명세: 토큰 정확 매칭이면 "쿠버네티스를"≠사전"쿠버네티스"(조사 문제), 부분 문자열이면 2자 alias(`ai`,`ml`)가 "said" 등에 오탐. RAKE의 불용어-구분자 방식은 교착어에서 원리적으로 실패. → 조사 접미 스트리핑 정규화(20~30개 목록)를 corpus_df 누적·사전 매칭 양쪽에 동일 적용, 한글 항목은 정규화 후 전방일치, 라틴 3자 미만은 단어 경계 필수. 02 §7에 알고리즘 수준으로 기술.

→ 반영: 02-TECH-SPEC.md · 08-DEVELOPMENT-PLAN.md (2026-07-20)

### ③ M5 현실화 — 토크나이저·배포 형태

yalue/onnxruntime_go는 **cgo 필수 + onnxruntime 공유 라이브러리(.dylib) 런타임 로드** (README 확인) — "CGO-free 단일 정적 바이너리" 주장이 M5부터 깨지므로 02/07/08 문구 정정. HF 토크나이저와 토큰 ID 일치하는 Go 토크나이저 태스크가 M5에 통째로 빠져 있음 — Week 1에 추가(sugarme/tokenizer 또는 hugot, Python 대비 토큰 일치 골든 테스트). 모델 후보에 multilingual-e5-small-ko 계열(ONNX 기제공, 384-dim) 추가 — KoSimCSE/ko-sroberta는 110M(base급)이고 KoSimCSE는 공식 ONNX 없음.

→ 반영: 02-TECH-SPEC.md · 07-DEPLOYMENT.md · 08-DEVELOPMENT-PLAN.md (2026-07-20)

### ④ 평가 프로토콜 확정

"top-3 정확도"의 수학적 정의가 없고, 튜닝과 판정이 같은 100개(과적합), eval이 라이브 재스크랩이면 비결정적. → golden set은 스크랩 스냅샷 포함 JSONL(`{url, snapshot:{title,description,...}, expected_tags}`)로 오프라인화, dev 50/test 50 분리(판정은 동결된 test만), "도메인 휴리스틱만" 베이스라인을 항상 함께 측정해 게이트를 상대 조건(베이스라인 +15pp 등)으로.

→ 반영: 02-TECH-SPEC.md · 08-DEVELOPMENT-PLAN.md · nlu/golden/README.md (2026-07-20)

### ⑤ 검증 매트릭스 — DoD를 커맨드로

`go test -bench`는 평균만 내므로 "p99 < 50ms" 판정 커맨드가 현재 존재하지 않는다. → `just bench-http`(HTTP 경로 p99 측정, 임계 초과 시 exit 1), `scripts/coldstart.sh`, `just test-crash`(바이너리 기동→kill -9→재기동→복구 단언, fixture HTTP 서버로 외부 의존 제거), `just seed 100000`(벤치용 시드 생성기). 08에 "마일스톤 × 검증 커맨드" 표 추가.

→ 반영: 08-DEVELOPMENT-PLAN.md · justfile(구 Makefile) (2026-07-20)

### ⑥ Share Extension 캡처 신뢰성

서버 불달(맥 잠자기·VPN 끊김) 시 저장이 유실되는 구조. → App Group 로컬 큐 우선 기록 → 2~3s 타임아웃 POST → 실패 시 큐 잔류 + 본앱/BGTaskScheduler 드레인. url_hash 멱등이라 재시도 안전. "POST 후 즉시 닫힘"은 in-flight 유실 경로라 금지(Apple 생명주기 문서 확인). M4 DoD를 "서버가 꺼져 있어도 공유 저장은 항상 2초 내 성공(큐 적재), 유실 0건"으로.

→ 반영: 02-TECH-SPEC.md · 04-DATA-FLOW.md · 06-API-SPECIFICATION.md · 08-DEVELOPMENT-PLAN.md (2026-07-20)

### ⑦ 07 운영 문서 보강

- 절전: launchd KeepAlive는 절전과 무관. 잠든 맥은 Tailscale로 못 깨움 → `pmset sleep 0`, 자동 로그인, `pmset autorestart 1`.
- ATS: IP 직결(`http://100.x.y.z`)은 면제로 동작, MagicDNS 이름 + 평문 HTTP는 차단 → IP만 쓰거나 `tailscale cert` HTTPS. Tailscale iOS는 VPN On-Demand(Always) 설정 필수.
- Apple Developer Program($99/년)을 M4 전제로 명시 — App Groups·Keychain Sharing은 무료 계정도 가능하지만(공식 표 확인, 점검 중 반대 주장은 반박됨), **무료 프로비저닝은 7일 만료**라 "매일 쓰는 앱"과 양립 불가.
- `/thumbs/` Bearer 인증 면제(AsyncImage가 커스텀 헤더 미지원, Tailscale이 이미 네트워크 경계).
- 스크래퍼 어댑터: X는 og 메타 없음 → `publish.twitter.com/oembed` 분기, 네이버 블로그는 `m.blog.naver.com` 재작성, Instagram은 메타 부재 허용 규칙 (셋 다 실측 재현됨).
- 검색 2자 쿼리: q<3 400 거부 대신 LIKE 폴백(실측 10만 행 풀스캔 37ms).

→ 반영: 02-TECH-SPEC.md · 04-DATA-FLOW.md · 06-API-SPECIFICATION.md · 07-DEPLOYMENT.md (2026-07-20)

### ⑧ 일정 구조

주당 투입 시간 가정 명문화, 22주 합계 vs 6개월의 차이 4주를 명시적 버퍼로 선언 + 소진 규칙. 지연 시 컷 순서: Live Activity(M6 이후로 강등) → 위젯 → M5 전체(Phase A 60%로 운영) → M4 검색 화면. 어떤 경우에도 지키는 앵커 = 실사용 시작 주차. M1 "backend 재편"은 신규 작성으로 정정(반영 완료 2026-07-20), M6 과밀 해소(기술 글 메모는 M5부터 축적).

→ 반영: 08-DEVELOPMENT-PLAN.md (2026-07-20)

## 4. 주요 팩트체크 결과 요약

| 주장 | 판정 |
|---|---|
| onnxruntime_go는 cgo + 공유 라이브러리 필요 (단일 정적 바이너리 불가) | 확인 |
| M5에 토크나이저 작업 부재 (HF 토큰 일치 필요) | 확인 |
| 무료 Apple 계정은 App Groups/Keychain Sharing 불가 | **반박** (공식 표상 가능) |
| 무료 프로비저닝 7일 만료 → 매일 사용 불가 | 확인 |
| iOS 단축어로 헤더 포함 POST 캡처 가능 | 확인 |
| ATS: IP 직결 면제, 호스트네임 평문 차단 | 확인 |
| 잠든 맥은 Tailscale로 깨울 수 없음 | 확인 |
| X/Instagram/네이버 블로그는 정적 fetch로 og 메타 불가 | 확인 (실측) |
| YouTube oEmbed에 description 없음 (태거 입력 빈약) | 확인 |
| modernc.org/sqlite: FTS5 trigram·RETURNING 동작, 성능 여유 | 확인 (실측) |
| `go test -bench`로는 p99 측정 불가 | 확인 |
