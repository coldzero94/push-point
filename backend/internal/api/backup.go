package api

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/coby/push-point/backend/internal/api/gen"
	"github.com/coby/push-point/backend/internal/store"
)

// maxRestoreBytes는 복원으로 받을 수 있는 파일의 상한.
//
// 개인 아카이브 하나가 이보다 클 일은 사실상 없다(10만 건 벤치 DB가 수백 MB대다). 상한이
// 있어야 하는 이유는 크기가 아니라 **받는 쪽이 디스크를 다 쓰기 전에 멈출 수 있어야** 하기
// 때문이다 — 폰에서 디스크가 차면 복원이 아니라 앱 전체가 못 쓰게 된다.
const maxRestoreBytes = 2 << 30 // 2GiB

// DownloadBackup — 아카이브 전체를 파일 하나로 내려준다.
//
// **임시 파일을 거친다.** `VACUUM INTO`는 파일에만 쓸 수 있고, 그 편이 낫기도 하다 —
// 응답을 열어 둔 채 DB를 훑으면 느린 클라이언트가 읽는 동안 읽기 트랜잭션이 살아 있게 된다.
// 파일로 먼저 받아 두면 그 시간이 사람의 다운로드 속도와 분리된다.
func (s *Server) DownloadBackup(ctx context.Context, _ gen.DownloadBackupRequestObject) (gen.DownloadBackupResponseObject, error) {
	tmp, err := os.CreateTemp("", "pushpoint-backup-*.db")
	if err != nil {
		return nil, fmt.Errorf("백업 임시 파일 생성 실패: %w", err)
	}
	path := tmp.Name()
	// VACUUM INTO는 대상이 이미 있으면 실패하므로 자리만 잡고 즉시 비운다.
	tmp.Close()
	os.Remove(path)

	if err := s.store.Backup(ctx, path); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("백업 파일 열기 실패: %w", err)
	}
	st, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("백업 파일 크기 확인 실패: %w", err)
	}
	// 임시 파일은 응답을 다 보낸 뒤에 지워야 한다. 열어 둔 채로 지우면 유닉스에서는 핸들이
	// 살아 있으므로 지금 지워도 전송은 끝까지 간다 — 응답 후 훅이 없는 구조에서 새는 것을
	// 막는 가장 단순한 방법이다.
	os.Remove(path)

	return gen.DownloadBackup200ApplicationoctetStreamResponse{
		Body:          readCloserOnce{f},
		ContentLength: st.Size(),
	}, nil
}

// readCloserOnce는 전송이 끝나면 파일을 닫는다. 생성 코드는 Body를 io.Reader로만 보므로
// Close를 부를 자리가 없다 — Read가 EOF에 닿는 순간이 유일하게 남은 자리다.
type readCloserOnce struct{ f *os.File }

func (r readCloserOnce) Read(p []byte) (int, error) {
	n, err := r.f.Read(p)
	if err != nil {
		r.f.Close()
	}
	return n, err
}

// RestoreBackup — 받은 파일을 검증하고 **다음 기동에 교체되도록** 대기시킨다.
//
// 200이 "되돌렸다"가 아니라 "다음에 열 때 되돌린다"인 이유는 계약 설명에 적혀 있다.
func (s *Server) RestoreBackup(_ context.Context, req gen.RestoreBackupRequestObject) (gen.RestoreBackupResponseObject, error) {
	if req.Body == nil {
		return gen.RestoreBackup400JSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, "복원 파일이 비어 있습니다")), nil
	}
	tmp, err := os.CreateTemp("", "pushpoint-restore-*.db")
	if err != nil {
		return nil, fmt.Errorf("복원 임시 파일 생성 실패: %w", err)
	}
	defer os.Remove(tmp.Name())

	// LimitReader가 상한을 건다. 상한에 정확히 닿았다면 잘렸다는 뜻이므로 받아들이지
	// 않는다 — 잘린 SQLite는 열리기도 해서, 크기로 거르지 않으면 반쪽 아카이브가 통과한다.
	n, err := io.Copy(tmp, io.LimitReader(req.Body, maxRestoreBytes+1))
	tmp.Close()
	if err != nil {
		return nil, fmt.Errorf("복원 파일 수신 실패: %w", err)
	}
	if n > maxRestoreBytes {
		return gen.RestoreBackup400JSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput,
			"복원 파일이 너무 큽니다")), nil
	}

	if err := store.StageRestore(s.dataDir, tmp.Name()); err != nil {
		// 검증 실패는 서버 오류가 아니라 사용자가 고를 수 있는 상태다 — 남의 SQLite를
		// 고른 경우가 대부분이고, 그 사실을 그대로 말해 준다.
		return gen.RestoreBackup400JSONResponse(apiErr(gen.ErrorErrorCodeInvalidInput, err.Error())), nil
	}
	s.logger.Info("복원 대기됨 — 다음 기동에 교체", "bytes", n,
		"staged", filepath.Join(s.dataDir, "pushpoint.db.restore"))
	return gen.RestoreBackup200JSONResponse{RestartRequired: true}, nil
}

// MarkRecognitionTapped — 알아봄 알림을 눌렀다는 사실을 원장에 남긴다.
//
// **표시할 이벤트가 없어도 204다.** 이미 눌렀거나, 알아봄 없이 그냥 열린 링크이거나 —
// 둘 다 정상이다. 여기서 404를 주면 앱이 알림을 누를 때마다 오류를 처리해야 하는데,
// 그 오류로 할 수 있는 일이 없다.
func (s *Server) MarkRecognitionTapped(ctx context.Context, req gen.MarkRecognitionTappedRequestObject) (gen.MarkRecognitionTappedResponseObject, error) {
	if _, err := s.store.GetLink(ctx, req.Id); err != nil {
		return gen.MarkRecognitionTapped404JSONResponse(apiErr(gen.ErrorErrorCodeNotFound, "링크가 없습니다")), nil
	}
	if err := s.store.MarkRecognitionTapped(ctx, req.Id); err != nil {
		return nil, err
	}
	return gen.MarkRecognitionTapped204Response{}, nil
}
