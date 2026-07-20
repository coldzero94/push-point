---
paths:
  - "docs/**"
---

# 문서 규칙

- `docs/v2/`가 단일 진실 원천. `docs/v1/`은 아카이브 — **어떤 이유로도 수정 금지** (깨진 링크도 당시 모습 그대로 보존).
- 문서는 한국어, 담백한 톤, 섹션 제목에 이모지 금지. 상단에 `> Push-Point v2 — 마지막 업데이트: YYYY-MM-DD` 한 줄.
- 원본-파생 관계: 스키마 DDL은 05, API는 `api/openapi.yaml`(기계 원본 — 06은 사람용 해설, 둘이 다르면 openapi.yaml 우선), 마일스톤·DoD는 08이 원본이다. 다른 문서(00/03/04 등)가 같은 내용을 실을 땐 원본과 글자 단위로 일치시켜라. 성능 수치는 5개 지표 표를 어디서든 동일하게.
- v1 스택(PostgreSQL/Redis/RabbitMQ/MinIO/OpenAI/JWT/k8s/HPA/Gin/Ent/React Native)은 "v1→v2 대비" 맥락에서만 언급. 현재 아키텍처 설명에 등장하면 안 된다.
- docs/v2 내부 상호 링크는 파일명만(`05-DATA-SCHEMA.md`), 리포 루트에서 참조할 땐 `docs/v2/...` 경로.
- 문서를 추가/개명하면 `docs/README.md` 비교 인덱스와 `docs/v2/00-README.md` 목차를 함께 갱신.
- 계획(08)을 수정할 땐 `09-PLAN-REVIEW.md`의 반영된 권고(v2.1 확정 사항)와 충돌하지 않는지 확인. 새 점검을 하면 09에 결과와 반영 일자를 기록.
