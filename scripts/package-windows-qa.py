#!/usr/bin/env python3
"""Bundle the exact Windows portable runtime and offline acceptance tests."""

import argparse
import hashlib
import json
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import zipfile

from windows_layout import active_runtime

ROOT = Path(__file__).resolve().parent.parent
VERSION = re.search(r'const Version = "([\d.]+)"', (ROOT / 'internal/ui/window.go').read_text())[1]
NAME = f'MochiPic-{VERSION}-windows-x64'
DIST = ROOT / 'dist'


def copy_camera_fixtures(source, destination):
    samples = json.loads((ROOT / 'scripts/raw-fixtures.json').read_text())
    destination.mkdir()
    for name, expected in samples.values():
        target = destination / name
        shutil.copyfile(source / name, target)
        with target.open('rb') as stream:
            if hashlib.file_digest(stream, 'sha256').hexdigest() != expected:
                raise ValueError(f'SHA-256 mismatch for {name}')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--raw-fixtures', type=Path, help='build the format QA kit with the nine pinned CC0 RAW samples')
    args = parser.parse_args()
    package = DIST / NAME
    portable = package / f'{NAME}-portable.zip'
    suffix = '-format-qa' if args.raw_fixtures else '-qa'
    destination = DIST / f'{NAME}{suffix}.zip'
    if destination.exists():
        raise SystemExit(f'Refusing to replace {destination}')
    expected = {name: digest for digest, name in (line.split() for line in (package / 'SHA256SUMS').read_text().splitlines())}
    with portable.open('rb') as stream:
        if hashlib.file_digest(stream, 'sha256').hexdigest() != expected[portable.name]:
            raise SystemExit('The portable ZIP does not match its SHA-256 checksum')
    with tempfile.TemporaryDirectory(dir=DIST, prefix='.windows-qa-') as tmp:
        work = Path(tmp)
        payload = work / 'MochiPic-QA'
        with zipfile.ZipFile(portable) as archive:
            archive.extractall(payload)
        runtime = active_runtime(payload / 'MochiPic')
        tests = ('imaging',) if args.raw_fixtures else ('files', 'imaging', 'batch', 'ui', 'winupdate')
        subprocess.run([ROOT / 'scripts/build-windows.sh', 'test', '-c', '-trimpath', '-o', str(runtime) + '/',
                        *(f'./internal/{name}' for name in tests)], cwd=ROOT, check=True)
        for name in tests:
            if not (runtime / f'{name}.test.exe').is_file():
                raise RuntimeError(f'Missing {name}.test.exe')
            (payload / 'internal' / name).mkdir(parents=True)
        shutil.copytree(ROOT / 'internal/imaging/testdata', payload / 'internal/imaging/testdata')
        entry = 'Run-Format-Checks.cmd' if args.raw_fixtures else 'Run-Checks.cmd'
        for name in (entry, 'Run-Checks.ps1'):
            shutil.copy2(ROOT / 'scripts/windows-qa' / name, payload / name)
        if args.raw_fixtures:
            copy_camera_fixtures(args.raw_fixtures, payload / 'internal/imaging/raw-fixtures')
            shutil.copy2(ROOT / 'scripts/raw-fixtures.json', payload / 'Camera-Fixtures.json')
            shutil.copy2(ROOT / 'docs/WINDOWS_TEST.md', payload / 'Windows-Test.md')
        candidate = work / destination.name
        with zipfile.ZipFile(candidate, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
            for path in sorted(payload.rglob('*')):
                archive.write(path, path.relative_to(work).as_posix())
            if archive.testzip() is not None:
                raise RuntimeError('QA archive integrity check failed')
        candidate.rename(destination)
    print(destination)


if __name__ == '__main__':
    main()
