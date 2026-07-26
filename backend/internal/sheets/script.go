package sheets

import "strings"

// AppsScript는 사용자가 자기 시트에 붙여넣을 스크립트를 만든다.
//
// **이 50줄이 권한의 전부다.** 사용자가 읽고 판단할 수 있는 분량으로 유지한다 —
// "이 코드가 내 계정에서 뭘 하는가"에 답할 수 없는 길이가 되면, 서비스 계정 키를
// 건네는 것과 신뢰 면에서 다를 게 없어진다.
//
// 스크립트는 **자기가 붙어 있는 시트만** 만진다(`getActiveSpreadsheet`). 드라이브의
// 다른 파일에 손댈 방법이 코드에 없다.
func AppsScript(token string) string {
	return strings.ReplaceAll(scriptTemplate, "__TOKEN__", token)
}

// scriptTemplate의 설계 메모:
//
//   - doPost만 있고 doGet이 없다. GET으로 열면 아무것도 안 하므로, 배포 URL이 어딘가
//     새어 브라우저로 열려도 데이터가 노출되지 않는다.
//   - 토큰을 먼저 본다. "모든 사용자" 배포라 URL을 아는 누구나 POST할 수 있고,
//     토큰이 유일한 관문이다.
//   - replace는 **비우고 쓴다.** 태그 수정·삭제가 시트에 반영돼야 하는데 덧붙이기만
//     하면 시트가 원본과 조용히 갈라진다.
//   - 탭이 없으면 만든다. 첫 동기화 전에는 없는 것이 정상이다.
const scriptTemplate = `// Push-Point → 이 시트로 저장 기록을 보내는 스크립트입니다.
// 하는 일은 아래가 전부입니다: 토큰이 맞으면, 이 시트의 탭 하나를 읽거나 통째로 다시 씁니다.
// 이 시트 밖의 어떤 파일도 건드리지 않습니다.

const TOKEN = '__TOKEN__';  // Push-Point가 만든 값입니다. 바꾸면 연결이 끊깁니다.

function doPost(e) {
  try {
    const body = JSON.parse(e.postData.contents);
    if (body.token !== TOKEN) return reply({ ok: false, error: 'token mismatch' });

    const ss = SpreadsheetApp.getActiveSpreadsheet();

    if (body.action === 'ping') {
      return reply({ ok: true, title: ss.getName(), url: ss.getUrl() });
    }

    const tab = body.tab || 'links';
    let sheet = ss.getSheetByName(tab);

    if (body.action === 'read') {
      if (!sheet) return reply({ ok: true, values: [] });
      return reply({ ok: true, values: sheet.getDataRange().getDisplayValues() });
    }

    if (body.action === 'replace') {
      if (!sheet) sheet = ss.insertSheet(tab);
      const values = body.values || [];
      const width = body.width || (values[0] ? values[0].length : 0);
      // 행이 모자라면 먼저 늘립니다. 새 시트는 1000행이라 링크가 그만큼 쌓이면
      // 쓰기가 실패하는데, 비우기가 먼저 실행되므로 그때부터는 매 동기화가
      // 시트를 비우고 죽어 저절로 낫지 않습니다. 그래서 **늘리기가 먼저**입니다.
      if (values.length > sheet.getMaxRows()) {
        sheet.insertRowsAfter(sheet.getMaxRows(), values.length - sheet.getMaxRows());
      }
      // 우리 열(A..width)만 비웁니다. 그 오른쪽에 적어 두신 것은 건드리지 않습니다.
      if (width > 0 && sheet.getLastRow() > 0) {
        sheet.getRange(1, 1, sheet.getMaxRows(), width).clearContent();
      }
      if (values.length > 0) {
        sheet.getRange(1, 1, values.length, values[0].length).setValues(values);
        sheet.setFrozenRows(1);
      }
      return reply({ ok: true, rows: values.length });
    }

    return reply({ ok: false, error: 'unknown action: ' + body.action });
  } catch (err) {
    return reply({ ok: false, error: String(err) });
  }
}

function reply(obj) {
  return ContentService.createTextOutput(JSON.stringify(obj))
    .setMimeType(ContentService.MimeType.JSON);
}
`
