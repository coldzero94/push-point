#!/usr/bin/env python3
"""시뮬레이터를 몰면서 녹화하고, 손가락 커서를 합성한다.

**왜 이게 필요한가.** 녹화만 하면 화면이 저절로 움직이는 것처럼 보인다. 어디를 눌렀는지가
안 보이니 "공유 시트를 열었다"가 아니라 "공유 시트가 떴다"로 읽힌다. 시뮬레이터의
`ShowSingleTouches` 기본값은 Xcode 26에서 동작하지 않는 것을 확인했다(2026-08-03).

**왜 시각을 기록하는가.** 대본에 적은 `sleep` 값으로 커서 위치를 계산하면 어긋난다 —
`axe` 한 번이 프로세스를 띄우느라 0.3~0.5초를 더 먹고, 그 오차가 누적된다. 그래서 각
동작의 **실제 경과 시각**을 재서 남기고, 합성은 그 값만 쓴다.

좌표는 전부 **포인트**다(402×874). 스크린샷 픽셀(1206×2622)이 아니다 —
`.claude/rules/ui-verification.md`가 이름을 대서 경고하는 그 함정이고, 실제로 한 번 빠졌다.

    python3 scripts/demo_record.py flow.json out.mp4
"""
import json
import pathlib
import subprocess
import sys
import time

SCALE = 3  # 포인트 → 픽셀
CURSOR = 46  # 손끝 지름(포인트)


def run(args):
    subprocess.run(args, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)


def drive(flow, udid, out):
    rec = subprocess.Popen(
        ["xcrun", "simctl", "io", "booted", "recordVideo", "--codec=h264", out],
        stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL,
    )
    t0 = time.time()
    events = []
    for step in flow["steps"]:
        time.sleep(step.get("wait", 0))
        kind = step["do"]
        at = time.time() - t0
        if kind == "tap":
            run(["axe", "tap", "-x", str(step["x"]), "-y", str(step["y"]), "--udid", udid])
            events.append({"t": at, "kind": "tap", "x": step["x"], "y": step["y"],
                           "dur": time.time() - t0 - at})
        elif kind == "swipe":
            run(["axe", "swipe", "--start-x", str(step["x"]), "--start-y", str(step["y"]),
                 "--end-x", str(step["x2"]), "--end-y", str(step["y2"]),
                 "--duration", str(step.get("duration", 0.45)), "--udid", udid])
            events.append({"t": at, "kind": "swipe", "x": step["x"], "y": step["y"],
                           "x2": step["x2"], "y2": step["y2"], "dur": time.time() - t0 - at})
        elif kind == "find_tap":
            # **공유 시트의 아이콘 위치는 설치 사이에 움직인다** — 2026-08-03에 x가
            # 261에서 245로 옮겨 앉아 저장이 통째로 안 됐고, 겉으로는 확장이 죽은 것과
            # 구분되지 않았다. 그래서 좌표를 믿지 않고 매번 찾는다.
            pt = locate_icon(udid, step["band"], step.get("rgb", [14, 79, 60]))
            if pt is None:
                raise SystemExit("공유 시트에서 Push-Point 아이콘을 못 찾았다")
            x, y = pt
            run(["axe", "tap", "-x", str(x), "-y", str(y), "--udid", udid])
            events.append({"t": at, "kind": "tap", "x": x, "y": y,
                           "dur": time.time() - t0 - at})
        elif kind == "type_paste":
            # **키보드를 안 거친다.** `axe type`은 US HID 키코드뿐이라 한글이 안 들어간다
            # (scripts/sim_type.sh 주석 참고). 페이스트보드로 넣으면 IME를 지나가고,
            # stream 모드는 한 글자씩 나타나 실제 타이핑처럼 보인다.
            run(["bash", "scripts/sim_type.sh", "--mode", step.get("mode", "stream"),
                 "--udid", udid, step["text"]])
        elif kind == "type":
            run(["axe", "type", step["text"], "--udid", udid])
        elif kind == "open":
            run(["xcrun", "simctl", "openurl", "booted", step["url"]])
            # **동기화 표식.** 영상 시계는 이벤트 시계와 같지 않다 — 2026-08-03에 한
            # 녹화가 7초 뒤처져 있었고(다른 녹화는 0초였다), 그대로 합성하니 공유 시트가
            # 떠 있는 화면에 7초 전 좌표의 손이 그려졌다. openurl은 화면을 통째로 바꾸므로
            # 영상에서 찾을 수 있는 첫 큰 변화가 되고, 그 두 시각의 차가 지연이다.
            events.append({"t": at, "kind": "sync"})
        elif kind == "launch":
            run(["xcrun", "simctl", "launch", "booted", step["bundle"]])
            events.append({"t": at, "kind": "hide"})
        elif kind == "terminate":
            run(["xcrun", "simctl", "terminate", "booted", step["bundle"]])
    time.sleep(flow.get("tail", 2.0))
    rec.send_signal(2)
    rec.wait()
    return events


def locate_icon(udid, band, rgb):
    """화면의 `band`(포인트 y 범위)에서 `rgb`에 가장 가까운 덩어리의 중심을 찾는다.

    Push-Point 아이콘은 짙은 초록 정사각형이라 공유 시트의 다른 아이콘과 색으로 갈린다.
    글자를 찾지 않는 이유는 `maestro hierarchy`가 시스템 공유 시트를 못 보기 때문이다.
    """
    from PIL import Image

    shot = "/tmp/pp-sheet.png"
    run(["axe", "screenshot", "--output", shot, "--udid", udid])
    im = Image.open(shot).convert("RGB")
    W, H = im.size
    px = im.load()
    y0, y1 = int(band[0] * SCALE), min(int(band[1] * SCALE), H)
    tr, tg, tb = rgb
    xs, ys, n = 0, 0, 0
    for y in range(y0, y1, 3):
        for x in range(0, W, 3):
            r, g, b = px[x, y]
            if abs(r - tr) < 26 and abs(g - tg) < 26 and abs(b - tb) < 26:
                xs += x; ys += y; n += 1
    if n < 40:
        return None
    return (round(xs / n / SCALE), round(ys / n / SCALE))


ZOOM = 1.45      # 배율. 1.6을 넘으면 402pt 폭에서 맥락이 잘려 어디를 눌렀는지 되레 흐려진다
ZOOM_IN = 0.45   # 들어가는 데 걸리는 시간(초)
ZOOM_HOLD = 0.5  # 탭 뒤 머무는 시간
ZOOM_OUT = 0.5   # 나오는 데 걸리는 시간
ZOOM_GAP = 1.2   # 이 간격 안에 다음 탭이 있으면 나왔다 다시 들어가지 않고 이어 간다


def camera(t, acts, W, H):
    """이 시각의 카메라(중심, 배율). 줌이 없으면 None.

    **탭 지점을 화면 한가운데 두지 않는다.** 손가락이 중앙에 박히면 무엇을 눌렀는지는
    보여도 그게 화면 어디쯤인지가 사라진다. 목표를 살짝 위로 올려(0.42) 아래쪽 맥락을
    남기고, 가장자리에서는 프레임이 화면 밖으로 나가지 않게 잡아 둔다.
    """
    taps = [e for e in acts if e["kind"] == "tap"]
    if not taps:
        return None

    # 가까이 붙은 탭들을 한 구간으로 묶는다 — 사이마다 줌아웃하면 화면이 펄떡인다
    groups, cur = [], []
    for e in taps:
        if cur and e["t"] - cur[-1]["t"] > ZOOM_GAP + ZOOM_HOLD + ZOOM_OUT:
            groups.append(cur); cur = []
        cur.append(e)
    groups.append(cur)

    for g in groups:
        a, b = g[0]["t"] - ZOOM_IN, g[-1]["t"] + ZOOM_HOLD
        if t < a - 0.01 or t > b + ZOOM_OUT:
            continue
        if t < g[0]["t"]:
            k = ease((t - a) / ZOOM_IN)
        elif t <= b:
            k = 1.0
        else:
            k = 1.0 - ease((t - b) / ZOOM_OUT)
        # 구간 안에서는 현재/다음 탭 사이를 따라 카메라가 흐른다
        cx, cy = g[0]["x"], g[0]["y"]
        for i in range(len(g) - 1):
            t0, t1 = g[i]["t"], g[i + 1]["t"]
            if t0 <= t <= t1:
                q = ease((t - t0) / max(t1 - t0, 1e-3))
                cx = g[i]["x"] + (g[i + 1]["x"] - g[i]["x"]) * q
                cy = g[i]["y"] + (g[i + 1]["y"] - g[i]["y"]) * q
            elif t > t1:
                cx, cy = g[i + 1]["x"], g[i + 1]["y"]
        z = 1.0 + (ZOOM - 1.0) * k
        if z <= 1.001:
            return None
        vw, vh = W / z, H / z
        px, py = cx * SCALE, cy * SCALE - vh * 0.08   # 목표를 살짝 위로
        px = min(max(px, vw / 2), W - vw / 2)
        py = min(max(py, vh / 2), H - vh / 2)
        return (px, py, z)
    return None


def window(c, W, H):
    """카메라 값 하나(중심·배율)를 원본 위의 크롭 사각형으로 바꾼다.

    **이 식은 `demo_check.to_source`가 그대로 뒤집는 식이다.** 검사기는 `<out>.camera.json`과
    `<out>.zoom.json`만 보고 커서가 그려진 자리를 원본 좌표로 되돌리는데, 여기서 한 픽셀이라도
    다른 규칙으로 자르면 검사기가 멀쩡한 영상을 틀렸다고 하거나(그럼 사람이 검사기를 끈다)
    틀린 영상을 통과시킨다. 두 파일을 고칠 일이 있으면 이 함수부터 맞춰 보라.

    딱 한 군데 어긋난다. 실제 확대율은 `W/w2`인데 검사기는 `z`로 나눈다 — `w2`를 짝수로
    깎느라 생긴 0.2% 차이다. 탭 지점은 화면 한가운데 근처라 오차가 0.3pt 안쪽이고(실측),
    허용치 12pt에 한참 못 미친다. `w2`를 반올림해 차이를 더 줄일 수도 있지만, 그러면 이
    함수가 `to_source`와 한 줄씩 대조되지 않는다. 대조되는 쪽을 택했다.
    """
    cx, cy, z = c
    w2 = int(W / z) // 2 * 2     # yuv420p가 홀수 크기를 안 받아서 짝수로 맞춘다
    h2 = int(H / z) // 2 * 2
    x0 = min(max(cx - w2 / 2, 0), W - w2)
    y0 = min(max(cy - h2 / 2, 0), H - h2)
    return x0, y0, w2, h2


def render_camera(video, cursor_dir, cam, out, W, H, fps):
    """커서를 얹은 화면을 카메라 궤적대로 잘라 키워서 최종 영상으로 낸다.

    **ffmpeg 필터 한 줄로는 안 된다.** 두 갈래를 다 재 보고 남은 길이다(2026-08-04 실측).

    - `sendcmd`+`crop`: `crop`은 `w`/`h`를 런타임 명령으로 못 바꾼다(exit 234). 크기를 고정하고
      위치만 움직이면 명령은 받지만, 그러면 배율이 구간 내내 상수라 줌인·줌아웃이 아니라
      컷 전환이 된다.
    - `zoompan`: 줌 자체는 된다. 하지만 프레임마다 다른 중심·배율을 넣을 통로가 표현식밖에
      없고, 1413프레임짜리 궤적을 `between(on,i,i)*v` 합으로 적으면 34KB 표현식이 되어
      필터 초기화가 ENOMEM으로 죽는다(exit 244).

    그래서 크롭을 PIL로 한다. ffmpeg는 잘하는 일(디코드·커서 합성·인코드)만 맡고, 프레임마다
    달라지는 사각형은 파이썬이 계산한다. 원본에서 바로 자르므로 평면 영상을 한 번 인코딩했다가
    다시 늘리는 세대 손실도 없다 — 830px 폭을 1206px로 키우는 그림이라 손실이 그대로 보인다.

    `Image.transform`을 쓰는 이유는 크롭 좌표가 정수가 아니어서다. `crop()`은 정수만 받아
    들어가고 나오는 동안 카메라가 1px씩 튀는데, 아핀 변환은 실수 좌표를 그대로 받아 한 번에
    잘라 키운다.
    """
    from PIL import Image

    dec = subprocess.Popen([
        "ffmpeg", "-v", "error", "-i", video,
        "-framerate", str(fps), "-i", str(cursor_dir / "c%05d.png"),
        "-filter_complex", f"[0:v]fps={fps}[v];[v][1:v]overlay=0:0:shortest=0[o]",
        "-map", "[o]", "-f", "rawvideo", "-pix_fmt", "rgb24", "-",
    ], stdout=subprocess.PIPE)
    enc = subprocess.Popen([
        "ffmpeg", "-v", "error", "-y", "-f", "rawvideo", "-pix_fmt", "rgb24",
        "-s", f"{W}x{H}", "-framerate", str(fps), "-i", "-",
        "-c:v", "libx264", "-preset", "medium", "-crf", "18",
        "-pix_fmt", "yuv420p", out,
    ], stdin=subprocess.PIPE)

    size = W * H * 3
    i, done = 0, False
    try:
        while True:
            buf = dec.stdout.read(size)       # BufferedReader는 EOF가 아니면 size만큼 채운다
            if len(buf) < size:
                break
            c = cam[i] if i < len(cam) else None
            if c is None:
                enc.stdin.write(buf)          # 줌이 없는 프레임은 손대지 않는다
            else:
                x0, y0, w2, h2 = window(c, W, H)
                im = Image.frombuffer("RGB", (W, H), buf, "raw", "RGB", 0, 1)
                enc.stdin.write(im.transform(
                    (W, H), Image.AFFINE, (w2 / W, 0, x0, 0, h2 / H, y0),
                    resample=Image.BICUBIC).tobytes())
            i += 1
        done = True
    finally:
        # 중간에 튕겨 나갈 때는 파이프를 닫기 **전에** 죽인다. 그냥 닫으면 디코더가
        # broken pipe를 진짜 오류로 뱉어서, 되돌아갈 준비가 된 상황인데도 콘솔에는
        # 무언가 망가진 것처럼 찍힌다.
        if not done:
            dec.kill()
            enc.kill()
        try:
            enc.stdin.close()
        except BrokenPipeError:
            pass
        dec.stdout.close()
        dec.wait()
        enc.wait()
    if i == 0:
        raise RuntimeError("디코더가 프레임을 하나도 안 냈다")
    if dec.returncode or enc.returncode:
        raise RuntimeError(f"ffmpeg 실패 (디코드 {dec.returncode}, 인코드 {enc.returncode})")
    return i


def measure_lag(video, events):
    """영상 시계가 이벤트 시계보다 얼마나 뒤처졌는지 잰다.

    **가정하지 않고 잰다.** `simctl io recordVideo`의 타임라인은 호스트 시계와 같을 때도
    있고 7초 뒤처질 때도 있다(2026-08-03 실측, 같은 기계에서 녹화마다 달랐다). 어느 쪽인지
    모르는 채로 합성하면 손이 화면보다 앞서거나 뒤서고, 그게 공유 시트 위에 주소창을 누르는
    손을 그렸다.

    `openurl`은 화면을 통째로 바꾸므로 영상에서 **첫 큰 변화**가 그 순간이다. 그 프레임
    시각과 이벤트에 찍힌 시각의 차가 지연이다. 표식이 없으면 0으로 두되 조용히 넘어가지
    않는다 — 틀린 커서보다 없는 커서가 낫다는 판단은 사람이 해야 한다.
    """
    sync = next((e["t"] for e in events if e["kind"] == "sync"), None)
    if sync is None:
        print("  경고: sync 표식이 없다 — 지연 보정 없이 합성한다")
        return 0.0

    import subprocess as sp
    out = sp.run(["ffmpeg", "-v", "error", "-i", video, "-vf",
                  "fps=10,scale=48:104,format=gray", "-f", "rawvideo", "-"],
                 capture_output=True).stdout
    n, size = 48 * 104, 48 * 104
    frames = [out[i * size:(i + 1) * size] for i in range(len(out) // size)]
    diffs = [sum(abs(a - b) for a, b in zip(frames[i], frames[i - 1])) / n
             for i in range(1, len(frames))]
    if not diffs:
        return 0.0
    peak = max(diffs)
    # 첫 번째로 크게 흔들린 지점 = openurl로 화면이 갈린 순간
    idx = next((i for i, d in enumerate(diffs) if d > peak * 0.35), 0)
    return round(idx / 10.0 - sync, 2)


def ease(p):
    """가감속. 등속으로 움직이면 로봇처럼 보이고, 사람이 손을 옮기는 리듬이 아니다."""
    return 3 * p * p - 2 * p * p * p


def overlay(video, events, out):
    from PIL import Image, ImageDraw

    probe = subprocess.run(
        ["ffprobe", "-v", "error", "-select_streams", "v:0",
         "-show_entries", "stream=width,height", "-show_entries", "format=duration",
         "-of", "json", video], capture_output=True, text=True)
    meta = json.loads(probe.stdout)
    W = meta["streams"][0]["width"]
    H = meta["streams"][0]["height"]
    dur = float(meta["format"]["duration"])
    fps = 30
    frames = int(dur * fps)

    # 각 시점의 커서 위치와 상태를 만든다.
    #
    # **끝난 동작만 원점이 될 수 있다.** 첫 판은 아직 오지 않은 동작을 `prev`로 잡아서,
    # 커서가 매 단계 엉뚱한 자리(대개 마지막 스와이프가 끝난 화면 한가운데)로 돌아갔다가
    # 목표로 튀었다. 사용자의 손이 아니라 순간이동으로 보였고, 그게 정확히 사용자가
    # 지적한 것이다. 지금은 순서대로 훑되 **완료된 동작의 끝점만** 원점으로 남긴다.
    acts = [e for e in events if e["kind"] in ("tap", "swipe", "hide")]
    MOVE = 0.7  # 다음 지점까지 옮겨 가는 시간(초)

    PRESS = 0.28  # 눌림 표시 상한(초)

    def span(e):
        """동작이 화면에서 실제로 일어난 구간.

        탭의 `dur`에는 `axe` 프로세스 시간이 통째로 들어 있다(실측 0.47~0.60초, find_tap은
        0.72초). 그걸로 눌림을 늘이면 손이 화면 반응보다 먼저 눌렀다 떼는 것처럼 보인다.
        스와이프의 `dur`은 요청한 제스처 길이라 그대로 쓴다.
        """
        d = e.get("dur", 0.2)
        if e["kind"] != "swipe":
            d = min(max(d, 0.12), PRESS)
        return e["t"], e["t"] + max(d, 0.12)

    def rest(e):
        """그 동작이 끝났을 때 손가락이 놓인 자리."""
        return (e["x2"], e["y2"]) if e["kind"] == "swipe" else (e["x"], e["y"])

    def state(t):
        origin = None  # 지금까지 **끝난** 동작의 마지막 자리
        for e in acts:
            if e["kind"] == "hide":
                # 앱이 뜨는 동안 손을 뺀다. **여기서 return하면 안 된다** — 그러면
                # 그 뒤 모든 탭이 영상 끝까지 보이지 않는다.
                if t >= e["t"]:
                    origin = None
                continue
            start, end = span(e)
            if t > end:
                origin = rest(e)
                continue
            if t >= start:  # 동작 중
                p = (t - start) / (end - start)
                if e["kind"] == "swipe":
                    q = ease(p)
                    return (e["x"] + (e["x2"] - e["x"]) * q,
                            e["y"] + (e["y2"] - e["y"]) * q, 0.3)
                return (e["x"], e["y"], 1.0 - p)
            # 아직 안 온 동작 — 이동 구간이거나, 그 전이면 제자리에서 기다린다
            if t >= start - MOVE:
                q = ease((t - (start - MOVE)) / MOVE)
                # 첫 동작에는 원점이 없다. 화면 아래에서 올라오게 해서 손이 들어오는
                # 것처럼 보이게 한다 — 없던 점이 갑자기 생기는 것보다 낫다.
                fx, fy = origin if origin else (e["x"], e["y"] + 150)
                return (fx + (e["x"] - fx) * q, fy + (e["y"] - fy) * q, 0.0)
            return (*origin, 0.0) if origin else None
        return (*origin, 0.0) if origin else None

    tmp = pathlib.Path("/tmp/pp-cursor")
    tmp.mkdir(exist_ok=True)
    for f in tmp.glob("*.png"):
        f.unlink()

    lag = measure_lag(video, events)
    print(f"  영상↔이벤트 지연 {lag:+.2f}s")

    r = CURSOR * SCALE // 2
    FR = 7 * SCALE // 2   # 검증용 표식 반지름
    for i in range(frames):
        img = Image.new("RGBA", (W, H), (0, 0, 0, 0))
        st = state(i / fps + lag)
        if st:
            x, y, pulse = st
            px, py = x * SCALE, y * SCALE
            d = ImageDraw.Draw(img)
            # **밝은 화면 위에서도 보여야 한다.** 처음에는 흰 반투명 원 하나였는데
            # 사파리의 흰 배경에서 거의 사라졌다. 어두운 테두리와 그림자를 붙여야
            # 배경 밝기와 무관하게 읽힌다.
            if pulse > 0:  # 누르는 순간 퍼지는 파문
                rr = r * (1 + 1.6 * (1 - pulse))
                d.ellipse([px - rr, py - rr, px + rr, py + rr],
                          outline=(255, 255, 255, int(230 * pulse)), width=max(3, SCALE + 2))
                d.ellipse([px - rr - SCALE, py - rr - SCALE, px + rr + SCALE, py + rr + SCALE],
                          outline=(0, 0, 0, int(110 * pulse)), width=max(2, SCALE))
            d.ellipse([px - r - SCALE, py - r - SCALE, px + r + SCALE, py + r + SCALE],
                      fill=(0, 0, 0, 46))                                   # 그림자
            d.ellipse([px - r, py - r, px + r, py + r],
                      fill=(255, 255, 255, 150), outline=(15, 15, 15, 210), width=SCALE)
            d.ellipse([px - r // 2, py - r // 2, px + r // 2, py + r // 2],
                      fill=(255, 255, 255, 225))
            # **검증용 표식.** 불투명(alpha 255)이라 배경과 무관하게 정확히 이 색이 나온다 —
            # 다른 층은 전부 알파 합성이라 사파리 흰 배경과 어두운 공유 시트에서 픽셀이
            # 달라져 색으로 찾을 수 없다. `scripts/demo_check.py`가 이걸 찾아 이벤트
            # 좌표와 대조한다.
            d.ellipse([px - FR, py - FR, px + FR, py + FR], fill=(255, 0, 200, 255))
        img.save(tmp / f"c{i:05d}.png")

    # 카메라 궤적을 남긴다 — 검사기가 크롭을 되돌리려면 이 값이 필요하고, 눈으로
    # 확인할 때도 어느 프레임이 줌 상태인지 알아야 한다.
    # 반올림한 값으로 렌더까지 한다 — 검사기가 읽는 숫자와 실제로 자른 숫자가 같아야
    # 검증이 근사가 아니라 검증이 된다.
    cam = [camera(i / fps + lag, acts, W, H) for i in range(frames)]
    cam = [None if c is None else [round(c[0], 2), round(c[1], 2), round(c[2], 4)] for c in cam]
    pathlib.Path(out + ".camera.json").write_text(json.dumps(cam))
    zs_ = [c[2] for c in cam if c is not None]
    pathlib.Path(out + ".zoom.json").write_text(json.dumps(max(zs_) if zs_ else 1.0))

    def flatten():
        """줌 없이 커서만 얹어 낸다 — 카메라가 실패했을 때 돌아갈 자리."""
        subprocess.run([
            "ffmpeg", "-v", "error", "-y", "-i", video,
            "-framerate", str(fps), "-i", str(tmp / "c%05d.png"),
            "-filter_complex", f"[0:v]fps={fps}[v];[v][1:v]overlay=0:0:shortest=0[o]",
            "-map", "[o]", "-c:v", "libx264", "-preset", "medium", "-crf", "18",
            "-pix_fmt", "yuv420p", out,
        ], check=True)

    if not any(c is not None for c in cam):
        flatten()
        return

    try:
        n = render_camera(video, tmp, cam, out, W, H, fps)
        zoomed = sum(1 for c in cam if c is not None)
        print(f"  카메라 합성 완료 · {n}프레임 중 {zoomed}프레임 줌 (최대 {max(zs_):.2f}배)")
    except Exception as exc:
        # **줌이 실패하면 줌 없이 낸다.** 카메라는 장식이고 커서는 내용이다 —
        # 장식 때문에 결과물이 없어지면 안 된다. 궤적 파일도 같이 비운다:
        # 줌이 안 걸린 영상에 궤적이 남아 있으면 `demo_check`가 걸지도 않은 크롭을
        # 되돌리면서 멀쩡한 커서를 틀렸다고 한다.
        print(f"  경고: 카메라 합성 실패({exc}) — 줌 없이 낸다")
        flatten()
        pathlib.Path(out + ".camera.json").write_text("[]")
        pathlib.Path(out + ".zoom.json").write_text("1.0")


if __name__ == "__main__":
    spec = json.loads(pathlib.Path(sys.argv[1]).read_text())
    dest = sys.argv[2]
    udid = spec.get("udid") or subprocess.run(
        ["xcrun", "simctl", "list", "devices", "booted", "-j"],
        capture_output=True, text=True).stdout
    raw = "/tmp/pp-demo-raw.mp4"
    pathlib.Path(raw).unlink(missing_ok=True)
    ev = drive(spec, spec["udid"], raw)
    pathlib.Path(dest + ".events.json").write_text(json.dumps(ev, indent=1))
    print(f"  녹화 완료 · 동작 {len(ev)}개")
    for e in ev:
        print(f"    {e['t']:6.2f}s  {e['kind']}")
    overlay(raw, ev, dest)
    print(f"  커서 합성 완료 → {dest}")
