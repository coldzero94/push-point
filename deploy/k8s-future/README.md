# deploy/k8s-future

> Push-Point v2 — 마지막 업데이트: 2026-07-20

이 디렉터리는 v1의 Kubernetes 매니페스트 보존소다. **현재 사용하지 않는다.**
v2는 단일 Go 바이너리(`pushpoint`)로 동작하며, `just dev` 한 번으로 전체 스택이 뜬다.
배경과 로드맵은 [docs/v2/ko/08-DEVELOPMENT-PLAN.md](../../docs/v2/ko/08-DEVELOPMENT-PLAN.md) 참고.

## 왜 접었나

유저 0명인 프로젝트에 오토스케일링(HPA)과 멀티 노드 구성은 역설계다.
지금 필요한 것은 인프라가 아니라 매일 쓰는 제품이다.
다만 **지금 접는 것이지 버리는 것이 아니다** — 그래서 삭제 대신 이 디렉터리로 이동해 보존한다.

## 보존된 파일

| 파일 | 설명 |
|---|---|
| `namespace.yaml` | `push-point` 네임스페이스 정의 |
| `configmap.yaml` | DB/Redis/MinIO 접속 설정 (비밀 아닌 환경 변수) |
| `secret.yaml` | DB 비밀번호, MinIO 키, JWT 시크릿, OpenAI 키 |
| `postgresql.yaml` | PostgreSQL StatefulSet/Service |
| `redis.yaml` | Redis (캐시 + Redis Streams 큐) |
| `minio.yaml` | MinIO 오브젝트 스토리지 (썸네일 저장) |
| `api-server.yaml` | API 서버 Deployment (기본 2 replicas) |
| `worker.yaml` | 비동기 워커 Deployment (기본 2 replicas) |
| `hpa.yaml` | HorizontalPodAutoscaler (CPU 70%, 2~10 replicas) |

## 부활 조건

- 외부 유저가 생겨 단일 프로세스로 감당이 안 될 때
- 멀티 디바이스 동기화 등으로 상시 원격 서버가 필요할 때

## 부활 시 바뀌는 것

v2는 Store / Queue / Tagger를 전부 인터페이스 뒤에 두었으므로, 부활은 구현체 교체다.

| v2 (현재) | 부활 시 |
|---|---|
| Store: SQLite | PostgreSQL |
| Queue: SQLite jobs 테이블 | Redis Streams |
| thumbs: 로컬 디스크 (`data/thumbs/`) | MinIO / S3 |
| 단일 바이너리 | api-server / worker 분리 배포 |

## 주의

`secret.yaml`에 들어 있는 자격증명은 전부 로컬 개발용 더미 값이었다.
부활 시 어느 것도 재사용하지 말고 전부 재발급할 것.
