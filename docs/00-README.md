# 링크 아카이브 프로젝트 - 기획서

## 📋 문서 구조

이 기획서는 총 8개의 문서로 구성되어 있습니다:

1. **[00-README.md](00-README.md)** ← 현재 문서
   - 프로젝트 소개
   - 문서 구조
   - 빠른 시작 가이드

2. **[01-프로젝트-개요.md](01-프로젝트-개요.md)**
   - 프로젝트 목표 및 비전
   - 핵심 기능
   - 사용자 시나리오
   - 경쟁 우위

3. **[02-기술-스택.md](02-기술-스택.md)**
   - Frontend (React Native)
   - Backend (Go)
   - 데이터베이스 (PostgreSQL, Redis)
   - 인프라 (K8s, MinIO, RabbitMQ)
   - 외부 서비스 (OpenAI)

4. **[03-시스템-아키텍처.md](03-시스템-아키텍처.md)**
   - 전체 시스템 구성도
   - 컴포넌트별 역할
   - 네트워크 구성
   - 보안 및 확장성

5. **[04-데이터-플로우.md](04-데이터-플로우.md)**
   - 링크 저장 플로우
   - 검색 및 필터링
   - 동기화 메커니즘
   - 실패 처리

6. **[05-데이터베이스-스키마.md](05-데이터베이스-스키마.md)**
   - PostgreSQL 테이블 정의
   - ERD (관계도)
   - Redis 데이터 구조
   - MinIO Bucket 구조
   - 주요 쿼리 예시

7. **[06-API-명세서.md](06-API-명세서.md)**
   - RESTful API 엔드포인트
   - 요청/응답 형식
   - 인증 방식
   - 에러 코드
   - Rate Limiting

8. **[07-K8s-배포-설정.md](07-K8s-배포-설정.md)**
   - Kubernetes YAML 설정
   - ConfigMap & Secret
   - Deployment & Service
   - Ingress 설정
   - 배포 순서 및 명령어

9. **[08-개발-계획.md](08-개발-계획.md)**
   - 전체 일정 (8-10주)
   - Phase별 상세 계획
   - 우선순위
   - 위험 관리
   - 성공 기준

---

## 🎯 프로젝트 개요

**링크 아카이브**는 유튜브 영상이나 웹 아티클을 공유하면 자동으로 AI가 태그를 생성하고, 태그별/날짜별로 쉽게 찾아볼 수 있는 개인 메모 앱입니다.

### 핵심 가치
- ⚡ **빠른 저장**: 공유 버튼 한 번으로 즉시 저장
- 🤖 **자동 태그**: AI가 콘텐츠를 분석해서 자동 분류
- 🔍 **쉬운 재발견**: 태그와 검색으로 필요한 링크를 빠르게 찾기
- 📱 **크로스 플랫폼**: iOS와 Android 모두 지원
- ☁️ **클라우드 동기화**: 여러 기기에서 동일한 데이터 접근

---

## 🏗️ 기술 스택 요약

### Frontend
- **React Native** (iOS, Android)
- SQLite (로컬 저장)

### Backend
- **Go** (API Server, Worker)
- **Gin** 프레임워크

### 데이터베이스
- **PostgreSQL** (메인 DB)
- **Redis** (캐시, 세션)

### 메시지 큐
- **RabbitMQ** (비동기 작업)

### 스토리지
- **MinIO** (S3 호환, 이미지 저장)

### 인프라
- **Kubernetes** (오케스트레이션)
- **Docker** (컨테이너화)

### 외부 서비스
- **OpenAI API** (자동 태그 생성)

---

## 🚀 빠른 시작

### 사전 요구사항

```bash
# 필수 설치
- Docker Desktop
- kubectl
- Kind or Minikube
- Go 1.21+
- Node.js 18+
- React Native CLI
```

### 1. 백엔드 로컬 실행

```bash
# 1. 저장소 클론
git clone https://github.com/your-org/link-archive.git
cd link-archive

# 2. K8s 클러스터 생성 (Kind 사용 예시)
kind create cluster --name link-archive

# 3. Namespace 생성
kubectl apply -f k8s/namespace.yaml

# 4. ConfigMap & Secret 생성
kubectl apply -f k8s/configmap.yaml
kubectl apply -f k8s/secret.yaml

# 5. 데이터베이스 배포
kubectl apply -f k8s/postgresql.yaml
kubectl apply -f k8s/redis.yaml
kubectl apply -f k8s/rabbitmq.yaml
kubectl apply -f k8s/minio.yaml

# 6. 준비 대기
kubectl wait --for=condition=ready pod -l app=postgresql -n link-archive --timeout=300s

# 7. 애플리케이션 배포
kubectl apply -f k8s/api-server.yaml
kubectl apply -f k8s/worker.yaml

# 8. Port Forward로 접근
kubectl port-forward svc/api-server 8080:80 -n link-archive
```

API 접근: `http://localhost:8080`

### 2. 모바일 앱 실행

```bash
# 1. 앱 디렉토리로 이동
cd mobile

# 2. 의존성 설치
npm install

# 3. iOS (macOS only)
npx pod-install
npm run ios

# 4. Android
npm run android
```

### 3. MinIO Bucket 생성

```bash
# MinIO Console 접근
kubectl port-forward svc/minio-nodeport 9001:9001 -n link-archive

# 브라우저에서 http://localhost:9001 접속
# ID: minioadmin, PW: minioadmin123
# Bucket 생성: thumbnails, content-cache
```

---

## 📊 시스템 아키텍처 개요

```
[Mobile App] 
    ↓
[Ingress Controller]
    ↓
[API Server] ←→ [Redis Cache]
    ↓              ↓
[PostgreSQL] ←→ [RabbitMQ] ←→ [Worker(s)]
    ↓                           ↓
[MinIO Object Storage]    [OpenAI API]
```

**데이터 흐름**:
1. 사용자가 앱에서 링크 공유
2. API Server가 기본 정보만 DB에 저장하고 즉시 응답
3. 백그라운드에서 Worker가 처리:
   - URL 크롤링 (메타데이터 추출)
   - OpenAI로 자동 태그 생성
   - 썸네일 다운로드 및 MinIO 저장
4. 완료 후 앱에 알림 (WebSocket or Polling)

---

## 📝 개발 단계

### Phase 1: MVP (3주)
- ✅ 링크 저장 (공유 기능)
- ✅ URL 크롤링
- ✅ 썸네일 저장
- ✅ 기본 목록 조회

### Phase 2: AI 태깅 (2주)
- 🔄 OpenAI API 연동
- 🔄 자동 태그 생성
- 🔄 태그 관리 UI

### Phase 3: 고급 기능 (2주)
- ⬜ 검색 기능
- ⬜ 필터링
- ⬜ 동기화

### Phase 4: 배포 (1-2주)
- ⬜ K8s 최적화
- ⬜ CI/CD 구축
- ⬜ 성능 튜닝

---

## 🗂️ 프로젝트 구조

```
link-archive/
├── docs/                      # 기획서 (이 문서들)
│   ├── 00-README.md
│   ├── 01-프로젝트-개요.md
│   ├── 02-기술-스택.md
│   ├── 03-시스템-아키텍처.md
│   ├── 04-데이터-플로우.md
│   ├── 05-데이터베이스-스키마.md
│   ├── 06-API-명세서.md
│   ├── 07-K8s-배포-설정.md
│   └── 08-개발-계획.md
│
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
│   │   ├── queue/            # RabbitMQ 클라이언트
│   │   ├── storage/          # MinIO 클라이언트
│   │   └── openai/           # OpenAI 클라이언트
│   ├── migrations/           # DB 마이그레이션
│   ├── docker/
│   │   ├── Dockerfile.api
│   │   └── Dockerfile.worker
│   └── go.mod
│
├── mobile/                    # React Native 앱
│   ├── src/
│   │   ├── screens/
│   │   ├── components/
│   │   ├── services/
│   │   │   ├── api.ts        # API 클라이언트
│   │   │   └── db.ts         # SQLite
│   │   ├── store/            # Zustand
│   │   ├── types/
│   │   └── utils/
│   ├── ios/
│   ├── android/
│   └── package.json
│
├── k8s/                       # Kubernetes 설정
│   ├── namespace.yaml
│   ├── configmap.yaml
│   ├── secret.yaml
│   ├── postgresql.yaml
│   ├── redis.yaml
│   ├── rabbitmq.yaml
│   ├── minio.yaml
│   ├── api-server.yaml
│   ├── worker.yaml
│   └── ingress.yaml
│
└── README.md                  # 프로젝트 루트 README
```

---

## 🔧 주요 명령어

### Kubernetes 관리

```bash
# 전체 상태 확인
kubectl get all -n link-archive

# Pod 로그 확인
kubectl logs -f deployment/api-server -n link-archive
kubectl logs -f deployment/worker -n link-archive

# Pod 접속
kubectl exec -it <pod-name> -n link-archive -- /bin/sh

# Port Forward
kubectl port-forward svc/api-server 8080:80 -n link-archive
kubectl port-forward svc/postgresql 5432:5432 -n link-archive
kubectl port-forward svc/rabbitmq 15672:15672 -n link-archive

# 리소스 모니터링
kubectl top pods -n link-archive
kubectl top nodes

# 전체 삭제
kubectl delete namespace link-archive
```

### 개발

```bash
# 백엔드 빌드
cd backend
go build -o bin/server cmd/server/main.go
go build -o bin/worker cmd/worker/main.go

# 백엔드 실행 (로컬)
./bin/server
./bin/worker

# 테스트
go test ./...

# Docker 이미지 빌드
docker build -t link-archive/api:latest -f docker/Dockerfile.api .
docker build -t link-archive/worker:latest -f docker/Dockerfile.worker .

# 앱 실행
cd mobile
npm run ios
npm run android
```

---

## 📚 추가 리소스

### API 문서
- Swagger UI: `http://localhost:8080/swagger` (예정)
- API 명세: [06-API-명세서.md](06-API-명세서.md)

### 관리 콘솔
- RabbitMQ Management: `http://localhost:15672`
  - ID: admin, PW: rabbitpass123
- MinIO Console: `http://localhost:9001`
  - ID: minioadmin, PW: minioadmin123

### 데이터베이스 접속
```bash
kubectl port-forward svc/postgresql 5432:5432 -n link-archive

# psql 접속
psql -h localhost -p 5432 -U linkuser -d linkarchive
```

---

## 🎨 UI/UX 컨셉

### 주요 화면

1. **홈 (링크 목록)**
   - 카드 형태의 링크 리스트
   - 썸네일 + 제목 + 태그
   - 무한 스크롤
   - Pull to refresh

2. **태그 화면**
   - 태그 클라우드
   - 태그별 사용 빈도
   - 태그 클릭 → 필터링된 링크

3. **검색**
   - 통합 검색바
   - 최근 검색어
   - 필터 옵션 (태그, 날짜)

4. **상세 화면**
   - 링크 메타데이터
   - 태그 편집
   - 메모 추가
   - 원본 링크 바로가기

5. **캘린더 (선택)**
   - 날짜별 저장 링크 수
   - 날짜 클릭 → 해당 날짜 링크

---

## 🔐 보안 고려사항

### 인증/인가
- JWT 기반 인증
- Access Token (1시간) + Refresh Token (7일)
- Redis 세션 관리

### 데이터 보호
- HTTPS/TLS 통신
- 데이터베이스 암호화 (선택)
- Secret 관리 (Kubernetes Secrets)

### Rate Limiting
- 사용자당 100 req/min
- 링크 저장: 30 req/min
- Redis 기반 구현

### Input Validation
- URL 형식 검증
- XSS 방지
- SQL Injection 방지 (Prepared Statements)

---

## 📈 성능 목표

| 지표 | 목표 |
|------|------|
| 링크 저장 응답 시간 | < 500ms |
| 목록 조회 (캐시 히트) | < 50ms |
| 목록 조회 (캐시 미스) | < 200ms |
| 검색 | < 300ms |
| 전체 처리 (크롤링+태깅) | < 15초 |
| 앱 시작 시간 | < 2초 |
| 가동률 | 99%+ |

---

## 🐛 버그 리포트 및 기여

### 이슈 생성
GitHub Issues를 통해 버그 리포트나 기능 제안을 해주세요.

### 기여 방법
1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Commit your changes (`git commit -m 'Add amazing feature'`)
4. Push to the branch (`git push origin feature/amazing-feature`)
5. Open a Pull Request

---

## 📄 라이선스

MIT License (예정)

---

## 👥 팀

- **Backend**: Go 개발자
- **Frontend**: React Native 개발자
- **DevOps**: K8s 관리자
- **AI**: OpenAI 프롬프트 엔지니어

---

## 📞 연락처

- 프로젝트 관리자: your-email@example.com
- GitHub: https://github.com/your-org/link-archive

---

## 🗺️ 로드맵

### v1.0 (MVP) - 2025년 12월
- [x] 링크 저장 및 크롤링
- [x] 자동 태그 생성
- [x] 기본 UI
- [ ] 검색 및 필터

### v1.1 - 2026년 1월
- [ ] 동기화
- [ ] 오프라인 모드
- [ ] 성능 최적화

### v2.0 - 2026년 2월
- [ ] 캘린더 뷰
- [ ] 통계 대시보드
- [ ] 공유 기능
- [ ] Chrome Extension

### Future
- [ ] 추천 시스템
- [ ] 소셜 기능
- [ ] AI 요약

---

## ⚠️ 알려진 이슈

- 일부 동적 웹사이트 크롤링 실패 → Headless browser 도입 예정
- OpenAI API 비용 → 월별 한도 설정 필요
- 대용량 이미지 처리 → 리사이징 최적화 필요

---

## 🙏 감사의 글

이 프로젝트는 다음 오픈소스 프로젝트들을 사용합니다:
- Go (Golang)
- React Native
- PostgreSQL
- Redis
- RabbitMQ
- MinIO
- Kubernetes
- OpenAI

---

**마지막 업데이트**: 2025-10-05