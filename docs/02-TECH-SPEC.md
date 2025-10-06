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
- **언어**: Go (Golang)
- **프레임워크**: Gin or Fiber
- **아키텍처**: RESTful API
- **인증**: JWT (JSON Web Token)

### Worker
- **언어**: Go (Golang)
- **역할**: 비동기 작업 처리
  - URL 메타데이터 크롤링
  - AI 태그 생성
  - 썸네일 다운로드 및 처리

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
- **데이터 타입**: String, Hash, Set, Sorted Set

## 메시지 큐

### RabbitMQ
- **용도**: 비동기 작업 큐
- **Exchange Type**: Topic
- **주요 큐**:
  - `scrape.queue` - URL 크롤링
  - `tag.queue` - AI 태그 생성
  - `thumbnail.queue` - 썸네일 처리
  - `dlq.queue` - 실패 작업 처리

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