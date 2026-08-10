// stdin의 HTML을 **진짜 extension/src/extract.js**로 캡처해 body_text만 낸다.
//
// 하네스가 클라이언트 경로를 재려면 그 파일을 실행해야 한다 — 규칙을 Go로 옮겨 적으면
// 재는 것이 출하품이 아니라 사본이 된다. linkedom으로 DOM을 만들고 원본을 평가한다.
// node가 Go 바이너리에 들어가는 일은 없다; 하네스가 밖에서 부를 뿐이다.
//
// **frontend/ 안에 사는 이유**는 linkedom이 거기 devDependency라서다. scripts/에 두면
// node가 패키지를 못 찾는다 — 실제로 그렇게 한 번 실패했다.
import { readFileSync } from 'node:fs'
import { parseHTML } from 'linkedom'

const src = readFileSync(new URL('../../extension/src/extract.js', import.meta.url), 'utf8')
const capture = new Function(`${src.replace(/^\s*captureOnce\(\);?\s*$/m, '')}\nreturn capture;`)()

let html = ''
for await (const chunk of process.stdin) html += chunk
const { document } = parseHTML(html)
process.stdout.write(capture(document, process.argv[2] || 'https://example.com/').body_text)
