#!/usr/bin/env bash
# gomobile 바인딩의 **입력** 해시를 낸다. `just ios-bind`가 이 값을 저장하고
# `just ios-bind-check`가 다시 계산해 비교한다.
#
# ## 왜 필요한가 (2026-07-28)
#
# `ios/Frameworks/`는 gitignore된 로컬 빌드물인데, 백엔드가 바뀌어도 재빌드를 강제하는
# 것이 아무것도 없었다. 실제로 **이틀 낡은 프레임워크**로 앱이 돌고 있었다 —
# 프레임워크는 07-26 11:08 빌드인데 마이그레이션 0008·0009·0010·0011이 그 뒤에
# 추가됐고, 그래서 앱 안의 사전이 42개가 아니라 **30개**였다. 일반 관심사 태그
# (football·sports·politics·game·health…)가 통째로 없었다는 뜻이다.
#
# 계약 생성물에는 게이트가 셋이나 있는데(`gen-check`·`web-gen-check`·`ios-stamp-check`)
# 여기만 비어 있었다. 이 스크립트가 그 자리를 메운다.
#
# ## 왜 mtime이 아니라 내용 해시인가
#
# git 체크아웃은 모든 파일의 mtime을 체크아웃 시각으로 만든다. 브랜치를 오갈 때마다
# 프레임워크가 "낡았다"고 나오면 아무도 그 게이트를 안 믿게 된다.
# `ios-stamp-check`가 openapi.yaml에 쓰는 방식과 같은 형태를 쓴다.
set -euo pipefail
cd "$(dirname "$0")/.."

# 바인딩에 실제로 들어가는 것만 센다. 테스트 파일은 산출물에 포함되지 않으므로 뺀다 —
# 넣으면 테스트만 고쳐도 재바인드를 요구하게 되고, 그건 15분짜리 거짓 경보다.
{
  find backend -name '*.go' ! -name '*_test.go' -print0 | sort -z | xargs -0 shasum -a 256
  find backend/migrations -name '*.sql' -print0 | sort -z | xargs -0 shasum -a 256
  shasum -a 256 backend/go.mod backend/go.sum
  # 캡처 규칙은 ios-bind가 같이 복사한다(원본은 확장 쪽 하나뿐).
  shasum -a 256 extension/src/extract.js
} | shasum -a 256 | awk '{print $1}'
