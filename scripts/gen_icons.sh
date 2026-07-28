#!/usr/bin/env bash
# 아이덴티티 마크 → 네 표면의 아이콘 생성 (`just icons`).
#
# 소스는 `design/icon/mark.svg` 하나이고 여기서 나오는 것은 전부 생성물이다.
# 생성물을 직접 편집하면 다음 실행에서 덮어써진다.
#
# **래스터라이저로 Chrome을 쓰는 이유**: 이 머신에 rsvg-convert·inkscape·ImageMagick이
# 없고, Chrome은 확장(extension/)이 이미 대상으로 삼는 브라우저라 새 의존성이 아니다.
# 축소는 macOS 기본 `sips`로 한다 — Chrome 헤드리스는 최소 창 크기가 있어서
# `--window-size=16,16`을 주면 1024 캔버스의 좌상단 16px을 잘라낸 빈 이미지가 나온다
# (2026-07-28에 실제로 그렇게 나왔다).
#
# 16·32px은 `mark-small.svg`(광학 변형)에서 나온다 — 이유는 그 파일 주석에.
set -euo pipefail

cd "$(dirname "$0")/.."
CHROME="/Applications/Google Chrome.app/Contents/MacOS/Google Chrome"
[ -x "$CHROME" ] || { echo "Chrome이 없습니다: $CHROME"; exit 1; }
command -v sips >/dev/null || { echo "sips가 없습니다 (macOS 전용)"; exit 1; }

tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT

# SVG를 뷰포트에 꽉 채워 1024로 래스터라이즈한다.
raster() { # $1=svg  $2=out.png
  local html="$tmp/$(basename "$1").html"
  {
    printf '<meta charset=utf-8><style>html,body{margin:0;padding:0;overflow:hidden}'
    printf 'svg{display:block;width:100vw;height:100vh}</style>'
    sed 's/width="1024" height="1024"/width="100%" height="100%"/' "$1"
  } > "$html"
  "$CHROME" --headless --disable-gpu --no-sandbox --force-device-scale-factor=1 \
    --hide-scrollbars --screenshot="$2" --window-size=1024,1024 "file://$html" >/dev/null 2>&1
}

resize() { cp "$1" "$2"; sips -Z "$3" "$2" >/dev/null; }

raster design/icon/mark.svg       "$tmp/master.png"
raster design/icon/mark-small.svg "$tmp/small.png"

# ── iOS 앱 아이콘 (1024, 알파 없음 — App Store가 거부한다) ──
cp "$tmp/master.png" ios/PushPoint/Assets.xcassets/AppIcon.appiconset/icon-1024.png

# ── 웹 ──
mkdir -p frontend/public
cp design/icon/mark.svg frontend/public/favicon.svg
resize "$tmp/master.png" frontend/public/apple-touch-icon.png 180
resize "$tmp/small.png"  frontend/public/favicon-32.png        32
resize "$tmp/small.png"  frontend/public/favicon-16.png        16

# ── 브라우저 확장 ──
mkdir -p extension/icons
resize "$tmp/small.png"  extension/icons/icon-16.png   16
resize "$tmp/master.png" extension/icons/icon-48.png   48
resize "$tmp/master.png" extension/icons/icon-128.png 128

echo "icons: 소스 design/icon/mark.svg → 8개 생성"
find ios/PushPoint/Assets.xcassets/AppIcon.appiconset frontend/public extension/icons \
     -name 'icon-*.png' -o -name 'favicon*' -o -name 'apple-touch-icon.png' \
  | sort | while read -r f; do printf "  %-58s %6s\n" "$f" "$(du -h "$f" | cut -f1)"; done
