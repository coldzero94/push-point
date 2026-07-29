import SwiftUI

/// 앱 안에서 링크를 저장한다.
///
/// ## 왜 필요한가 (2026-07-29)
///
/// 이 앱에는 **저장 경로가 공유 시트 하나뿐**이었다. 앱을 켜 놓고 있어도 링크를 넣을
/// 방법이 없고, 빈 화면은 "공유 시트로 링크를 보내면 여기에 쌓입니다"라고 안내만 한 채
/// 아무 버튼도 주지 않았다 — **읽고 나서 할 수 있는 일이 앱을 나가는 것뿐인 화면**이다.
///
/// 파리티 문서 ② 축(진입은 각자 최적으로)이 "웹은 URL 입력창, iOS는 공유 시트"라고
/// 적어 두었는데, 그 문장이 "iOS는 앱 안에서 저장할 수 없다"를 정당화하지는 않는다.
/// ②가 말하는 것은 **형태를 각자 최적으로 하라**는 것이지 한쪽을 비우라는 것이 아니다.
///
/// ## 왜 붙여넣기인가
///
/// 폰에서 URL을 손으로 치는 사람은 없다. 링크는 거의 항상 **클립보드에 있다** — 다른 앱에서
/// 복사해 왔거나 공유 시트를 열다 말았거나. 그래서 시트가 열릴 때 클립보드에 URL이 있으면
/// 그것을 먼저 제안한다. `UIPasteboard.general.string`을 그냥 읽으면 iOS가 붙여넣기 알림을
/// 띄우므로, **`hasURLs`로 먼저 물어본다** — 이 값은 내용을 읽지 않아 알림이 뜨지 않는다.
/// 사용자가 제안을 눌렀을 때만 실제로 읽는다.
struct SaveSheet: View {
    /// 저장 요청. 성공하면 목록을 다시 불러오는 것은 호출부의 몫이다.
    let onSave: (String, String) async -> String?

    @Environment(\.dismiss) private var dismiss
    @State private var url = ""
    @State private var note = ""
    @State private var saving = false
    @State private var error: String?
    /// 클립보드에 URL이 있는지. **내용은 아직 안 읽었다** — 읽으면 알림이 뜬다.
    @State private var clipboardHasURL = false
    @FocusState private var urlFocused: Bool

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    TextField("https://", text: $url, axis: .vertical)
                        .font(PP.Typo.metaMono) // URL은 기계 데이터다(R2)
                        .textInputAutocapitalization(.never)
                        .autocorrectionDisabled()
                        .keyboardType(.URL)
                        .focused($urlFocused)
                        .lineLimit(1 ... 3)

                    // 클립보드 제안. 입력창이 비었을 때만 — 이미 무언가 치고 있으면
                    // 그 위에 제안을 얹는 것이 방해다.
                    if clipboardHasURL, url.isEmpty {
                        Button {
                            // 여기서 처음 읽는다. 붙여넣기 알림은 이 시점에만 뜬다.
                            if let s = UIPasteboard.general.string { url = s }
                        } label: {
                            Label("클립보드에서 붙여넣기", systemImage: "doc.on.clipboard")
                                .font(PP.Typo.body)
                        }
                    }
                } header: {
                    Text("주소")
                } footer: {
                    if let error {
                        Text(error).foregroundStyle(PP.Palette.danger)
                    } else {
                        Text("제목·설명·태그는 저장한 뒤 자동으로 채워집니다.")
                    }
                }

                Section("메모") {
                    TextField("나중에 이걸 왜 저장했는지", text: $note, axis: .vertical)
                        .font(PP.Typo.body)
                        .lineLimit(1 ... 4)
                }
            }
            .navigationTitle("링크 저장")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("취소") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("저장") { Task { await save() } }
                        // 빈 입력으로 누를 수 있게 두면 "왜 안 되지"를 사용자가 풀어야 한다.
                        .disabled(url.trimmed.isEmpty || saving)
                }
            }
            .overlay {
                if saving { ProgressView() }
            }
        }
        .onAppear {
            // 내용이 아니라 **유무만** 본다 — 알림이 뜨지 않는 호출이다.
            clipboardHasURL = UIPasteboard.general.hasURLs
            urlFocused = true
        }
    }

    private func save() async {
        saving = true
        error = nil
        let failure = await onSave(url.trimmed, note.trimmed)
        saving = false
        if let failure {
            // **닫지 않는다.** 닫아 버리면 사용자가 방금 친 URL을 잃고, 실패했다는
            // 사실도 목록 화면의 토스트로 밀려나 원인과 멀어진다.
            error = failure
        } else {
            dismiss()
        }
    }
}

private extension String {
    var trimmed: String { trimmingCharacters(in: .whitespacesAndNewlines) }
}
