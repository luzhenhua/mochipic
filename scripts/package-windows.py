#!/usr/bin/env python3
"""Build the Windows x64 QA installer and a portable bundle from a local SDK."""
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import struct
import subprocess
import tempfile
import zipfile

from windows_layout import UPDATE_FEED, update_flags

ROOT = Path(__file__).resolve().parent.parent
VERSION = re.search(r'const Version = "([\d.]+)"', (ROOT / 'internal/ui/window.go').read_text())[1]
NAME = f'MochiPic-{VERSION}-windows-x64'
SDK = Path(os.environ.get('MOCHIPIC_WINDOWS_SDK', ROOT / '.deps/windows-cross')).resolve()
VIPS = SDK / 'vips-dev-8.18'
LLVM = SDK / 'llvm-mingw-20261117-ucrt-macos-universal/bin'
NSIS = SDK / 'makensis/3.13'
DEST = ROOT / 'dist' / NAME


def run(*args, **options):
    return subprocess.run([str(x) for x in args], cwd=ROOT, check=True, **options)


def windows_icon(stage):
    png = (ROOT / 'design/brand/mochipic-rounded-256.png').read_bytes()
    icon = stage / 'MochiPic.ico'
    icon.write_bytes(struct.pack('<HHHBBBBHHII', 0, 1, 1, 0, 0, 0, 0, 1, 32, len(png), 22) + png)
    return icon


def include_dlls(app, plugins, payload):
    dlls = {p.name.lower(): p for p in (VIPS / 'bin').glob('*.dll')}
    # Use the two codecs built together and the matching LLVM C++ runtimes.
    for p in (
        SDK / 'codecs/heif-install/bin/libheif.dll',
        SDK / 'codecs/de265-install/bin/libde265.dll',
        LLVM.parent / 'x86_64-w64-mingw32/bin/libc++.dll',
        LLVM.parent / 'x86_64-w64-mingw32/bin/libunwind.dll',
    ):
        dlls[p.name.lower()] = p
    system = set()
    copied = set()
    queue = [app, *plugins]
    while queue:
        image = queue.pop()
        listing = run(LLVM / 'llvm-objdump', '-p', image, capture_output=True, text=True).stdout
        for name in re.findall(r'DLL Name: (\S+)', listing):
            key = name.lower()
            if key in copied:
                continue
            if key not in dlls:
                system.add(key)
                continue
            lib = dlls[key]
            shutil.copy2(lib, payload / lib.name)
            copied.add(key)
            queue.append(lib)
    return sorted(copied), sorted(system)


def build_launcher(stage, resources, output, flags):
    # Reuse the app icon and manifest without adding a Windows-only object to
    # the source tree or pulling CGO DLLs into the launcher.
    launcher = stage / 'launcher'
    launcher.mkdir()
    for source in (ROOT / 'cmd/mochipic-launcher').glob('*.go'):
        shutil.copy2(source, launcher)
    shutil.copy2(resources, launcher / 'resources_windows_amd64.syso')
    run('go', 'build', '-trimpath', '-ldflags', f'-H windowsgui {flags}',
        '-o', output, './' + launcher.relative_to(ROOT).as_posix(),
        env={**os.environ, 'GOOS': 'windows', 'GOARCH': 'amd64', 'CGO_ENABLED': '0'})


def write_runtime_receipt(runtime):
    files = {}
    for path in sorted(runtime.rglob('*')):
        if path.is_file():
            with path.open('rb') as stream:
                files[path.relative_to(runtime).as_posix()] = hashlib.file_digest(stream, 'sha256').hexdigest()
    (runtime / 'runtime-files.json').write_text(json.dumps({'schema': 1, 'files': files}, indent=2) + '\n', encoding='ascii')


def main():
    if DEST.exists():
        raise SystemExit(f'Refusing to replace {DEST}')
    DEST.parent.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(dir=DEST.parent, prefix='.windows-build-') as tmp:
        stage = Path(tmp)
        payload, result = stage / 'payload', stage / 'result'
        payload.mkdir()
        result.mkdir()
        icon = windows_icon(stage)
        runtime_id = base64.b32encode(secrets.token_bytes(16)).decode('ascii').rstrip('=')
        runtime = payload / f'runtime-{runtime_id}'
        runtime.mkdir()
        app = runtime / 'MochiPic.exe'
        resources = stage / 'mochipic.rc'
        manifest = ROOT / 'scripts/windows-app.manifest'
        resources.write_text(f'1 ICON "{icon.as_posix()}"\n1 24 "{manifest.as_posix()}"\n')
        resource_object = stage / 'mochipic-resources.o'
        run(LLVM / 'x86_64-w64-mingw32-windres', '-i', resources, '-o', resource_object)
        release_public_key = (ROOT / 'scripts/update-public-key.txt').read_text(encoding='ascii').strip()
        flags = update_flags(release_public_key, UPDATE_FEED)
        run(ROOT / 'scripts/build-windows.sh', 'build', '-trimpath', '-ldflags',
            f'-H windowsgui {flags} -extldflags "{resource_object}"', '-o', app, './cmd/imageflow')
        build_launcher(stage, resource_object, payload / app.name, flags)
        (payload / 'current-runtime').write_text(runtime.name + '\n', encoding='ascii')
        (payload / '.runtime-lock').touch(exist_ok=False)
        (runtime / 'runtime-version').write_text(VERSION + '\n', encoding='ascii')
        # The upstream Windows SDK uses the directory of libvips-42.dll as its libdir.
        sdk_modules = VIPS / 'bin/vips-modules-8.18'
        modules = runtime / sdk_modules.name
        modules.mkdir()
        for name in ('vips-jxl.dll', 'vips-magick.dll'):
            shutil.copy2(sdk_modules / name, modules / name)
        copied, system = include_dlls(app, list(modules.iterdir()), runtime)
        shutil.copytree(VIPS / 'etc/fonts', runtime / 'etc/fonts')
        magick = runtime / 'etc/ImageMagick-7'
        magick.mkdir()
        (magick / 'policy.xml').write_text('''<?xml version="1.0"?>
<policymap>
  <policy domain="delegate" rights="none" pattern="*"/>
  <policy domain="filter" rights="none" pattern="*"/>
  <policy domain="coder" rights="none" pattern="*"/>
  <policy domain="coder" rights="read" pattern="{BMP,BMP2,BMP3,ICO,ICON,PSD}"/>
  <policy domain="resource" name="memory" value="512MiB"/>
  <policy domain="resource" name="map" value="512MiB"/>
  <policy domain="resource" name="disk" value="0"/>
  <policy domain="resource" name="list-length" value="256"/>
</policymap>
''', encoding='utf-8')
        notices = runtime / 'ThirdParty'
        notices.mkdir()
        for file in ('LICENSE', 'README.md', 'versions.json'):
            shutil.copy2(VIPS / file, notices / file)
        (notices / 'codec-build.txt').write_text('libheif 1.23.7 with libde265 1.1.3 and AOM 3.15.1\n')
        shutil.copy2(SDK / 'codecs/libheif-1.23.7/COPYING', notices / 'libheif-COPYING')
        shutil.copy2(SDK / 'codecs/libde265-1.1.3/COPYING', notices / 'libde265-COPYING')
        shutil.copy2(ROOT / 'internal/ui/assets/welcome/CREDITS.txt' , notices / 'PhotoCredits.txt')
        (notices / 'runtime-dlls.txt').write_text('\n'.join(copied) + '\n\nWindows system imports:\n' + '\n'.join(system) + '\n')
        shutil.copy2(ROOT / 'docs/WINDOWS_TEST.md', runtime / 'Windows-Test.md')
        write_runtime_receipt(runtime)
        files = sorted(p.relative_to(payload) for p in payload.rglob('*') if p.is_file())
        dirs = sorted((p.relative_to(payload) for p in payload.rglob('*') if p.is_dir()),
                      key=lambda p: (-len(p.parts), str(p)))

        def quote(path):
            return '"$INSTDIR\\' + '\\'.join(path.parts) + '"'

        path_guards = stage / 'guard-paths.nsh'
        protected = [*reversed(dirs), *files, Path('Uninstall.exe'), Path(f'.activate-{runtime.name}')]
        path_guards.write_text(''.join('!insertmacro RejectReparse ' + quote(p) + '\n' for p in protected))
        installer = result / f'{NAME}-setup.exe'
        run(NSIS / 'bin/makensis', '-V2', f'-DPAYLOAD={payload}', f'-DINSTALLER={installer}', f'-DAPP_ICON={icon}',
            f'-DAPP_VERSION={VERSION}', f'-DRUNTIME_DIRECTORY={runtime.name}',
            f'-DPATH_GUARDS={path_guards}', ROOT / 'scripts/windows-installer.nsi',
            env={**os.environ, 'NSISDIR': str(NSIS / 'share/nsis')})
        with zipfile.ZipFile(result / f'{NAME}-portable.zip', 'w', compression=zipfile.ZIP_DEFLATED) as archive:
            for rel in files:
                archive.write(payload / rel, '/'.join(('MochiPic', *rel.parts)))
            if archive.testzip() is not None:
                raise RuntimeError('portable archive integrity check failed')
        sums = []
        for file in sorted(result.iterdir()):
            with file.open('rb') as stream:
                digest = hashlib.file_digest(stream, 'sha256').hexdigest()
            sums.append(f'{digest}  {file.name}')
        (result / 'SHA256SUMS').write_text('\n'.join(sums) + '\n')
        result.rename(DEST)
    print(f'{DEST} ({len(copied)} private runtime DLLs; {len(system)} system imports)')


if __name__ == '__main__':
    main()
