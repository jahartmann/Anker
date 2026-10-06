#!/bin/sh
# Run from the downloaded release directory, with a previously trusted public key.
set -eu
ANKER_PUBLIC_KEY=${1:?Pfad zum vertrauenswürdigen public.pem}
ANKER_PACKAGE=${2:?Name des Release-Pakets}
ANKER_VERIFY_TMP=$(mktemp -d)
trap 'rm -rf "$ANKER_VERIFY_TMP"' EXIT HUP INT TERM
openssl base64 -d -in release.json.sig -out "$ANKER_VERIFY_TMP/signature"
openssl pkeyutl -verify -pubin -inkey "$ANKER_PUBLIC_KEY" -rawin -in release.json -sigfile "$ANKER_VERIFY_TMP/signature"
python3 - "$ANKER_PACKAGE" <<'PY'
import hashlib,json,pathlib,sys
name=sys.argv[1]
assert pathlib.Path(name).name==name, 'Nur Dateinamen angeben'
manifest=json.loads(pathlib.Path('release.json').read_text())
assert manifest['format']==1, 'Nicht unterstütztes Releaseformat'
entries=[e for e in manifest['assets'] if e['name']==name]
assert len(entries)==1, 'Paket fehlt im signierten Manifest'
entry=entries[0];path=pathlib.Path(name)
assert path.stat().st_size==entry['size'], 'Größe stimmt nicht'
with path.open('rb') as stream:
    digest=hashlib.sha256()
    for chunk in iter(lambda: stream.read(1024*1024), b''):
        digest.update(chunk)
    assert digest.hexdigest()==entry['sha256'], 'Prüfsumme stimmt nicht'
print('Signatur und Paket geprüft:', name)
PY
