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
import { t } from '../lib/i18n'
import { EmptyState } from '../components/ui'
import { activeDays, cappedStreak, daysSinceLastSave, groupedTags, streak } from '../lib/rhythm'
import { facetLabel } from '../lib/tags/facet'
import type { Stats, TagFacet } from '../lib/api/types'


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
        <h2 className="text-title text-fg-1">{t('rhythm.title')}</h2>

        {stats.isError ? (
          <div className="space-y-4" role="status">
            <p className="text-body text-danger">{t('rhythm.loadFailed')}</p>
            <button
              type="button"
              onClick={() => void stats.refetch()}
              className="text-label text-fg-2 underline underline-offset-4 hover:text-fg-1"
            >
              {t('common.tryAgain')}
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
          <p className="text-body text-fg-2">{t('rhythm.emptyBody')}</p>
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
 * 실패했으면 전부 neutral이 되어 **묶음이 하나로 뭉치고 색이 전부 회색이 된다.**
 * 그래서 호출부가 `tags.isPending`을 기다린다.
 *
 * 예전에는 이 실패가 관심사 문장을 **통째로 사라지게** 만들었다 — 네트워크 오류가
 * "말할 것이 없음"으로 위장됐다. 그 문장은 14 §D4로 삭제됐으므로 지금 남는 피해는
 * 묶음과 색뿐이고, 연속·리듬·태그 목록은 사전 없이도 정확하다.
 */
function facetLookup(list: ReturnType<typeof useTags>['data']): (name: string) => TagFacet {
  const byName = new Map((list ?? []).map((tag) => [tag.name, tag.facet]))
  return (name) => byName.get(name) ?? 'neutral'
}

function Rhythm({ s, facetOf }: { s: Stats; facetOf: (name: string) => TagFacet }) {
  // **빈 아카이브에 계기판을 그리지 않는다.** 총계가 0이면 여기서 끝낸다 — 그러지 않으면
  // 0막대 30개와 빈 묶음과 "전체 0"이 그려진다. iOS는 `ContentUnavailableView`로 이미
  // 가로채고 있었고 웹만 안 하고 있어서, **같은 상태를 두 화면이 다르게 그렸다.**
  //
  // 이건 취향이 아니라 이 화면의 교리다 — "빈 상태에서 0을 세 개 보여주는 것은 정보가
  // 아니라 소음"(11 §8)이고, 14의 재설계가 지적한 세 문제 중 하나가 **화면이 데이터
  // 부족을 인정하지 않는 것**이었다.
  //
  // 문구는 iOS와 같다(13 §3). 형태만 플랫폼 관용을 따른다.
  if (s.total_links === 0) {
    return (
      <EmptyState title={t('rhythm.emptyTitle')} description={t('rhythm.emptyBody')} />
    )
  }

  const days = streak(s.by_day)
  const active = activeDays(s.by_day)
  const groups = groupedTags(s.by_tag, facetOf)
  const max = Math.max(1, ...s.by_day.map((d) => d.count))

  return (
    <div className="space-y-16">
      <p className="text-body text-fg-1">{narrative(s)}</p>

      <p className="text-body text-accent">{goalLine(days, cappedStreak(s.by_day, days))}</p>

      <div className="space-y-4">
        <div className="flex items-baseline justify-between">
          <span className="text-label text-fg-3">{t('common.last30Days')}</span>
          <span className="font-mono text-label text-fg-3">
            {t('rhythm.daysSaved', { count: active })}
          </span>
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
          <span className="text-label text-fg-3">{t('rhythm.axisStart')}</span>
          <span className="text-label text-fg-3">{t('time.today')}</span>
        </div>
      </div>


      {/* 무엇을 모았나 — iOS와 같은 묶음이다(13 §1 ① 축).
          예전에는 상위 5개를 평면으로 늘어놨는데, 그러면 "내가 무엇에 관심이 있나"라는
          이 섹션의 질문에 답하지 못한다: 상위 5개가 전부 같은 facet일 수도 있고, 6번째
          이후는 아예 없는 것처럼 보인다. facet으로 묶으면 그 분포가 곧 답이 된다. */}
      {groups.length > 0 && (
        <div className="space-y-8">
          <div className="flex items-baseline justify-between">
            <span className="text-label text-fg-3">{t('rhythm.collected')}</span>
            <span className="text-label text-fg-3">{t('rhythm.collectedHint')}</span>
          </div>
          {groups.map((g) => (
            <div key={g.facet} className="space-y-2">
              <div className="flex items-center gap-6">
                <span aria-hidden className={`size-6 shrink-0 rounded-full ${FACET_DOT[g.facet]}`} />
                <span className="text-label text-fg-2">{facetLabel(g.facet)}</span>
                <span className="font-mono text-label text-fg-3">{g.total}</span>
              </div>
              {g.tags.map((tag) => (
                <Link
                  key={tag.name}
                  to="/"
                  search={{ tag: tag.name }}
                  className="flex items-baseline justify-between gap-8 rounded-control py-2 pl-12 pr-4 text-body text-fg-2 hover:bg-hover hover:text-fg-1"
                >
                  <span className="truncate">{tag.name}</span>
                  <span className="font-mono text-label text-fg-3">{tag.count}</span>
                </Link>
              ))}
            </div>
          ))}
        </div>
      )}

      {/* **실패한 링크는 통계가 아니라 할 일이다.** 개수만 알려주고 끝내면 그 링크는 영원히
          실패로 남는다 — 눌러서 그 목록으로 갈 수 있어야 한다. iOS에는 이 섹션이 처음부터
          있었고 웹에는 계약에 수가 없어서 없었다(13 §2). 이제 있다. */}
      {s.failed_links > 0 ? (
        <Link
          to="/"
          search={{ status: 'failed' }}
          className="flex items-center gap-6 text-body text-danger underline underline-offset-4"
        >
          {t('rhythm.failedLinks', { count: s.failed_links })}
        </Link>
      ) : null}

      <p className="text-meta text-fg-3">
        {t('rhythm.total')} <span className="font-mono">{s.total_links.toLocaleString('ko-KR')}</span>
      </p>
    </div>
  )
}

/**
 * 화면이 사람에게 하는 말. **지지되는 수만 쓴다.**
 *
 * 예전 문장은 네 절이었고 그중 둘을 데이터가 받치지 못했다(14 §1). "지난주보다 N개"는
 * 행동이 전혀 안 변해도 평균 2.41개가 나오고 방향 단어가 사흘에 한 번 뒤집혔다.
 * "최근 30일은 X요일에 가장 많이"는 **어떤 저장 속도에서도** 성립하지 않았다 —
 * 30일은 4주+2일이라 오늘·어제 요일만 5칸을 갖고 그 둘이 매일 회전하므로, 하루 두 건씩
 * 한 번도 거르지 않는 사용자조차 매일 다른 답을 들었다.
 *
 * 남은 것은 **사실의 개수**다. 활성 일수와 연속일은 추론이 아니라 세는 것이고, 흔들릴
 * 때는 진짜로 무언가 일어났다는 뜻이다. 이 프로젝트는 이미 그 둘을 알고 있었다 — M6
 * 완료 판정을 `scripts/streak.sh`에 맡길 때 고른 것이 정확히 이 둘이다.
 *
 * facet 절도 뺐다. `by_tag`에 날짜 조건이 없어 전 기간 누계이므로 "이번 주"로 시작한
 * 문단의 두 번째 자리에서 최근성 주장으로 읽혔는데, 실제로는 링크 100건이면 200일에
 * 한 번 움직이는 영구 사실이다. iOS가 facet 도넛을 걷어내며 쓴 이유가 그대로 적용된다.
 * 구성은 아래 "무엇을 모았나" 목록이 근거와 함께 보여 준다.
 *
 * 문장은 iOS와 **글자까지 같아야 한다**(13 §3). 두 화면이 같은 데이터로 다른 말을 하면
 * 어느 쪽이 맞는지 사용자가 판단해야 한다.
 */
function narrative(s: Stats): string {
  // total_links === 0은 여기 안 온다 — 호출부가 EmptyState로 먼저 가로챈다.
  const active = activeDays(s.by_day)
  const days = streak(s.by_day)
  const capped = cappedStreak(s.by_day, days)

  // 창 안에 아무것도 없으면 활성 일수는 0이고, "0일 저장했어요"는 정보가 아니다.
  if (active === 0) return t('rhythm.narrativeNoRecent', { count: s.total_links })

  // 이어 붙이는 조각은 전부 그 자체로 완결된 문장이다 — 조각으로 문장을 조립하면
  // 어순이 다른 언어에서 무너진다.
  const first = t('rhythm.narrativeActive', { n: active })
  if (days > 0) {
    return capped
      ? `${first} ${t('rhythm.narrativeStreakCapped')}`
      : `${first} ${t('rhythm.narrativeStreak', { n: days })}`
  }

  // 끊긴 것은 사실이므로 말하되, 되돌리라고 요구하지 않는다. 자가추적 연구가
  // 반복해서 찾아낸 이탈 원인이 정확히 그 반대편이다(14 §D1).
  const gap = daysSinceLastSave(s.by_day)
  if (gap === null) return first
  return gap === 1
    ? `${first} ${t('rhythm.narrativeLastYesterday')}`
    : `${first} ${t('rhythm.narrativeLastDaysAgo', { n: gap })}`
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
  if (days === 0) return t('rhythm.goalStart')
  if (capped) return t('rhythm.goalMetCapped', { n: days })
  if (days < 28) return t('rhythm.goalProgress', { n: days, count: 28 - days })
  return t('rhythm.goalMet', { n: days })
}
