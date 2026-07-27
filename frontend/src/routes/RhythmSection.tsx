// 설정의 리듬 섹션 — §11 8 (3-1).
//
// **총계를 나열하지 않는다.** 이 화면의 주인공은 쌓인 양이 아니라 리듬이고, 저장이 끊긴
// 구간이 보이는 것이 값이다. 순서가 그 판단을 담는다: 문단 → 연속 → 리듬 → 태그 → 총계.
// 총계가 맨 뒤인 것은 그것이 가장 덜 쓸모 있는 수이기 때문이다.
//
// **문단이 먼저 오는 이유**는 iOS 쪽 주석이 적어 두었다 — 대시보드는 숫자를 놓고 해석을
// 사람에게 떠넘긴다. "1 · 4 · 14"를 보고 잘 되고 있는지 판단하려면 매번 머릿속에서 문장을
// 만들어야 하는데, 그 일은 화면이 해야 한다. 아래 섹션들은 그 문단의 **근거**이지 결론이 아니다.
//
// 파생 계산은 `lib/rhythm.ts`에 있다 — 같은 규칙의 구현이 셋이라(iOS·streak.sh·여기)
// 순수 함수로 떼어 두어야 어긋났을 때 한 곳만 보면 된다.

import { Link } from '@tanstack/react-router'
import { useStats } from '../hooks/useStats'
import { activeDays, streak, weekOverWeek, weekdayCounts } from '../lib/rhythm'
import { FACET_LABELS } from '../lib/tags/facet'
import { useTags } from '../hooks/useTags'
import type { Stats } from '../lib/api/types'

const WEEKDAYS = ['일', '월', '화', '수', '목', '금', '토'] as const

export function RhythmSection() {
  const stats = useStats()
  const tags = useTags()

  // 401은 §1.4의 키 배너가 이미 말하고 있고, 404는 통계를 못 쓰는 배포다.
  // 둘 다 여기서 에러 블록을 남기지 않고 **섹션 전체를 숨긴다** — 고칠 수 없는 것을
  // 화면에 남겨 두면 사용자가 할 수 있는 일이 없다.
  if (stats.isError) return null

  return (
    <div className="space-y-12">
      <h2 className="text-title text-fg-1">리듬</h2>

      {stats.isPending ? (
        // 카운트업 애니메이션 금지(§8 (5)) — 높이만 예약한다.
        <div className="h-64 animate-pulse rounded-(--r-control) bg-hover" />
      ) : !stats.data || stats.data.total_links === 0 ? (
        // 빈 상태에서 0을 세 개 보여주는 것은 정보가 아니라 소음이다.
        <p className="text-body text-fg-2">링크를 저장하면 여기에 리듬이 쌓입니다.</p>
      ) : (
        <Rhythm s={stats.data} facetOf={facetLookup(tags.data)} />
      )}
    </div>
  )
}

/** 태그 이름 → facet. 사전을 아직 못 받았으면 전부 neutral로 둔다(색은 못 써도 수는 맞다). */
function facetLookup(list: ReturnType<typeof useTags>['data']) {
  const byName = new Map((list ?? []).map((t) => [t.name, t.facet]))
  return (name: string) => byName.get(name) ?? 'neutral'
}

function Rhythm({ s, facetOf }: { s: Stats; facetOf: (name: string) => string }) {
  const days = streak(s.by_day)
  const active = activeDays(s.by_day)
  const top = [...s.by_tag].sort((a, b) => b.count - a.count).slice(0, 5)
  const max = Math.max(1, ...s.by_day.map((d) => d.count))

  return (
    <div className="space-y-16">
      <p className="text-body text-fg-1">{narrative(s, facetOf)}</p>

      <p className="text-body text-accent">{goalLine(days)}</p>

      <div className="space-y-4">
        <div className="flex items-baseline justify-between">
          <span className="text-caption text-fg-3">최근 30일</span>
          <span className="text-mono text-caption text-fg-3">{active}일 저장</span>
        </div>
        {/* x축을 **날짜가 아니라 30칸 고정**으로 둔다. 데이터가 있는 날만 칸을 만들면
            하루치만 있을 때 그 막대가 화면을 채워 "저장이 폭발했다"로 읽히는 거짓말이
            된다. 칸을 고정하면 빈 구간이 빈 구간으로 보인다. */}
        <div className="flex h-32 items-end gap-1" aria-hidden>
          {Array.from({ length: 30 }, (_, i) => {
            const day = s.by_day[s.by_day.length - 30 + i]
            const n = day?.count ?? 0
            return (
              <div
                key={i}
                className={cnBar(n)}
                style={{ height: `${Math.max(n > 0 ? 12 : 4, (n / max) * 100)}%` }}
              />
            )
          })}
        </div>
      </div>

      {top.length > 0 && (
        <div className="space-y-4">
          <span className="text-caption text-fg-3">상위 태그</span>
          {/* 모든 항목이 어딘가로 닿는다 — 숫자만 보여주는 화면은 한 번 보고 다시 오지 않는다. */}
          <div className="flex flex-wrap gap-x-12 gap-y-4">
            {top.map((t) => (
              <Link
                key={t.name}
                to="/"
                search={{ tag: t.name }}
                className="text-body text-fg-2 underline-offset-4 hover:text-fg-1 hover:underline"
              >
                {t.name} <span className="text-mono text-caption text-fg-3">{t.count}</span>
              </Link>
            ))}
          </div>
        </div>
      )}

      <p className="text-caption text-fg-3">
        전체 <span className="text-mono">{s.total_links.toLocaleString('ko-KR')}</span>
      </p>
    </div>
  )
}

function cnBar(n: number): string {
  return n > 0 ? 'flex-1 rounded-xs bg-accent' : 'flex-1 rounded-xs bg-line-2'
}

/**
 * 데이터에서 한 문단을 만든다 — 지난주 대비·지배 관심사·주 활동 요일까지 담아
 * "무엇이 어떻게 바뀌었나"를 읽고 끝낼 수 있게 한다.
 *
 * iOS `StatsView.narrative`와 **같은 문장·같은 순서**다. 두 클라이언트가 같은 데이터로
 * 다른 말을 하면 어느 쪽이 맞는지 사용자가 판단해야 한다.
 */
function narrative(s: Stats, facetOf: (name: string) => string): string {
  if (s.total_links === 0) return '아직 저장한 링크가 없어요'
  if (s.links_this_week === 0) {
    return `이번 주에는 아직 저장한 게 없네요. 지금까지 ${s.total_links}개를 모았어요.`
  }

  const out = [`이번 주에 ${s.links_this_week}개를 저장했어요.`]

  // 지난주 대비 — "바뀌었다"는 비교가 있어야 성립한다. null은 "아직 비교할 수 없다"이고
  // 0("같다")과 다르므로 문장을 아예 만들지 않는다.
  const delta = weekOverWeek(s.by_day)
  if (delta !== null) {
    out.push(delta > 0 ? `지난주보다 ${delta}개 많아요.` : delta < 0 ? `지난주보다 ${-delta}개 적어요.` : '지난주와 같은 수예요.')
  }

  // 무엇에 관심이 갔나 — facet 라벨은 iOS와 같은 단어를 쓴다(§8.1).
  const byFacet = new Map<string, number>()
  for (const t of s.by_tag) {
    const f = facetOf(t.name)
    byFacet.set(f, (byFacet.get(f) ?? 0) + t.count)
  }
  const topFacet = [...byFacet.entries()].filter(([f]) => f !== 'neutral').sort((a, b) => b[1] - a[1])[0]
  if (topFacet) out.push(`주로 '${FACET_LABELS[topFacet[0] as keyof typeof FACET_LABELS]}'에 관심이 갔고,`)

  const counts = weekdayCounts(s.by_day)
  const peak = Math.max(...counts)
  if (peak > 0) out.push(`${WEEKDAYS[counts.indexOf(peak)]}요일에 가장 많이 저장했어요.`)

  return out.join(' ')
}

/**
 * 연속과 4주 목표.
 *
 * **목표선을 화면에 두는 것은 대가를 아는 선택이다** — 4주 연속이 이 제품의 성공 판정
 * 지표라(08 §2 M6), 남은 일수를 띄우면 의미 없는 링크로 연속을 잇는 유인이 생긴다.
 * 동기부여 쪽을 택했고, **판정 자체는 화면이 아니라 `scripts/streak.sh`가 한다**
 * (exit code가 있는 쪽이 판정기다).
 */
function goalLine(days: number): string {
  if (days === 0) return '오늘 하나 저장하면 연속이 시작돼요'
  if (days < 28) return `${days}일 연속 — 4주(28일)까지 ${28 - days}일 남았어요`
  return `${days}일 연속 — 4주 목표를 넘겼습니다`
}
