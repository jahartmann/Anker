#!/usr/bin/env python3
"""Collect the upstream license texts shipped with release dependencies."""
import json
import pathlib
import subprocess
import sys

out = pathlib.Path(sys.argv[1])
text = ['Anker third-party license notices\n\nAnker itself is licensed under MIT. The following projects retain their own licenses.\n']
raw = subprocess.check_output(['go', 'list', '-m', '-json', 'all'], text=True)
decoder = json.JSONDecoder()
modules = []
while raw.strip():
    module, end = decoder.raw_decode(raw.lstrip())
    modules.append(module)
    raw = raw.lstrip()[end:]
roots = [(m['Path'], pathlib.Path(m['Dir'])) for m in modules if m.get('Dir') and not m.get('Main')]
roots.append(('Go standard library', pathlib.Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip())))
lock = json.loads(pathlib.Path('web/package-lock.json').read_text())
for name in lock['packages']:
    if name.startswith('node_modules/') and not lock['packages'][name].get('dev'):
        root = pathlib.Path('web') / name
        if not root.exists() and lock['packages'][name].get('optional'):
            continue
        roots.append((name.removeprefix('node_modules/'), root))
for name, root in roots:
    files = sorted(p for p in root.iterdir() if p.is_file() and p.name.lower().startswith(('license', 'licence', 'copying', 'notice')))
    if not files:
        raise SystemExit(f'License text missing: {name}; review before distributing.')
    for path in files:
        text.append(f'\n{"=" * 72}\n{name} — {path.name}\n{"=" * 72}\n{path.read_text(errors="replace")}\n')
out.write_text(''.join(text))
