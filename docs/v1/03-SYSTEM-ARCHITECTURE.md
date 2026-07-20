# 시스템 아키텍처

## 전체 시스템 구성도

```
┌─────────────────────────────────────────────────────────────────┐
│                     Mobile App (React Native)                    │
│                      - iOS / Android                             │
└────────────────────────────┬────────────────────────────────────┘
                             │ HTTPS
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                    Ingress Controller (nginx)                    │
│                      - TLS Termination                           │
│                      - Rate Limiting                             │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                      API Gateway (Optional)                      │
│                    - Authentication                              │
│                    - Request Validation                          │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ↓
         ┌───────────────────┴───────────────────┐
         │                                       │
         ↓                                       ↓
┌─────────────────────┐              ┌──────────────────────┐
│   API Server (Go)   │              │   Worker(s) (Go)     │
│   - REST API        │              │   - Job Processing   │
│   - WebSocket       │              │   - AI Tagging       │
│   - Auth            │              │   - Web Scraping     │
└──────┬──────────────┘              └──────┬───────────────┘
       │                                    │
       │ ┌──────────────────────────────────┤
       │ │                                  │
       ↓ ↓                                  ↓
┌─────────────────────────────────────────────────────────┐
│  Redis (Cache + Message Queue)                          │
│  - Session Store                                         │
│  - API Cache                                             │
│  - Rate Limit                                            │
│  - Redis Streams (비동기 작업 큐)                         │
│    * tasks:scraping, tasks:tagging, tasks:thumbnail     │
└────────────────────────┬────────────────────────────────┘
                         ↓
┌─────────────────────────────────────────────────────────────────┐
│                    PostgreSQL (Primary DB)                       │
│                    - User Data                                   │
│                    - Links Metadata                              │
│                    - Tags & Relations                            │
│                    - Notes                                       │
└─────────────────────────────────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│              MinIO (S3-Compatible Object Storage)                │
│              - Thumbnails                                        │
│              - Original Images                                   │
│              - Cached Web Content (Optional)                     │
└─────────────────────────────────────────────────────────────────┘
                             ↓
┌─────────────────────────────────────────────────────────────────┐
│                      External Services                           │
│                      - OpenAI API (Tagging)                      │
│                      - Web Scraping Target Sites                 │
└─────────────────────────────────────────────────────────────────┘
```

## 컴포넌트별 상세 역할

### 1. Mobile App (React Native)
**책임**:
- 사용자 인터페이스
- 로컬 데이터 캐싱 (SQLite)
- 공유 기능 처리
- 오프라인 모드 지원

**통신**:
- API Server와 HTTPS 통신
- 백그라운드 동기화

### 2. Ingress Controller
**책임**:
- HTTPS/TLS 종료
- 로드 밸런싱
- Rate Limiting
- DDoS 방어

**구현**: nginx-ingress

### 3. API Server (Go + Gin)
**책임**:
- RESTful API 제공 (Gin 프레임워크)
- 인증/인가 (JWT 기반)
- 비즈니스 로직 처리
- 경량 작업 동기 처리
- 무거운 작업 큐 등록

**기술 스택**:
- **웹 프레임워크**: Gin
- **ORM**: Ent (Facebook의 엔티티 프레임워크)
- **인증**: golang-jwt/jwt
- **캐시 & 메시징**: go-redis (Redis Streams)
- **스토리지**: aws-sdk-go-v2 (S3 호환)

**연결**:
- PostgreSQL: Ent ORM을 통한 타입 안전한 데이터 CRUD
- Redis: go-redis를 통한 세션, 캐시, Rate Limiting, 메시지 큐
- MinIO: AWS SDK S3로 Pre-signed URL 생성

**레이어 구조**:
```
HTTP Request
    ↓
Handler (Gin) - 요청 검증, 응답 직렬화
    ↓
Service - 비즈니스 로직
    ↓
Repository (Ent) - 데이터 접근
    ↓
PostgreSQL
```

**포트**: 8080

**스케일링**: Horizontal (여러 Pod)

### 4. Worker (Go + gocron)
**책임**:
- 비동기 작업 전담 처리
- URL 메타데이터 크롤링 (colly)
- OpenAI API 호출 (태그 생성)
- 썸네일 다운로드 및 리사이징
- MinIO 업로드
- 주기적 작업 스케줄링 (gocron)

**기술 스택**:
- **ORM**: Ent (API Server와 동일)
- **스케줄러**: gocron
- **웹 스크래핑**: chromedp 또는 HTTP 클라이언트
- **OpenAI**: go-openai
- **이미지 처리**: Go 표준 image 패키지
- **메시징**: go-redis (Redis Streams)
- **스토리지**: aws-sdk-go-v2 (S3)

**연결**:
- Redis Streams: go-redis를 통한 Job 소비 (Consumer Group)
- PostgreSQL: Ent ORM을 통한 처리 결과 저장
- MinIO: AWS SDK S3로 이미지 업로드
- OpenAI API: go-openai로 태그 생성 요청
- Redis: 분산 락 (중복 처리 방지)

**작업 흐름**:
```
Redis Streams (tasks:*)
    ↓
Worker Consumer (XReadGroup)
    ↓
Scraper → OpenAI (태그) → Image Processing
    ↓
Ent ORM → PostgreSQL
    ↓
AWS S3 SDK → MinIO
```

**스케일링**: Horizontal (여러 Worker 동시 실행)

### 5. PostgreSQL
**책임**:
- 관계형 데이터 저장
- ACID 트랜잭션
- 복잡한 쿼리 (JOIN, 집계)

**저장 데이터**:
- 사용자 정보
- 링크 메타데이터
- 태그 및 관계
- 메모
- 동기화 로그

**고가용성**: 
- Master-Replica 구조 (향후)
- 자동 백업

### 6. Redis (Cache + Message Queue)
**책임**:
- 인메모리 캐싱
- 세션 관리
- Rate Limiting
- 분산 락
- **비동기 작업 큐 (Redis Streams)**

**데이터 구조**:
```
Session: user:{user_id}:session
Cache: cache:link:{link_id}:metadata (TTL: 1h)
Rate Limit: ratelimit:{user_id}:{endpoint}
Lock: lock:scrape:{url_hash}

# Redis Streams (메시지 큐)
tasks:scraping     - URL 크롤링 작업
tasks:tagging      - AI 태그 생성 작업
tasks:thumbnail    - 썸네일 처리 작업
```

**Redis Streams 메시지 형식**:
```
XADD tasks:scraping * job_id uuid-v4 link_id 12345 url https://example.com retry_count 0
```

**Consumer Groups**:
```
Group: workers-scraping
  ├─ Consumer: worker-1
  ├─ Consumer: worker-2
  └─ Consumer: worker-3

Group: workers-tagging
  └─ Consumer: worker-1
```

**영속성**: AOF (Append Only File) 사용

### 7. MinIO
**책임**:
- S3 호환 객체 스토리지
- 이미지 저장 및 서빙
- 버전 관리

**Bucket 구조**:
```
thumbnails/
  ├─ original/           # 원본 썸네일
  └─ resized/
      ├─ small/          # 150x150
      ├─ medium/         # 300x300
      └─ large/          # 600x600

content-cache/           # (Optional) 웹 페이지 캐시
```

**접근 제어**:
- Public Read (resized thumbnails)
- Private (original)

## 네트워크 구성

### 클러스터 내부 통신
```
api-server.link-archive.svc.cluster.local:80
postgresql.link-archive.svc.cluster.local:5432
redis.link-archive.svc.cluster.local:6379
rabbitmq.link-archive.svc.cluster.local:5672
minio.link-archive.svc.cluster.local:9000
```

### 외부 노출
```
Ingress: api.linkarchive.local → api-server:80
NodePort (로컬): 
  - RabbitMQ Management: localhost:15672
  - MinIO Console: localhost:9001
```

## 데이터 영속성

### PersistentVolume 사용
- PostgreSQL: 10Gi
- MinIO: 50Gi
- RabbitMQ: 5Gi (Optional)

### 백업 전략 (향후)
- PostgreSQL: Daily 풀백업, WAL 아카이빙
- MinIO: S3 복제 or 스냅샷

## 보안 계층

### 1. 네트워크 보안
- Ingress TLS 종료
- Network Policy (Pod 간 통신 제한)

### 2. 인증/인가
- JWT 토큰 (Access Token + Refresh Token)
- Redis 세션 검증

### 3. 데이터 암호화
- 전송 중: TLS 1.3
- 저장: 데이터베이스 암호화 (향후)

### 4. Secret 관리
- Kubernetes Secrets
- 환경변수 주입

## 확장성 고려사항

### Horizontal Scaling
- API Server: HPA (CPU > 70%)
- Worker: 큐 크기 기반 Auto Scaling
- PostgreSQL: Read Replica 추가

### Vertical Scaling
- 리소스 요청/제한 조정
- DB 인스턴스 사이즈 업그레이드

### 캐싱 전략
- L1: 애플리케이션 메모리 (짧은 TTL)
- L2: Redis (중간 TTL)
- L3: CDN for MinIO (긴 TTL)

## 장애 복구

### 자동 복구
- Kubernetes Liveness/Readiness Probe
- 자동 Pod 재시작
- RabbitMQ 메시지 재시도

### 수동 복구
- PostgreSQL PITR (Point-in-Time Recovery)
- MinIO 백업 복원
- Dead Letter Queue 처리