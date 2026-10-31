#!/bin/sh
set -eu
cd "$(dirname "$0")"
if [ "${PLOST_REBUILD_UI:-0}" = "1" ]; then
  (cd frontend && bun install --frozen-lockfile && bun run typecheck && bun run check:unused && bun run build)
fi
mkdir -p dist
for plost_target in darwin/arm64 linux/amd64 linux/arm64; do
  plost_os=${plost_target%/*}
  plost_arch=${plost_target#*/}
  CGO_ENABLED=0 GOOS="$plost_os" GOARCH="$plost_arch" go build -trimpath -ldflags='-s -w' -o "dist/plost-${plost_os}-${plost_arch}" ./cmd/plost
  # A separate module, so the PostgreSQL driver never becomes part of the server.
  (cd cmd/plost-import-umami && CGO_ENABLED=0 GOOS="$plost_os" GOARCH="$plost_arch" go build -trimpath -ldflags='-s -w' -o "../../dist/plost-import-umami-${plost_os}-${plost_arch}" .)
done
