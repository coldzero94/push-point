# nlu — NLU 오프라인 자산

> Push-Point v2 — 마지막 업데이트: 2026-07-20

**여기는 런타임 코드가 아니다.** 태깅 추론(규칙 태거, ONNX 추론)은 전부
`backend/internal/tagger`의 Go 코드에서 실행된다. 이 워크스페이스는 그 추론이
소비하는 자산만 관리하며, backend는 여기 산출물(사전 시드, .onnx 파일)을 읽기만 한다.

## 현재 상태

- `dictionary/` — 채워짐 (M3 Stage 1). `golden/` — M3 Stage 3 예정. `models/` — M5 예정 (`.gitkeep`).

## 구성

- `dictionary/` — 통제된 태그 사전. (M3)
  - `tags.json` — 태그 30개: `{name, facet, aliases}`. 마이그레이션 시드(`0002` aliases + `0003` facet)의 **커밋된 미러**다. 런타임 태거는 DB `tags` 테이블(마이그레이션이 시드)을 읽으므로 이 파일 자체를 임베드하지는 않지만, 사람이 읽는 정본이자 드리프트 검사 대상이다. 새 태그는 새 마이그레이션 + 이 파일 동시 갱신으로 들어온다(마이그레이션은 불변).
  - `domains.json` — 도메인→태그 휴리스틱 맵. Phase A 태거의 강한 신호이자 `just eval`의 "도메인 휴리스틱만" 베이스라인. 값 태그는 전부 `tags.json`에 존재해야 한다.
  - **드리프트 검사**: `just dict-lint`(CI 포함)이 `tags.json` ↔ 시드 마이그레이션, `domains.json` ↔ `tags.json`을 대조한다(`enum-lint`와 대칭).
- `golden/` — 태깅 품질 golden set. 실제 저장 링크 100개 기반 JSONL, `just eval`의 입력. 커밋 대상. (M3 Stage 3)
- `models/` — ONNX 변환 Python 스크립트 + 모델 아티팩트. **리포에서 Python이 허용되는 곳은 여기뿐이다.** (M5)

## 파이프라인 요약 (Phase A/B 2단계)

원칙: 자유 태그 "생성"이 아니라, 통제된 태그 사전에 대한 "분류"로 문제를 좁힌다.

- **Phase A — 규칙 + 통계 (순수 Go, M3)**
  도메인 휴리스틱 + 조사 접미 정규화 기반 후보구 추출·TF-IDF 스코어링 +
  태그 사전 name/aliases 매칭 → 점수 병합 → 상위 k(≤5), threshold 컷.
- **Phase B — 임베딩 분류 (ONNX, M5)**
  경량 한국어 문장 임베딩 모델을 ONNX(int8 양자화)로 변환, Go에서
  yalue/onnxruntime_go로 추론. 문서 임베딩 vs 태그 사전 임베딩 코사인
  유사도 → Phase A와 점수 앙상블.

## 품질 게이트

`just eval`이 `golden/`의 JSONL(snapshot 입력, 네트워크 접근 0)로 top-3 Recall을 측정한다.
게이트는 상대 조건이다 — **M5 진입: Phase A가 "도메인 휴리스틱만" 베이스라인 대비 +15pp,
M5 종료: 앙상블이 Phase A 대비 +10pp** (절대 60%/80%는 참고치). 판정은 동결된 test 50건.
상세는 [golden/README.md](golden/README.md). 측정 없는 "잘 되는 것 같다"는 금지.

## 더 보기

- NLU 파이프라인 상세: [../docs/v2/02-TECH-SPEC.md](../docs/v2/02-TECH-SPEC.md)의 NLU 절
