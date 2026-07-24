package tagger

import (
	_ "embed"
	"encoding/json"
	"strings"
	"sync"
)

// domains.json은 nlu/dictionary/domains.json의 바이트 동일 사본이다 — nlu/는 backend Go
// 모듈 밖이라 cross-module go:embed가 불가능하기 때문. lint_dict.sh의 diff가 둘의 드리프트를
// 막는다(CI). 값은 tags.json에 있는 태그명만 참조한다(dict-lint 보장).
//
//go:embed domains.json
var domainsJSON []byte

var (
	domainOnce sync.Once
	domainMap  map[string][]string
)

func loadDomainMap() {
	// 임베드된 커밋 자산이라 파싱 실패는 빌드/테스트에서 잡혀야 할 프로그래머 오류 → panic.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(domainsJSON, &raw); err != nil {
		panic("tagger: domains.json 파싱 실패: " + err.Error())
	}
	domainMap = make(map[string][]string, len(raw))
	for host, v := range raw {
		if strings.HasPrefix(host, "_") { // _comment 등 메타 키 스킵
			continue
		}
		var tags []string
		if err := json.Unmarshal(v, &tags); err != nil {
			panic("tagger: domains.json 값 파싱 실패 (" + host + "): " + err.Error())
		}
		domainMap[host] = tags
	}
}

// DomainTags는 host(예: "www.youtube.com")에 매핑된 사전 태그 이름들을 돌려준다.
// www. 제거 후 전체 호스트 정확 매칭 → 미스면 최좌측 라벨을 순차 제거하며 폴백(등록 도메인
// 탐색). 명시 등록된 서브도메인(blog.naver.com)은 폴백 전 정확 매칭으로 잡힌다. 미등록이면 nil.
func DomainTags(host string) []string {
	domainOnce.Do(loadDomainMap)
	host = strings.TrimPrefix(strings.ToLower(host), "www.")
	for {
		if t, ok := domainMap[host]; ok {
			return t
		}
		// 최좌측 라벨 제거. 점이 1개(등록 도메인 example.com)까지만 폴백 — 그 이하는 TLD.
		i := strings.IndexByte(host, '.')
		if i < 0 || strings.Count(host, ".") <= 1 {
			return nil
		}
		host = host[i+1:]
	}
}
