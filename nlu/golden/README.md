# nlu/golden — 태깅 품질 golden set

> Push-Point v2.1 — 마지막 업데이트: 2026-07-20

`just eval`이 읽는 태깅 정확도 측정용 데이터셋. JSONL 형식으로 커밋한다.

## 스키마 (한 라인 = 한 링크)

```json
{"url": "...", "snapshot": {"title": "...", "description": "...", "body_text": "..."}, "expected_tags": ["...", "..."]}
```

- eval은 **네트워크 접근 0** — 태거 입력은 snapshot 필드만 사용한다. URL을 다시 fetch하지 않으므로 결과가 시점에 무관하게 재현된다.
- snapshot은 **런타임 태거 입력과 정확히 같은 표면**이다(`title`·`description`·`body_text`, 도메인은 `url`에서 파생). 이게 train/serve skew를 없앤다 — 그래서 golden은 **프로덕션 스크랩 경로**(`pushpoint golden-capture`, go-trafilatura 본문 추출)로 캡처한다. 더 풍부한 소스에서 뜨지 않는다.
- `meta_keywords`는 스키마에서 제외했다 — 태거(`tagger.Content`)가 소비하지 않으므로 넣으면 skew가 생긴다. 현대 웹에서 대체로 죽은 신호라, 훗날 eval이 이득을 보이면 그때 스크래퍼·태거에 함께 추가한다.
- `expected_tags`는 태그 사전(`nlu/dictionary/`)에 존재하는 name만 사용한다.

## 분할: dev.jsonl 50 / test.jsonl 50

- **dev.jsonl (50건)**: 규칙·threshold·사전 튜닝은 여기에만 대고 한다.
- **test.jsonl (50건)**: 동결. 게이트 판정(마일스톤 진입/종료 조건)은 동결된 test로만 한다.

## 동결 규칙

- 태거를 golden set에 맞추는 것은 허용, **golden set을 태거 출력에 맞추는 역방향은 금지**.
- test.jsonl 수정의 유일한 예외는 라벨 오류(잘못 부여된 expected_tags) 수정이며, 수정 사유를 커밋 메시지에 남긴다.

## 지표 정의 (`just eval` 출력)

- **hit**: 예측 top-3 ∩ expected_tags ≥ 1
- **Recall@3**: hit 수 / 전체 건수
- **세 변형을 동시 측정**: `full`(도메인+제목+설명+본문) / `no-body`(본문 제외 — 본문 기여 Δ) / `baseline`(도메인 휴리스틱만 — 규칙 기여 Δ).
- 태그별 precision/recall(full, top-3 기준)과 태그별 golden 등장 빈도를 표로 함께 출력한다.

## 측정 기록 (2026-07-25, 초기 golden set — dev/test 각 50건, 사전 30태그)

`pushpoint eval` 결과:

| 세트 | Recall@3 full | no-body (Δbody) | baseline 도메인만 (Δrules) |
|---|---|---|---|
| dev  | 0.900 | 0.820 (+0.080) | 0.420 (+0.480) |
| test | 0.880 | 0.780 (+0.100) | 0.400 (+0.480) |

- **규칙 태거가 도메인 베이스라인을 +48pp 상회** — M5 진입 게이트(+15pp)를 크게 넘는다(M3엔 게이트 없음, 기록만).
- **본문(body_text)이 Recall@3를 +8~10pp 올린다** — go-trafilatura 본문 추출의 실측 이득.
- 튜닝 여지(dev 관찰, 후속): `data`(정밀도 0.06 — alias `데이터`가 한국어 텍스트에 과다 매칭)·`backend` 정밀도 낮음, `news`·`life`·`video` 등 저빈도 태그 미탐. 사전 alias 정제는 새 마이그레이션이 필요해 별도 작업(또는 M5).
- body 얇은 20여 건은 velog·toss·brunch 등 **한국 SPA 블로그**(JS 렌더 — 프로덕션도 동일하게 title/description만) → golden==runtime 유지, headless는 측정 게이트 후(로드맵 M6+).

## 베이스라인과 상대 게이트

- eval은 항상 "도메인 휴리스틱만" 베이스라인 구성을 동시 측정한다.
- 게이트는 상대 조건이다: M5 진입 = Phase A가 베이스라인 대비 +15pp 이상(절대 60%는 참고치), M5 종료 = 앙상블이 Phase A 대비 +10pp 이상(절대 80%는 참고치).

## 수집 방법 — `pushpoint golden-capture`

- 입력 TSV(한 줄 `url<TAB>tag,tag,tag`)를 **프로덕션 스크랩 경로**로 돌려 snapshot을 채운 JSONL을 낸다:
  `go run ./cmd/pushpoint golden-capture urls.tsv > out.jsonl`. 이게 golden==runtime을 강제한다.
- URL·`expected_tags`는 사전 30태그를 고루 덮는 실제·대표 URL을 큐레이션하고(한국어 40 / 영어 60),
  `expected_tags`는 태거 출력이 아니라 **콘텐츠에 실제로 맞는 사전 태그**를 독립적으로 라벨한다(동결 규칙).
- 도메인·content_type 비율을 유지하는 층화 샘플링. M5 시작 시 실사용 축적분으로 두 번째 셋을 추가한다.
