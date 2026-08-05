import SwiftUI

/// 설정 — 지금은 **모양 둘**뿐이다.
///
/// 화면이 아니라 시트인 이유: 여기 담긴 것은 목적지가 아니다. 통계는 훑으러 들어가는 면이라
/// 탭이 맞지만, 이건 한 번 정하고 닫는 것이라 탭 하나를 상시로 내주면 그 자리가 아깝다
/// (10 §4.5.1이 둘을 "링크를 담지 않는 면"으로 묶는데, 묶이는 것은 시각 처리이지 도달 방법이
/// 아니다).
///
/// **언어는 여기 없다.** 툴바의 globe에 그대로 둔다 — 설정 안에 넣으면, 화면이 이미 못 읽는
/// 언어로 쓰여 있을 때 그 화면을 찾아가야 하는 순환이 된다. 웹도 같은 이유로 헤더에 둔다
/// (`LangToggle.tsx`). 13 §2에 ① 축으로 등재돼 있고, "크롬에 둔다"가 그 판정의 내용이다.
///
/// **밀도는 툴바에도 남는다.** 중복이지만 웹이 이미 같은 모양이다 — 헤더의 2-state 토글과
/// 설정의 3-state 세그먼트가 같은 키를 쓴다. 훑는 밀도는 훑는 중에 바꾸는 것이라 목록에서
/// 손이 닿아야 하고, 설정에는 그것이 무엇인지 이름과 함께 있어야 한다.
struct SettingsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject private var theme = Theme.Store.shared
    @AppStorage("pushpoint.density") private var density: ListDensity = .card

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    // **라벨을 직접 그린다.** `Form` 안에서 `.pickerStyle(.segmented)`는
                    // Picker의 라벨을 숨기므로, 그냥 두면 이름 없는 세그먼트 둘이 세로로
                    // 쌓인다 — 화면을 보고서야 알았다. 옵션 낱말만으로 짐작은 되지만
                    // 짐작하게 두는 것과 말해 주는 것은 다르다.
                    //
                    // 세그먼트 자체는 시스템 것을 그대로 쓰고 칠하지 않는다(10 §8.4).
                    // 이 화면은 테마를 바꾸는 화면이라, 우리가 칠한 색이 바뀐 테마를
                    // 반쯤 따라가면 그게 곧 고장으로 보인다.
                    labelled(t("settings.theme")) {
                        Picker(t("settings.theme"), selection: themeBinding) {
                            ForEach(Theme.Pref.allCases, id: \.self) { p in
                                Text(Theme.label(p)).tag(p)
                            }
                        }
                        .pickerStyle(.segmented)
                        .labelsHidden()
                        .accessibilityIdentifier("settings.theme")
                    }

                    labelled(t("list.density")) {
                        Picker(t("list.density"), selection: $density) {
                            Text(t("list.densityCard")).tag(ListDensity.card)
                            Text(t("list.densityCompact")).tag(ListDensity.compact)
                        }
                        .pickerStyle(.segmented)
                        .labelsHidden()
                        .accessibilityIdentifier("settings.density")
                    }
                } header: {
                    Text(t("settings.appearance"))
                } footer: {
                    // 기본값이 무엇인지 화면이 말한다. 이게 없으면 "시스템"이 무엇을
                    // 따라가는지가 라벨 하나에만 걸려 있다.
                    Text(t("settings.themeFooter"))
                }
            }
            .navigationTitle(t("settings.title"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button(t("common.done")) { dismiss() }
                }
            }
        }
    }

    /// 세그먼트 위에 이름을 얹는다. 라벨은 VoiceOver가 Picker에서 이미 읽으므로
    /// 여기서는 `.accessibilityHidden` — 안 그러면 같은 낱말을 두 번 말한다.
    @ViewBuilder
    private func labelled(_ title: String, @ViewBuilder _ control: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(title)
                .font(PP.Typo.meta)
                .foregroundStyle(PP.Palette.fg2)
                .accessibilityHidden(true)
            control()
        }
        .padding(.vertical, 2)
    }

    /// 저장은 `Theme.pref`가, 화면 갱신은 Store가 맡는다. 세그먼트가 Store를 직접
    /// 쓰면 값이 `UserDefaults`에 안 남고, `Theme.pref`만 쓰면 화면이 안 바뀐다.
    private var themeBinding: Binding<Theme.Pref> {
        Binding(get: { theme.pref }, set: { Theme.pref = $0 })
    }
}
