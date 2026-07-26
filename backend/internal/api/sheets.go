package api

// 스프레드시트 내보내기 핸들러.
//
// **연결은 여기서 하지 않는다.** 연결에는 사용자가 브라우저에서 구글 승인을 밟는 단계가
// 있어 서버가 대신할 수 없다 — `pushpoint sheets-setup`이 그 안내를 맡는다. 이 API가 하는
// 일은 **상태를 보여주고 지금 한 번 돌리는 것**뿐이다.
//
// 그래도 이 두 개가 필요한 이유: 연결해 두고 나면 그다음부터는 터미널을 열 이유가 없어야
// 한다. 화면에 버튼이 없으면 "저장한 걸 시트에서 본다"는 습관이 터미널 습관에 묶인다.

import (
	"context"
	"time"

	"github.com/coby/push-point/backend/internal/api/gen"
	"github.com/coby/push-point/backend/internal/sheetsync"
)

// GetSheetsStatus — 연결 여부와 마지막 동기화 결과.
func (s *Server) GetSheetsStatus(_ context.Context, _ gen.GetSheetsStatusRequestObject) (gen.GetSheetsStatusResponseObject, error) {
	return gen.GetSheetsStatus200JSONResponse(statusOf(sheetsync.Load(s.dataDir))), nil
}

// SyncSheets — 지금 한 번 동기화한다.
//
// 동기 호출이다. 링크 수에 비례해 몇 초 걸릴 수 있지만 저장 API가 아니므로 p99 게이트
// 대상이 아니고, 사용자가 버튼을 누르고 기다리는 자리라 비동기로 만들면 "됐는지"를
// 알려줄 방법을 따로 만들어야 한다.
func (s *Server) SyncSheets(ctx context.Context, _ gen.SyncSheetsRequestObject) (gen.SyncSheetsResponseObject, error) {
	tr, st, err := sheetsync.Open(s.dataDir)
	if err != nil {
		// 연결 안 됨은 서버 오류가 아니라 상태다 — 409로 돌려 화면이 안내를 띄우게 한다.
		return gen.SyncSheets409JSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, err.Error())), nil
	}

	n, syncErr := sheetsync.Run(ctx, tr, s.dataDir, sheetsync.DefaultTab)
	st.LastSyncAt = time.Now().Unix()
	st.LastRows = n
	st.LastError = ""
	if syncErr != nil {
		st.LastError = syncErr.Error()
		s.logger.Error("시트 동기화 실패", "err", syncErr)
	}
	if err := sheetsync.Save(s.dataDir, st); err != nil {
		s.logger.Error("시트 상태 저장 실패", "err", err)
	}
	// 실패해도 200으로 상태를 돌려준다 — 화면이 last_error를 그대로 보여주는 편이
	// 500을 던지고 사유를 삼키는 것보다 낫다. 사용자는 무엇이 잘못됐는지 알아야 고친다.
	return gen.SyncSheets200JSONResponse(statusOf(st)), nil
}

func statusOf(st sheetsync.State) gen.SheetsStatus {
	out := gen.SheetsStatus{
		Connected: sheetsync.Connected(st),
		LastRows:  &st.LastRows,
	}
	if st.SheetURL != "" {
		out.SheetUrl = &st.SheetURL
	}
	if st.LastSyncAt > 0 {
		at := int(st.LastSyncAt)
		out.LastSyncAt = &at
	}
	if st.LastError != "" {
		out.LastError = &st.LastError
	}
	return out
}
