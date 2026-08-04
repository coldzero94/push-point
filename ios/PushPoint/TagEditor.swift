import SwiftUI

/// 태그 편집 시트 — 사전에서 고른다, 자유 입력이 아니다.
///
/// 자유 입력이 아닌 이유는 UI 취향이 아니라 계약이다: `updateLink`의 `tags`는 **사전에
/// 있는 이름만** 받고 없는 이름은 400이다. 그리고 통제된 사전이야말로 이 앱이 LLM 없이
/// 품질을 내는 근거다 — 자유 태그를 허용하는 순간 같은 개념이 `k8s`·`쿠버네티스`·
/// `kubernetes`로 흩어져 색도 통계도 검색도 같이 무너진다.
///
/// **facet으로 묶는다.** 사전이 40개를 넘어가면 한 줄로 늘어놓은 목록에서는 원하는 것을
/// 못 찾는다. facet은 이미 색의 출처이므로(§5.2), 같은 축으로 묶으면 화면에서 본 색과
/// 여기서 찾는 순서가 일치한다.
///
/// 저장은 **전체 교체 한 번**이다(계약에 부분 추가/삭제가 없다). 그래서 토글은 로컬
/// 상태만 바꾸고, 시트를 닫을 때 한 번 PATCH한다 — 체크할 때마다 요청을 보내면 중간
/// 상태가 서버에 남고, 취소했을 때 되돌릴 방법이 없다.
struct TagEditor: View {
    /// 사전 전체 (이름 → facet). 목록·통계와 **같은 출처**를 쓴다.
    let dictionary: [String: PP.Facet]
    /// 지금 이 링크에 붙어 있는 태그 이름.
    let current: [String]
    /// 확인을 누르면 최종 이름 배열로 부른다. 취소면 불리지 않는다.
    let onSave: ([String]) -> Void

    @State private var selected: Set<String>
    @Environment(\.dismiss) private var dismiss

    init(dictionary: [String: PP.Facet], current: [String], onSave: @escaping ([String]) -> Void) {
        self.dictionary = dictionary
        self.current = current
        self.onSave = onSave
        _selected = State(initialValue: Set(current))
    }

    var body: some View {
        NavigationStack {
            List {
                ForEach(PP.Facet.allCases, id: \.self) { facet in
                    let names = names(in: facet)
                    if !names.isEmpty {
                        Section {
                            ForEach(names, id: \.self) { name in
                                row(name, facet: facet)
                            }
                        } header: {
                            // PP.Facet.label을 쓴다. 여기서 문구를 새로 적으면 웹·디자인
                            // 문서와 갈라지는데, 실제로 그렇게 갈라진 적이 있다 —
                            // DesignSystem.swift에 "두 클라이언트가 같은 단어를 써야 한다"고
                            // 적혀 있는데도 40줄 옆에서 다른 문구를 하드코딩했다.
                            Text(facet.label)
                                .font(PP.Typo.label)
                                .foregroundStyle(PP.Palette.fg3)
                        }
                    }
                }
            }
            .listStyle(.insetGrouped)
            .navigationTitle(t("common.tags"))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button(t("common.cancel")) { dismiss() }
                        .accessibilityIdentifier("tagEditor.cancel")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button(t("common.done")) {
                        // 순서를 고정한다 — 집합은 순서가 없어서, 그대로 보내면 같은
                        // 선택이 요청마다 다른 배열이 되어 diff를 읽을 수 없다.
                        onSave(selected.sorted())
                        dismiss()
                    }
                    .fontWeight(.semibold)
                    .disabled(selected == Set(current))
                    // 시트가 열렸다는 것을 **네비게이션 바 제목("태그")이 아니라**
                    // 이 버튼으로 판정한다. 바 제목은 곧 표시 문구라 언어를 타는데,
                    // 확인 버튼은 이 시트에만 있고 잠김 여부까지 함께 볼 수 있다.
                    .accessibilityIdentifier("tagEditor.done")
                }
            }
        }
    }

    private func row(_ name: String, facet: PP.Facet) -> some View {
        Button {
            if selected.contains(name) { selected.remove(name) } else { selected.insert(name) }
        } label: {
            HStack {
                Chip(name: name, facet: facet,
                     fill: selected.contains(name) ? .selected : .machine)
                Spacer()
                // 체크는 색과 **별개의** 표식이다. 칩 채움만으로 선택을 표시하면
                // 색을 못 보는 조건에서 무엇이 켜졌는지 알 수 없다(§5.2의 2축 원칙).
                if selected.contains(name) {
                    Image(systemName: "checkmark")
                        .font(PP.Typo.label)
                        .foregroundStyle(PP.Palette.accent)
                }
            }
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        // UI 테스트가 사전 40여 개 중 특정 태그를 집을 수 있게 하는 손잡이.
        // 표시 이름으로 찾으면 화면 문구를 다듬는 순간 테스트가 깨진다 —
        // 식별자는 그 둘을 분리한다.
        .accessibilityIdentifier("tag-\(name)")
    }

    private func names(in facet: PP.Facet) -> [String] {
        dictionary.filter { $0.value == facet }.keys.sorted()
    }
}
