-- 별칭 보강: security · productivity (2026-07-27)
--
-- dev의 "정답 0점" 미스를 하나씩 뜯어보다 나온 공백이다. 두 건 다 **글 전체가 그 개념으로
-- 가득한데 사전에 걸릴 표면이 하나도 없는** 경우였고, 순위 문제가 아니라 어휘 문제다.
--
-- (1) security — `blog.cloudflare.com`의 edge DDoS 방어 글(본문 7,984자)에서 실측:
--       security  0회      ← 사전에 있던 유일한 영어 표면
--       ddos     26회
--       firewall  5회
--     "security"라는 낱말만 없고 보안 어휘로 포화돼 있었다. 정답 `security`가 0점이었다.
--
-- (2) productivity — `jamesclear.com/habit-guide`(본문 4,821자)에서 `habit`이 11회로 글의
--     중심인데 사전에 그 개념이 없었다. 기존 별칭은 생산성·워크플로우·도구뿐이다.
--
-- **일부러 넣지 않은 것들** — 개념상 맞아 보여도 이 앱의 코퍼스에서 오탐을 만든다:
--   `attack`/`공격`  → 축구 기사의 '공격수'에 걸린다(test에 축구 항목이 있다)
--   `mitigation`     → 기후 완화 등 보안과 무관한 용법이 흔하다
--   `routine`        → 코드의 '루틴'을 뜻하는 용법이 개발 문서에 흔하다
-- 넣은 표면은 전부 그 개념 밖에서 거의 안 쓰이는 낱말로만 골랐다.

UPDATE tags SET aliases = '["보안","시큐리티","해킹","hacking","ddos","firewall","방화벽","vulnerability","취약점","encryption","암호화"]' WHERE name = 'security';
UPDATE tags SET aliases = '["생산성","워크플로우","workflow","도구","tools","습관","habit","시간관리"]' WHERE name = 'productivity';
