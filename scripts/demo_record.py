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
        elif kind == "open":
            run(["xcrun", "simctl", "openurl", "booted", step["url"]])
        elif kind == "launch":
            run(["xcrun", "simctl", "launch", "booted", step["bundle"]])
            events.append({"t": at, "kind": "hide"})
        elif kind == "terminate":
            run(["xcrun", "simctl", "terminate", "booted", step["bundle"]])
    time.sleep(flow.get("tail", 2.0))
    rec.send_signal(2)
    rec.wait()
    return events


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

    # 각 시점의 커서 위치와 상태를 만든다. **동작 사이에는 다음 지점으로 미리 이동한다** —
    # 눌러야 비로소 나타나면 어디서 왔는지가 안 보이고, 그게 "직접 움직인다"의 반대다.
    acts = [e for e in events if e["kind"] in ("tap", "swipe", "hide")]
    MOVE = 0.55  # 다음 지점까지 옮겨 가는 시간(초)

    def state(t):
        prev = None
        for i, e in enumerate(acts):
            if e["kind"] == "hide":
                if t >= e["t"]:
                    return None
                continue
            start = e["t"]
            end = start + max(e.get("dur", 0.2), 0.2)
            if t < start - MOVE:
                prev = e
                continue
            if t < start:  # 이동 구간
                p = ease((t - (start - MOVE)) / MOVE)
                fx, fy = (prev["x2"] if prev and prev["kind"] == "swipe" else prev["x"],
                          prev["y2"] if prev and prev["kind"] == "swipe" else prev["y"]) if prev else (e["x"], e["y"] - 120)
                return (fx + (e["x"] - fx) * p, fy + (e["y"] - fy) * p, 0.0)
            if t <= end:
                p = (t - start) / (end - start)
                if e["kind"] == "swipe":
                    q = ease(p)
                    return (e["x"] + (e["x2"] - e["x"]) * q,
                            e["y"] + (e["y2"] - e["y"]) * q, 0.35)
                return (e["x"], e["y"], 1.0 - p)
            prev = e
        if prev is None:
            return None
        return (prev["x2"] if prev["kind"] == "swipe" else prev["x"],
                prev["y2"] if prev["kind"] == "swipe" else prev["y"], 0.0)

    tmp = pathlib.Path("/tmp/pp-cursor")
    tmp.mkdir(exist_ok=True)
    for f in tmp.glob("*.png"):
        f.unlink()

    r = CURSOR * SCALE // 2
    for i in range(frames):
        img = Image.new("RGBA", (W, H), (0, 0, 0, 0))
        st = state(i / fps)
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
        img.save(tmp / f"c{i:05d}.png")

    subprocess.run([
        "ffmpeg", "-v", "error", "-y", "-i", video,
        "-framerate", str(fps), "-i", str(tmp / "c%05d.png"),
        "-filter_complex", "[0:v]fps=30[v];[v][1:v]overlay=0:0:shortest=0[o]",
        "-map", "[o]", "-c:v", "libx264", "-preset", "medium", "-crf", "20",
        "-pix_fmt", "yuv420p", out,
    ], check=True)


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
