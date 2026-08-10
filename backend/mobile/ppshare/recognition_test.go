package ppshare

import (
	"encoding/json"
	"testing"
)

// 중복 저장이 **원본의 시각과 메모**를 돌려주는가.
//
// 0단의 전부가 이 반환값이다. 배너는 이 값을 문장으로 만들 뿐이라(Swift의
// RecognitionLineTests가 그쪽을 본다), 여기서 값이 안 오면 화면에서 할 수 있는 일이 없다.
//
// **공유 확장 경로로 잰다.** HTTP 저장 응답에는 이 필드들이 없고, 중복 저장이 실제로
// 일어나는 자리가 여기다 — 같은 링크를 두 번 공유하는 것은 흔한 일이고 앱을 여는 것보다 흔하다.
func TestSave_DuplicateReturnsPriorNoteAndDate(t *testing.T) {
	dir := t.TempDir()
	if err := Open(dir); err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = Close() }()

	first, err := Save(`{"url":"https://go.dev/blog/generics-next-step","note":"실무 적용 전에 다시 읽기","title":"Generics"}`)
	if err != nil {
		t.Fatalf("첫 저장: %v", err)
	}
	var a struct {
		ID        int64  `json:"id"`
		CreatedAt int64  `json:"created_at"`
		Duplicate bool   `json:"duplicate"`
		PriorNote string `json:"prior_note"`
	}
	if err := json.Unmarshal([]byte(first), &a); err != nil {
		t.Fatal(err)
	}
	if a.Duplicate {
		t.Fatal("첫 저장이 중복으로 나왔다")
	}
	// **새 저장에는 과거를 싣지 않는다** — 돌려줄 과거가 없고, 저장 경로는 2초 예산 안이다.
	if a.PriorNote != "" {
		t.Errorf("새 저장인데 prior_note가 왔다: %q", a.PriorNote)
	}

	second, err := Save(`{"url":"https://go.dev/blog/generics-next-step","title":"Generics"}`)
	if err != nil {
		t.Fatalf("두 번째 저장: %v", err)
	}
	var b struct {
		ID        int64  `json:"id"`
		CreatedAt int64  `json:"created_at"`
		Duplicate bool   `json:"duplicate"`
		PriorNote string `json:"prior_note"`
	}
	if err := json.Unmarshal([]byte(second), &b); err != nil {
		t.Fatal(err)
	}
	if !b.Duplicate {
		t.Fatal("두 번째 저장이 중복으로 안 잡혔다")
	}
	if b.ID != a.ID {
		t.Errorf("중복인데 다른 id다: %d vs %d", b.ID, a.ID)
	}
	// **원본의 시각**이어야 한다. 새로 저장한 시각이면 배너가 "방금 저장했다"고 말하게 되고,
	// 그건 알아봄이 아니라 확인으로 되돌아가는 것이다.
	if b.CreatedAt != a.CreatedAt {
		t.Errorf("중복이 원본 시각을 안 돌려줬다: %d vs %d", b.CreatedAt, a.CreatedAt)
	}
	// **그때 쓴 메모.** 두 번째 저장은 메모를 안 보냈는데도 와야 한다 — 사람이 잊은 것이
	// 정확히 그것이기 때문이다.
	if b.PriorNote != "실무 적용 전에 다시 읽기" {
		t.Errorf("메모가 안 왔다: %q", b.PriorNote)
	}
}
