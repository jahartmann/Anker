#!/bin/sh
set -eu
ANKER_VERSION=${1:?Release-Tag vX.Y.Z angeben}
ANKER_OUT=${2:-dist}
# The signing tool validates the tag; reject it before passing it to a linker argument as well.
python3 -c 'import re,sys; sys.exit(0 if re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)",sys.argv[1]) else 1)' "$ANKER_VERSION"
[ ! -e "$ANKER_OUT" ] || { echo 'Ausgabeverzeichnis muss neu sein.' >&2; exit 1; }
mkdir -p "$ANKER_OUT"
go run ./cmd/anker-release -mode public -dir "$ANKER_OUT"
cmp "$ANKER_OUT/public.key" internal/updater/official_public.key >/dev/null || { echo 'Release-Schlüssel stimmt nicht mit dem eingebetteten offiziellen Schlüssel überein.' >&2; exit 1; }
npm ci --prefix web
npm run build --prefix web
python3 scripts/release-notices.py "$ANKER_OUT/THIRD_PARTY_NOTICES.txt"
for ANKER_ARCH in amd64 arm64; do
 CGO_ENABLED=0 GOOS=linux GOARCH="$ANKER_ARCH" go build -trimpath -ldflags="-s -w -X anker/internal/buildinfo.Version=$ANKER_VERSION" -o "$ANKER_OUT/anker-linux-$ANKER_ARCH" ./cmd/anker
 ANKER_PACKAGE="$ANKER_OUT/package-$ANKER_ARCH"
 mkdir -p "$ANKER_PACKAGE"
 cp "$ANKER_OUT/anker-linux-$ANKER_ARCH" "$ANKER_PACKAGE/anker"
 mkdir -p "$ANKER_PACKAGE/scripts" "$ANKER_PACKAGE/deploy" "$ANKER_PACKAGE/host" "$ANKER_PACKAGE/docs"
 cp scripts/install-server.sh scripts/install-state.py scripts/install-release.py scripts/verify-release.sh scripts/install-host.sh scripts/host-authorize.py scripts/setup-sftp-export.sh "$ANKER_PACKAGE/scripts/"
 cp deploy/anker.service deploy/anker-updater.service deploy/anker.1 "$ANKER_PACKAGE/deploy/"
 cp host/anker_host.py "$ANKER_PACKAGE/host/"
 cp docs/OPERATIONS.md docs/RECOVERY.md docs/UPDATES.md docs/SUPPORT.md "$ANKER_PACKAGE/docs/"
 cp README.md LICENSE CHANGELOG.md CONTRIBUTING.md SECURITY.md "$ANKER_OUT/THIRD_PARTY_NOTICES.txt" "$ANKER_OUT/public.key" "$ANKER_OUT/public.pem" "$ANKER_PACKAGE/"
 tar -czf "$ANKER_OUT/anker-linux-$ANKER_ARCH.tar.gz" -C "$ANKER_PACKAGE" .
 rm -rf "$ANKER_PACKAGE"
done
go run ./cmd/anker-release -version "$ANKER_VERSION" -dir "$ANKER_OUT"
