# nlu/golden — 태깅 품질 golden set

> Push-Point v2.1 — 마지막 업데이트: 2026-07-20

`just eval`이 읽는 태깅 정확도 측정용 데이터셋. JSONL 형식으로 커밋한다.

## 스키마 (한 라인 = 한 링크)

```json
{"url": "...", "snapshot": {"title": "...", "description": "...", "meta_keywords": "...", "body_text": "..."}, "expected_tags": ["...", "..."]}
```

- eval은 **네트워크 접근 0** — 태거 입력은 snapshot 필드만 사용한다. URL을 다시 fetch하지 않으므로 결과가 시점에 무관하게 재현된다.
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
- 태그별 precision/recall과 태그별 부착 빈도를 표로 함께 출력한다.

## 베이스라인과 상대 게이트

- eval은 항상 "도메인 휴리스틱만" 베이스라인 구성을 동시 측정한다.
- 게이트는 상대 조건이다: M5 진입 = Phase A가 베이스라인 대비 +15pp 이상(절대 60%는 참고치), M5 종료 = 앙상블이 Phase A 대비 +10pp 이상(절대 80%는 참고치).

## 수집 방법

- M2 임포트(북마크·Takeout) + 실사용 축적분에서 도메인·content_type 비율을 유지하는 층화 샘플링으로 구축한다.
- M5 시작 시 실사용 축적분으로 두 번째 셋을 추가한다.
