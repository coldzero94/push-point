import SwiftUI

/// 상세 — 카드뉴스.
///
/// 저장한 것을 다시 볼 수단이 없으면 아카이브가 아니라 쓰레기통이다. 원문으로 나가지
/// 않고도 "이게 무슨 글이었는지"를 되짚을 수 있어야 한다.
///
/// 요약(추출식, 2~3문장)을 문장마다 카드로 세운다. **LLM 없이 만든 요약이 제품 표면에
/// 드러나는 유일한 자리**라, 요약 품질이 나빠지면 여기서 바로 보인다 — 실제로 문장이
/// 조각나는 버그를 이 값으로 잡았다.
///
/// `LinkDetail`은 계약에서 `allOf`라 생성기가 `value1`/`value2`로 감싼다. 계약을 납작하게
/// 만드는 대신(Go·TS 생성기는 정상 처리한다) 아래 전달 프로퍼티로 그 이음매를 이 파일
/// 안에 가둔다.
struct LinkDetailView: View {
    let linkID: Int
    let facetOf: (String) -> PP.Facet
    /// 사전 전체 (이름 → facet). 태그 편집이 고를 후보다 — 목록·통계와 같은 출처를 쓴다.
    let dictionary: [String: PP.Facet]

    @EnvironmentObject private var backend: Backend
    @State private var detail: Components.Schemas.LinkDetail?
    @State private var loadError: String?
    @State private var confirmingDelete = false
    @State private var editingTags = false
    /// 편집 중인 메모. detail의 값과 분리해 둔다 — 타이핑 도중 서버 응답이 들어와
    /// 커서가 튀거나 입력이 되돌려지면 안 된다.
    @State private var noteDraft = ""
    /// 저장 실패를 화면 전체 오류로 바꾸지 않는다 — 상세는 멀쩡히 보이는 상태여야 한다.
    @State private var saveError: String?
    @Environment(\.dismiss) private var dismiss

    var body: some View {
        ScrollView {
            if let detail {
                body(for: detail)
            } else if let loadError {
                ContentUnavailableView(t("detail.loadFailed"), systemImage: "exclamationmark.triangle",
                                       description: Text(loadError))
            } else {
                ProgressView().padding(.top, 60)
            }
        }
        .background(PP.Palette.canvas)
        .navigationBarTitleDisplayMode(.inline)
        // 내용을 보고 "이건 아니네" 하는 자리가 여기라, 목록으로 돌아가지 않고
        // 그 자리에서 지울 수 있어야 한다.
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button(role: .destructive) { confirmingDelete = true } label: {
                    Image(systemName: "trash")
                }
                .tint(PP.Palette.danger)
            }
        }
        .confirmationDialog(t("detail.deleteTitle"), isPresented: $confirmingDelete,
                            titleVisibility: .visible) {
            Button(t("common.delete"), role: .destructive) { Task { await delete() } }
            Button(t("common.cancel"), role: .cancel) {}
        }
        .sheet(isPresented: $editingTags) {
            TagEditor(dictionary: dictionary,
                      current: detail?.value1.tags.map(\.name) ?? []) { names in
                Task { await save(tags: names) }
            }
            .pushPointTheme()
        }
        // `id:` 없이 두면 서버가 준비되기 전에 열렸을 때 영원히 스피너가 된다
        // (StatsView 주석 참조).
        .task(id: backend.state) { await load() }
    }

    @ViewBuilder
    private func body(for d: Components.Schemas.LinkDetail) -> some View {
        VStack(alignment: .leading, spacing: 18) {
            header(d)
            summaryCards(d)
            if !d.value2.summary.isEmpty || !d.value1.description.isEmpty {
                Divider().overlay(PP.Palette.line1)
            }
            noteEditor()
            meta(d)
            if let saveError {
                Text(saveError)
                    .font(PP.Typo.meta)
                    .foregroundStyle(PP.Palette.danger)
            }
            openButton(d)
        }
        .padding(.horizontal, 16)
        .padding(.vertical, 20)
    }

    private func header(_ d: Components.Schemas.LinkDetail) -> some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(d.value1.title.isEmpty ? d.value1.domain : d.value1.title)
                .font(PP.Typo.display)
                .tracking(PP.Tracking.display)
                .foregroundStyle(PP.Palette.fg1)
                .fixedSize(horizontal: false, vertical: true)

            // 태그가 없을 때도 이 줄을 그린다. 기계가 못 붙인 링크야말로 손으로 붙일
            // 자리가 필요한데, 비었다고 줄을 숨기면 붙일 방법이 화면에서 사라진다.
            HStack(spacing: 5) {
                ForEach(d.value1.tags, id: \.id) { tag in
                    Chip(name: tag.name, facet: facetOf(tag.name),
                         fill: .init(source: tag.source.rawValue, isActive: false))
                }
                Button { editingTags = true } label: {
                    Label(t(d.value1.tags.isEmpty ? "detail.attachTags" : "detail.editTags"),
                          systemImage: "tag")
                        .font(PP.Typo.label)
                        .foregroundStyle(PP.Palette.fg3)
                }
                .buttonStyle(.plain)
                // **문구가 둘이다** — 태그가 없으면 "태그 붙이기", 있으면 "고치기".
                // 그래서 표시 문구로 겨냥하면 언어뿐 아니라 링크 상태에 따라서도
                // 셀렉터가 달라진다. 식별자는 둘 다에서 같다.
                .accessibilityIdentifier("detail.tags.edit")
            }

            Text(d.value1.domain)
                .font(PP.Typo.metaMono)
                .foregroundStyle(PP.Palette.fg3)
        }
    }

    /// 요약 문장을 각각 카드로. 한 덩어리 문단으로 두면 "요약"이 아니라 그냥 짧은 본문이
    /// 되어, 문장을 골라 뽑았다는 사실이 전달되지 않는다.
    @ViewBuilder
    private func summaryCards(_ d: Components.Schemas.LinkDetail) -> some View {
        let sentences = d.value2.summary
            .split(separator: "\n")
            .map(String.init)
            .filter { !$0.isEmpty }

        if sentences.isEmpty {
            // 요약이 없는 것은 실패가 아니라 정상이다(본문이 얇거나 산문이 부족하거나
            // description과 사실상 같으면 가드가 걸린다). 그 사실을 숨기지 않되,
            // 대신 description을 보여 준다.
            if !d.value1.description.isEmpty {
                Text(d.value1.description)
                    .font(PP.Typo.body)
                    .tracking(PP.Tracking.body)
                    .foregroundStyle(PP.Palette.fg2)
                    .fixedSize(horizontal: false, vertical: true)
            }
        } else {
            VStack(alignment: .leading, spacing: 10) {
                ForEach(Array(sentences.enumerated()), id: \.offset) { index, sentence in
                    HStack(alignment: .top, spacing: 12) {
                        // 순번은 기계 데이터다(R2).
                        Text("\(index + 1)")
                            .font(PP.Typo.metaMono)
                            .foregroundStyle(PP.Palette.fg3)
                            .frame(width: 16, alignment: .trailing)
                        Text(sentence)
                            .font(PP.Typo.body)
                            .tracking(PP.Tracking.body)
                            .foregroundStyle(PP.Palette.fg1)
                            .fixedSize(horizontal: false, vertical: true)
                    }
                    .frame(maxWidth: .infinity, alignment: .leading)
                    .padding(14)
                    .background(PP.Palette.surface)
                    .clipShape(RoundedRectangle(cornerRadius: PP.Radius.card, style: .continuous))
                    .overlay(
                        RoundedRectangle(cornerRadius: PP.Radius.card, style: .continuous)
                            .strokeBorder(PP.Palette.line1, lineWidth: 1)
                    )
                }
            }
        }
    }

    /// 메모.
    ///
    /// 태그가 "무엇에 관한 글인가"라면 메모는 "내가 왜 담았나"다. 후자는 기계가 절대
    /// 만들어 줄 수 없어서, 자동 태깅이 아무리 좋아져도 이 칸은 사람 몫으로 남는다.
    ///
    /// 저장은 편집이 끝날 때 한 번. 글자마다 PATCH하면 폰 안의 서버라 더 잘 보이고,
    /// 그때마다 `updated_at`이 움직여 목록 정렬까지 흔들린다.
    @ViewBuilder
    private func noteEditor() -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Text(t("common.note"))
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.fg3)
            TextField(t("detail.notePlaceholder"), text: $noteDraft, axis: .vertical)
                .font(PP.Typo.body)
                .foregroundStyle(PP.Palette.fg1)
                .lineLimit(1 ... 6)
                .padding(12)
                .background(PP.Palette.surface)
                .clipShape(RoundedRectangle(cornerRadius: PP.Radius.control, style: .continuous))
                .overlay(
                    RoundedRectangle(cornerRadius: PP.Radius.control, style: .continuous)
                        .strokeBorder(PP.Palette.line1, lineWidth: 1)
                )
                .onSubmit { Task { await saveNoteIfChanged() } }
                // 플레이스홀더가 곧 접근성 라벨이라, 그것으로 찾으면 문구를 다듬는
                // 순간 테스트가 칸을 못 찾는다.
                .accessibilityIdentifier("detail.note.field")
            if noteDraft != (detail?.value1.note ?? "") {
                // 아직 안 보냈다는 사실을 숨기지 않는다. 저장 버튼을 따로 두지 않는 대신
                // "언제 저장되는지"는 말해 줘야 한다.
                Button(t("detail.saveNote")) { Task { await saveNoteIfChanged() } }
                    .font(PP.Typo.label)
                    .foregroundStyle(PP.Palette.accent)
                    // 이 버튼의 **존재 여부**가 "초안이 서버 값과 다르다"는 신호라,
                    // UI 테스트가 사라지는 것을 저장 성공의 증거로 쓴다.
                    .accessibilityIdentifier("detail.note.save")
            }
        }
    }

    /// 값이 빈 행은 아예 숨긴다 — 빈칸을 만들지 않는다(§8.1 폴백 규칙).
    private func meta(_ d: Components.Schemas.LinkDetail) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            row(t("inspector.metaSaved"), absoluteTime(d.value1.created_at))
            // **웹 인스펙터에는 "마지막 열람"이 있고 여기엔 없었다.**
            //
            // 기록은 양쪽 다 한다 — iOS도 원문을 열 때 `recordOpen()`으로 `markOpened`를
            // 부른다. 없던 것은 표시뿐이고, 그럴 만한 이유가 있었다: `opened_at`이 계약에
            // `oneOf: [EpochSeconds, "null"]`로 적혀 있어 **swift-openapi-generator가
            // 그 프로퍼티를 통째로 뺐다.** 생성이 성공으로 끝나므로 아무도 몰랐고,
            // 2026-07-30에 `EpochSecondsNullable`로 바꾸면서 비로소 iOS에 생겼다.
            if let opened = d.value2.opened_at { row(t("detail.metaOpened"), absoluteTime(opened)) }
            if !d.value2.author.isEmpty { row(t("detail.metaAuthor"), d.value2.author) }
            // word_count는 계약상 nullable — 스크랩이 세지 못한 경우가 있다.
            if let words = d.value2.word_count, words > 0 {
                row(t("detail.metaLength"),
                    t(words == 1 ? "detail.wordCountOne" : "detail.wordCount", ["n": words]))
            }
            if !d.value2.error.isEmpty {
                row(t("common.error"), d.value2.error, tone: PP.Palette.danger)
            }
        }
    }

    private func row(_ key: String, _ value: String, tone: Color = PP.Palette.fg2) -> some View {
        HStack(alignment: .top, spacing: 12) {
            Text(key)
                .font(PP.Typo.meta)
                .foregroundStyle(PP.Palette.fg3)
                .frame(width: 44, alignment: .leading)
            Text(value)
                .font(PP.Typo.metaMono)
                .foregroundStyle(tone)
        }
    }

    /// 원문 열기 — 여는 동작은 그대로 두고 **열었다는 사실만** 기록한다.
    ///
    /// fire-and-forget이다. 실패해도 재시도하지 않는다 — 저장 경로 밖이고, 계측이
    /// 원문을 여는 흐름을 방해하면 본말이 전도된다.
    private func recordOpen() {
        guard let client = backend.client else { return }
        Task { _ = try? await client.markOpened(.init(path: .init(id: linkID))) }
    }

    private func openButton(_ d: Components.Schemas.LinkDetail) -> some View {
        Link(destination: URL(string: d.value1.url) ?? URL(string: "https://example.com")!) {
            Text(t("common.openOriginal"))
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.onAccent)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 11)
                .background(PP.Palette.accent)
                .clipShape(RoundedRectangle(cornerRadius: PP.Radius.control, style: .continuous))
        }
        .simultaneousGesture(TapGesture().onEnded { recordOpen() })
    }

    private func absoluteTime(_ epoch: Int) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "ko_KR")
        f.dateFormat = "yyyy.MM.dd HH:mm"
        return f.string(from: Date(timeIntervalSince1970: TimeInterval(epoch)))
    }

    /// 삭제 후에는 상세에 머물 이유가 없다 — 목록으로 돌아간다. 목록은 다시 나타날 때
    /// 스스로 갱신하므로(task(id:)) 여기서 목록을 건드리지 않는다.
    private func delete() async {
        guard let client = backend.client else { return }
        do {
            _ = try await client.deleteLink(.init(path: .init(id: linkID))).noContent
            dismiss()
        } catch {
            loadError = error.localizedDescription
        }
    }

    /// 태그 전체 교체. 계약에 부분 추가/삭제가 없어서 항상 최종 배열을 보낸다.
    ///
    /// 서버는 추가분을 `source='manual'`로 넣고 `tag_feedback`에 added/removed를 남긴다 —
    /// **M5 재랭킹의 학습 데이터가 여기서 만들어진다.** 저장이 iOS에서 일어나는데
    /// 고치기가 웹에만 있으면 그 데이터가 정작 저장이 일어나는 기기에서 안 모인다.
    private func save(tags: [String]) async {
        await patch(.init(tags: tags))
    }

    private func saveNoteIfChanged() async {
        guard noteDraft != (detail?.value1.note ?? "") else { return }
        await patch(.init(note: noteDraft))
    }

    private func patch(_ body: Components.Schemas.LinkUpdateInput) async {
        guard let client = backend.client else { return }
        do {
            let out = try await client.updateLink(.init(path: .init(id: linkID), body: .json(body)))
            // 응답이 수정 반영된 상세다 — 다시 GET하지 않는다(계약이 그러라고 준 값이다).
            detail = try out.ok.body.json
            noteDraft = detail?.value1.note ?? ""
            saveError = nil
        } catch {
            saveError = t("detail.saveFailed", ["error": error.localizedDescription])
        }
    }

    private func load() async {
        guard let client = backend.client else { return }
        do {
            let out = try await client.getLink(.init(path: .init(id: linkID)))
            detail = try out.ok.body.json
            // 서버 값이 초안의 출처다. 단, 사용자가 이미 고치고 있었다면 덮지 않는다.
            if noteDraft.isEmpty { noteDraft = detail?.value1.note ?? "" }
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }
}
