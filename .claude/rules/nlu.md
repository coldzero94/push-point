---
paths:
  - "nlu/**"
---

# NLU 워크스페이스 규칙

- `nlu/`는 **오프라인 자산 전용**이다. 런타임 추론 코드(규칙 태거, ONNX 추론)는 `backend/internal/tagger`(Go)에 두고, 여기에는 두지 않는다.
- Python은 리포 전체에서 `nlu/models/`(모델 변환·양자화 스크립트)에서만 허용된다.
- `nlu/dictionary/` — 통제된 태그 사전(초기 30~50개, name + aliases). 자유 태그 "생성"이 아니라 이 사전에 대한 "분류"가 원칙이다. 사전 변경은 평가 수치에 영향을 주므로 변경 시 `just eval`을 다시 돌려 기록한다.
- `nlu/golden/` — 평가용 golden set (JSONL, 커밋 대상). golden set은 평가 기준이므로 태거를 golden set에 맞춰 고치는 건 허용, **golden set을 태거에 맞춰 고치는 건 금지** (라벨 오류 수정만 예외).
- 품질 게이트는 상대 조건: M5 진입 = Phase A가 "도메인 휴리스틱만" 베이스라인 대비 +15pp, M5 종료 = 앙상블이 Phase A 대비 +10pp (절대 60%/80%는 참고치). 판정은 동결된 test 50건만. 측정 없이 품질을 주장하지 않는다.
- 파이프라인 상세 원본: `docs/v2/02-TECH-SPEC.md`의 NLU 절, 평가 프로토콜(스냅샷 오프라인·dev/test 분리)은 `nlu/golden/README.md`. 배경·근거는 `docs/v2/09-PLAN-REVIEW.md` (반영 완료).
