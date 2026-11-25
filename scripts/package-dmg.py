#!/usr/bin/env python3
"""Build and verify localized drag-to-install disk images from a signed .app."""
import argparse
import hashlib
import json
import os
import pathlib
import platform
import plistlib
import re
import subprocess
import tempfile
import unicodedata
from contextlib import contextmanager

try:
    import dmgbuild
    from ds_store import DSStore
except ModuleNotFoundError as err:
    raise SystemExit("Install scripts/installer-requirements.txt in the installer virtual environment.") from err

ROOT = pathlib.Path(__file__).resolve().parent.parent
COPY = ROOT / "scripts/installer-copy.json"
COMMAND = 'xattr -dr com.apple.quarantine "/Applications/MochiPic.app"'
APP = "MochiPic.app"
# Finder stores the outer window size; allow room for the title and path bars.
SIZE = (676, 510)
APP_CENTER = (172, 212)
APPLICATIONS_CENTER = (505, 212)
HELP_CENTER = (338, 326)


def run(*args, capture=False):
    cmd = [str(arg) for arg in args]
    result = subprocess.run(cmd, stdout=subprocess.PIPE if capture else None, check=True)
    return result.stdout


def bundle_info(bundle):
    with (bundle / "Contents/Info.plist").open("rb") as f:
        return plistlib.load(f)


def sha256(file):
    with file.open("rb") as f:
        digest = hashlib.sha256()
        for chunk in iter(lambda: f.read(1 << 20), b""):
            digest.update(chunk)
    return digest.hexdigest()


def localized_copy():
    locales = json.loads(COPY.read_text(encoding="utf-8"))
    expected = {p.stem for p in (ROOT / "internal/l10n/locales").glob("*.json")}
    if set(locales) != expected:
        raise ValueError("installer copy must cover the same locales as the app")
    for tag, data in locales.items():
        for field in ("applications", "helpFile"):
            name = data[field]
            if not name or name.startswith(".") or "/" in name or ":" in name or "\0" in name:
                raise ValueError(f"{tag}: invalid {field} filename")
        if not data["helpFile"].endswith(".txt") or not data["helpHint"]:
            raise ValueError(f"{tag}: missing installer help")
        if data["help"].count("{command}") != 1:
            raise ValueError(f"{tag}: help needs one quarantine command")
    return locales


def help_text(data):
    return "\n".join(data["help"]).replace("{command}", COMMAND) + "\n"


@contextmanager
def mounted(image, mountpoint):
    mountpoint.mkdir()
    result = run(
        "/usr/bin/hdiutil", "attach", "-plist", "-readonly", "-nobrowse", "-noautoopen",
        "-mountpoint", mountpoint, image, capture=True,
    )
    partition = next(item for item in plistlib.loads(result)["system-entities"] if item.get("mount-point"))
    try:
        yield pathlib.Path(partition["mount-point"])
    finally:
        run("/usr/bin/hdiutil", "detach", "-quiet", partition["dev-entry"])


def verify_image(image, mountpoint, app_info, executable_sha, locations, folder, help_name, text):
    run("/usr/bin/hdiutil", "verify", "-quiet", image)
    with mounted(image, mountpoint) as disk:
        entries = {unicodedata.normalize("NFD", p.name) for p in disk.iterdir() if not p.name.startswith(".")}
        if entries != {APP, folder, help_name} or os.readlink(disk / folder) != "/Applications":
            raise ValueError(f"{image.name}: wrong visible files or Applications link")
        if (disk / help_name).read_text(encoding="utf-8") != text:
            raise ValueError(f"{image.name}: help text was altered")
        run("/usr/bin/xattr", "-p", "com.apple.ResourceFork", disk / help_name, capture=True)
        with DSStore.open(str(disk / ".DS_Store"), "r") as store:
            view = store["."]["icvp"]
            if view["backgroundType"] != 2 or not view["backgroundImageAlias"]:
                raise ValueError(f"{image.name}: missing Finder background")
            if any(store[name]["Iloc"] != pos for name, pos in locations.items()):
                raise ValueError(f"{image.name}: wrong Finder icon positions")
        if not (disk / ".background.tiff").is_file():
            raise ValueError(f"{image.name}: missing installer artwork")
        app = disk / APP
        exe = app / "Contents/MacOS" / app_info["CFBundleExecutable"]
        if bundle_info(app) != app_info or sha256(exe) != executable_sha:
            raise ValueError(f"{image.name}: app does not match the signed build")
        run("/usr/bin/codesign", "--verify", "--deep", "--strict", app)


def create_image(image, app, app_info, executable_sha, art_tool, work, tag, copy):
    work.mkdir()
    run(art_tool, COPY, tag, work)
    # HFS+ stores filenames decomposed. The Finder records must use that spelling.
    folder = unicodedata.normalize("NFD", copy["applications"])
    help_name = unicodedata.normalize("NFD", copy["helpFile"])
    guide = work / help_name
    text = help_text(copy)
    guide.write_text(text, encoding="utf-8")
    run(art_tool, "--set-help-icon", work / "help.png", guide)
    locations = {APP: APP_CENTER, folder: APPLICATIONS_CENTER, help_name: HELP_CENTER}
    dmgbuild.build_dmg(str(image), "MochiPic", settings={
        "format": "UDZO",
        "filesystem": "HFS+",
        "files": [(str(app), APP), str(guide)],
        "symlinks": {folder: "/Applications"},
        "background": str(work / "background.tiff"),
        "window_rect": ((160, 140), SIZE),
        "icon_locations": locations,
        "default_view": "icon-view",
        "icon_size": 172,
        "text_size": 15,
    })
    verify_image(image, work / "mounted", app_info, executable_sha, locations, folder, help_name, text)
    print(f"{tag}: {image.name} verified", flush=True)


def main():
    if platform.system() != "Darwin" or platform.machine() != "arm64":
        raise SystemExit("Run on an Apple Silicon Mac.")
    locales = localized_copy()
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--app", type=pathlib.Path, default=ROOT / "dist" / APP)
    parser.add_argument("--locale", choices=tuple(locales), action="append", help="build only this locale (repeatable)")
    parser.add_argument("--output", type=pathlib.Path, help="new output directory (default: dist/installers-vVERSION)")
    args = parser.parse_args()
    app = args.app.resolve(strict=True)
    info = bundle_info(app)
    version = info["CFBundleShortVersionString"]
    if info["CFBundleExecutable"] != "MochiPic" or not re.fullmatch(r"[0-9]+\.[0-9]+\.[0-9]+", version):
        parser.error("--app must point to a versioned MochiPic bundle")
    output = (args.output or ROOT / "dist" / f"installers-v{version}").absolute()
    if output.exists() or output.is_symlink():
        parser.error(f"output already exists: {output}")
    executable = app / "Contents/MacOS/MochiPic"
    if run("/usr/bin/lipo", "-archs", executable, capture=True).strip() != b"arm64":
        parser.error("expected an arm64 app bundle")
    run("/usr/bin/codesign", "--verify", "--deep", "--strict", app)
    executable_sha = sha256(executable)
    output.parent.mkdir(parents=True, exist_ok=True)
    tags = list(dict.fromkeys(args.locale or locales))
    with tempfile.TemporaryDirectory(prefix=".installer-build-", dir=output.parent) as tmp:
        work = pathlib.Path(tmp)
        art_tool = work / "installer-art"
        run("/usr/bin/xcrun", "swiftc", "-O", ROOT / "scripts/installer-art.swift", "-o", art_tool)
        publish = work / "verified"
        publish.mkdir()
        for tag in tags:
            name = f"MochiPic-{version}-arm64-{tag}.dmg"
            create_image(publish / name, app, info, executable_sha, art_tool, work / tag, tag, locales[tag])
        (publish / "SHA256SUMS").write_text(
            "".join(f"{sha256(p)}  {p.name}\n" for p in sorted(publish.glob("*.dmg"))), encoding="ascii"
        )
        publish.rename(output)
    print(f"Installers: {output}", flush=True)


if __name__ == "__main__":
    main()
