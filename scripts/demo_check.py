#!/usr/bin/env python3
"""합성된 데모 영상에서 커서가 **실제로 누른 자리**에 그려졌는지 확인한다.

빌드는 이걸 못 잡고, 사람이 매번 볼 수도 없다. 2026-08-03에 커서가 공유 시트 위에서
주소창 자리를 짚고 있는 영상이 그대로 배포됐고, 사용자가 보고 나서야 알았다.

방법: 커서 한가운데에 **불투명한 자홍색 점**을 심어 두고(`demo_record.overlay`), 각 탭
시각의 프레임에서 그 색을 찾아 이벤트 좌표와 거리를 잰다. 불투명이어야 하는 이유는 다른
층이 전부 알파 합성이라 배경 밝기에 따라 픽셀이 달라지기 때문이다.

    python3 scripts/demo_check.py <합성된.mp4> <이벤트.json> [허용거리pt]
"""
import json
import pathlib
import subprocess
import sys

SCALE = 3
FID = (255, 0, 200)
TOL = float(sys.argv[3]) if len(sys.argv) > 3 else 12.0


def find_fiducial(video, t):
    from PIL import Image
    out = "/tmp/pp-check.png"
    subprocess.run(["ffmpeg", "-v", "error", "-y", "-ss", f"{t:.3f}", "-i", video,
                    "-frames:v", "1", out], check=True)
    im = Image.open(out).convert("RGB")
    px, (W, H) = im.load(), im.size
    xs = ys = n = 0
    for y in range(0, H, 2):
        for x in range(0, W, 2):
            r, g, b = px[x, y]
            if r > 200 and g < 70 and 150 < b < 235:
                xs += x; ys += y; n += 1
    if n < 8:
        return None
    return (xs / n / SCALE, ys / n / SCALE)


video, ev_path = sys.argv[1], sys.argv[2]
events = json.loads(pathlib.Path(ev_path).read_text())
lag = 0.0
sync = next((e["t"] for e in events if e["kind"] == "sync"), None)
if sync is not None:
    sys.path.insert(0, str(pathlib.Path(__file__).parent))
    from demo_record import measure_lag
    lag = measure_lag(video, events)

taps = [e for e in events if e["kind"] == "tap"]
fail = []
for e in taps:
    t = e["t"] - lag + 0.05
    got = find_fiducial(video, t)
    if got is None:
        fail.append(f"t={t:.2f}s 커서를 못 찾았다 (이벤트 {e['x']},{e['y']})")
        continue
    d = ((got[0] - e["x"]) ** 2 + (got[1] - e["y"]) ** 2) ** 0.5
    mark = "ok  " if d <= TOL else "✗   "
    print(f"  {mark} t={t:5.2f}s  이벤트 ({e['x']},{e['y']})  그려진 곳 "
          f"({got[0]:.0f},{got[1]:.0f})  거리 {d:.1f}pt")
    if d > TOL:
        fail.append(f"t={t:.2f}s 커서가 {d:.1f}pt 떨어져 있다")

if fail:
    print(f"\n데모 검증 실패 — {len(fail)}건")
    for f in fail:
        print("  -", f)
    sys.exit(1)
print(f"demo-check OK — 탭 {len(taps)}개 전부 {TOL:.0f}pt 안에 그려졌다 (지연 {lag:+.2f}s)")
