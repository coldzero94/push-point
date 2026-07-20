# 개발 계획

## 전체 일정 (약 8-10주)

```
Phase 1: MVP (3주)
  ├─ Week 1: 백엔드 기본 구조
  ├─ Week 2: 앱 기본 기능
  └─ Week 3: 통합 및 테스트

Phase 2: AI 태깅 (2주)
  ├─ Week 4: OpenAI 연동
  └─ Week 5: 태그 관리 기능

Phase 3: 고급 기능 (2주)
  ├─ Week 6: 검색 & 필터
  └─ Week 7: 동기화

Phase 4: 배포 & 최적화 (1-2주)
  ├─ Week 8: K8s 배포
  └─ Week 9-10: 성능 최적화 & 버그 수정
```

---

## Phase 1: MVP (3주)

### Week 1: 백엔드 기본 구조

#### Day 1-2: 프로젝트 초기 설정
- [x] Go 프로젝트 구조 생성
- [x] 로컬 K8s 환경 구축 (Minikube)
- [x] PostgreSQL, Redis, RabbitMQ, MinIO 배포
- [x] Go 패키지 설치
  - [x] Gin (웹 프레임워크)
  - [x] Ent (ORM)
  - [x] go-redis (Redis 클라이언트)
  - [x] amqp091-go (RabbitMQ)
  - [x] aws-sdk-go-v2 (S3/MinIO)
  - [x] golang-jwt/jwt (JWT)
  - [x] viper (설정 관리)
  - [x] zap (로깅)
- [ ] 기본 헬스체크 엔드포인트

**디렉토리 구조**:
```
link-archive-api/
├── cmd/
│   ├── server/
│   │   └── main.go
│   └── worker/
│       └── main.go
├── internal/
│   ├── config/
│   ├── handler/
│   ├── service/
│   ├── repository/
│   ├── model/
│   └── middleware/
├── pkg/
│   ├── scraper/
│   ├── queue/
│   └── storage/
├── migrations/
├── docker/
│   ├── Dockerfile.api
│   └── Dockerfile.worker
├── k8s/
└── go.mod
```

#### Day 3-4: 데이터베이스 & 모델
- [x] PostgreSQL 스키마 생성
- [x] GORM 모델 정의
- [x] Migration 설정
- [x] Repository 패턴 구현

#### Day 5-7: 기본 API 구현
- [x] 사용자 인증 (JWT)
  - 회원가입
  - 로그인
  - 토큰 갱신
- [x] 링크 CRUD
  - 링크 저장 (기본 정보만)
  - 링크 목록 조회
  - 링크 상세 조회
  - 링크 삭제

**목표**:
- ✅ API 서버 실행
- ✅ PostgreSQL 연결
- ✅ 기본 CRUD 동작

---

### Week 2: 앱 기본 기능

#### Day 1-2: React Native 프로젝트 설정
- [x] React Native 프로젝트 생성
- [x] 네비게이션 설정 (React Navigation)
- [x] 상태 관리 (Zustand)
- [x] SQLite 설정

**디렉토리 구조**:
```
LinkArchive/
├── src/
│   ├── screens/
│   │   ├── HomeScreen.tsx
│   │   ├── LoginScreen.tsx
│   │   ├── LinkDetailScreen.tsx
│   │   └── TagsScreen.tsx
│   ├── components/
│   ├── services/
│   │   ├── api.ts
│   │   └── db.ts
│   ├── store/
│   ├── types/
│   └── utils/
├── ios/
├── android/
└── package.json
```

#### Day 3-4: 공유 기능 구현
- [x] iOS Share Extension
- [x] Android Share Intent
- [x] 링크 저장 API 연동

#### Day 5-7: 기본 UI 구현
- [x] 로그인/회원가입 화면
- [x] 링크 목록 화면
  - 카드 형태
  - 무한 스크롤
  - Pull to refresh
- [x] 링크 상세 화면
- [x] 로컬 DB 동기화

**목표**:
- ✅ 앱에서 링크 공유 → 저장
- ✅ 저장된 링크 목록 조회
- ✅ 기본 UI/UX

---

### Week 3: 통합 및 테스트

#### Day 1-3: 비동기 처리
- [x] RabbitMQ 연동
- [x] Worker 구현
  - 큐에서 작업 수신
  - URL 메타데이터 크롤링
  - DB 업데이트

#### Day 4-5: MinIO 연동
- [x] 썸네일 다운로드
- [x] 이미지 리사이징
- [x] MinIO 업로드
- [x] Pre-signed URL 생성

#### Day 6-7: 통합 테스트
- [x] End-to-End 플로우 테스트
- [x] 에러 처리
- [x] 로깅 개선
- [x] 기본 문서화

**목표**:
- ✅ 링크 저장 → 크롤링 → 썸네일 저장 전체 플로우
- ✅ 앱에서 완료된 링크 확인
- ✅ 안정적인 MVP

---

## Phase 2: AI 태깅 (2주)

### Week 4: OpenAI 연동

#### Day 1-2: OpenAI 클라이언트 구현
- [ ] OpenAI API 클라이언트
- [ ] 프롬프트 엔지니어링
- [ ] 응답 파싱

**프롬프트 예시**:
```
시스템: "당신은 웹 콘텐츠를 분석하고 태그를 생성하는 AI입니다."

사용자: "
다음 콘텐츠를 분석해서 태그를 생성해주세요:
제목: {title}
설명: {description}
도메인: {domain}

JSON 형식으로 응답:
{
  "category": "카테고리",
  "content_type": "콘텐츠 타입",
  "keywords": ["키워드1", "키워드2"],
  "mood": "감정/톤",
  "difficulty": "난이도"
}
"
```

#### Day 3-4: 태그 생성 워커
- [ ] AI 태그 생성 작업 큐
- [ ] 태그 저장 로직
- [ ] 신뢰도 점수 저장

#### Day 5-7: 최적화 & 테스트
- [ ] 배치 처리
- [ ] 캐싱 전략
- [ ] 비용 최적화
- [ ] 다양한 콘텐츠 타입 테스트

**목표**:
- ✅ 자동 태그 생성 동작
- ✅ 정확도 80% 이상
- ✅ 평균 처리 시간 < 5초

---

### Week 5: 태그 관리 기능

#### Day 1-3: 태그 API 구현
- [ ] 태그 목록 조회
- [ ] 태그별 링크 조회
- [ ] 태그 수정/삭제
- [ ] 수동 태그 추가

#### Day 4-7: 앱 UI 구현
- [ ] 태그 화면
  - 태그 클라우드
  - 태그 목록
  - 사용 빈도 표시
- [ ] 태그 필터링
- [ ] 태그 편집 기능

**목표**:
- ✅ 태그 기반 필터링
- ✅ 수동 태그 편집
- ✅ 직관적인 태그 UI

---

## Phase 3: 고급 기능 (2주)

### Week 6: 검색 & 필터

#### Day 1-3: 검색 기능
- [ ] PostgreSQL Full-Text Search
- [ ] 검색 API 구현
- [ ] 검색 결과 랭킹

#### Day 4-5: 고급 필터
- [ ] 복합 필터 (태그 + 날짜)
- [ ] 정렬 옵션
- [ ] 필터 프리셋 저장

#### Day 6-7: 앱 UI
- [ ] 검색 바
- [ ] 필터 UI
- [ ] 검색 히스토리

**목표**:
- ✅ 빠른 검색 (< 300ms)
- ✅ 정확한 검색 결과
- ✅ 사용하기 쉬운 필터

---

### Week 7: 동기화

#### Day 1-3: 동기화 로직
- [ ] Pull 동기화 API
- [ ] Push 동기화 API
- [ ] 충돌 해결 로직
- [ ] 버전 관리

#### Day 4-7: 앱 구현
- [ ] 백그라운드 동기화
- [ ] 오프라인 모드
- [ ] 동기화 상태 UI
- [ ] 충돌 처리 UI

**목표**:
- ✅ 여러 기기 동기화
- ✅ 오프라인 작업 가능
- ✅ 데이터 일관성 유지

---

## Phase 4: 배포 & 최적화 (1-2주)

### Week 8: K8s 배포

#### Day 1-3: 프로덕션 준비
- [ ] Docker 이미지 최적화
- [ ] Multi-stage build
- [ ] 환경별 설정 분리
- [ ] Secret 관리

#### Day 4-5: K8s 설정
- [ ] Deployment YAML 작성
- [ ] Service, Ingress 설정
- [ ] HPA 설정
- [ ] Resource Limits

#### Day 6-7: CI/CD
- [ ] GitHub Actions 설정
- [ ] 자동 빌드
- [ ] 자동 배포
- [ ] 테스트 자동화

**목표**:
- ✅ 로컬 K8s 완벽 동작
- ✅ CI/CD 파이프라인
- ✅ 배포 자동화

---

### Week 9-10: 성능 최적화 & 버그 수정

#### 성능 최적화
- [ ] Database 쿼리 최적화
  - 인덱스 튜닝
  - N+1 쿼리 제거
- [ ] Redis 캐싱 전략
  - Cache warming
  - TTL 최적화
- [ ] API 응답 시간 개선
  - 불필요한 데이터 제거
  - 페이지네이션 최적화
- [ ] Worker 성능
  - 병렬 처리
  - 배치 크기 조정

#### 모니터링 (선택사항)
- [ ] Prometheus 설정
- [ ] Grafana 대시보드
- [ ] 알림 설정

#### 버그 수정 & 안정화
- [ ] 엣지 케이스 처리
- [ ] 에러 핸들링 개선
- [ ] 로깅 체계화
- [ ] 사용자 피드백 반영

**목표**:
- ✅ API 응답 시간 < 200ms (95 percentile)
- ✅ 메모리 사용량 최적화
- ✅ 99% 가동률

---

## 개발 우선순위

### Must Have (필수)
1. ✅ 링크 저장 (공유 기능)
2. ✅ URL 메타데이터 크롤링
3. ✅ 자동 태그 생성
4. ✅ 링크 목록 조회
5. ✅ 태그별 필터링

### Should Have (중요)
6. ✅ 검색 기능
7. ✅ 메모 추가
8. ✅ 동기화
9. ✅ 오프라인 모드

### Nice to Have (선택)
10. ⬜ 캘린더 뷰
11. ⬜ 통계 대시보드
12. ⬜ 태그 자동 완성
13. ⬜ 링크 공유
14. ⬜ 다크 모드

---

## 기술적 도전 과제

### 1. 크롤링 안정성
**문제**: 다양한 웹사이트 형식, 동적 콘텐츠
**해결**: 
- User-Agent 설정
- Headless browser (Chromedp) 사용
- Retry 로직
- Timeout 설정

### 2. OpenAI 비용
**문제**: API 호출 비용
**해결**:
- GPT-4o-mini 사용
- 캐싱 (같은 도메인/채널)
- 배치 처리
- 월별 사용량 제한

### 3. 이미지 저장
**문제**: 스토리지 용량
**해결**:
- 이미지 리사이징
- WebP 포맷 사용
- 주기적 정리 (미사용 이미지)

### 4. 동기화 충돌
**문제**: 여러 기기에서 동시 수정
**해결**:
- 낙관적 잠금 (version 필드)
- Last-Write-Wins 전략
- 충돌 UI 제공

---

## 테스트 전략

### 단위 테스트
- Repository 계층
- Service 로직
- 유틸리티 함수

### 통합 테스트
- API 엔드포인트
- 데이터베이스 연동
- 큐 처리

### E2E 테스트
- 전체 링크 저장 플로우
- 동기화 시나리오

**목표 커버리지**: 70% 이상

---

## 위험 관리

| 위험 | 영향 | 확률 | 대응 방안 |
|------|------|------|----------|
| OpenAI API 장애 | 높음 | 낮음 | Fallback: 룰 기반 태깅 |
| 크롤링 실패율 높음 | 중간 | 중간 | 재시도, DLQ 처리 |
| 동기화 충돌 | 중간 | 중간 | 명확한 충돌 해결 정책 |
| 성능 저하 | 높음 | 낮음 | 캐싱, 인덱싱 |
| 비용 초과 | 중간 | 중간 | 사용량 모니터링, 제한 |

---

## 성공 기준

### 기능적
- ✅ 링크 저장 성공률 > 95%
- ✅ 자동 태그 정확도 > 80%
- ✅ 검색 결과 만족도

### 성능적
- ✅ 링크 저장 응답 < 500ms
- ✅ 목록 조회 < 200ms
- ✅ 전체 처리 (크롤링+태깅) < 15초

### 사용성
- ✅ 최소 클릭으로 저장 (2회 이하)
- ✅ 직관적인 UI
- ✅ 빠른 검색/필터

---

## 다음 단계 (MVP 이후)

1. **모바일 최적화**
   - 오프라인 우선
   - 백그라운드 동기화
   - 푸시 알림

2. **소셜 기능**
   - 링크 공유
   - 공개 프로필
   - 팔로우

3. **고급 분석**
   - 읽기 패턴 분석
   - 추천 시스템
   - 트렌드 분석

4. **추가 통합**
   - Pocket, Instapaper 가져오기
   - Chrome Extension
   - Zapier 연동