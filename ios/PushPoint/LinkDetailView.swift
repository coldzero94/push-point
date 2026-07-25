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

    @EnvironmentObject private var backend: Backend
    @State private var detail: Components.Schemas.LinkDetail?
    @State private var loadError: String?

    var body: some View {
        ScrollView {
            if let detail {
                body(for: detail)
            } else if let loadError {
                ContentUnavailableView("불러오지 못했습니다", systemImage: "exclamationmark.triangle",
                                       description: Text(loadError))
            } else {
                ProgressView().padding(.top, 60)
            }
        }
        .background(PP.Palette.canvas)
        .navigationBarTitleDisplayMode(.inline)
        .task { await load() }
    }

    @ViewBuilder
    private func body(for d: Components.Schemas.LinkDetail) -> some View {
        VStack(alignment: .leading, spacing: 18) {
            header(d)
            summaryCards(d)
            if !d.value2.summary.isEmpty || !d.value1.description.isEmpty {
                Divider().overlay(PP.Palette.line1)
            }
            meta(d)
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

            if !d.value1.tags.isEmpty {
                HStack(spacing: 5) {
                    ForEach(d.value1.tags, id: \.id) { tag in
                        Chip(name: tag.name, facet: facetOf(tag.name),
                             fill: .init(source: tag.source.rawValue, isActive: false))
                    }
                }
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

    /// 값이 빈 행은 아예 숨긴다 — 빈칸을 만들지 않는다(§8.1 폴백 규칙).
    private func meta(_ d: Components.Schemas.LinkDetail) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            row("저장", absoluteTime(d.value1.created_at))
            if !d.value2.author.isEmpty { row("작성", d.value2.author) }
            // word_count는 계약상 nullable — 스크랩이 세지 못한 경우가 있다.
            if let words = d.value2.word_count, words > 0 { row("분량", "\(words)단어") }
            if !d.value2.error.isEmpty { row("오류", d.value2.error, tone: PP.Palette.danger) }
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

    private func openButton(_ d: Components.Schemas.LinkDetail) -> some View {
        Link(destination: URL(string: d.value1.url) ?? URL(string: "https://example.com")!) {
            Text("원문 열기")
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.onAccent)
                .frame(maxWidth: .infinity)
                .padding(.vertical, 11)
                .background(PP.Palette.accent)
                .clipShape(RoundedRectangle(cornerRadius: PP.Radius.control, style: .continuous))
        }
    }

    private func absoluteTime(_ epoch: Int) -> String {
        let f = DateFormatter()
        f.locale = Locale(identifier: "ko_KR")
        f.dateFormat = "yyyy.MM.dd HH:mm"
        return f.string(from: Date(timeIntervalSince1970: TimeInterval(epoch)))
    }

    private func load() async {
        guard let client = backend.client else { return }
        do {
            let out = try await client.getLink(.init(path: .init(id: linkID)))
            detail = try out.ok.body.json
            loadError = nil
        } catch {
            loadError = error.localizedDescription
        }
    }
}
