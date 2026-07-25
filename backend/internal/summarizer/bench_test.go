package summarizer

import (
	"strings"
	"testing"
)

// benchBody는 실측 최대치(32KB, body_text 캡)에 가까운 한·영 혼합 본문을 만든다.
// 요약은 비동기 tag 잡에서 도므로 save API p99 게이트와는 무관하지만, 대량 임포트 시
// tagConcurrency=4 세마포어 안에서 CPU를 얼마나 쓰는지는 알고 있어야 한다.
func benchBody() string {
	sents := []string{
		"쿠버네티스는 컨테이너화된 애플리케이션을 선언적으로 배포하고 운영하는 오케스트레이터다.",
		"파드는 배포의 최소 단위이며 하나 이상의 컨테이너를 함께 스케줄링한다.",
		"서비스는 파드 집합에 안정적인 네트워크 주소를 부여해 트래픽을 분산한다.",
		"수평 파드 오토스케일러는 관측된 지표를 기준으로 레플리카 수를 조정한다.",
		"A neural network is a function that maps inputs to outputs through layers of weights.",
		"Training adjusts those weights by gradient descent on a loss surface.",
		"The backpropagation algorithm computes gradients efficiently by the chain rule.",
		"Regularization techniques such as dropout reduce overfitting on small datasets.",
	}
	var b strings.Builder
	for b.Len() < 32<<10 {
		for _, s := range sents {
			b.WriteString(s)
			b.WriteByte(' ')
		}
	}
	return b.String()
}

func BenchmarkSummarize(b *testing.B) {
	body := benchBody()
	desc := "쿠버네티스와 신경망을 함께 다루는 긴 글이다."
	b.ReportAllocs()
	for b.Loop() {
		_ = Summarize(body, desc)
	}
}

func BenchmarkSplit(b *testing.B) {
	body := benchBody()
	b.ReportAllocs()
	for b.Loop() {
		_ = Split(body)
	}
}
