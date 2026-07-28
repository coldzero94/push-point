import { defineConfig } from 'vitest/config'

export default defineConfig({
  test: {
    include: ['src/**/*.test.ts'],
    // 순수 함수만 테스트한다 — DOM도 React도 필요 없어서 기본 node 환경 그대로다.
    // 컴포넌트 테스트(jsdom/testing-library)를 들일지는 별건으로 남긴다.
    environment: 'node',

    // **타임존을 그리니치 서쪽으로 고정한다.** 이건 취향이 아니라 검출력 문제다.
    //
    // `rhythm.ts`의 weekdayCounts는 `new Date('2026-07-27')`가 UTC 자정으로 해석되는
    // 함정을 피하려고 손으로 파싱한다. 그런데 개발 머신이 KST(UTC+9)라 UTC 자정이
    // 같은 날 09시로 떨어지고, **동쪽에서는 그 버그가 나타나지 않는다** — 실제로
    // 파싱을 UTC로 되돌리는 변이가 KST에서 전 테스트를 통과했다(2026-07-28).
    //
    // 서쪽 존을 하나 박아 두면 그 변이가 빨개진다. 나머지 테스트는 날짜를 고정값에서
    // 만들기 때문에 어느 존이든 결정적이다.
    env: { TZ: 'America/Los_Angeles' },
  },
})
