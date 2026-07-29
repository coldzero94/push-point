// 설정의 리듬 섹션 — 11 §8 (3-1).
//
// **총계를 나열하지 않는다.** 이 화면의 주인공은 쌓인 양이 아니라 리듬이고, 저장이 끊긴
// 구간이 보이는 것이 값이다. 순서가 그 판단을 담는다: 문단 → 연속 → 리듬 → 요일 → 태그 → 총계.
// 총계가 맨 뒤인 것은 그것이 가장 덜 쓸모 있는 수이기 때문이다.
//
// **문단이 먼저 오는 이유**는 iOS 쪽 주석이 적어 두었다 — 대시보드는 숫자를 놓고 해석을
// 사람에게 떠넘긴다. "1 · 4 · 14"를 보고 잘 되고 있는지 판단하려면 매번 머릿속에서 문장을
// 만들어야 하는데, 그 일은 화면이 해야 한다. 아래 섹션들은 그 문단의 **근거**이지 결론이 아니다.
//
// 파생 계산은 전부 `lib/rhythm.ts`에 있고 `rhythm.test.ts`가 그걸 검증한다 — 같은 규칙의
// 구현이 셋이라(iOS·streak.sh·여기) 순수 함수로 떼어 두어야 어긋났을 때 한 곳만 보면 된다.

import { Link } from '@tanstack/react-router'
import { useStats } from '../hooks/useStats'
import { useTags } from '../hooks/useTags'
import { isUnauthorized } from '../lib/api/client'
import { activeDays, cappedStreak, dominantFacet, groupedTags, streak, weekOverWeek, weekdayCounts } from '../lib/rhythm'
import { FACET_LABELS } from '../lib/tags/facet'
import type { Stats, TagFacet } from '../lib/api/types'

const WEEKDAYS = ['일', '월', '화', '수', '목', '금', '토'] as const

// facet -> 점 색. **리터럴이어야 한다** — `bg-${token}`은 Tailwind 스캐너가 못 보고
// CSS가 생성되지 않아 점이 투명해진다(클래스는 붙고 화면만 틀리는 종류).
// `components/ui/Chip.tsx`가 같은 이유로 같은 형태의 맵을 갖고 있다.
const FACET_DOT: Record<TagFacet, string> = {
  craft: 'bg-tag-craft-ink',
  media: 'bg-tag-media-ink',
  life: 'bg-tag-life-ink',
  neutral: 'bg-fg-3',
}

export function RhythmSection() {
  const stats = useStats()
  const tags = useTags()

  // 401만 숨긴다 — §1.4의 키 배너가 이미 그 사유를 화면 맨 위에서 말하고 있고, 여기서
  // 한 번 더 말하면 같은 사고에 대한 두 번째 문장일 뿐이다.
  //
  // **나머지는 보여준다.** 원래는 `stats.isError`면 섹션 전체를 사라지게 했는데, 그러면
  // 500도 타임아웃도 네트워크 끊김도 전부 "리듬 섹션이 없는 빌드"와 구분되지 않는다.
  // 바로 아래 SheetsSection이 같은 파일에서 반대로 하고 있고("사유를 삼키면 사용자가
  // 무엇을 고쳐야 할지 알 수 없다") iOS도 불러오기 실패를 화면에 띄운다.
  //
  // **구분선을 이 컴포넌트가 들고 있는 이유**: 섹션이 사라질 때 위아래 구분선이 남아
  // 빈 줄 두 개가 겹쳐 보인다. 설정 화면에 띄워 보고서야 알았다.
  if (stats.isError && isUnauthorized(stats.error)) return null

  return (
    <>
      <div className="border-t border-line-2" />
      <div className="space-y-12">
        <h2 className="text-title text-fg-1">리듬</h2>

        {stats.isError ? (
          <div className="space-y-4" role="status">
            <p className="text-body text-danger">통계를 불러오지 못했습니다.</p>
            <button
              type="button"
              onClick={() => void stats.refetch()}
              className="text-label text-fg-2 underline underline-offset-4 hover:text-fg-1"
            >
              다시 시도
            </button>
          </div>
        ) : stats.isPending || tags.isPending ? (
          // 스켈레톤은 움직이지 않는다(10 §4.9) — 높이만 예약한다. 카운트업 애니메이션도
          // 금지다(11 §8 (5)). tags까지 기다리는 이유는 아래 Rhythm 주석에 있다.
          <div className="space-y-16">
            {/* 높이 예약. 예전에는 `h-64` 한 줄이었는데 12스텝(2·4·6·8·12·16·20·24·
                32·40·56·80)에 64가 없어서 **아무 높이도 예약하지 않고 있었다** —
                클래스는 붙어 있고 CSS만 없는 형태라 코드로는 안 보인다. */}
            <div className="h-40 rounded-control bg-hover" />
            <div className="h-(--size-rhythm) rounded-control bg-hover" />
            <div className="h-(--size-weekday) rounded-control bg-hover" />
          </div>
        ) : !stats.data || stats.data.total_links === 0 ? (
          // 빈 상태에서 0을 세 개 보여주는 것은 정보가 아니라 소음이다.
          <p className="text-body text-fg-2">링크를 저장하면 여기에 리듬이 쌓입니다.</p>
        ) : (
          <Rhythm s={stats.data} facetOf={facetLookup(tags.data)} />
        )}
      </div>
    </>
  )
}

/**
 * 태그 이름 → facet.
 *
 * `Stats.by_tag`는 id 없이 이름만 준다(집계 결과라 그렇다). 사전을 아직 못 받았거나
 * 실패했으면 전부 neutral이 되고, 그러면 `dominantFacet`이 null을 내서 관심사 문장이
 * **통째로 사라진다** — 색이 빠지는 정도가 아니다. 그래서 호출부가 tags.isPending을
 * 기다린다. 실패한 경우에는 문장 하나가 빠진 채로 나머지가 그려지는데, 그것이 섹션
 * 전체를 죽이는 것보다 낫다(연속·리듬·태그 목록은 사전 없이도 전부 정확하다).
 */
function facetLookup(list: ReturnType<typeof useTags>['data']): (name: string) => TagFacet {
  const byName = new Map((list ?? []).map((t) => [t.name, t.facet]))
  return (name) => byName.get(name) ?? 'neutral'
}

function Rhythm({ s, facetOf }: { s: Stats; facetOf: (name: string) => TagFacet }) {
  const days = streak(s.by_day)
  const active = activeDays(s.by_day)
  const groups = groupedTags(s.by_tag, facetOf)
  const max = Math.max(1, ...s.by_day.map((d) => d.count))
  const weekdays = weekdayCounts(s.by_day)
  const peak = Math.max(...weekdays)

  return (
    <div className="space-y-16">
      <p className="text-body text-fg-1">{narrative(s, facetOf)}</p>

      <p className="text-body text-accent">{goalLine(days, cappedStreak(s.by_day, days))}</p>

      <div className="space-y-4">
        <div className="flex items-baseline justify-between">
          <span className="text-label text-fg-3">최근 30일</span>
          <span className="font-mono text-label text-fg-3">{active}일 저장</span>
        </div>
        {/* 계약이 by_day를 **빈 날까지 채운 30칸**으로 보장하므로(openapi.yaml Stats.by_day)
            i번째 칸이 곧 i번째 날이다. 예전에는 GROUP BY 결과를 그대로 받아 행 순서로
            그렸고, 그래서 띄엄띄엄 저장한 사람의 막대가 한쪽 끝에 몰려 "최근에 몰아서
            저장함"으로 읽혔다. 서버가 채우게 바꾼 뒤로 여기서 할 일이 없어졌다. */}
        <div className="flex h-(--size-rhythm) items-end gap-2" aria-hidden>
          {s.by_day.map((d) => (
            <div
              key={d.date}
              className={d.count > 0 ? 'flex-1 rounded-bar bg-accent' : 'flex-1 rounded-bar bg-line-2'}
              style={{ height: `${Math.max(d.count > 0 ? 12 : 4, (d.count / max) * 100)}%` }}
            />
          ))}
        </div>
        <div className="flex justify-between">
          <span className="text-label text-fg-3">30일 전</span>
          <span className="text-label text-fg-3">오늘</span>
        </div>
      </div>

      {/* 30일 막대는 "얼마나 꾸준한가"에 답하지만 "언제"에는 답하지 못한다 — iOS의
          `언제 저장하나`와 같은 화면이다(11 §8 (3-1), 13 §2 ① 축). */}
      {peak > 0 && (
        <div className="space-y-4">
          <div className="flex items-baseline justify-between">
            <span className="text-label text-fg-3">언제 저장하나</span>
            <span className="text-label text-fg-3">
              {WEEKDAYS[weekdays.indexOf(peak)]}요일에 가장 많이
            </span>
          </div>
          {/* 막대와 라벨을 **두 줄로 나눈다.** 한 칸 안에 세로로 쌓으면 막대의 height:%가
              높이 미정인 flex-col 부모에 걸려 0으로 접힌다 — 타입도 빌드도 통과하고
              화면에서만 사라지는 종류라, 실제로 브라우저에 띄워 보고 찾았다(2026-07-28). */}
          <div className="flex h-(--size-weekday) items-end gap-4">
            {weekdays.map((n, i) => (
              // 높이로만 말한다 — 라벨 7개면 축이 따로 필요 없다.
              <div
                key={WEEKDAYS[i]}
                className={n > 0 ? 'flex-1 rounded-bar bg-accent' : 'flex-1 rounded-bar bg-line-2'}
                style={{ height: `${Math.max(n > 0 ? 10 : 3, (n / peak) * 100)}%` }}
              />
            ))}
          </div>
          <div className="flex gap-4">
            {WEEKDAYS.map((label, i) => (
              <span
                key={label}
                className={
                  weekdays[i] === peak
                    ? 'flex-1 text-center text-label text-fg-1'
                    : 'flex-1 text-center text-label text-fg-3'
                }
              >
                {label}
              </span>
            ))}
          </div>
        </div>
      )}

      {/* 무엇을 모았나 — iOS와 같은 묶음이다(13 §1 ① 축).
          예전에는 상위 5개를 평면으로 늘어놨는데, 그러면 "내가 무엇에 관심이 있나"라는
          이 섹션의 질문에 답하지 못한다: 상위 5개가 전부 같은 facet일 수도 있고, 6번째
          이후는 아예 없는 것처럼 보인다. facet으로 묶으면 그 분포가 곧 답이 된다. */}
      {groups.length > 0 && (
        <div className="space-y-8">
          <div className="flex items-baseline justify-between">
            <span className="text-label text-fg-3">무엇을 모았나</span>
            <span className="text-label text-fg-3">누르면 그 목록으로</span>
          </div>
          {groups.map((g) => (
            <div key={g.facet} className="space-y-2">
              <div className="flex items-center gap-6">
                <span aria-hidden className={`size-6 shrink-0 rounded-full ${FACET_DOT[g.facet]}`} />
                <span className="text-label text-fg-2">{FACET_LABELS[g.facet]}</span>
                <span className="font-mono text-label text-fg-3">{g.total}</span>
              </div>
              {g.tags.map((t) => (
                <Link
                  key={t.name}
                  to="/"
                  search={{ tag: t.name }}
                  className="flex items-baseline justify-between gap-8 rounded-control py-2 pl-12 pr-4 text-body text-fg-2 hover:bg-hover hover:text-fg-1"
                >
                  <span className="truncate">{t.name}</span>
                  <span className="font-mono text-label text-fg-3">{t.count}</span>
                </Link>
              ))}
            </div>
          ))}
        </div>
      )}

      <p className="text-meta text-fg-3">
        전체 <span className="font-mono">{s.total_links.toLocaleString('ko-KR')}</span>
      </p>
    </div>
  )
}

/**
 * 데이터에서 한 문단을 만든다 — 지난주 대비·지배 관심사·주 활동 요일까지 담아
 * "무엇이 어떻게 바뀌었나"를 읽고 끝낼 수 있게 한다.
 *
 * iOS `StatsView.narrative`와 **같은 문장·같은 순서**여야 한다. 두 클라이언트가 같은
 * 데이터로 다른 말을 하면 어느 쪽이 맞는지 사용자가 판단해야 한다. 2026-07-28까지는
 * 실제로 갈라져 있었다 — 관심사 문장을 고르는 순서가 달랐다(`dominantFacet` 주석).
 */
function narrative(s: Stats, facetOf: (name: string) => TagFacet): string {
  if (s.total_links === 0) return '아직 저장한 링크가 없어요'
  if (s.links_this_week === 0) {
    return `이번 주에는 아직 저장한 게 없네요. 지금까지 ${s.total_links}개를 모았어요.`
  }

  const out = [`이번 주에 ${s.links_this_week}개를 저장했어요.`]

  // 지난주 대비 — "바뀌었다"는 비교가 있어야 성립한다. null은 "아직 비교할 수 없다"이고
  // 0("같다")과 다르므로 문장을 아예 만들지 않는다.
  const delta = weekOverWeek(s)
  if (delta !== null) {
    out.push(
      delta > 0
        ? `지난주보다 ${delta}개 많아요.`
        : delta < 0
          ? `지난주보다 ${-delta}개 적어요.`
          : '지난주와 같은 수예요.',
    )
  }

  // 무엇에 관심이 갔나 — facet 라벨은 iOS와 같은 단어를 쓴다(§8.1).
  const facet = dominantFacet(s.by_tag, facetOf)
  if (facet) out.push(`주로 '${FACET_LABELS[facet]}'에 관심이 갔고,`)

  // 언제 — 이 절은 **30일치** 통계다. 앞의 두 문장이 이번 주 얘기라 그냥 이어 붙이면
  // 이번 주 요일로 읽힌다. 기간을 말에 넣어서 그 오독을 막는다.
  const counts = weekdayCounts(s.by_day)
  const peak = Math.max(...counts)
  if (peak > 0) out.push(`최근 30일은 ${WEEKDAYS[counts.indexOf(peak)]}요일에 가장 많이 저장했어요.`)

  return out.join(' ')
}

/**
 * 연속과 4주 목표.
 *
 * **목표선을 화면에 두는 것은 대가를 아는 선택이다** — 4주 연속이 이 제품의 성공 판정
 * 지표라(08 §2 M6), 남은 일수를 띄우면 의미 없는 링크로 연속을 잇는 유인이 생긴다.
 * 동기부여 쪽을 택했고, **판정 자체는 화면이 아니라 `scripts/streak.sh`가 한다**
 * (exit code가 있는 쪽이 판정기다).
 *
 * `capped`는 연속이 30일 창 끝까지 닿아 진짜 길이를 모르는 경우다. `streak.sh`는 이걸
 * 이미 밝히고 있었는데("30일 창 상한 — 실제로는 더 길 수 있습니다") 화면만 모르는 척
 * 하고 있었다. 판정기가 정직한 자리에서 화면이 더 정확한 척하면 안 된다.
 */
function goalLine(days: number, capped: boolean): string {
  if (days === 0) return '오늘 하나 저장하면 연속이 시작돼요'
  if (capped) return `${days}일 이상 연속 — 4주 목표를 넘겼습니다 (30일 창 상한)`
  if (days < 28) return `${days}일 연속 — 4주(28일)까지 ${28 - days}일 남았어요`
  return `${days}일 연속 — 4주 목표를 넘겼습니다`
}
