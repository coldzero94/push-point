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
	"strings"
	"time"

	"github.com/coby/push-point/backend/internal/sheets"

	"github.com/coby/push-point/backend/internal/api/gen"
	"github.com/coby/push-point/backend/internal/sheetsync"
)

// GetSheetsStatus — 연결 여부와 마지막 동기화 결과.
func (s *Server) GetSheetsStatus(_ context.Context, _ gen.GetSheetsStatusRequestObject) (gen.GetSheetsStatusResponseObject, error) {
	return gen.GetSheetsStatus200JSONResponse(statusOf(sheetsync.Load(s.dataDir))), nil
}

// GetSheetsScript — 화면이 보여 줄 Apps Script와 그 토큰.
//
// **토큰을 부를 때마다 새로 만들지 않는다.** 사용자가 스크립트를 붙여넣는 도중에 화면을
// 새로 고치면 붙여넣은 스크립트와 서버가 아는 토큰이 갈라지고, 그러면 배포는 성공하는데
// ping만 실패한다 — 원인이 화면 어디에도 안 보이는 종류의 실패다. 이미 연결돼 있으면
// 그 토큰을 그대로 준다(재배포용).
func (s *Server) GetSheetsScript(_ context.Context, _ gen.GetSheetsScriptRequestObject) (gen.GetSheetsScriptResponseObject, error) {
	token := sheetsync.Load(s.dataDir).Token
	if token == "" {
		var err error
		if token, err = sheets.NewToken(); err != nil {
			return nil, err
		}
		// 아직 연결 전이라도 토큰은 남겨 둔다 — 위 문단의 이유.
		st := sheetsync.Load(s.dataDir)
		st.Token = token
		if err := sheetsync.Save(s.dataDir, st); err != nil {
			return nil, err
		}
	}
	return gen.GetSheetsScript200JSONResponse{Script: sheets.AppsScript(token), Token: token}, nil
}

// ConnectSheets — 배포 URL을 받아 **찔러 보고** 성공했을 때만 저장한다.
//
// 저장부터 하면 화면은 "연결됨"인데 동기화만 조용히 안 되는 상태가 된다. 그건 연결이 안
// 된 것보다 나쁘다 — 사용자는 어디를 고쳐야 할지 모른 채 버튼만 누르게 된다.
func (s *Server) ConnectSheets(ctx context.Context, req gen.ConnectSheetsRequestObject) (gen.ConnectSheetsResponseObject, error) {
	if req.Body == nil || strings.TrimSpace(req.Body.DeployUrl) == "" {
		return gen.ConnectSheets400JSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "배포 URL이 비어 있습니다")), nil
	}
	st := sheetsync.Load(s.dataDir)
	token := st.Token
	if token == "" {
		return gen.ConnectSheets400JSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput,
			"먼저 스크립트를 받아 배포하세요 (GET /api/v1/sheets/script)")), nil
	}

	wh, err := sheets.NewWebhook(strings.TrimSpace(req.Body.DeployUrl), token)
	if err != nil {
		return gen.ConnectSheets400JSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, err.Error())), nil
	}
	pingCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	title, sheetURL, err := wh.Ping(pingCtx)
	if err != nil {
		// **사유만 돌려주고 조언은 붙이지 않는다.** 가장 흔한 실패는 배포의 액세스 권한이
		// "모든 사용자"가 아닌 경우인데, 그 안내를 여기서 한국어로 붙이면 영어 화면에
		// 한국어 문장이 섞인다(실제로 그렇게 나왔다). 화면이 자기 언어로 덧붙인다.
		return gen.ConnectSheets400JSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, err.Error())), nil
	}

	st.Mode = "webhook"
	st.DeployURL = strings.TrimSpace(req.Body.DeployUrl)
	st.SheetURL = sheetURL
	if st.CreatedAt == 0 {
		st.CreatedAt = time.Now().Unix()
	}
	if err := sheetsync.Save(s.dataDir, st); err != nil {
		return nil, err
	}
	s.logger.Info("시트 연결됨", "title", title, "url", sheetURL)
	return gen.ConnectSheets200JSONResponse(statusOf(st)), nil
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
