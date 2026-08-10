// @vitest-environment node
//
// `extension/src/extract.js`의 블록 경계 규칙 테스트.
//
// **이 파일이 이 저장소에서 유일한 크로스플랫폼 로직인데 테스트가 0이었다.** 같은 파일이
// `just ios-bind`로 `ios/PushPointShare/extract.js`에 복사되어 사파리 공유 확장의 캡처가
// 되므로, 여기서 나는 결함은 브라우저 확장과 iOS 양쪽에 동시에 난다.
//
// **환경을 공유 설정에서 바꾸지 않는다.** `frontend/vitest.config.ts`는 `environment: 'node'`
// 이고 나머지 테스트는 순수 로직이라 DOM이 필요 없다. 전역으로 DOM을 켜면 그 테스트들이
// 필요 없는 비용을 물고, 브라우저 전역이 있다는 이유로 잘못된 코드가 통과할 여지가 생긴다.
// 그래서 이 파일만 linkedom으로 문서를 만들어 넘긴다.
//
// **import가 아니라 읽어서 평가한다.** extract.js는 일부러 모듈이 아니다 — `var
// ExtensionPreprocessingJS`를 할당하고 파일 끝에서 `captureOnce()`를 그냥 부른다(사파리
// 확장이 그렇게 요구한다). 그래서 `import`할 수 없고, 텍스트로 읽어 `capture`만 꺼내 쓴다.
import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { parseHTML } from 'linkedom'
import { describe, expect, it } from 'vitest'

const SRC = resolve(dirname(fileURLToPath(import.meta.url)), '../../../extension/src/extract.js')

/** extract.js에서 `capture(doc, url)`만 꺼낸다. 최상단의 `captureOnce()` 호출은 실행하지 않는다. */
function loadCapture(): (doc: Document, url: string) => { body_text: string; title: string } {
  const src = readFileSync(SRC, 'utf8')
  // 파일 끝의 부작용 호출을 잘라낸다. 남는 것은 순수한 함수 선언들이다.
  const body = src.replace(/^\s*captureOnce\(\);?\s*$/m, '')
  // eslint-disable-next-line no-new-func
  return new Function(`${body}\nreturn capture;`)() as never
}

function bodyOf(html: string): string {
  const { document } = parseHTML(`<html><body><article>${html}</article></body></html>`)
  return loadCapture()(document as unknown as Document, 'https://example.com/x').body_text
}

describe('extract.js 블록 경계', () => {
  // 이 규칙이 왜 있는지는 extract.js의 BLOCK_SELECTOR 주석에 적혀 있다 — 복제본의
  // innerText는 textContent처럼 동작하므로, 경계를 우리가 넣지 않으면 압축된 HTML에서
  // 5KB 본문이 두 줄이 된다.
  it('문단 사이에 경계를 넣는다', () => {
    const out = bodyOf('<p>첫 문단이다.</p><p>둘째 문단이다.</p>')
    expect(out).toContain('\n')
    expect(out).not.toContain('첫 문단이다.둘째')
  })

  it('소제목이 뒤 문장에 붙지 않는다', () => {
    const out = bodyOf('<h2>수평 확장</h2><p>파드 레플리카를 늘린다.</p>')
    expect(out).not.toContain('수평 확장파드')
    expect(out.split('\n')[0]).toBe('수평 확장')
  })

  // **표 셀.** BLOCK_SELECTOR는 `tr`을 갖고 있었지만 `td`/`th`가 없어서, 한 행 안의 셀들이
  // 경계 없이 이어붙었다 — `Column AColumn B`. 그 모양은 요약기의 접착 토큰 휴리스틱에
  // 걸려서(`backend/internal/summarizer` 참조) **캡처된 표가 통째로 요약에서 버려진다.**
  // 화면에서는 "이 링크는 요약이 없네"로만 보이고, 원인은 어디에도 안 나타난다.
  it('표 셀이 이어붙지 않는다', () => {
    const out = bodyOf('<table><tr><th>이름</th><th>크기</th></tr><tr><td>알파</td><td>10</td></tr></table>')
    expect(out).not.toContain('이름크기')
    expect(out).not.toContain('알파10')
  })

  it('코드 블록은 한 덩어리로 남는다', () => {
    const out = bodyOf('<p>예시:</p><pre><code>go build ./...</code></pre><p>끝.</p>')
    expect(out).toContain('go build ./...')
    expect(out).not.toContain('예시:go build')
  })

  it('경계를 세 줄 이상으로 부풀리지 않는다', () => {
    const out = bodyOf('<div><div><div><p>깊게 중첩된 한 문단.</p></div></div></div>')
    expect(out).not.toMatch(/\n{3,}/)
  })
})
