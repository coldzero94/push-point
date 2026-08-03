import { useSyncExternalStore } from 'react'
import { getLang, setLang, subscribeLang, t } from '../lib/i18n'

/// 언어 전환. 헤더에서 테마 토글 옆에 선다.
///
/// **왜 헤더인가.** 설정 화면 안에 두면 그 화면이 이미 못 읽는 언어로 쓰여 있을 때 찾아갈
/// 수가 없다 — 언어를 못 읽는 사람이 언어 설정에 도달해야 하는 순환이다. 그래서 항상 보이는
/// 자리에 두고, 라벨은 **바뀔 언어의 이름**을 그 언어로 쓴다(한국어 화면에서는 "EN",
/// 영어 화면에서는 "한"). 어느 쪽을 읽든 자기 언어가 보인다.
///
/// `useSyncExternalStore`인 이유는 이 버튼만 다시 그려서는 소용이 없고 화면 전체가 다시
/// 그려져야 하기 때문이다 — 구독은 `i18n.ts`가 갖고, 여기서는 스냅샷만 읽는다.
export function LangToggle() {
  const lang = useSyncExternalStore(subscribeLang, getLang, () => 'ko' as const)
  const next = lang === 'ko' ? 'en' : 'ko'
  return (
    <button
      type="button"
      onClick={() => setLang(next)}
      aria-label={t('settings.switchLanguage')}
      // ThemeToggle과 같은 ghost 컨트롤 규격이다(§8) — 나란히 서므로 높이·모서리·hover가
      // 어긋나면 바로 보인다. 글자를 담으므로 폭만 auto다.
      className="relative hit-target inline-flex h-32 items-center justify-center rounded-control px-8 text-label font-medium text-fg-2 transition-colors duration-(--dur-out) ease-ui hover:bg-hover"
    >
      {next === 'en' ? 'EN' : '한'}
    </button>
  )
}
