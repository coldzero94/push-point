package summarizer

import (
	"math"
	"sort"
)

// TextRank 파라미터 — 전부 원논문(Mihalcea & Tarau 2004) 값이다. 임의로 고른 손잡이가
// 아니므로 튜닝 대상이 아니다.
const (
	damping = 0.85 // 원논문 d
	epsilon = 1e-4 // 원논문 수렴 임계
	maxIter = 50   // 실측 20~30회 수렴의 안전 상한. 초과해도 에러가 아니라 그 시점 값을 쓴다
	// lambda는 MMR의 중심성:중복 가중치. 0.7은 중심성 우선이되 중복을 실질적으로 누른다.
	lambda = 0.7
	// mmrCandidates는 MMR이 검토할 중심성 상위 문장 수 — 하위 문장은 어차피 안 뽑힌다.
	mmrCandidates = 10
)

// similarity는 두 문장의 어휘 겹침 유사도(원논문 식): 공통 토큰 수를 로그 길이 합으로
// 정규화해 긴 문장이 유리해지는 편향을 없앤다. 토큰이 2개 미만인 문장은 유사도를 0으로
// 둔다 — 우연한 한 단어 겹침이 그래프를 오염시키기 때문.
func similarity(a, b []string) float64 {
	if len(a) < 2 || len(b) < 2 {
		return 0
	}
	inter := float64(intersectCount(a, b))
	if inter == 0 {
		return 0
	}
	// 분모 클램프: 짧은 문장 쌍에서 로그 합이 0에 가까워져 유사도가 폭발하는 것을 막는다.
	den := math.Max(math.Log(float64(len(a)))+math.Log(float64(len(b))), 0.5)
	return inter / den
}

// centrality는 문장 그래프에 TextRank 멱법을 돌려 [0,1]로 정규화된 중심성을 돌려준다.
// 고립 문장(다른 어떤 문장과도 안 겹침)은 갱신에서 빼고 (1-d)로 고정한다 — 0으로 나누는
// 대신 "최소 점수"를 주는 원논문 관례.
func centrality(tokens [][]string) []float64 {
	n := len(tokens)
	w := make([]float64, n*n) // 평면 행렬 — n<=200이라 40k float64(320KB)면 충분
	rowSum := make([]float64, n)
	for i := 0; i < n; i++ {
		for j := i + 1; j < n; j++ {
			s := similarity(tokens[i], tokens[j])
			if s == 0 {
				continue
			}
			w[i*n+j], w[j*n+i] = s, s
			rowSum[i] += s
			rowSum[j] += s
		}
	}

	score := make([]float64, n)
	next := make([]float64, n)
	for i := range score {
		score[i] = 1.0
	}
	for range maxIter {
		diff := 0.0
		for i := 0; i < n; i++ {
			if rowSum[i] == 0 {
				next[i] = 1 - damping // 고립 문장
				continue
			}
			sum := 0.0
			for j := 0; j < n; j++ {
				if j == i || w[i*n+j] == 0 || rowSum[j] == 0 {
					continue
				}
				sum += w[i*n+j] / rowSum[j] * score[j]
			}
			next[i] = (1 - damping) + damping*sum
			if d := math.Abs(next[i] - score[i]); d > diff {
				diff = d
			}
		}
		copy(score, next)
		if diff < epsilon {
			break
		}
	}

	max := 0.0
	for _, s := range score {
		if s > max {
			max = s
		}
	}
	if max > 0 {
		for i := range score {
			score[i] /= max
		}
	}
	return score
}

// selectMMR은 중심성이 높으면서 **이미 고른 문장·description과 덜 겹치는** 문장을 k개
// 고른다(Maximal Marginal Relevance). description 항이 이 설계의 핵심이다 — 요약이
// 인스펙터에서 바로 아래 description과 같은 말을 반복하지 않게 만든다.
//
// 반환은 문장 인덱스이며 **선택 순서**다(호출자가 문서 순서로 재정렬한다).
// 동점은 인덱스가 작은 쪽 — 맵 순회가 없고 정렬이 stable이라 출력이 결정적이다.
func selectMMR(tokens [][]string, cent []float64, descTok []string, k int) []int {
	n := len(tokens)
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	sort.SliceStable(idx, func(a, b int) bool { return cent[idx[a]] > cent[idx[b]] })
	if len(idx) > mmrCandidates {
		idx = idx[:mmrCandidates]
	}

	picked := make([]int, 0, k)
	used := make([]bool, n)
	for len(picked) < k && len(picked) < len(idx) {
		best, bestScore := -1, math.Inf(-1)
		for _, c := range idx {
			if used[c] {
				continue
			}
			// 중복 페널티 = (이미 고른 문장들과의 최대 겹침, description과의 겹침) 중 큰 값.
			redundancy := Containment(tokens[c], descTok)
			for _, p := range picked {
				if r := Containment(tokens[c], tokens[p]); r > redundancy {
					redundancy = r
				}
			}
			s := lambda*cent[c] - (1-lambda)*redundancy
			if s > bestScore {
				best, bestScore = c, s
			}
		}
		if best < 0 {
			break
		}
		used[best] = true
		picked = append(picked, best)
	}
	return picked
}
