# API 명세서

> Push-Point v2.1 — 마지막 업데이트: 2026-07-22

> 이 문서는 사람용 해설·예시다. **기계가 읽는 원본은 [api/openapi.yaml](../../api/openapi.yaml)**이며 백엔드·iOS 클라이언트 코드가 여기서 생성된다. 두 문서가 다르면 openapi.yaml이 우선하고, 스펙 변경 시 이 문서를 같은 커밋에서 갱신한다.

Push-Point v2의 HTTP API 전체 명세다. 서버는 단일 Go 바이너리(`pushpoint`) 하나이며, 모든 엔드포인트는 이 프로세스가 직접 서빙한다. 스키마 필드 정의는 [05-DATA-SCHEMA.md](05-DATA-SCHEMA.md), 저장 이후 비동기 처리 흐름은 [04-DATA-FLOW.md](04-DATA-FLOW.md)를 참고한다.

## 1. 기본 정보

- **Base URL**: `http://localhost:8420` (`PUSHPOINT_ADDR`로 변경 가능, 기본 `:8420`)
- **응답 형식**: JSON
- **시간 표현**: 모든 시간 필드(`created_at`, `published_at` 등)는 **unix epoch 초 단위 정수**다. ISO 8601 문자열을 쓰지 않는다.

### 인증

단일 사용자 앱이므로 계정 개념이 없다. 정적 API 키 1개(`PUSHPOINT_API_KEY`)를 모든 요청 헤더에 실어 보낸다. `GET /healthz`와 `GET /thumbs/{dir}/{file}`만 인증이 면제된다 (썸네일 면제 이유는 8절).

```
Authorization: Bearer {PUSHPOINT_API_KEY}
```

```bash
curl -H "Authorization: Bearer dev-key" http://localhost:8420/api/v1/links
```

`just dev`로 띄운 서버는 키가 `dev-key`로 설정된다. iOS 자립 모드는 앱이 실행마다 난수 키를 만들어 자기 인프로세스 서버에 넘기므로 보관 대상이 없고, 홈서버 모드의 키는 Keychain(앱 그룹 공유)에 둔다.

### 에러 형식

모든 에러는 동일한 형태로 반환한다.

```json
{
  "error": {
    "code": "invalid_input",
    "message": "url is required"
  }
}
```

| code | HTTP Status | 의미 |
|------|-------------|------|
| `unauthorized` | 401 | API 키 누락 또는 불일치 |
| `invalid_input` | 400 | 잘못된 요청 (URL 형식 오류 등) |
| `not_found` | 404 | 리소스 없음 (소프트 삭제된 링크 포함) |
| `internal` | 500 | 서버 내부 오류 |

에러 코드는 이 4개가 전부다. 단일 사용자이므로 `forbidden`, 큐가 인프로세스이므로 `rate_limit` 같은 코드는 존재하지 않는다.

**모든 오퍼레이션에 공통으로 붙는 응답이 둘 있다.** 인증이 필요한 모든 엔드포인트는 401 `unauthorized`를 낼 수 있고(면제는 `GET /healthz`와 `GET /thumbs/{dir}/{file}` 둘뿐), 500 `internal`은 `GET /healthz` **하나만** 면제된다 — healthz 핸들러는 조건 없는 단일 반환이라 실패 경로 자체가 없다. 썸네일 서빙은 인증은 면제지만 `os.Open`/`Stat`이 ENOENT 아닌 이유로 실패하면 500을 내므로 500 면제가 아니다(권한 없는 파일에 대해 실측 확인). 계약(`api/openapi.yaml`)에도 `Unauthorized` / `InternalError` 응답 컴포넌트 참조로 이 경계 그대로 선언돼 있다. 아래 각 엔드포인트의 "상태 코드" 줄은 이 둘을 매번 반복하지 않는다. 500은 핸들러가 처리하지 못한 에러의 공통 종착점이며, 서버는 이때도 같은 `{error:{code,message}}` 형식을 지킨다(HTML이나 빈 본문을 내보내지 않는다).

## 2. 커서 페이지네이션 규약

목록(`GET /api/v1/links`)과 검색(`GET /api/v1/search`)은 **keyset 커서** 방식을 쓴다.

- `cursor` (string, optional): 이전 응답의 `next_cursor` 값을 그대로 전달. 첫 페이지는 생략. 불투명(opaque) 토큰이므로 클라이언트가 내용을 해석하거나 조작하지 않는다. 목록 커서와 검색(FTS) 커서는 형식이 달라 서로 호환되지 않는다 — 모드가 다르거나 형식이 깨진 커서는 400 `invalid_input`.
- `limit` (int, 기본 20, 최대 100): 페이지 크기. 1 미만은 400 `invalid_input`, 100 초과는 100으로 보정된다.
- 응답의 `next_cursor` (string | null): 다음 페이지 커서. `null`이면 마지막 페이지.

```json
{
  "links": [ ... ],
  "next_cursor": "eyJjcmVhdGVkX2F0IjoxNzUyOTgwMDAwLCJpZCI6MTIzNH0"
}
```

OFFSET 페이지네이션은 사용하지 않는다 — OFFSET은 건너뛰는 행을 전부 스캔하므로 깊은 페이지일수록 느려지고, keyset은 `(created_at, id)` 인덱스를 타므로 링크 10만 건에서도 목록 API p99 < 50ms 목표를 지킬 수 있다.

## 3. 헬스체크

### GET /healthz

인증 없음. 프로세스 생존 확인용.

**Response** (200 OK):
```json
{"status": "ok"}
```

## 4. 링크 (Links)

### 4.1 링크 저장

```
POST /api/v1/links
```

**Request Body**:
```json
{
  "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "note": "주말에 볼 것"
}
```

- `url` (string, 필수)
- `note` (string, optional): 개인 메모

**Response** (201 Created):
```json
{
  "id": 1234,
  "status": "pending",
  "created_at": 1752980000
}
```

동작: `INSERT INTO links` + `INSERT INTO jobs(kind='scrape')`를 한 트랜잭션으로 커밋하고 즉시 응답한다. 동기 작업은 이 INSERT 두 번이 전부이므로 p99 < 50ms를 보장한다 — Share Extension이 공유 시트에서 한 탭으로 저장하고 바로 닫힐 수 있는 근거다. 스크랩·태깅·썸네일은 전부 백그라운드 잡으로 처리되며, 진행 상황은 상세 조회의 `status`와 `jobs`로 확인한다.

**중복 저장 시** (200 OK): 동일 `url_hash`(SHA-256(url))가 이미 존재하면 새로 만들지 않고 기존 id를 돌려준다.
```json
{
  "id": 987,
  "duplicate": true
}
```

이 API는 `url_hash` 기반으로 멱등하다 — 클라이언트가 같은 요청을 재시도해도 중복 생성이 없다(중복 시 200 `duplicate:true`). 오프라인 큐에 남은 요청을 안심하고 다시 보낼 수 있는 근거다. 소프트 삭제된 링크의 URL을 다시 저장하면 같은 행을 복원(`pending` 복귀, `note` 교체, scrape 재-enqueue)하고 신규 저장처럼 201로 응답한다.

**클라이언트 캡처 필드 (optional)** — 서버가 fetch할 수 없는 페이지(SPA·봇 차단·로그인 벽)에서, 이미 그 페이지를 렌더한 클라이언트(브라우저 확장·iOS Share Extension)가 콘텐츠를 함께 보낸다.

| 필드 | 상한 | 설명 |
|---|---|---|
| `title` | 512바이트 | 클라이언트가 캡처한 제목 |
| `description` | 2048바이트 | 클라이언트가 캡처한 설명 |
| `body_text` | 32KB | 클라이언트가 추출한 본문 평문 (개행 유지 — 요약이 문장 구분에 쓴다) |
| `keywords` | 512바이트 | **발행자가 스스로 붙인 분류** — `meta[name=keywords]`·`news_keywords`·`article:section`·`article:tag`·JSON-LD `articleSection`을 모아 콤마로 이은 값. 우리가 본문에서 추론한 값이 아니라 사이트가 선언한 값이라 태거가 제목과 같은 가중치를 준다. **태거 입력 전용** — 응답 어디에도 노출되지 않는다 |

- 상한 초과는 **400이 아니라 절단**이다. 클라이언트가 룬·바이트 경계를 서버와 똑같이 맞출 방법이 없으므로, 경계에서 거부하면 정상 캡처가 조용히 실패한다.
- `body_text`가 있으면 서버는 본문 출처를 `client`로 표시하고 **이후 스크랩이 제목·설명·본문·분류를 덮어쓰지 않는다.** 스크랩은 계속 돌아 썸네일·author·published_at을 채우며, 확정 실패해도 그 링크는 `failed`가 아니라 `done`으로 남는다(사유는 `error`에 기록).
- `body_text`가 있으면 **태깅 잡이 저장 시점에 함께 생성된다** — 스크랩 성공을 기다리지 않는다(스크랩이 실패하는 바로 그 페이지이기 때문).
- **중복(200) 경로의 1회 보충**: 저장된 본문이 서버 출처인데 요청이 클라이언트 본문을 실어 오면 제목·설명·본문·분류를 보충하고(제목·설명·분류는 요청이 준 것만) 태깅을 다시 돌린 뒤, `failed`였다면 `done`으로 올린다. 이미 클라이언트 본문이면 아무것도 하지 않으므로 반복 호출은 같은 상태로 수렴한다 — **이미 저장돼 실패해 있던 링크를 나중에 채우는 경로**다.

**상태 코드**: 201(신규 저장) / 200(중복) / 400(`invalid_input`, url 누락·형식 오류)

### 4.2 링크 목록 조회

```
GET /api/v1/links?cursor=&limit=&tag=&status=
```

**Query Parameters**:
- `cursor`, `limit`: 2절의 페이지네이션 규약
- `tag` (string, optional): 태그 이름으로 필터
- `status` (string, optional): `pending` | `scraping` | `tagging` | `done` | `failed`

**Response** (200 OK):
```json
{
  "links": [
    {
      "id": 1234,
      "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
      "domain": "youtube.com",
      "title": "Go 동시성 패턴 완전 정복",
      "description": "goroutine과 채널을 이용한 워커 풀 구성부터 singleflight까지…",
      "content_type": "video",
      "thumb_url": "/thumbs/a3/a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8.jpg",
      "status": "done",
      "tags": [
        {"id": 3, "name": "dev", "source": "rules", "confidence": 0.82},
        {"id": 7, "name": "video", "source": "rules", "confidence": 0.95}
      ],
      "note": "주말에 볼 것",
      "created_at": 1752980000
    }
  ],
  "next_cursor": "eyJjcmVhdGVkX2F0IjoxNzUyOTgwMDAwLCJpZCI6MTIzNH0"
}
```

- `description`은 목록에서 200자로 절단된다 (전체는 상세 조회).
- `thumb_url`은 `/thumbs/{dir}/{file}` 형태의 서버 상대 경로다 (경로 규칙은 8절). 썸네일이 없으면 `null`.
- `tags[].source`는 `rules` | `embed` | `manual`, `confidence`는 `manual`이면 `null`.

**상태 코드**: 200 / 400(`invalid_input` — 잘못된 커서·limit·status 형식)

### 4.3 링크 상세 조회

```
GET /api/v1/links/{id}
```

**Response** (200 OK):
```json
{
  "id": 1234,
  "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
  "domain": "youtube.com",
  "title": "Go 동시성 패턴 완전 정복",
  "description": "goroutine과 채널을 이용한 워커 풀 구성부터 singleflight, errgroup을 활용한 에러 전파까지 실전 예제로 다룬다.",
  "content_type": "video",
  "thumb_url": "/thumbs/a3/a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8.jpg",
  "status": "done",
  "tags": [
    {"id": 3, "name": "dev", "source": "rules", "confidence": 0.82},
    {"id": 7, "name": "video", "source": "rules", "confidence": 0.95}
  ],
  "note": "주말에 볼 것",
  "created_at": 1752980000,
  "author": "Some Channel",
  "published_at": 1752800000,
  "duration_sec": 1420,
  "word_count": null,
  "lang": "ko",
  "summary": "쿠버네티스는 컨테이너를 선언적으로 운영하는 오케스트레이터다.\n파드가 배포의 최소 단위이고, 서비스가 트래픽을 분산한다.",
  "error": "",
  "jobs": {
    "scrape": "done",
    "tag": "done",
    "thumb": "failed"
  }
}
```

- 목록 항목의 전체 필드에 `author`, `published_at`, `duration_sec`, `word_count`, `lang`, `summary`, `error`가 추가된다. `published_at`·`duration_sec`·`word_count`는 값이 없으면 `null`, `author`·`lang`·`error`는 빈 문자열이다 (05 스키마의 NOT NULL DEFAULT '' 정의와 일치).
- `summary`는 본문에서 고른 핵심 문장 2~3개를 개행으로 이은 **추출식 요약**이다 (M5 Phase A — LLM 없이 원문 문장을 그대로 선택하므로 환각이 없다). 본문이 얇거나(200룬 미만) 산문이 3문장 미만이거나 `description`과 사실상 같으면 **빈 문자열**이고, 그때 클라이언트는 요약 영역을 그리지 않는다. **목록(`Link`)과 검색(`SearchResult`)에는 실리지 않는다** — 요약은 원문 대체재가 아니라 "열까 말까"의 판단 보조이고, 목록 응답을 가볍게 유지한다.
- `jobs`는 이 링크에 연결된 잡의 상태 요약 `{scrape, tag, thumb: status}`다. 각 값은 `pending` | `running` | `done` | `failed`. 해당 kind의 잡이 아직 없으면 필드가 생략된다 — `scrape` 잡은 저장 트랜잭션에서 항상 함께 생성되므로 항상 존재하고, `tag`는 scrape 성공 후, `thumb`은 og:image가 있을 때만 생긴다 (M1에서는 `scrape`만 있고, M2에서는 tagger가 아직 없어 scrape 성공 시 `tag` 잡을 만들지 않고 `links.status`가 `scraping`에서 곧바로 `done`이 된다 — `tag` 필드와 `tagging` 상태는 M3에서 tagger 핸들러가 등록돼야 도달한다). 위 예시처럼 `thumb`이 `failed`여도 링크 `status`는 `done`일 수 있다 — 썸네일 잡은 best-effort이며 실패해도 링크 상태에 영향을 주지 않는다 (`thumb_url`만 `null`로 남는다).

**상태 코드**: 200 / 400(`invalid_input` — 정수가 아닌 id) / 404(`not_found`)

### 4.4 링크 수정 (메모/태그)

```
PATCH /api/v1/links/{id}
```

**Request Body**:
```json
{
  "note": "다시 보니 워커 풀 파트만 유용함",
  "tags": ["dev", "golang"]
}
```

- `note` (string, optional): 전달 시 메모 교체.
- `tags` (string 배열, optional): 태그 **이름** 배열. 전달 시 이 링크의 태그를 **전체 교체**한다 — 부분 추가/삭제 API는 없다. `null`이거나 필드를 생략하면 태그를 유지하고, 빈 배열 `[]`이면 전체 제거로 처리한다.
  - 추가된 태그: `link_tags(source='manual', confidence=NULL)` 저장 + `tag_feedback(action='added')` 기록
  - 제거된 태그: `link_tags`에서 삭제 + `tag_feedback(action='removed')` 기록

`tag_feedback`은 단순 로그가 아니라 M5 임베딩 태거의 재랭킹 가중치 보정에 쓰이는 학습 데이터다. 사용자가 태그를 고칠수록 자동 태깅이 좋아지는 구조이므로, 클라이언트는 태그 수정을 반드시 이 API로 보낸다.

**Response** (200 OK): 수정 반영된 링크 상세(4.3과 동일 형태).

**상태 코드**: 200 / 400(존재하지 않는 태그 이름) / 404

### 4.5 링크 삭제

```
DELETE /api/v1/links/{id}
```

소프트 삭제 — `deleted_at`만 기록하며 행은 남는다. 이후 목록/검색/상세에서 제외된다. 같은 URL을 다시 저장(4.1)하면 행이 복원된다.

**Response**: 204 No Content

**상태 코드**: 204 / 400(`invalid_input` — 정수가 아닌 id) / 404

### 4.6 실패 링크 재시도

```
POST /api/v1/links/{id}/retry
```

`status='failed'`인 링크의 잡을 다시 enqueue한다. 링크는 `pending`으로 되돌아가고 워커가 처음부터 다시 처리한다.

**Response** (202 Accepted):
```json
{
  "id": 1234,
  "status": "pending"
}
```

**상태 코드**: 202 / 400(`failed` 상태가 아닌 링크) / 404

## 5. 태그 (Tags)

태그는 자유 문자열이 아니라 **통제된 사전**(초기 30~50개 시드, 사용자 수정 가능)이다. NLU 태거는 이 사전에 대한 분류만 수행하므로, 사전 관리 API가 곧 태깅 품질 관리 도구다.

**facet — 태그의 분류 축**

각 태그는 `facet`을 하나 갖는다: `craft` / `media` / `life` / `neutral` (기본값 `neutral`).

| facet | 의미 | 시드 배정 |
|---|---|---|
| `craft` | 내가 만드는 것에 직접 쓰는 레퍼런스 | 18개 (`dev`, `golang`, `ios`, `ai`, `design` 등) |
| `media` | 형식 자체가 정보인 태그 — 다시 열 때의 시간 비용을 알려준다 | 5개 (`article`, `video`, `tutorial`, `book`, `podcast`) |
| `life` | 세상과 일상과 나 자신 | 7개 (`news`, `science`, `finance`, `career`, `productivity`, `travel`, `life`) |
| `neutral` | 아직 분류되지 않음 | 시드 30개에는 없음 — 새로 만든 태그가 여기서 태어난다 |

**서버는 "어느 facet인가"(데이터)만 소유하고, "그 facet이 무슨 색인가"(표현)는 각 클라이언트가 소유한다.** 계약에 색 값(hex)을 넣지 않는 이유는 색이 라이트/다크 2벌인데 계약은 1벌만 줄 수 있고, 그렇게 하면 토큰 체계를 서버가 아는 역전이 생기기 때문이다. 웹과 iOS는 같은 원본(`Tag.facet`)에서 각자의 플랫폼 토큰으로 매핑한다.

`facet`은 **`Tag`에만 있고 `LinkTag`(링크에 부착된 태그)에는 없다.** 목록 화면은 필터 바를 위해 이미 `GET /api/v1/tags` 전량을 들고 있으므로 `Map<tagId, facet>`으로 해석하면 되고, 링크 10만 건 목표에서 `LinkTag`마다 facet 문자열을 실으면 페이로드가 링크당 태그 수만큼 늘어난다. 캐시에 없는 태그는 `neutral`로 렌더하는 것이 정확한 폴백이다.

### 5.1 태그 사전 조회

```
GET /api/v1/tags
```

**Response** (200 OK):
```json
[
  {"id": 3, "name": "dev", "aliases": ["개발", "프로그래밍", "coding"], "link_count": 42, "facet": "craft"},
  {"id": 7, "name": "video", "aliases": ["영상"], "link_count": 28, "facet": "media"}
]
```

`link_count`는 해당 태그가 붙은 (삭제되지 않은) 링크 수. `facet`은 required — 클라이언트는 이 응답을 태그 색 해석의 유일한 원본으로 삼는다.

### 5.2 태그 생성

```
POST /api/v1/tags
```

**Request Body**:
```json
{
  "name": "ml",
  "aliases": ["머신러닝", "machine learning"],
  "facet": "craft"
}
```

`facet`은 optional이며 생략하면 `neutral`로 저장된다.

**Response** (201 Created):
```json
{
  "id": 15,
  "name": "ml",
  "aliases": ["머신러닝", "machine learning"],
  "link_count": 0,
  "facet": "craft"
}
```

**상태 코드**: 201 / 400(이름 중복 — `name`은 대소문자 무시 UNIQUE, 또는 enum 밖 `facet`)

### 5.3 태그 수정

```
PATCH /api/v1/tags/{id}
```

**Request Body**:
```json
{
  "name": "ml",
  "aliases": ["머신러닝", "machine learning", "딥러닝"],
  "facet": "craft"
}
```

`name`, `aliases`, `facet` 모두 optional이고 전달한 필드만 교체된다. `aliases`는 동의어·영문/한글 표기를 담는 배열로, 규칙 기반 태거의 매칭 대상이다 — alias를 잘 채우는 것이 태깅 정확도를 올리는 가장 싼 방법이다. `facet`을 바꾸면 모든 클라이언트에서 그 태그의 칩 색이 함께 바뀐다.

**Response** (200 OK): 수정된 태그(5.2 응답과 동일 형태).

**상태 코드**: 200 / 400(이름 중복, enum 밖 `facet`) / 404

### 5.4 태그 삭제

```
DELETE /api/v1/tags/{id}
```

사전에서 제거한다. `link_tags`의 연결은 FK CASCADE로 함께 삭제된다.

**Response**: 204 No Content

**상태 코드**: 204 / 400(`invalid_input` — 정수가 아닌 id) / 404

## 6. 검색 (Search)

### GET /api/v1/search

```
GET /api/v1/search?q=&tag=&from=&to=&cursor=&limit=
```

**Query Parameters**:
- `q` (string, 필수): 검색어. 앞뒤 공백 제거 후 빈 문자열이면 400 `invalid_input`. 길이에 따라 검색 경로가 갈린다.
  - **3자 이상**: FTS5 trigram `MATCH` + `bm25` 랭킹. 한국어도 형태소 분석 없이 부분 문자열 매칭이 된다. 응답 `"mode": "fts"`.
  - **3자 미만**: trigram 특성상 FTS5 매칭이 불가능하지만 400으로 거부하지 않고 **LIKE 폴백**으로 동작한다 — `links` 테이블의 title/note/description을 `LIKE`로 스캔(검색어의 `%`·`_`는 ESCAPE 처리)하고 `created_at DESC`로 정렬한다. 응답 `"mode": "like"`, `rank`는 `null`. 실측 10만 행 풀스캔 37ms로 예산 내다.
- `tag` (string, optional): 태그 이름 필터
- `from`, `to` (int, optional): `created_at` 범위 (unix epoch 초)
- `cursor`, `limit`: 2절의 페이지네이션 규약

3자 이상일 때 검색 대상은 `links_fts`(title, description, note, tags)이며 FTS5 `MATCH` + `bm25` 랭킹으로 정렬한다. 3자 미만이면 `links` 테이블을 직접 읽는다.

**Response** (200 OK): 목록(4.2)과 동일한 항목 형태에 `rank`가 추가되고, 최상위에 어느 검색 경로를 탔는지 나타내는 `mode` 필드(`"fts"` | `"like"`)가 붙는다.
```json
{
  "mode": "fts",
  "links": [
    {
      "id": 1234,
      "url": "https://www.youtube.com/watch?v=dQw4w9WgXcQ",
      "domain": "youtube.com",
      "title": "Go 동시성 패턴 완전 정복",
      "description": "goroutine과 채널을 이용한 워커 풀 구성부터 singleflight까지…",
      "content_type": "video",
      "thumb_url": "/thumbs/a3/a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8.jpg",
      "status": "done",
      "tags": [
        {"id": 3, "name": "dev", "source": "rules", "confidence": 0.82}
      ],
      "note": "주말에 볼 것",
      "created_at": 1752980000,
      "rank": -7.42
    }
  ],
  "next_cursor": null
}
```

`rank`는 bm25 점수로, 값이 작을수록 관련도가 높다. `"mode": "like"` 응답에서는 모든 항목의 `rank`가 `null`이고 정렬은 `created_at DESC`다. 성능 목표: 1만 링크 기준 < 30ms (FTS5 경로).

FTS 모드의 커서는 bm25 rank 기반 keyset이므로, 페이지 사이에 쓰기(저장·삭제·재색인)가 발생하면 FTS 페이지 경계가 이동할 수 있다 (단일 사용자 규모에서 허용).

## 7. 통계 (Stats)

### GET /api/v1/stats

**구현 완료** — iOS 통계 탭과 웹 설정의 리듬 섹션이 한 번의 호출로 쓴다 (위젯 활용은 M6).

**Response** (200 OK):
```json
{
  "total_links": 1560,
  "links_this_week": 12,
  "by_tag": [
    {"name": "dev", "count": 420},
    {"name": "video", "count": 280}
  ],
  "by_day": [
    {"date": "2026-06-28", "count": 0},
    {"date": "2026-06-29", "count": 3},
    "...",
    {"date": "2026-07-27", "count": 5}
  ]
}
```

**`by_day`는 세 가지를 보장한다** — 항상 **정확히 30개**(저장이 없는 날도 `count: 0`),
`date` 오름차순, 그리고 **마지막 원소가 서버 로컬타임 기준 오늘**.

세 번째가 핵심이다. 이게 없으면 클라이언트가 "오늘"을 자기 타임존으로 만들어 날짜 문자열과
맞춰 봐야 하는데, 날짜는 서버 로컬타임에서 만들어진다. 보장이 있으면 **뒤에서부터 세면 되고**
날짜 연산이 아예 필요 없다 — 연속 저장 계산이 세 클라이언트(iOS·웹·`scripts/streak.sh`)에서
같은 답을 내는 근거다.

> **2026-07-28 이전에는 `GROUP BY` 결과를 그대로 줬다.** 저장이 없는 날은 행이 아예 없었고,
> 그 배열을 위치로 인덱싱한 클라이언트가 조용히 틀렸다 — 한 달에 다섯 번 저장한 사람의 막대
> 다섯 개가 한쪽 끝에 **붙어서** "최근에 몰아서 저장함"으로 보였다. 웹은 오른쪽, iOS는
> 왼쪽으로 몰아서, **같은 응답을 두 화면이 반대로 그렸다.** 채우는 쪽을 서버로 정한 것은
> 소비자가 셋이기 때문이다 — 같은 코드를 세 언어로 세 번 짜는 것을 피한다.

`links_this_week`은 **그 창의 마지막 7칸 합**이다. 즉 달력상의 주가 아니라 오늘로 끝나는
롤링 7일이고, 그래서 **화면은 이 값을 "이번 주"라고 부르지 않는다** — 일요일이 아닌 모든
날에 그 라벨이 사실과 어긋난다(14 §1.3). 필드 이름은 계약 호환을 위해 그대로 둔다.
예전에는 `unixepoch() - 7*86400`이라 롤링 **초** 단위였고, 그때는 창과 기준이 아예 달랐다.

**상태 코드**: 200 — 404는 없다. 집계 전용 엔드포인트라 "없음" 상태가 존재하지 않고, 빈 DB에서도 `total_links: 0` + `by_day` 30칸(전부 0)으로 200을 낸다.

## 7.1 스프레드시트 (Sheets)

### GET /api/v1/sheets

연결 여부와 마지막 동기화 결과(`connected`, `sheet_url`, `last_sync_at`, `last_rows`, `last_error`).

### POST /api/v1/sheets/sync

아카이브 전량을 시트에 다시 쓴다. 동기 호출이며 링크 수에 비례해 몇 초 걸릴 수 있다 —
저장 API가 아니므로 p99 게이트 대상이 아니다.

**연결은 이 API가 하지 않는다.** 구글 승인을 브라우저에서 밟는 단계가 있어 서버가 대신할 수
없고, `pushpoint sheets-setup`이 그 안내를 맡는다. 연결돼 있지 않으면 409다.

동기화가 실패해도 **200으로 상태를 돌려준다**(`last_error`에 사유). 500으로 던지고 사유를
삼키면 화면이 무엇을 고쳐야 할지 보여줄 수 없다. 자세한 배경은 [07-DEPLOYMENT.md](07-DEPLOYMENT.md) §7.1.

---

## 8. 썸네일 정적 서빙

### GET /thumbs/{dir}/{file}

`data/thumbs/` 아래의 썸네일 JPEG를 그대로 서빙한다. 경로 형식은 `{url_hash 앞 2자리}/{url_hash}.jpg` (dir = 앞 2자리, file = `{url_hash}.jpg`)이며, 목록/상세 응답의 `thumb_url`이 정확히 이 경로를 가리킨다.

```
GET /thumbs/a3/a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8a3f1b2c4d5e6f7a8.jpg
```

이 엔드포인트는 **인증이 면제된다** — Tailscale이 네트워크 경계를 이루고, iOS `AsyncImage`가 커스텀 헤더를 지원하지 않기 때문이다. 썸네일은 단일 사이즈(최대 폭 640px, JPEG q80) 하나뿐이므로 v1처럼 사이즈 변형을 경로로 고르는 개념이 없다.

**상태 코드**: 200(`image/jpeg`) / 404 / 500(파일은 있으나 열기·`stat`에 실패 — 인증만 면제일 뿐 500 면제는 아니다)

## 9. 프로파일링

### GET /debug/pprof/*

Go 표준 `net/http/pprof`가 기본 탑재되어 있다. 성능 목표(저장 p99 < 50ms 등)를 검증하거나 병목을 추적할 때 사용한다.

```bash
go tool pprof http://localhost:8420/debug/pprof/profile?seconds=10
```

로컬 단일 사용자 서버이므로 프로덕션 노출 걱정 없이 항상 켜 둔다.

## 10. v1에서 삭제된 API

아래 API들은 v2에 존재하지 않는다. 클라이언트 코드나 문서에서 참조가 남아 있다면 제거 대상이다.

| v1 API | 삭제 이유 |
|--------|----------|
| `POST /api/v1/auth/register` | 단일 사용자 앱 — 회원가입 자체가 비목표 |
| `POST /api/v1/auth/login` | JWT 발급 불필요, 정적 API 키 1개로 대체 |
| `POST /api/v1/auth/refresh` | 만료되는 토큰이 없으므로 갱신도 없음 |
| `POST /api/v1/auth/logout` | 세션 상태가 서버에 없으므로 로그아웃 개념 없음 |
| `GET /api/v1/sync/pull` | 서버가 단일 진실 원천 — 클라이언트는 항상 API를 직접 조회 |
| `POST /api/v1/sync/push` | 기기 간 충돌 해소가 필요한 멀티 디바이스 동기화는 비목표 |
| WebSocket (`/ws`) | 저장 API가 50ms에 응답하고 처리 상태는 폴링으로 충분 — 상시 연결 유지 비용이 이득보다 큼 |
| Rate Limiting (429 + `X-RateLimit-*` 헤더) | 사용자가 본인 1명 — 자기 자신을 스로틀링할 이유 없음 |
| Pre-signed URL (MinIO) | 오브젝트 스토리지 제거, 로컬 디스크에서 `/thumbs/` 직접 서빙 |
