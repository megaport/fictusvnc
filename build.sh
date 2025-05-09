#!/bin/bash
set -e

NAME="fictusvnc"
OUTDIR="build"
mkdir -p "$OUTDIR"

FLAGS=(-ldflags="-s -w")

PLATFORMS=(
  "linux/amd64"
  "linux/arm64"
  "linux/386"

  "windows/amd64"
  "windows/386"

  "darwin/amd64"
  "darwin/arm64"
)


echo "📦 Starting multi-platform build..."

for platform in "${PLATFORMS[@]}"; do
  IFS="/" read -r GOOS GOARCH <<< "$platform"
  EXT=""
  [[ "$GOOS" == "windows" ]] && EXT=".exe"
  OUTFILE="${OUTDIR}/${NAME}-${GOOS}-${GOARCH}${EXT}"

  echo "🛠️  Building $GOOS/$GOARCH → $OUTFILE"
  env GOOS=$GOOS GOARCH=$GOARCH go build "${FLAGS[@]}" -o "$OUTFILE" .
done

echo "✅ All builds complete. Binaries saved to: $OUTDIR/"
