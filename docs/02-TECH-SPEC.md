# 기술 스택

## Frontend

### Mobile App
- **Framework**: React Native
- **플랫폼**: iOS, Android
- **상태 관리**: Zustand or React Query
- **로컬 DB**: SQLite (오프라인 지원)
- **UI 라이브러리**: React Native Paper or NativeBase

### 주요 기능
- Share Extension (iOS/Android)
- 오프라인 모드
- 백그라운드 동기화
- Push Notification (선택)

## Backend

### API Server
- **언어**: Go 1.21+
- **웹 프레임워크**: [Gin](https://github.com/gin-gonic/gin) - HTTP 서버 및 라우팅
- **아키텍처**: RESTful API
- **인증**: JWT (JSON Web Token)

### Worker
- **언어**: Go 1.21+
- **역할**: 비동기 작업 처리
  - URL 메타데이터 크롤링
  - AI 태그 생성
  - 썸네일 다운로드 및 처리
- **스케줄러**: [gocron](https://github.com/go-co-op/gocron) - 크론 작업 관리

### Go 핵심 패키지

#### 데이터베이스 & ORM
- **ORM**: [Ent](https://entgo.io/) - Facebook의 엔티티 프레임워크
  - 타입 안전한 코드 생성
  - 강력한 쿼리 빌더
  - 마이그레이션 자동 생성
- **PostgreSQL 드라이버**: [pgx](https://github.com/jackc/pgx) - 고성능 PostgreSQL 드라이버
- **마이그레이션**: [golang-migrate](https://github.com/golang-migrate/migrate) - DB 스키마 버전 관리

#### 캐시 & 메시징
- **Redis 클라이언트**: [go-redis](https://github.com/redis/go-redis)
  - 세션 관리
  - API 응답 캐싱
  - Rate Limiting
  - 분산 락
  - **비동기 작업 큐 (Redis Streams)**
    - URL 크롤링 작업
    - AI 태그 생성 작업
    - 썸네일 처리 작업
    - Consumer Group 지원
    - 메시지 영속성 (AOF)

#### 스토리지 & AWS
- **AWS SDK**: [aws-sdk-go-v2](https://github.com/aws/aws-sdk-go-v2)
  - S3 서비스 (MinIO 호환)
  - STS 서비스 (임시 자격 증명)
- **S3 클라이언트**: MinIO Go SDK 또는 AWS SDK S3

#### 인증 & 보안
- **JWT**: [golang-jwt/jwt](https://github.com/golang-jwt/jwt)
  - Access Token / Refresh Token 생성
  - 토큰 검증 및 파싱

#### 외부 API
- **OpenAI 클라이언트**: [go-openai](https://github.com/sashabaranov/go-openai)
- **웹 스크래핑**: [colly](https://github.com/gocolly/colly) 또는 [chromedp](https://github.com/chromedp/chromedp)

#### 테스팅
- **통합 테스트**: [testcontainers-go](https://github.com/testcontainers/testcontainers-go)
  - Docker 컨테이너 기반 테스트 환경
  - PostgreSQL, Redis, RabbitMQ 테스트
- **단위 테스트**: Go 표준 `testing` 패키지
- **Mock**: [gomock](https://github.com/golang/mock) 또는 [testify](https://github.com/stretchr/testify)

#### Kubernetes
- **K8s 클라이언트**: [client-go](https://github.com/kubernetes/client-go)
  - Pod 상태 확인
  - ConfigMap/Secret 읽기
  - Health Check

#### 유틸리티
- **환경 변수**: [viper](https://github.com/spf13/viper) - 설정 관리
- **로깅**: [zap](https://github.com/uber-go/zap) 또는 [logrus](https://github.com/sirupsen/logrus)
- **Validation**: [validator](https://github.com/go-playground/validator)
- **HTTP 클라이언트**: [resty](https://github.com/go-resty/resty)

## 데이터베이스

### PostgreSQL (메인 DB)
- **버전**: 16
- **용도**: 
  - 사용자 데이터
  - 링크 메타데이터
  - 태그 및 관계
  - 메모

### Redis
- **용도**:
  - Session Store
  - API Response Cache
  - Rate Limiting
  - Distributed Lock
  - **Message Queue (Redis Streams)**
- **데이터 타입**: String, Hash, Set, Sorted Set, Streams

### Redis Streams (메시지 큐)
- **용도**: 비동기 작업 큐
- **주요 Streams**:
  - `tasks:scraping` - URL 크롤링 작업
  - `tasks:tagging` - AI 태그 생성 작업
  - `tasks:thumbnail` - 썸네일 처리 작업
- **Consumer Groups**:
  - 여러 Worker가 병렬로 작업 처리
  - 자동 재시도 로직
  - ACK 메커니즘
- **영속성**: AOF (Append Only File) 사용

## 스토리지

### MinIO (S3-Compatible)
- **용도**: 객체 스토리지
- **저장 데이터**:
  - 썸네일 이미지 (원본, 리사이징)
  - 캐시된 웹 콘텐츠 (선택)
- **Bucket 구조**:
  - `thumbnails/original/`
  - `thumbnails/resized/small/`
  - `thumbnails/resized/medium/`
  - `thumbnails/resized/large/`

## 외부 서비스

### OpenAI API
- **모델**: GPT-4o-mini (비용 효율적)
- **용도**: 자동 태그 생성
- **응답 형식**: JSON

### Web Scraping
- **라이브러리**: Go Colly or Chromedp
- **타겟**: 
  - 웹 페이지 메타데이터
  - Open Graph 태그
  - YouTube API (선택)

## 인프라

### 컨테이너화
- **Docker**: 모든 서비스 컨테이너화
- **Docker Compose**: 로컬 개발 환경

### 오케스트레이션
- **Kubernetes (K8s)**: 배포 및 관리
- **로컬 개발**: Kind or Minikube
- **프로덕션**: AWS EKS, GKE, or Self-hosted

### CI/CD (향후)
- GitHub Actions
- Docker Registry
- Helm Charts

## 모니터링 및 로깅 (향후)

### 모니터링
- **Prometheus**: 메트릭 수집
- **Grafana**: 대시보드

### 로깅
- **구조화 로깅**: JSON 형식
- **로그 레벨**: DEBUG, INFO, WARN, ERROR

### 알림
- Slack 또는 Discord Webhook

## 개발 도구

### 버전 관리
- Git
- GitHub

### API 문서
- Swagger/OpenAPI

### 테스팅
- Go: `testing` 패키지
- React Native: Jest

### 코드 품질
- Go: golangci-lint
- JavaScript: ESLint, Prettier

## 보안

- HTTPS/TLS
- JWT 토큰 기반 인증
- Rate Limiting (Redis)
- Input Validation
- SQL Injection 방지 (Prepared Statements)
- CORS 설정

## 성능 최적화

- Redis 캐싱
- Database Indexing
- CDN for MinIO (선택)
- Horizontal Pod Autoscaling (HPA)
- Connection Pooling