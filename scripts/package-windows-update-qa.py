#!/usr/bin/env python3
"""Prepare a standalone offline QA bundle for MyGo's Windows updater."""

import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tarfile
import tempfile
import zipfile
from urllib.parse import urlsplit

from windows_layout import QA_FEED, active_runtime, update_flags

ROOT = Path(__file__).resolve().parent.parent
VERSION = re.search(r'const Version = "([\d.]+)"', (ROOT / 'internal/ui/window.go').read_text())[1]
NAME = f'MochiPic-{VERSION}-windows-x64'
TARGET = 'windows-amd64'


def run(*args, **options):
    return subprocess.run(args, cwd=ROOT, check=True, **options)


def write_update(runtime, updates, key, public, notes, url_prefix=''):
    archive = updates / f'mochipic-{VERSION}-{TARGET}.tar.gz'
    if (runtime / 'runtime-version').read_text(encoding='ascii').strip() != VERSION:
        raise ValueError('runtime version differs from the update version')

    def normalize(entry):
        entry.uid = entry.gid = 0
        entry.uname = entry.gname = ''
        return entry

    # MyGo replaces the top-level entries in the installed app's directory.
    # The NSIS-generated Uninstall.exe must never be one of those entries.
    entries = sorted(path.name for path in runtime.iterdir())
    if any(name.casefold() in {'.mygo-files', 'uninstall.exe'} for name in entries):
        raise ValueError('unexpected updater list or uninstaller in portable payload')
    (runtime / '.mygo-files').write_text('\n'.join(entries) + '\n', encoding='ascii')
    with tarfile.open(archive, 'w:gz', format=tarfile.USTAR_FORMAT) as tar:
        for name in [*entries, '.mygo-files']:
            tar.add(runtime / name, arcname=name, filter=normalize)

    signature = run('go', 'run', './cmd/sign-update', key, public, archive,
                    capture_output=True, text=True).stdout.strip()
    manifest = {
        'version': VERSION,
        'notes': notes,
        'date': datetime.datetime.now(datetime.timezone.utc).strftime('%Y-%m-%dT%H:%M:%SZ'),
        'url': url_prefix + archive.name,
        'size': archive.stat().st_size,
        'signature': signature,
    }
    (updates / f'update-{TARGET}.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--release-key', type=Path, help='prepare signed release assets using the existing key outside the repository')
    parser.add_argument('--notes', type=Path, help='release notes for a signed release')
    args = parser.parse_args()
    if args.notes and not args.release_key:
        parser.error('--notes requires --release-key')
    dist = ROOT / 'dist'
    package = dist / NAME
    portable = package / f'{NAME}-portable.zip'
    destination = dist / (f'release-v{VERSION}-windows-x64' if args.release_key else f'{NAME}-safe-update-qa.zip')
    if destination.exists():
        raise SystemExit(f'Refusing to replace {destination}')
    rows = (package / 'SHA256SUMS').read_text().splitlines()
    checksums = {name: digest for digest, name in (row.split(maxsplit=1) for row in rows if row)}
    with portable.open('rb') as stream:
        if hashlib.file_digest(stream, 'sha256').hexdigest() != checksums[portable.name]:
            raise SystemExit('The portable ZIP does not match its SHA-256 checksum')

    if args.release_key:
        key = args.release_key.resolve(strict=True)
        if key.is_relative_to(ROOT):
            parser.error('keep the release key outside the repository')
        notes = args.notes.read_text(encoding='utf-8') if args.notes else ''
        if len(notes.encode('utf-8')) > 128 * 1024:
            parser.error('release notes must be at most 128 KiB')
        with tempfile.TemporaryDirectory(dir=dist, prefix='.windows-update-release-') as temp:
            stage = Path(temp)
            with zipfile.ZipFile(portable) as archive:
                archive.extractall(stage)
            output = stage / 'release'
            output.mkdir()
            write_update(active_runtime(stage / 'MochiPic'), output, key, ROOT / 'scripts/update-public-key.txt',
                         notes, f'https://github.com/luzhenhua/mochipic/releases/download/v{VERSION}/')
            output.rename(destination)
        print(destination)
        return

    with tempfile.TemporaryDirectory(dir=dist, prefix='.windows-update-qa-') as tmp, \
            tempfile.TemporaryDirectory(prefix='mochipic-update-qa-key-') as keys:
        stage = Path(tmp)
        key = Path(keys) / 'mygo-update.key'
        public = Path(keys) / 'mygo-update.pub'
        run('mygo', 'keygen', '-o', keys, stdout=subprocess.DEVNULL)
        public_key = public.read_text(encoding='ascii').strip()

        payload = stage / 'MochiPic-Update-QA'
        updates = payload / 'Updates'
        updates.mkdir(parents=True)
        with zipfile.ZipFile(portable) as archive:
            archive.extractall(stage)
        installation = stage / 'MochiPic'
        runtime = active_runtime(installation).rename(payload / 'Runtime')
        write_update(runtime, updates, key, public, 'Offline updater acceptance test.')
        (runtime / '.mygo-files').unlink()
        shutil.copy2(public, updates / 'mygo-update.pub')

        env = {**os.environ, 'GOOS': 'windows', 'GOARCH': 'amd64', 'CGO_ENABLED': '0'}
        flags = update_flags(public_key, QA_FEED)
        run('go', 'build', '-trimpath', '-ldflags', f'-H windowsgui {flags}',
            '-o', payload / 'Launcher.exe', './cmd/mochipic-launcher', env=env)
        run('go', 'test', '-c', '-trimpath', '-ldflags', f'-X imageflow/internal/releaseqa.fixtureAddress={urlsplit(QA_FEED).netloc}',
            '-o', payload / 'updater.test.exe', './internal/releaseqa',
            env=env)
        samples = payload / 'Samples'
        samples.mkdir()
        shutil.copy2(ROOT / 'internal/imaging/testdata/still.jxl', samples)
        for filename in ('Run-Update-Checks.cmd', 'Run-Update-Checks.ps1'):
            shutil.copy2(ROOT / 'scripts/windows-qa' / filename, payload)

        output = stage / destination.name
        with zipfile.ZipFile(output, 'w', compression=zipfile.ZIP_DEFLATED) as archive:
            for path in sorted(payload.rglob('*')):
                if path.is_file():
                    archive.write(path, path.relative_to(stage).as_posix())
            if archive.testzip() is not None:
                raise ValueError('offline updater QA ZIP failed its integrity check')
        output.rename(destination)
    print(destination)


if __name__ == '__main__':
    main()
