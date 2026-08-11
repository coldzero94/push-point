package main

// recognition — 알아봄 원장을 읽는다 (`just recognition`).
//
// **이 명령이 마이그레이션과 같은 변경에서 들어오는 이유.** 이 저장소에는 이미 아무도
// 안 읽는 원장이 있다 — `tag_feedback`은 1년 가까이 INSERT만 됐고 테스트 밖 SELECT가
// 0이었다. 쌓기만 하는 원장은 자산이 아니라 부채다. 그래서 `recognition_events`는
// 읽을 수단 없이는 태어나지 않는다.
//
// 답하는 질문은 하나다: **알아봄이 값을 하는가.** 노출과 탭을 단별로 세고, 노출이
// 없으면 없다고 말한다(0을 그럴듯하게 그리지 않는다).

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/coby/push-point/backend/internal/store"
)

func runRecognition(args []string) error {
	fs := flag.NewFlagSet("recognition", flag.ExitOnError)
	days := fs.Int("days", 30, "최근 며칠")
	if err := fs.Parse(args); err != nil {
		return err
	}

	db, err := store.Open(dataDir())
	if err != nil {
		return err
	}
	defer db.Close()
	st := store.New(db, nil)

	stats, err := st.RecognitionStats(context.Background(), *days)
	if err != nil {
		return err
	}
	if len(stats) == 0 {
		fmt.Printf("최근 %d일: 알아봄이 한 번도 안 떴다.\n", *days)
		// **오류가 아니다.** 아직 안 뜬 것과 떴는데 무시당한 것은 다른 사실이고,
		// 전자를 실패로 만들면 아무도 이 명령을 안 돌리게 된다.
		return nil
	}

	names := map[int]string{store.RungDuplicate: "중복(그때의 메모)", store.RungDomain: "도메인 재조우"}
	fmt.Printf("최근 %d일\n\n%-22s %8s %8s %8s\n", *days, "단", "노출", "탭", "탭률")
	totalShown, totalTapped := 0, 0
	for _, s := range stats {
		name := names[s.Rung]
		if name == "" {
			name = fmt.Sprintf("rung %d", s.Rung)
		}
		fmt.Printf("%-22s %8d %8d %7.0f%%\n", name, s.Shown, s.Tapped, rate(s.Tapped, s.Shown))
		totalShown += s.Shown
		totalTapped += s.Tapped
	}
	fmt.Printf("%-22s %8d %8d %7.0f%%\n", "합계", totalShown, totalTapped, rate(totalTapped, totalShown))
	fmt.Fprintln(os.Stdout, "\n탭률은 **판정이 아니라 관측**이다 — 낮다고 기능이 나쁜 것이 아니라,\n낮은 채로 넉 주가 지나면 그때 지울 근거가 된다.")
	return nil
}

func rate(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a) / float64(b) * 100
}
