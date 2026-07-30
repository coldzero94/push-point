import SwiftUI
import UserNotifications

/// 목록 — 시간 척추로 끊은 카드 보드.
///
/// 행이 아니라 카드인 이유(§4.4): 계약이 이미 주는 `description`을 행이 한 글자도 쓰지
/// 않아 화면이 "내가 모은 것"이 아니라 "레코드 목록"으로 읽혔다. 그리고 개인 아카이브에서
/// 회상의 단서는 대개 "언제"이므로 날짜로 끊는다 — keyset 커서가 `(created_at, id)`
/// 정렬이라 페이지 경계와 섹션 경계가 자연스럽게 맞는다.
struct ContentView: View {
    /// 태그 이름 → facet. 탭 컨테이너(RootView)가 받아 두 탭이 나눠 쓴다 —
    /// 탭마다 따로 받으면 같은 태그가 화면마다 다른 색이 될 수 있다.
    let facets: [String: PP.Facet]
    /// 통계에서 넘어온 필터. 켜져 있으면 목록이 좁혀지고 해제 칩이 뜬다.
    @Binding var filter: ListFilter?

    @EnvironmentObject private var backend: Backend
    /// 백그라운드에서는 폴링하지 않는다 — 화면에 없는 목록을 갱신할 이유가 없다.
    @Environment(\.scenePhase) private var scenePhase
    @State private var feed = FeedModel()
    /// 검색 페이지네이션 전용 플래그. **목록과 공유하지 않는다** — 예전에는 하나였고,
    /// 한쪽이 진행 중이면 다른 쪽 onAppear가 조용히 반환한 뒤 다시 불리지 않아
    /// 페이지네이션이 멎었다(리뷰 지적).
    @State private var searchLoadingMore = false
    /// 방금 지운 링크. 되돌릴 수 있는 동안만 들고 있는다.
    @State private var justDeleted: Components.Schemas.Link?
    @State private var undoTask: Task<Void, Never>?
    /// 열어 볼 링크. NavigationLink 대신 이 값으로 이동한다 — 아래 row 주석 참조.
    @State private var opening: OpeningLink?
    /// 검색어. 비어 있으면 평소의 보드, 있으면 검색 결과가 그 자리를 대신한다.
    @State private var query = ""
    @State private var results: [Components.Schemas.Link] = []
    /// 서버가 어느 경로로 찾았는지(fts | like). 두 글자 이하로 친 사용자가 결과가
    /// 적은 이유를 알 수 있게 화면에 남긴다 — 계약이 이걸 알려주는 이유가 그것이다.
    @State private var searchMode: String?
    @State private var searching = false
    /// 검색 실패. **loadError와 따로 둔다** — searchContent는 loadError를 읽지 않고,
    /// 검색이 실패했다고 목록 화면 전체를 오류로 바꾸면 저장한 것이 사라진 것처럼 보인다.
    @State private var searchError: String?
    /// 검색 결과 다음 장 실패. 첫 장 실패와 화면에서 다뤄야 할 자리가 다르다 —
    /// 이쪽은 이미 보이는 결과 아래에 붙는 푸터다.
    @State private var searchMoreError: String?
    /// 다음 페이지 커서. nil이면 마지막 페이지다 — 계약이 그렇게 말한다.
    /// **페이지 번호가 아니다.** keyset 커서라 목록에 쓰기가 일어나도 항목이 건너뛰거나
    /// 중복되지 않는다(.claude/rules/ios.md).
    @State private var searchCursor: String?
    /// 다음 장을 이미 받고 있는지. 없으면 마지막 카드가 화면에 머무는 동안 같은 요청이
    /// 여러 번 나간다 — onAppear는 스크롤 중 여러 번 불린다.
    /// **변경(삭제·재시도·되돌리기) 실패 전용 채널.**
    ///
    /// 예전에는 이것들이 `loadError`에 썼는데, 그러면 화면 전체가
    /// "목록을 불러오지 못했습니다 / wifi.exclamationmark"로 바뀐다 — 목록은 멀쩡히
    /// 불러왔고 삭제만 실패한 것인데 **저장한 것이 전부 사라진 것처럼 보인다.**
    /// 검색 쪽은 같은 이유로 이미 채널을 나눠 뒀다(searchError 주석).
    @State private var actionError: String?
    /// 확장이 알림을 못 띄우고 버린 횟수. 0보다 크면 배너로 알린다.
    @State private var droppedNotices = 0
    /// 알림 권한 상태. **아직 안 물어봤는가 / 거부됐는가**로 배너의 동작이 갈린다.
    @State private var notifyStatus: UNAuthorizationStatus = .notDetermined
    /// 목록 밀도. 기기에 남는다 — 매번 고르게 하면 그건 선택지가 아니라 잡일이다.
    @AppStorage("pushpoint.density") private var density: ListDensity = .card
    /// 앱 안에서 링크를 저장하는 시트. 공유 시트만 있던 시절에는 앱을 켜 놓고도
    /// 링크를 넣을 방법이 없었다(SaveSheet 주석).
    @State private var saving = false

    var body: some View {
        NavigationStack {
            // **배너는 `safeAreaInset`으로 붙인다 — `VStack`의 형제로 두면 안 된다.**
            //
            // `.searchable`의 검색 필드는 네비게이션 바 **안**에 살고, 그 바는 스크롤
            // 최상단에서 투명해진다. 형제로 두면 바가 늘어나도 배너가 따라 움직이지 않아,
            // **당겨서 새로고침할 때 검색 필드와 같은 자리를 차지했다.** `safeAreaInset`은
            // 배너를 바 아래에 명시적으로 놓고 그만큼 안전 영역을 줄여 그 겹침을 없앤다.
            // 배너가 없을 때는 EmptyView라 0높이다.
            //
            // **헤더가 경고색으로 물들던 것은 이걸로 낫지 않았다** — 원인이 다르다.
            // `notificationBanner`의 배경 주석을 볼 것.
            content
                .safeAreaInset(edge: .top, spacing: 0) { notificationBanner }
                .background(PP.Palette.canvas)
                .navigationTitle("Push-Point")
                .toolbar {
                    // 밀도 전환. 아이콘 하나로 토글한다 — 상태가 둘뿐인데 메뉴를
                    // 열게 하면 두 번 눌러야 한다.
                    ToolbarItem(placement: .topBarTrailing) {
                        Button {
                            density = density.next
                        } label: {
                            Image(systemName: density.next.symbol)
                        }
                        .accessibilityLabel("\(density.next.label)로 보기")
                        // 조밀 모드를 테스트가 누를 수 있어야 한다 — 없던 동안 그 모드에
                        // 자동 게이트가 0이었다.
                        .accessibilityIdentifier("density.toggle")
                    }
                    ToolbarItem(placement: .topBarTrailing) {
                        Button { saving = true } label: { Image(systemName: "plus") }
                            .accessibilityLabel("링크 저장")
                    }
                }
            .sheet(isPresented: $saving) {
                SaveSheet(onSave: saveLink)
            }
            .navigationDestination(item: $opening) { target in
                LinkDetailView(linkID: target.id,
                               facetOf: { facets[$0] ?? .neutral },
                               dictionary: facets)
            }
        }
        // 검색은 목록 안에 있다 — 별도 탭으로 빼지 않았다. 찾는 대상이 바로 이 목록이고,
        // 탭을 나누면 "필터가 걸린 목록"과 "검색 결과"라는 거의 같은 두 화면을 사용자가
        // 구분해 가며 써야 한다. `.searchable`은 iOS가 이미 가르쳐 둔 자리이기도 하다.
        .searchable(text: $query, prompt: "제목 · 메모 · 태그")
        // **진행 중인 링크가 있는 동안만 폴링한다.**
        //
        // 이 앱의 핵심 주장은 "저장하면 3초 안에 분류된다"인데, iOS에는 폴러가 없어서
        // 그 순간이 화면에 나타나지 않았다(2026-07-29 발견). 공유 시트로 저장한 링크가
        // 도메인만 적힌 빈 카드로 남아 있다가, 사용자가 손으로 당겨 새로고침해야
        // 제목·태그·커버가 채워졌다 — 제품이 하는 일 중 가장 중요한 것이 보이지 않았다.
        // 디자인 시스템 §1.4 S2가 "폴러가 링크를 갱신하면 슬롯이 켜진다"고 규정해 둔
        // 그 폴러다.
        //
        // 타이머가 아니라 **상태 조건**이다: 종단이 아닌 링크가 하나라도 있으면 돌고,
        // 전부 끝나면 스스로 멈춘다. 그래서 아무 일도 없는 아카이브에서는 요청이 0이다.
        .task(id: feed.pollKey) { await pollWhileWorking() }
        // 앱이 앞으로 나올 때마다 다시 본다 — 확장은 앱이 없는 동안 돈다.
        .task(id: scenePhase) {
            guard scenePhase == .active else { return }
            await refreshNotifyState()
        }
        .task(id: backend.state) { await load() }
        .task(id: filter) { await load() }
        // 타이핑마다 요청을 보내지 않는다. 한 글자마다 FTS를 때리면 폰 안에서 도는
        // 서버라 더 잘 보인다 — 입력이 멎은 뒤에 한 번만 간다.
        .task(id: query) { await runSearch() }
        // 확인창 대신 되돌리기. 확인창은 **모든** 삭제를 느리게 만들어 흔한 경우에
        // 세금을 매기고, 서버가 소프트 삭제라 되살릴 수단이 실제로 있다.
        .overlay(alignment: .bottom) {
            // 되돌리기 토스트가 있으면 그것이 우선이다 — 되돌리기는 시간 제한이 있는
            // 동작이고, 오류 문구는 그 위에 겹치지 않아야 한다.
            if let link = justDeleted {
                UndoToast(message: "삭제했습니다") { Task { await undo(link) } }
            } else if let actionError {
                // **변경 실패는 여기서 말한다.** loadError로 보내면 화면 전체가
                // "목록을 불러오지 못했습니다"가 되어 링크가 사라진 것처럼 보인다.
                UndoToast(message: actionError, actionLabel: "닫기", isError: true) { self.actionError = nil }
            }
        }
        .animation(.smooth(duration: 0.25), value: justDeleted?.id)
        .animation(.smooth(duration: 0.25), value: actionError)
        // 오류 토스트는 스스로 사라진다 — 사용자가 닫아야만 없어지면 다음 조작을 막는다.
        .task(id: actionError) {
            guard actionError != nil else { return }
            try? await Task.sleep(for: .seconds(5))
            if !Task.isCancelled { actionError = nil }
        }
    }

    /// 공유 시트로 저장한 결과를 사용자가 **볼 수 없는 상태**임을 알린다.
    ///
    /// 확장은 화면을 그리지 않고 즉시 닫히므로 알림이 유일한 통로다. 권한이 없으면
    /// 성공·중복·실패가 전부 "아무 일도 없음"으로 똑같이 보인다 — 저장된 것과 잃은 것을
    /// 구분할 수 없다는 뜻이고, 아카이브에서 가장 나쁜 실패 방식이다.
    @ViewBuilder
    private var notificationBanner: some View {
        if droppedNotices > 0 {
            Button {
                Task {
                    // **아직 안 물어봤으면 여기서 물어본다.** 설정 앱으로 내보내는 것은
                    // 이미 거부한 사용자에게만 맞는 동작이고, 그 경우에도 iOS가 어디로
                    // 떨어뜨릴지는 앱이 정하지 못한다(권한을 한 번도 요청하지 않은 앱은
                    // 설정에 항목 자체가 없어 최상위로 간다 — 실제로 그렇게 나왔다).
                    //
                    // notDetermined에서 시스템 프롬프트를 띄우는 쪽이 **한 번에 끝난다.**
                    if notifyStatus == .notDetermined {
                        await SaveNotifier.requestAuthorization()
                        await refreshNotifyState()
                        return
                    }
                    let target = URL(string: UIApplication.openNotificationSettingsURLString)
                        ?? URL(string: UIApplication.openSettingsURLString)
                    if let target { await UIApplication.shared.open(target) }
                }
            } label: {
                HStack(spacing: 8) {
                    Image(systemName: "bell.slash.fill").font(PP.Typo.label)
                    Text("알림이 꺼져 있어 공유 저장 결과를 알 수 없습니다 (\(droppedNotices)건)")
                        .font(PP.Typo.label)
                        .multilineTextAlignment(.leading)
                    Spacer(minLength: 4)
                    Text("설정 열기").font(PP.Typo.label).underline()
                }
                .foregroundStyle(PP.Palette.fg1)
                .padding(.horizontal, 14)
                .padding(.vertical, 12)
                .frame(maxWidth: .infinity, alignment: .leading)
                // **화면 끝까지 채우지 않는다.** 전면 밴드였을 때 두 가지가 깨졌다.
                //
                // 네비게이션 바는 최상단에서 투명해지며 **바로 아래 콘텐츠의 배경을 위로
                // 확장한다.** 배너가 전면이면 그 `warnTint`가 제목·검색 필드·툴바까지 칠해서,
                // 한 줄짜리 알림 때문에 화면 전체가 경고 상태로 읽혔다. `warn`은 예약된 상태
                // hue이므로 hue를 쓴 것 자체는 위반이 아니다 — 깨진 것은 **형태** 쪽이고,
                // 경계를 잃은 색은 자기 진술의 범위를 잃는다.
                //
                // 관측: 배너 없는 통계 탭 헤더 `canvas`(#DEF0E8) vs 목록 탭 `warnTint`
                // (#FDF3E2). 통제된 비교는 아니다 — StatsView에는 `.searchable`도 없다.
                // `safeAreaInset`으로 옮겨도 그대로였던 것이 더 강한 근거다.
                //
                // **정당화를 지어내지 않는다.** 처음에는 "R1의 채움 예산을 넘는다"고 적었는데
                // R1에 면적 예산은 없고, §2.1.2는 오히려 `--warn-tint`의 용처를 "경고 배너
                // 배경"으로 §2.1.5는 "API 키 미설정 배너"로 **명시 승인**한다. 전면 배너
                // 자체는 명세가 허용한 형태다. 문제는 색의 양이 아니라 **가장자리를 물어서
                // 경계를 잃는 것**이었고, 카드로 들이는 이유도 그것 하나다 —
                // R1이 상태에 "예약된 hue + 형태 + 문장" 셋을 함께 요구하는 이유다.
                .background(PP.Palette.warnTint, in: .rect(cornerRadius: PP.Radius.card))
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("notice.notifications")
            // **여백은 버튼 밖이다.** 안에 두면 `.buttonStyle(.plain)`에서 패딩까지
            // 히트 영역과 접근성 프레임에 들어가, 카드를 노리고 옆 여백을 누른 손가락이
            // 알림 설정을 연다.
            .padding(.horizontal, 16)
            .padding(.bottom, 12)
            // **그리고 그 여백을 칠한다.** 안 칠하면 좌우 16pt와 아래 12pt가 투명한 채로
            // safeAreaInset 영역에 남는데, 그 영역은 스크롤 뷰의 인셋만 줄일 뿐 bounds는
            // 그대로라 카드가 밑을 지나간다 — **그 틈으로 카드가 비친다.** 아래 구간
            // 머리글에서 고친 것과 같은 결함이고("인셋을 남겨 두면 좌우 16pt가 안 칠해져
            // 그 틈으로 계속 비친다"), 같은 diff 안에서 한쪽만 고칠 뻔했다.
            .background(PP.Palette.canvas)
        }
    }

    @ViewBuilder
    private var content: some View {
        switch backend.state {
        case .idle, .starting:
            ProgressView("서버 시작 중…").padding(.top, 60)
        case let .failed(message):
            ContentUnavailableView("서버를 시작하지 못했습니다",
                                   systemImage: "exclamationmark.triangle",
                                   description: Text(message))
        case .running:
            if !query.isEmpty {
                searchContent
            } else if let loadError = feed.loadError {
                // 빈/오류 상태도 스크롤 뷰에 담는다. ContentUnavailableView는 스크롤이
                // 아니라서 그대로 두면 **바로 그때** — 다시 시도하고 싶은 순간 — 당겨서
                // 새로고침이 안 된다.
                refreshableState {
                    ContentUnavailableView("목록을 불러오지 못했습니다",
                                           systemImage: "wifi.exclamationmark",
                                           description: Text(loadError))
                }
            } else if feed.links.isEmpty {
                refreshableState {
                    // **액션이 있는 빈 상태다.** 예전에는 "공유 시트로 보내면 쌓입니다"만
                    // 적혀 있어서, 읽고 나서 할 수 있는 일이 앱을 나가는 것뿐이었다.
                    ContentUnavailableView {
                        Label("아직 저장한 링크가 없습니다", systemImage: "tray")
                    } description: {
                        Text("다른 앱에서 공유 시트로 보내거나, 여기서 주소를 붙여넣어 저장하세요.")
                    } actions: {
                        Button("링크 저장") { saving = true }
                            .buttonStyle(.borderedProminent)
                            .tint(PP.Palette.accent)
                    }
                }
            } else {
                board
            }
        }
    }

    /// 검색 결과.
    ///
    /// 결과도 **같은 카드**로 그린다. 검색이 따로 생긴 화면이 아니라 같은 보드를 좁혀 본
    /// 것이라는 감각이 유지돼야 하고, 카드가 이미 제목·설명·태그·커버를 다 보여주므로
    /// 검색 전용 행을 새로 만들 이유가 없다.
    ///
    /// 다만 날짜 척추는 없앤다. 검색 결과의 정렬은 bm25 관련도이지 시간이 아니라서,
    /// 시간으로 끊으면 있지도 않은 시간 순서를 주장하게 된다.
    @ViewBuilder
    private var searchContent: some View {
        if searching && results.isEmpty {
            refreshableState { ProgressView().padding(.top, 60) }
        } else if let searchError {
            // **빈 결과보다 먼저 본다.** 순서가 뒤바뀌면 실패가 "결과가 없습니다"라는
            // 단정문으로 위장되고, 그건 사용자에게 "이 검색어로는 저장한 게 없다"는
            // 거짓말이 된다 — 아카이브에서 가장 나쁜 실패 방식이다.
            refreshableState {
                ContentUnavailableView {
                    Label("검색하지 못했습니다", systemImage: "wifi.exclamationmark")
                } description: {
                    Text(searchError)
                } actions: {
                    Button("다시 시도") { Task { await runSearch(debounce: false) } }
                }
            }
        } else if results.isEmpty {
            refreshableState {
                // 빈 결과에도 나갈 길을 준다. 검색어를 지우는 것이 유일하게 확실한
                // 다음 동작인데, 그걸 사용자가 스스로 알아내게 두면 화면이 막힌다.
                ContentUnavailableView {
                    Label("결과가 없습니다", systemImage: "magnifyingglass")
                } description: {
                    Text("\u{201C}\(query)\u{201D}와 맞는 링크를 찾지 못했습니다.")
                } actions: {
                    Button("검색어 지우기") { query = "" }
                }
            }
        } else {
            List {
                if let mode = searchMode, mode == "like" {
                    // 세 글자 미만은 FTS가 아니라 LIKE로 간다(계약). 결과가 적을 때
                    // 사용자가 "없구나"가 아니라 "더 치면 되는구나"로 읽어야 한다.
                    Text("두 글자 이하는 제목·메모만 훑습니다. 세 글자부터 전문 검색으로 바뀝니다.")
                        .font(PP.Typo.meta)
                        .foregroundStyle(PP.Palette.fg3)
                        .plainRow(top: 10, bottom: 2)
                }
                ForEach(results, id: \.id) { link in
                    row(link)
                        .plainRow(top: 5, bottom: 5)
                        .onAppear {
                            if link.id == results.last?.id { Task { await searchMore() } }
                        }
                }
                // 다음 장 실패는 화면상 "여기가 끝"과 구분되지 않는다. 커서는 보존돼
                // 있으므로 같은 자리에서 그대로 재시도된다.
                if let searchMoreError {
                    HStack(spacing: 10) {
                        Text(searchMoreError)
                            .font(PP.Typo.meta)
                            .foregroundStyle(PP.Palette.danger)
                        Button("다시 시도") { Task { await searchMore() } }
                            .font(PP.Typo.label)
                            .foregroundStyle(PP.Palette.accent)
                        Spacer()
                    }
                    .plainRow(top: 10, bottom: 10)
                }
            }
            .listStyle(.plain)
            .scrollContentBackground(.hidden)
            .refreshable { await runSearch() }
        }
    }

    /// 스크롤되지 않는 상태 화면을 당길 수 있게 감싼다. 내용이 화면보다 짧아도
    /// 제스처가 잡히도록 최소 높이를 준다.
    private func refreshableState<Content: View>(@ViewBuilder _ content: () -> Content)
        -> some View {
        ScrollView {
            content()
                .frame(maxWidth: .infinity, minHeight: 420)
        }
        .scrollBounceBehavior(.always)
        .refreshable { await load() }
    }

    /// 보드를 `List`로 만든다. 카드 모양은 그대로지만 스와이프 액션과 셀 재활용이
    /// 딸려 온다 — §8.4가 행 액션으로 `.swipeActions`를 지정하고 있고, 링크 10만 건이
    /// 목표라 재활용도 공짜로 얻을 이유가 없다. List가 기본으로 얹는 구분선·배경·여백은
    /// 전부 걷어내야 보드처럼 보인다.
    private var board: some View {
        List {
            if let filter {
                filterBar(filter).plainRow(top: 8, bottom: 0)
            }
            ForEach(sections, id: \.title) { section in
                Section {
                    ForEach(section.links, id: \.id) { link in
                        row(link)
                            .plainRow(top: 5, bottom: 5)
                            // 마지막 카드가 보이면 다음 장을 받는다. **날짜 구간이 아니라
                            // 전체 목록의 마지막**을 기준으로 삼는다 — 구간 단위로 보면
                            // "오늘" 섹션 끝에서도 발동해 아직 필요 없는 장을 당겨 온다.
                            .onAppear {
                                if link.id == feed.links.last?.id { Task { await loadMore() } }
                            }
                    }
                } header: {
                    // **불투명 행이어야 한다** — 헤더는 고정되고 카드가 그 밑으로 흐른다.
                    spine(section).plainHeaderRow(top: 14, bottom: 6)
                }
            }
        }
        .listStyle(.plain)
        .scrollContentBackground(.hidden)
        .environment(\.defaultMinListHeaderHeight, 0)
        // 당겨서 새로고침은 **스크롤 뷰 자신**에 붙어야 한다. 바깥 컨테이너에 걸면
        // 목록이 비어 있을 때(ContentUnavailableView는 스크롤이 아니다) 제스처를 받을
        // 대상이 없어 조용히 아무 일도 일어나지 않는다.
        .refreshable { await load() }
    }

    /// `NavigationLink` 대신 버튼 + `navigationDestination`을 쓴다.
    ///
    /// List 안의 NavigationLink는 오른쪽에 화살표(disclosure indicator)를 붙이는데,
    /// 그건 **행의 어휘이지 카드의 어휘가 아니다** — §4.4의 카드 명세에 화살표는 없고,
    /// 카드는 그 자체가 탭 대상이라 "여기를 눌러라" 표시가 따로 필요 없다. 게다가
    /// 화살표는 카드 바깥에 떠서 카드와 분리돼 보인다.
    ///
    /// 스와이프가 있다는 힌트를 대신 넣지는 않았다. 목록을 옆으로 미는 것은 iOS에서
    /// 이미 배워진 동작이고, 발견되지 않는 경우를 위해 길게 누르기(컨텍스트 메뉴)를
    /// 함께 뒀다. 힌트 UI를 더하면 그건 화면에 상시로 남는 비용인데, 한 번 배우면
    /// 다시 필요 없는 정보에 그 자리를 내줄 이유가 없다(§1.3이 온보딩 투어를 금지하는 것과 같은 판단).
    private func row(_ link: Components.Schemas.Link) -> some View {
        Button {
            opening = OpeningLink(id: link.id)
        } label: {
            LinkCard(link: link,
                     facetOf: { facets[$0] ?? .neutral },
                     activeTag: activeTagName,
                     resolveThumb: backend.absoluteURL,
                     // 실패 복구를 스와이프에만 두지 않는다 — 발견되지 않는 동작이라
                     // 그 링크가 영원히 실패로 남는다(§4.7).
                     onRetry: { Task { await retry(link) } },
                     density: density,
                     // 보드는 시간 척추로 끊는다 — 머리글이 이미 날을 말했다.
                     dayStated: true)
        }
        .buttonStyle(.plain)
        // 스와이프와 길게 누르기 둘 다 둔다(§8.4). 스와이프는 빠르지만 발견되지 않고,
        // 컨텍스트 메뉴는 느리지만 항상 찾을 수 있다.
        // 끝까지 밀면 버튼을 거치지 않고 바로 지운다 — 메시지 앱과 같은 동작이고,
        // 손이 이미 그렇게 배워 있다. 안전망은 아래 되돌리기 토스트다.
        .swipeActions(edge: .trailing, allowsFullSwipe: true) {
            Button(role: .destructive) { Task { await delete(link) } } label: {
                Label("삭제", systemImage: "trash")
            }
        }
        // 방향으로 성격을 나눈다. **오른쪽 끝은 파괴적인 것만** — iOS 전반의 관용이고,
        // 그래야 손이 기억한 방향이 다른 화면에서 배신하지 않는다.
        //
        // 왼쪽 끝은 되돌릴 수 있는 것. 정상 링크에는 **공유**를 둔다 — 원문 열기는
        // 카드를 눌러 들어간 상세의 기본 버튼과 같은 동작이라 여기 두면 중복이고,
        // "저장한 것을 남에게 보낸다"는 목록에서만 할 수 있는 다른 동작이다.
        // 실패한 링크는 공유할 내용 자체가 없으므로 재시도가 그 자리를 가져간다.
        .swipeActions(edge: .leading, allowsFullSwipe: false) {
            if link.status == .failed {
                Button { Task { await retry(link) } } label: {
                    Label("재시도", systemImage: "arrow.clockwise")
                }
                .tint(PP.Palette.warn)
            } else if let url = URL(string: link.url) {
                ShareLink(item: url) { Label("공유", systemImage: "square.and.arrow.up") }
                    .tint(PP.Palette.accent)
            }
        }
        .contextMenu {
            if let url = URL(string: link.url) {
                Link(destination: url) { Label("원문 열기", systemImage: "safari") }
                    .simultaneousGesture(TapGesture().onEnded { recordOpen(link) })
                ShareLink(item: url) { Label("공유", systemImage: "square.and.arrow.up") }
            }
            Button(role: .destructive) { Task { await delete(link) } } label: {
                Label("삭제", systemImage: "trash")
            }
        }
    }

    /// 켜진 필터를 화면에 남긴다. 통계에서 넘어왔는데 목록이 조용히 좁아져 있으면
    /// 사용자는 링크가 사라졌다고 읽는다 — 무엇이 걸려 있는지와 푸는 법이 같이 있어야 한다.
    private func filterBar(_ active: ListFilter) -> some View {
        HStack(spacing: 8) {
            Text(label(for: active))
                .font(PP.Typo.label)
                .foregroundStyle(PP.Palette.fg2)
            Button { filter = nil } label: {
                Image(systemName: "xmark.circle.fill")
                    .font(PP.Typo.label)
                    .foregroundStyle(PP.Palette.fg3)
            }
            .buttonStyle(.plain)
            Spacer()
        }
        .padding(.horizontal, 11)
        .padding(.vertical, 7)
        .background(PP.Palette.hover)
        .clipShape(Capsule())
        .overlay(Capsule().strokeBorder(PP.Palette.lineControl, lineWidth: 1))
    }

    private func label(for active: ListFilter) -> String {
        switch active {
        case let .tag(name): "태그: \(name)"
        case .failed: "수집 실패만"
        }
    }

    /// 시간 척추 — serif 머리글 + 건수 + 하한선. serif가 쓰이는 유일한 자리다(§2.2.5).
    private func spine(_ section: DaySection) -> some View {
        HStack(alignment: .firstTextBaseline, spacing: 9) {
            Text(section.title)
                .font(PP.Typo.spine)
                .tracking(PP.Tracking.spine)
                .foregroundStyle(PP.Palette.fg1)
            Text("\(section.links.count)")
                .font(PP.Typo.metaMono)
                .foregroundStyle(PP.Palette.fg3)
            Rectangle().fill(PP.Palette.line1).frame(height: 1)
        }
    }

    private var activeTagName: String? {
        if case let .tag(name) = filter { return name }
        return nil
    }

    /// navigationDestination(item:)이 Identifiable을 요구해서 id만 감싼다.
    private struct OpeningLink: Identifiable, Hashable {
        let id: Int
    }

    // MARK: - 구간

    private struct DaySection {
        let title: String
        let links: [Components.Schemas.Link]
    }

    /// 오늘 · 어제 · 이번 주 · 이전. 절대 날짜가 아니라 상대 구간인 이유는, 찾을 때
    /// 떠오르는 것이 "며칠 전"이지 "7월 12일"이 아니기 때문이다.
    private var sections: [DaySection] {
        let cal = Calendar.current
        let now = Date()
        var buckets: [(String, [Components.Schemas.Link])] = [
            ("오늘", []), ("어제", []), ("이번 주", []), ("이전", []),
        ]
        for link in feed.links {
            let date = Date(timeIntervalSince1970: TimeInterval(link.created_at))
            let index: Int
            if cal.isDateInToday(date) {
                index = 0
            } else if cal.isDateInYesterday(date) {
                index = 1
            } else if let days = cal.dateComponents([.day], from: cal.startOfDay(for: date),
                                                     to: cal.startOfDay(for: now)).day, days < 7 {
                // **달력 하루 단위로 센다.** 앞의 두 분기가 isDateInToday/isDateInYesterday라
                // 달력 기준인데 여기만 경과 시간으로 재면 축이 섞인다 — 6일 2시간 전 링크가
                // 달력으로는 7일 차이라 "이번 주"와 "이전" 사이에서 경계가 어긋난다.
                index = 2
            } else {
                index = 3
            }
            buckets[index].1.append(link)
        }
        return buckets.filter { !$0.1.isEmpty }.map { DaySection(title: $0.0, links: $0.1) }
    }

    /// 수집에 실패한 링크를 다시 큐에 넣는다. 실패는 통계가 아니라 할 일이라(통계 탭의
    /// "손이 필요한 것"과 같은 판단), 목록에서 바로 손댈 수 있어야 한다.
    private func retry(_ link: Components.Schemas.Link) async {
        guard let client = backend.client else { return }
        do {
            // `try`는 400·404에서 던지지 않는다(APIOutcome). 분기하지 않으면
            // "이미 실패 상태가 아님"·"없는 링크"가 성공으로 읽힌다.
            switch try await client.retryLink(.init(path: .init(id: link.id))) {
            case .accepted:
                // 상태가 pending으로 돌아가면 레일이 다시 켜지므로 목록을 새로 받는다.
                await load()
            case let .badRequest(r):
                actionError = APIOutcome.message(try? r.body.json, fallback: "다시 시도할 수 없는 링크입니다.")
            case .notFound:
                actionError = "링크를 찾을 수 없습니다."
            case .unauthorized:
                actionError = "인증에 실패했습니다."
            case let .internalServerError(r):
                actionError = APIOutcome.message(try? r.body.json, fallback: "다시 시도하지 못했습니다.")
            case let .undocumented(statusCode, _):
                actionError = "다시 시도하지 못했습니다 (HTTP \(statusCode))."
            }
        } catch {
            actionError = error.localizedDescription
        }
    }

    /// 삭제 후 목록을 다시 받지 않고 **그 자리에서 빼는** 이유: 재조회는 왕복이 있어
    /// 방금 지운 카드가 잠깐 남아 있고, 그 사이 다시 누르면 404가 난다.
    private func delete(_ link: Components.Schemas.Link) async {
        guard let client = backend.client else { return }
        do {
            _ = try await client.deleteLink(.init(path: .init(id: link.id))).noContent
            withAnimation(.smooth(duration: 0.25)) {
                apply(.removed(link.id))
            }
            justDeleted = link
            // 되돌리기 창을 닫는 타이머. 새로 지우면 앞의 타이머는 취소된다 —
            // 그러지 않으면 먼저 걸린 타이머가 나중 토스트를 지운다.
            undoTask?.cancel()
            undoTask = Task {
                try? await Task.sleep(for: .seconds(5))
                guard !Task.isCancelled else { return }
                if justDeleted?.id == link.id { justDeleted = nil }
            }
        } catch {
            actionError = error.localizedDescription
        }
    }

    /// 되돌리기는 같은 URL을 다시 저장하는 것이다 — store가 소프트 삭제된 행을 만나면
    /// undelete한다(별도 복구 엔드포인트가 없다). 단, 그 경로는 링크를 pending으로
    /// 되돌리고 다시 스크랩하므로 **태그·요약은 새로 만들어진다**. 링크가 돌아오는 것이
    /// 요점이고 그 값들은 어차피 파생물이라 받아들일 만하다.
    /// 시트에서 온 저장. 실패하면 **문구를 돌려주고 시트를 열어 둔다** — 닫아 버리면
    /// 방금 친 주소를 잃는다.
    private func saveLink(_ url: String, _ note: String) async -> String? {
        guard let client = backend.client else { return "서버가 아직 준비되지 않았습니다." }
        do {
            // **Output을 반드시 분기한다.** `try`는 400·401·500에서 던지지 않는다
            // (APIOutcome 주석). 예전에는 여기서 바로 성공 처리해서, 잘못된 주소를
            // 붙여넣으면 시트가 조용히 닫히고 아무 일도 일어나지 않았다.
            let out = try await client.createLink(.init(body: .json(
                .init(url: url, note: note.isEmpty ? nil : note))))
            switch out {
            case .created, .ok: // .ok는 이미 저장된 URL(중복) — 사용자에겐 성공이다
                await load()
                return nil
            case let .badRequest(r):
                return APIOutcome.message(try? r.body.json, fallback: "주소를 확인해 주세요.")
            case .unauthorized:
                return "인증에 실패했습니다."
            case let .internalServerError(r):
                return APIOutcome.message(try? r.body.json, fallback: "서버에서 저장하지 못했습니다.")
            case let .undocumented(statusCode, _):
                return "저장하지 못했습니다 (HTTP \(statusCode))."
            }
        } catch {
            return error.localizedDescription
        }
    }

    /// 알림 권한 상태와 버려진 알림 수를 다시 읽는다.
    private func refreshNotifyState() async {
        notifyStatus = await SaveNotifier.status()
        droppedNotices = AppGroup.defaults?.integer(forKey: SaveNotifier.droppedKey) ?? 0
        // 권한이 다시 켜졌으면 배너를 치우고 카운트도 비운다.
        if droppedNotices > 0, await SaveNotifier.canNotify() {
            AppGroup.defaults?.set(0, forKey: SaveNotifier.droppedKey)
            droppedNotices = 0
        }
    }

    // MARK: - 피드 (FeedModel 위임)
    //
    // 뷰가 client·filter를 알고 모델은 모른다. 그래서 모델은 스텁 클라이언트로 테스트할
    // 수 있고, 뷰는 여전히 backend 상태 하나만 확인하면 된다.

    private func load() async {
        guard case .running = backend.state, let client = backend.client else { return }
        await feed.load(client, filter: filter)
    }

    private func loadMore() async {
        guard case .running = backend.state, let client = backend.client, feed.hasMore else { return }
        await feed.loadMore(client, filter: filter)
    }

    private func pollRefresh() async {
        guard case .running = backend.state, let client = backend.client else { return }
        await feed.pollRefresh(client, filter: filter)
    }

    /// **진행 중인 링크가 있는 동안만 폴링한다.** 타이머가 아니라 상태 조건이라
    /// 전부 끝나면 스스로 멈추고, 아무 일 없는 아카이브에서는 요청이 0이다.
    private func pollWhileWorking() async {
        guard feed.hasWorkInFlight else { return }
        while !Task.isCancelled, scenePhase == .active {
            try? await Task.sleep(for: .milliseconds(1500))
            if Task.isCancelled { return }
            await pollRefresh()
            if !feed.hasWorkInFlight { return }
        }
    }

    /// 변화를 **화면에 있는 모든 목록에** 반영한다. 보드는 모델이, 검색 결과는 여기가 든다.
    private func apply(_ change: LinkChange) {
        feed.apply(change)
        switch change {
        case let .removed(id): results.removeAll { $0.id == id }
        case let .replaced(l):
            if let i = results.firstIndex(where: { $0.id == l.id }) { results[i] = l }
        }
    }

    private func undo(_ link: Components.Schemas.Link) async {
        guard let client = backend.client else {
            actionError = "서버가 아직 준비되지 않았습니다."
            return
        }
        undoTask?.cancel()
        // **토스트를 미리 지우지 않는다.** 예전에는 요청 전에 justDeleted를 비웠고,
        // 요청이 실패하면 토스트도 링크도 사라져서 **되살릴 방법이 아예 없었다** —
        // URL이 화면 어디에도 남지 않아 손으로 다시 저장할 수도 없었다.
        do {
            switch try await client.createLink(.init(body: .json(.init(url: link.url)))) {
            case .created, .ok:
                justDeleted = nil
                await load()
            case let .badRequest(r):
                actionError = APIOutcome.message(try? r.body.json, fallback: "되돌리지 못했습니다.")
            case .unauthorized:
                actionError = "인증에 실패했습니다."
            case let .internalServerError(r):
                actionError = APIOutcome.message(try? r.body.json, fallback: "되돌리지 못했습니다.")
            case let .undocumented(statusCode, _):
                actionError = "되돌리지 못했습니다 (HTTP \(statusCode))."
            }
        } catch {
            actionError = error.localizedDescription
        }
    }

    /// 열람 기록 — fire-and-forget. 실패는 무시한다(계측이 흐름을 막으면 안 된다).
    private func recordOpen(_ link: Components.Schemas.Link) {
        guard let client = backend.client else { return }
        Task { _ = try? await client.markOpened(.init(path: .init(id: link.id))) }
    }

    // MARK: - 검색

    /// 입력이 멎은 뒤에 한 번만 요청한다.
    ///
    /// `.task(id:)`는 id가 바뀌면 이전 작업을 **취소**하므로, 앞에 슬립을 두면 그것만으로
    /// 디바운스가 된다 — 타이머도 상태도 필요 없다. 취소된 작업은 슬립에서 죽고 요청까지
    /// 가지 않는다.
    private func runSearch(debounce: Bool = true) async {
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else {
            results = []
            searchMode = nil
            searchCursor = nil
            searchError = nil
            searchMoreError = nil
            return
        }
        if debounce {
            do {
                try await Task.sleep(for: .milliseconds(250))
            } catch {
                return // 취소됨 — 다음 글자가 들어왔다
            }
        }
        guard case .running = backend.state, let client = backend.client else { return }
        searching = true
        defer { searching = false }
        do {
            var q2 = Operations.search.Input.Query(q: q, limit: 50)
            // 태그 필터가 걸려 있으면 검색에도 그대로 적용한다 — 화면에 필터 칩이 떠
            // 있는데 검색만 그걸 무시하면 사용자가 본 것과 다른 결과가 나온다.
            if case let .tag(name) = filter { q2.tag = name }
            let page = try await client.search(.init(query: q2)).ok.body.json
            // SearchResult는 allOf(Link + rank)라 생성기가 value1/value2로 감싼다.
            // 카드가 필요한 것은 Link 쪽이다.
            results = page.links.map(\.value1)
            searchCursor = page.next_cursor
            searchMode = page.mode.rawValue
            searchError = nil
            searchMoreError = nil
        } catch {
            // 검색 실패를 목록 오류 자리에 쓰지 않는다 — 목록은 멀쩡한데 화면 전체가
            // 오류로 바뀌면 사용자는 저장한 것이 사라졌다고 읽는다. 대신 **검색 화면에서**
            // 드러낸다. 이걸 안 하면 실패가 "결과가 없습니다"라는 단정문이 되어,
            // 저장해 둔 것이 없다는 거짓말을 하게 된다.
            results = []
            searchMode = nil
            searchCursor = nil
            searchError = error.localizedDescription
        }
    }

    /// 검색 결과의 다음 장. 목록과 같은 이유로 필요하다 — 50건에서 끊긴 결과는
    /// "더 없다"로 읽히는데, 검색에서 그 오해는 목록에서보다 비싸다.
    ///
    /// 검색 커서는 목록 커서와 **형식이 다르고 서로 호환되지 않는다**(계약). 그래서 상태를
    /// 따로 들고 있고, 검색어가 바뀌면 버린다.
    private func searchMore() async {
        guard !searchLoadingMore, let cursor = searchCursor,
              case .running = backend.state, let client = backend.client else { return }
        searchLoadingMore = true
        defer { searchLoadingMore = false }
        let q = query.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !q.isEmpty else { return }
        var q2 = Operations.search.Input.Query(q: q, limit: 50)
        q2.cursor = cursor
        if case let .tag(name) = filter { q2.tag = name }
        do {
            let page = try await client.search(.init(query: q2)).ok.body.json
            let known = Set(results.map(\.id))
            results.append(contentsOf: page.links.map(\.value1).filter { !known.contains($0.id) })
            searchCursor = page.next_cursor
            searchMoreError = nil
        } catch {
            // 삼키면 "여기가 끝"과 구분되지 않는다 — 이 함수의 주석이 경고하는 바로 그
            // 오해를 만들게 된다. searchCursor를 그대로 두므로 재시도가 같은 자리에서 이어진다.
            searchMoreError = "다음 결과를 불러오지 못했습니다"
        }
    }

    // MARK: - 로드






}
