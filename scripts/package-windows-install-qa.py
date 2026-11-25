#!/usr/bin/env python3
"""Bundle the exact Windows installers and a clean-machine acceptance runner."""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import zipfile


ROOT = Path(__file__).resolve().parent.parent
DIST = ROOT / 'dist'
VERSION = re.search(r'const Version = "([\d.]+)"', (ROOT / 'internal/ui/window.go').read_text())[1]


def setup_asset(version, destination):
    name = f'MochiPic-{version}-windows-x64'
    directory = DIST / name
    setup = directory / f'{name}-setup.exe'
    records = (directory / 'SHA256SUMS').read_text().splitlines()
    checksums = {name: digest for digest, name in (record.split() for record in records)}
    with setup.open('rb') as stream:
        digest = hashlib.file_digest(stream, 'sha256').hexdigest()
    if digest != checksums[setup.name]:
        raise ValueError(f'SHA-256 mismatch for {setup.name}')
    shutil.copy2(setup, destination / setup.name)
    return {'name': setup.name, 'version': version, 'sha256': digest}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--previous', required=True, help='previous packaged version used for the upgrade exercise')
    args = parser.parse_args()
    if not re.fullmatch(r'\d+\.\d+\.\d+', args.previous) or tuple(map(int, args.previous.split('.'))) >= tuple(map(int, VERSION.split('.'))):
        parser.error('--previous must identify an older three-part version')
    destination = DIST / f'MochiPic-{VERSION}-windows-x64-install-qa.zip'
    if destination.exists():
        raise SystemExit(f'Refusing to replace {destination}')

    with tempfile.TemporaryDirectory(dir=DIST, prefix='.windows-install-qa-') as tmp:
        work = Path(tmp)
        top = work / 'MochiPic-Install-QA'
        payload = top / 'Payload'
        installers = payload / 'Installers'
        installers.mkdir(parents=True)
        versions = {'previous': setup_asset(args.previous, installers), 'candidate': setup_asset(VERSION, installers)}
        (payload / 'installers.json').write_text(json.dumps(versions, indent=2) + '\n', encoding='ascii')

        subprocess.run(['go', 'test', '-c', '-trimpath', '-o', payload / 'installer.test.exe', './internal/releaseqa'],
                       cwd=ROOT, check=True, env={**os.environ, 'GOOS': 'windows', 'GOARCH': 'amd64', 'CGO_ENABLED': '0'})
        subprocess.run([ROOT / 'scripts/build-windows.sh', 'test', '-c', '-trimpath', '-o', payload / 'batch.test.exe',
                        './internal/batch'], cwd=ROOT, check=True)
        shutil.copytree(ROOT / 'internal/imaging/testdata', payload / 'internal/imaging/testdata')
        (payload / 'internal/batch').mkdir()

        runners = ROOT / 'scripts/windows-qa'
        for name in ('Run-In-Sandbox.cmd', 'Run-In-Sandbox.ps1', 'Run-On-Clean-Windows.cmd'):
            shutil.copy2(runners / name, top / name)
        shutil.copy2(runners / 'Run-Install-Checks.ps1', payload)
        shutil.copy2(ROOT / 'docs/WINDOWS_TEST.md', top / 'Windows-Test.md')

        output = work / destination.name
        with zipfile.ZipFile(output, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
            for path in sorted(top.rglob('*')):
                archive.write(path, path.relative_to(work).as_posix())
            if archive.testzip() is not None:
                raise RuntimeError('installer QA ZIP failed its integrity check')
        output.rename(destination)
    print(destination)


if __name__ == '__main__':
    main()
