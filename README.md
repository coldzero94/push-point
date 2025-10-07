# Push-Point

링크 아카이브 - 자동 AI 태그 기반 개인 메모 앱

## 🚀 로컬 개발 환경 시작하기

### 사전 요구사항

```bash
# 필수 설치
- Docker Desktop
- Minikube
- kubectl
- Go 1.21+ (goenv 사용 권장)
- make
```

## 🎯 빠른 시작 (Makefile 사용)

### 한 번에 전체 배포

```bash
# Minikube 시작 + 전체 인프라 배포
make deploy

# 또는 단계별로
make minikube-start  # Minikube 클러스터 시작
make k8s-up          # K8s 리소스 배포
```

### 주요 Makefile 명령어

```bash
# === 기본 명령어 ===
make help            # 사용 가능한 모든 명령어 보기
make k8s-status      # 전체 리소스 상태 확인
make pods            # Pod 목록 및 상태

# === 클러스터 관리 ===
make minikube-start  # Minikube 시작
make minikube-stop   # Minikube 중지
make minikube-status # Minikube 상태 확인

# === 배포 관리 ===
make k8s-up          # 전체 배포
make k8s-down        # 전체 삭제
make k8s-restart     # 재시작 (삭제 후 재배포)

# === 모니터링 ===
make logs-api        # API Server 로그
make logs-worker     # Worker 로그
make logs-redis      # Redis 로그
make metrics         # 리소스 사용량

# === 스케일링 ===
make scale-api REPLICAS=5      # API Server 5개로 스케일
make scale-worker REPLICAS=10  # Worker 10개로 스케일
make hpa-status                # HPA 상태 확인

# === Port Forward ===
make port-forward-api    # API Server: http://localhost:8080
make port-forward-minio  # MinIO Console: http://localhost:9001
```

---

## 📖 상세 가이드 (수동 설정)

### 1. Go 설치 (goenv 사용)

```bash
# goenv로 Go 설치
goenv install 1.25.1
goenv local 1.25.1

# 설치 확인
go version  # go version go1.25.1 darwin/arm64
```

### 2. Minikube 클러스터 시작

```bash
# Minikube 시작 (CPU 4개, 메모리 6GB)
minikube start --cpus=4 --memory=6144 --driver=docker

# 클러스터 확인
kubectl cluster-info
```

### 3. Metrics Server 활성화 (HPA를 위해 필요)

```bash
# Minikube에서 Metrics Server 활성화
minikube addons enable metrics-server

# 활성화 확인
kubectl top nodes
```

### 4. K8s 인프라 배포

```bash
# Namespace 및 Config 생성
kubectl apply -f k8s/namespace.yaml
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/secret.yaml

# 인프라 서비스 배포
kubectl apply -f k8s/postgresql.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/minio.yaml

# 배포 상태 확인
kubectl get pods -n push-point

# 모든 Pod가 Running 상태가 될 때까지 대기 (1-2분 소요)
kubectl wait --for=condition=ready pod --all -n push-point --timeout=300s
```

### 5. 외부 서비스 접근 설정

```bash
# MinIO Console 접근
minikube service minio-nodeport -n push-point
# 또는
kubectl port-forward -n push-point svc/minio 9001:9001
# 접속: http://localhost:9001 (ID: minioadmin, PW: minioadmin123)

# MinIO에서 'thumbnails' 버킷 생성
# Console에서 Buckets > Create Bucket > "thumbnails"
```

### 6. Go 백엔드 실행 (개발 모드)

```bash
cd backend

# 의존성 설치
go mod tidy

# API Server 실행
go run cmd/server/main.go

# (별도 터미널에서) Worker 실행
go run cmd/worker/main.go
```

### 7. 데이터베이스 접근 (선택사항)

```bash
# PostgreSQL Port Forward
kubectl port-forward -n push-point svc/postgresql 5432:5432

# psql로 접속
psql -h localhost -p 5432 -U linkuser -d linkarchive
# Password: linkpass123
```

## 📊 서비스 상태 확인

```bash
# 전체 Pod 상태
kubectl get pods -n push-point

# 특정 서비스 로그 확인
kubectl logs -f -n push-point -l app=postgresql
kubectl logs -f -n push-point -l app=redis

# 전체 리소스 확인
kubectl get all -n push-point

# HPA 상태 확인
kubectl get hpa -n push-point

# 리소스 사용량 확인
kubectl top pods -n push-point
kubectl top nodes
```

## ⚖️ 자동 스케일링 (HPA)

Push-Point는 Horizontal Pod Autoscaler를 사용하여 자동으로 스케일링됩니다.

### API Server HPA
- **최소 Pod**: 2개
- **최대 Pod**: 10개
- **스케일 조건**:
  - CPU 사용률 70% 이상
  - 메모리 사용률 80% 이상

### Worker HPA
- **최소 Pod**: 2개
- **최대 Pod**: 20개
- **스케일 조건**:
  - CPU 사용률 75% 이상
  - 메모리 사용률 85% 이상

### HPA 관리 명령어

```bash
# HPA 상태 확인
kubectl get hpa -n push-point
kubectl describe hpa api-server-hpa -n push-point
kubectl describe hpa worker-hpa -n push-point

# 수동 스케일 조정 (테스트용)
kubectl scale deployment api-server -n push-point --replicas=5
kubectl scale deployment worker -n push-point --replicas=10

# HPA 비활성화 (테스트 시)
kubectl delete hpa api-server-hpa -n push-point
kubectl delete hpa worker-hpa -n push-point

# HPA 재활성화
kubectl apply -f k8s/hpa.yaml
```

## 🛠️ 개발 명령어

### Makefile로 쉽게 관리

```bash
# === 클러스터 관리 ===
make minikube-start      # Minikube 시작
make minikube-stop       # Minikube 중지
make minikube-delete     # Minikube 삭제 (전체 초기화)
make minikube-status     # Minikube 상태 확인

# === K8s 리소스 관리 ===
make k8s-up              # 전체 배포
make k8s-down            # 전체 리소스 삭제
make k8s-restart         # 재시작 (삭제 후 재배포)
make k8s-status          # 전체 상태 확인
make pods                # Pod 목록

# === 애플리케이션 개발 ===
make build-api           # API Server 이미지 빌드
make build-worker        # Worker 이미지 빌드
make docker-build        # 전체 이미지 빌드

# === 로그 확인 ===
make logs-api            # API Server 로그 실시간 확인
make logs-worker         # Worker 로그 실시간 확인
make logs-redis          # Redis 로그 확인
make logs-postgres       # PostgreSQL 로그 확인

# === 스케일링 ===
make scale-api REPLICAS=5       # API Server 5개로 스케일
make scale-worker REPLICAS=10   # Worker 10개로 스케일
make hpa-status                 # HPA 상태 확인

# === 모니터링 ===
make metrics             # 리소스 사용량 확인

# === Port Forward ===
make port-forward-api    # API Server (http://localhost:8080)
make port-forward-minio  # MinIO Console (http://localhost:9001)
```

### Go 빌드 및 테스트 (로컬)

```bash
# 의존성 설치
cd backend
go mod tidy

# 로컬 빌드
go build -o bin/server cmd/server/main.go
go build -o bin/worker cmd/worker/main.go

# 로컬 실행
./bin/server
./bin/worker

# 테스트 실행
go test ./...
go test -v ./internal/...

# 테스트 커버리지
go test -cover ./...
```

## 📁 프로젝트 구조

```
push-point/
├── backend/                   # Go 백엔드
│   ├── cmd/
│   │   ├── server/           # API Server
│   │   └── worker/           # Worker
│   ├── internal/
│   │   ├── config/
│   │   ├── handler/          # API 핸들러
│   │   ├── service/          # 비즈니스 로직
│   │   ├── repository/       # DB 접근
│   │   ├── model/            # 데이터 모델
│   │   └── middleware/       # 미들웨어
│   ├── pkg/
│   │   ├── scraper/          # 웹 크롤러
│   │   ├── queue/            # Redis Streams 클라이언트
│   │   ├── storage/          # MinIO 클라이언트
│   │   └── openai/           # OpenAI 클라이언트
│   ├── migrations/           # DB 마이그레이션
│   └── go.mod
├── k8s/                      # Kubernetes 설정
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── secret.yaml
│   ├── postgresql.yaml
│   ├── redis.yaml           # Cache + Message Queue (Redis Streams)
│   ├── minio.yaml
│   ├── api-server.yaml      # API Server Deployment + Service
│   ├── worker.yaml          # Worker Deployment
│   └── hpa.yaml             # Horizontal Pod Autoscaler
├── Makefile                 # K8s 관리 명령어
├── docs/                     # 프로젝트 문서
└── README.md
```

## 🔧 트러블슈팅

### Minikube가 시작되지 않는 경우

```bash
# Makefile 사용
make minikube-delete
make minikube-start

# 또는 직접 명령
minikube delete
minikube start --cpus=4 --memory=6144 --driver=docker
```

### Pod가 Pending 상태인 경우

```bash
# Pod 상태 확인
make pods

# 특정 Pod 상세 정보
kubectl describe pod <pod-name> -n push-point

# 리소스 부족 시 Minikube 재시작
make minikube-delete
make minikube-start
make k8s-up
```

### Redis가 연결되지 않는 경우

```bash
# Redis 로그 확인
make logs-redis

# Redis 재시작
kubectl rollout restart deployment/redis -n push-point

# 전체 재시작
make k8s-restart
```

### HPA가 작동하지 않는 경우

```bash
# Metrics Server 활성화 확인
minikube addons list | grep metrics-server

# Metrics Server 활성화
minikube addons enable metrics-server

# HPA 상태 확인
make hpa-status

# 리소스 사용량 확인
make metrics
```

### API Server 또는 Worker 이미지가 없는 경우

```bash
# Docker 이미지 빌드 (Minikube Docker 환경에서)
make docker-build

# 개별 빌드
make build-api
make build-worker

# Deployment 재시작
kubectl rollout restart deployment/api-server -n push-point
kubectl rollout restart deployment/worker -n push-point
```

### 전체 환경 초기화

```bash
# 완전히 새로 시작
make k8s-down
make minikube-delete
make deploy
```

## 📚 추가 문서

### 프로젝트 문서
- [프로젝트 개요](docs/00-README.md)
- [기술 스택](docs/02-TECH-SPEC.md) - **Redis Streams 메시지 큐 사용**
- [시스템 아키텍처](docs/03-SYSTEM-ARCHITECTURE.md) - **HPA 자동 스케일링 포함**
- [데이터 플로우](docs/04-DATA-FLOW.md)
- [데이터베이스 스키마](docs/05-DATA-SCHEMA.md)
- [API 명세서](docs/06-API-SPECIFICATION.md)
- [K8s 배포 설정](docs/07-K8S-SETTINGS.md)
- [개발 계획](docs/08-DEVLOPMENT-PLAN.md)

### 설정 파일
- [Makefile](Makefile) - K8s 클러스터 관리 명령어
- [K8s 매니페스트](k8s/) - Kubernetes 배포 설정
  - [namespace.yaml](k8s/namespace.yaml) - Namespace
  - [configmap.yaml](k8s/configmap.yaml) - 환경 설정
  - [secret.yaml](k8s/secret.yaml) - 비밀 정보
  - [postgresql.yaml](k8s/postgresql.yaml) - PostgreSQL
  - [redis.yaml](k8s/redis.yaml) - Redis (캐시 + 메시지 큐)
  - [minio.yaml](k8s/minio.yaml) - MinIO (객체 스토리지)
  - [api-server.yaml](k8s/api-server.yaml) - API Server + Service
  - [worker.yaml](k8s/worker.yaml) - Worker
  - [hpa.yaml](k8s/hpa.yaml) - Horizontal Pod Autoscaler

### Makefile 주요 타겟

| 명령어 | 설명 |
|--------|------|
| `make help` | 모든 명령어 목록 보기 |
| `make deploy` | 전체 배포 (Minikube + K8s) |
| `make k8s-up` | K8s 리소스 배포 |
| `make k8s-down` | K8s 리소스 삭제 |
| `make k8s-restart` | 재시작 (삭제 후 재배포) |
| `make k8s-status` | 전체 상태 확인 |
| `make pods` | Pod 목록 |
| `make logs-api` | API Server 로그 |
| `make logs-worker` | Worker 로그 |
| `make scale-api REPLICAS=N` | API Server 스케일 조정 |
| `make scale-worker REPLICAS=N` | Worker 스케일 조정 |
| `make hpa-status` | HPA 상태 확인 |
| `make metrics` | 리소스 사용량 확인 |
| `make port-forward-api` | API Server 포트 포워드 |
| `make port-forward-minio` | MinIO Console 포트 포워드 |
| `make docker-build` | Docker 이미지 빌드 |
| `make minikube-start` | Minikube 시작 |
| `make minikube-stop` | Minikube 중지 |

