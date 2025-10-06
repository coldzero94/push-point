# API 명세서

## 기본 정보

- **Base URL**: `https://api.linkarchive.com` (프로덕션)
- **Base URL**: `http://localhost:8080` (로컬)
- **API Version**: v1
- **인증 방식**: JWT Bearer Token
- **응답 형식**: JSON

## 공통 응답 형식

### 성공 응답

```json
{
  "success": true,
  "data": { ... },
  "message": "Success",
  "timestamp": "2025-10-05T10:00:00Z"
}
```

### 에러 응답

```json
{
  "success": false,
  "error": {
    "code": "INVALID_INPUT",
    "message": "Invalid URL format",
    "details": { ... }
  },
  "timestamp": "2025-10-05T10:00:00Z"
}
```

## 에러 코드

| 코드 | HTTP Status | 설명 |
|------|-------------|------|
| `UNAUTHORIZED` | 401 | 인증 실패 |
| `FORBIDDEN` | 403 | 권한 없음 |
| `NOT_FOUND` | 404 | 리소스 없음 |
| `INVALID_INPUT` | 400 | 잘못된 입력 |
| `DUPLICATE` | 409 | 중복 데이터 |
| `RATE_LIMIT` | 429 | 요청 제한 초과 |
| `INTERNAL_ERROR` | 500 | 서버 에러 |

---

## 1. 인증 (Authentication)

### 1.1 회원가입

```
POST /api/v1/auth/register
```

**Request Body**:
```json
{
  "email": "user@example.com",
  "password": "SecurePass123!",
  "display_name": "홍길동"
}
```

**Response** (201 Created):
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid-v4",
      "email": "user@example.com",
      "display_name": "홍길동",
      "created_at": "2025-10-05T10:00:00Z"
    },
    "tokens": {
      "access_token": "eyJhbGci...",
      "refresh_token": "eyJhbGci...",
      "expires_in": 3600
    }
  }
}
```

---

### 1.2 로그인

```
POST /api/v1/auth/login
```

**Request Body**:
```json
{
  "email": "user@example.com",
  "password": "SecurePass123!"
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "user": {
      "id": "uuid-v4",
      "email": "user@example.com",
      "display_name": "홍길동"
    },
    "tokens": {
      "access_token": "eyJhbGci...",
      "refresh_token": "eyJhbGci...",
      "expires_in": 3600
    }
  }
}
```

---

### 1.3 토큰 갱신

```
POST /api/v1/auth/refresh
```

**Request Body**:
```json
{
  "refresh_token": "eyJhbGci..."
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "access_token": "eyJhbGci...",
    "expires_in": 3600
  }
}
```

---

### 1.4 로그아웃

```
POST /api/v1/auth/logout
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "message": "Logged out successfully"
}
```

---

## 2. 링크 (Links)

### 2.1 링크 저장

```
POST /api/v1/links
Authorization: Bearer {access_token}
```

**Request Body**:
```json
{
  "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "source": "youtube_share"  // optional
}
```

**Response** (201 Created):
```json
{
  "success": true,
  "data": {
    "id": 12345,
    "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
    "status": "pending",
    "created_at": "2025-10-05T10:00:00Z"
  },
  "message": "Link saved successfully. Processing in background."
}
```

---

### 2.2 링크 목록 조회

```
GET /api/v1/links?page=1&limit=20&tag=tech&sort=created_at&order=desc
Authorization: Bearer {access_token}
```

**Query Parameters**:
- `page` (int, default: 1): 페이지 번호
- `limit` (int, default: 20, max: 100): 페이지 크기
- `tag` (string, optional): 태그 필터
- `sort` (string, default: created_at): 정렬 필드 (created_at, updated_at, title)
- `order` (string, default: desc): 정렬 순서 (asc, desc)
- `status` (string, optional): 상태 필터 (pending, completed, failed)

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "links": [
      {
        "id": 12345,
        "url": "https://www.youtube.com/watch?v=...",
        "title": "Amazing Video Title",
        "description": "This is a great video about...",
        "thumbnail_url": "https://minio.local/thumbnails/resized/medium/12345_hash.jpg",
        "domain": "youtube.com",
        "content_type": "youtube",
        "author": "Channel Name",
        "status": "completed",
        "tags": [
          {
            "id": 1,
            "name": "기술",
            "category": "topic",
            "color": "#FF5733"
          },
          {
            "id": 5,
            "name": "튜토리얼",
            "category": "type",
            "color": "#3357FF"
          }
        ],
        "note": {
          "id": 999,
          "content": "나중에 다시 보기"
        },
        "created_at": "2025-10-05T10:00:00Z",
        "updated_at": "2025-10-05T10:05:00Z"
      }
    ],
    "pagination": {
      "page": 1,
      "limit": 20,
      "total": 156,
      "total_pages": 8,
      "has_next": true,
      "has_prev": false
    }
  }
}
```

---

### 2.3 링크 상세 조회

```
GET /api/v1/links/:id
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "id": 12345,
    "url": "https://www.youtube.com/watch?v=...",
    "title": "Amazing Video Title",
    "description": "Detailed description...",
    "thumbnail_url": "https://minio.local/thumbnails/...",
    "domain": "youtube.com",
    "content_type": "youtube",
    "author": "Channel Name",
    "published_at": "2025-10-01T12:00:00Z",
    "duration": 720,
    "language": "ko",
    "status": "completed",
    "tags": [...],
    "note": {...},
    "view_count": 5,
    "last_viewed_at": "2025-10-05T09:30:00Z",
    "created_at": "2025-10-05T10:00:00Z",
    "updated_at": "2025-10-05T10:05:00Z"
  }
}
```

---

### 2.4 링크 수정 (메모/태그)

```
PATCH /api/v1/links/:id
Authorization: Bearer {access_token}
```

**Request Body**:
```json
{
  "note": "이 영상 꼭 다시 보기!",
  "tags": ["기술", "React", "웹개발"]  // 태그 이름 배열
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "id": 12345,
    "note": {
      "id": 999,
      "content": "이 영상 꼭 다시 보기!",
      "updated_at": "2025-10-05T11:00:00Z"
    },
    "tags": [...]
  }
}
```

---

### 2.5 링크 삭제 (Soft Delete)

```
DELETE /api/v1/links/:id
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "message": "Link deleted successfully"
}
```

---

### 2.6 링크 처리 상태 확인

```
GET /api/v1/links/:id/status
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "id": 12345,
    "status": "completed",
    "progress": {
      "scraping": "completed",
      "tagging": "completed",
      "thumbnail": "completed"
    },
    "updated_at": "2025-10-05T10:05:00Z"
  }
}
```

---

## 3. 태그 (Tags)

### 3.1 전체 태그 조회

```
GET /api/v1/tags?sort=usage&limit=50
Authorization: Bearer {access_token}
```

**Query Parameters**:
- `sort` (string, default: usage): 정렬 기준 (usage, name, created_at)
- `limit` (int, default: 50): 최대 개수
- `category` (string, optional): 카테고리 필터

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "tags": [
      {
        "id": 1,
        "name": "기술",
        "category": "topic",
        "color": "#FF5733",
        "icon": "💻",
        "usage_count": 42,
        "created_at": "2025-09-01T10:00:00Z"
      },
      {
        "id": 2,
        "name": "튜토리얼",
        "category": "type",
        "color": "#3357FF",
        "icon": "📚",
        "usage_count": 28,
        "created_at": "2025-09-05T14:00:00Z"
      }
    ]
  }
}
```

---

### 3.2 새 태그 생성

```
POST /api/v1/tags
Authorization: Bearer {access_token}
```

**Request Body**:
```json
{
  "name": "AI",
  "category": "topic",
  "color": "#00FF00",
  "icon": "🤖"
}
```

**Response** (201 Created):
```json
{
  "success": true,
  "data": {
    "id": 15,
    "name": "AI",
    "category": "topic",
    "color": "#00FF00",
    "icon": "🤖",
    "usage_count": 0,
    "created_at": "2025-10-05T11:00:00Z"
  }
}
```

---

### 3.3 특정 태그의 링크 조회

```
GET /api/v1/tags/:id/links?page=1&limit=20
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "tag": {
      "id": 1,
      "name": "기술",
      "category": "topic"
    },
    "links": [...],
    "pagination": {...}
  }
}
```

---

### 3.4 태그 수정

```
PUT /api/v1/tags/:id
Authorization: Bearer {access_token}
```

**Request Body**:
```json
{
  "name": "기술 & IT",
  "color": "#FF0000"
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "id": 1,
    "name": "기술 & IT",
    "color": "#FF0000",
    "updated_at": "2025-10-05T11:30:00Z"
  }
}
```

---

### 3.5 태그 삭제

```
DELETE /api/v1/tags/:id
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "message": "Tag deleted successfully"
}
```

---

## 4. 검색 (Search)

### 4.1 통합 검색

```
GET /api/v1/search?q=react+hooks&tags=tech,tutorial&from=2025-09-01&to=2025-10-05&page=1&limit=20
Authorization: Bearer {access_token}
```

**Query Parameters**:
- `q` (string, required): 검색어
- `tags` (string, optional): 태그 필터 (쉼표 구분)
- `from` (date, optional): 시작 날짜 (YYYY-MM-DD)
- `to` (date, optional): 종료 날짜 (YYYY-MM-DD)
- `page` (int, default: 1): 페이지
- `limit` (int, default: 20): 페이지 크기

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "query": "react hooks",
    "links": [
      {
        "id": 12345,
        "title": "React Hooks Tutorial",
        "relevance_score": 0.95,
        ...
      }
    ],
    "pagination": {...},
    "filters_applied": {
      "tags": ["tech", "tutorial"],
      "date_range": {
        "from": "2025-09-01",
        "to": "2025-10-05"
      }
    }
  }
}
```

---

## 5. 동기화 (Sync)

### 5.1 동기화 상태 확인

```
GET /api/v1/sync/status
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "last_sync_at": "2025-10-05T09:00:00Z",
    "pending_changes": 0,
    "sync_enabled": true
  }
}
```

---

### 5.2 서버 변경사항 가져오기 (Pull)

```
GET /api/v1/sync/pull?since=2025-10-05T09:00:00Z&limit=100
Authorization: Bearer {access_token}
```

**Query Parameters**:
- `since` (ISO 8601 datetime, required): 마지막 동기화 시간
- `limit` (int, default: 100, max: 500): 최대 개수

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "links": [
      {
        "id": 12345,
        "action": "created",  // created, updated, deleted
        "data": {...}
      },
      {
        "id": 12346,
        "action": "updated",
        "data": {...}
      }
    ],
    "sync_timestamp": "2025-10-05T10:30:00Z",
    "has_more": false
  }
}
```

---

### 5.3 로컬 변경사항 전송 (Push)

```
POST /api/v1/sync/push
Authorization: Bearer {access_token}
```

**Request Body**:
```json
{
  "changes": [
    {
      "link_id": 12345,
      "type": "note_update",
      "data": {
        "content": "이 영상 나중에 다시 보기"
      },
      "timestamp": "2025-10-05T10:15:00Z",
      "version": 3
    },
    {
      "link_id": 12346,
      "type": "tag_update",
      "data": {
        "tags": ["AI", "머신러닝"]
      },
      "timestamp": "2025-10-05T10:20:00Z",
      "version": 2
    }
  ]
}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "applied": 2,
    "conflicts": 0,
    "failed": 0,
    "results": [
      {
        "link_id": 12345,
        "status": "success",
        "new_version": 4
      },
      {
        "link_id": 12346,
        "status": "success",
        "new_version": 3
      }
    ]
  }
}
```

---

## 6. 통계 (Statistics)

### 6.1 전체 통계

```
GET /api/v1/stats/overview
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "total_links": 156,
    "total_tags": 23,
    "total_notes": 87,
    "links_this_week": 12,
    "links_this_month": 45,
    "most_active_day": "2025-10-03",
    "average_links_per_day": 2.3
  }
}
```

---

### 6.2 태그별 분포

```
GET /api/v1/stats/tags
Authorization: Bearer {access_token}
```

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "tags": [
      {
        "name": "기술",
        "count": 42,
        "percentage": 26.9
      },
      {
        "name": "디자인",
        "count": 28,
        "percentage": 17.9
      }
    ]
  }
}
```

---

### 6.3 시간별 저장 추이

```
GET /api/v1/stats/timeline?from=2025-09-01&to=2025-10-05&interval=day
Authorization: Bearer {access_token}
```

**Query Parameters**:
- `from` (date, required): 시작 날짜
- `to` (date, required): 종료 날짜
- `interval` (string, default: day): 집계 단위 (day, week, month)

**Response** (200 OK):
```json
{
  "success": true,
  "data": {
    "timeline": [
      {
        "date": "2025-09-01",
        "count": 3
      },
      {
        "date": "2025-09-02",
        "count": 5
      }
    ]
  }
}
```

---

## Rate Limiting

- **기본 제한**: 100 requests/minute per user
- **링크 저장**: 30 requests/minute per user
- **검색**: 60 requests/minute per user

**헤더**:
```
X-RateLimit-Limit: 100
X-RateLimit-Remaining: 95
X-RateLimit-Reset: 1696502400
```

**초과 시 응답** (429):
```json
{
  "success": false,
  "error": {
    "code": "RATE_LIMIT",
    "message": "Too many requests. Please try again later.",
    "retry_after": 45
  }
}
```

---

## WebSocket (선택사항)

### 연결

```
ws://api.linkarchive.com/ws?token={access_token}
```

### 이벤트

**링크 처리 완료**:
```json
{
  "type": "link.completed",
  "data": {
    "link_id": 12345,
    "status": "completed",
    "timestamp": "2025-10-05T10:05:00Z"
  }
}
```

**동기화 알림**:
```json
{
  "type": "sync.available",
  "data": {
    "changes_count": 5,
    "timestamp": "2025-10-05T10:10:00Z"
  }
}
```