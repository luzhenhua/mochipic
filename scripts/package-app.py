#!/usr/bin/env python3
"""Build a relocatable, locally signed Apple Silicon .app from the pinned dependencies."""
import argparse
import base64
import datetime
import hashlib
import json
import pathlib
import plistlib
import re
import shutil
import subprocess
import tarfile
import tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
DIST = ROOT / "dist"
NATIVE = ROOT / ".deps/native"
VIPS = ROOT / ".deps/vips"
CATALOGS = ROOT / "internal/l10n/locales"
FORMATS = ROOT / "internal/imageformat/formats.json"
BRAND = json.loads((CATALOGS / "en.json").read_text(encoding="utf-8"))["app.name"]
ARTWORK = ROOT / "internal/ui/assets/mochipic.png"
IMAGE_DOCUMENT_KIND = f"{BRAND} Image"
BUNDLE_ID = "com.imageflow.local"
SYSTEM = ("/usr/lib/", "/System/Library/")
VERSION = re.search(r'const Version = "([\d.]+)"', (ROOT / "internal/ui/window.go").read_text()).group(1)
REPO = "https://github.com/luzhenhua/mochipic"
UPDATE_TARGET = "darwin-arm64"
UPDATE_MANIFEST = f"update-{UPDATE_TARGET}.json"
UPDATE_FEED = f"{REPO}/releases/latest/download/{UPDATE_MANIFEST}"
PUBLIC_KEY = ROOT / "scripts/update-public-key.txt"


def call(*args, capture=False):
    return subprocess.run(
        [str(a) for a in args], cwd=ROOT, text=True, check=True,
        stdout=subprocess.PIPE if capture else None,
    ).stdout


def dependencies(binary, dylib=False):
    rows = call("otool", "-L", binary, capture=True).splitlines()[1:]
    return [line.strip().split(" (compatibility version ", 1)[0] for line in rows[int(dylib):]]


def paths(binary):
    return re.findall(r"cmd LC_RPATH\s+cmdsize \d+\s+path (.+?) \(offset \d+\)",
                      call("otool", "-l", binary, capture=True))


def lookup(dependency, loader):
    name = pathlib.Path(dependency).name
    candidates = [VIPS / "lib" / name, NATIVE / "lib" / name]
    if dependency.startswith("@loader_path/"):
        candidates.insert(0, loader.parent / dependency[len("@loader_path/"):])
    if dependency.startswith("/"):
        candidates.insert(0, pathlib.Path(dependency))
    for item in candidates:
        if item.is_file() and any(item.resolve().is_relative_to(p) for p in (VIPS, NATIVE)):
            return item.resolve()
    raise RuntimeError(f"{loader.name}: cannot resolve {dependency}")


def strip_rpaths(binary):
    for rpath in sorted(set(paths(binary))):
        call("install_name_tool", "-delete_rpath", rpath, binary)


def collect_libraries(exe, libs):
    sources = {}
    queue = [(exe, exe, False)]
    min_os = (13, 0, 0)
    while queue:
        source, dest, dylib = queue.pop(0)
        if call("lipo", "-archs", source, capture=True).split() != ["arm64"]:
            raise RuntimeError(f"unexpected architecture: {source}")
        for version in re.findall(r"\bminos\s+(\d+(?:\.\d+){0,2})", call("otool", "-l", source, capture=True)):
            parts = tuple(int(part) for part in version.split("."))
            min_os = max(min_os, parts + (0,) * (3 - len(parts)))
        for ref in dependencies(source, dylib):
            if ref.startswith(SYSTEM):
                continue
            origin = lookup(ref, source)
            name = pathlib.Path(ref).name
            local = libs / name
            if name not in sources:
                sources[name] = origin
                shutil.copy2(origin, local)
                local.chmod(0o755)
                queue.append((origin, local, True))
            elif sources[name] != origin:
                raise RuntimeError(f"ambiguous dylib: {name}")
            install_name = ("@loader_path/" if dylib else "@executable_path/../Frameworks/") + name
            if ref != install_name:
                call("install_name_tool", "-change", ref, install_name, dest)
        if dylib:
            call("install_name_tool", "-id", "@rpath/" + dest.name, dest)
        strip_rpaths(dest)
    return sources, ".".join(str(i) for i in min_os[:3] if i is not None)


def write_notices(dest, sources):
    licenses = dest / "Licenses"
    licenses.mkdir()
    version = "8.18.7"
    source_dir = ROOT / f".deps/vips-{version}"
    shutil.copy2(source_dir / "LICENSE", licenses / "libvips-LICENSE")
    shutil.copy2(source_dir / "libvips/foreign/libnsgif/COPYING", licenses / "libnsgif-COPYING")
    rows = [{
        "package": "libvips", "version": version, "license": "LGPL-2.1-or-later",
        "source": f"https://github.com/libvips/libvips/releases/tag/v{version}",
        "libraries": sorted(name for name, p in sources.items() if p.is_relative_to(VIPS)),
    }, {
        "package": "libnsgif", "license": "MIT", "includedIn": "libvips",
        "source": f"https://github.com/libvips/libvips/tree/v{version}/libvips/foreign/libnsgif",
    }]
    by_file = {}
    for meta in (NATIVE / "conda-meta").glob("*.json"):
        data = json.loads(meta.read_text())
        for f in data["files"]:
            by_file[f] = data
    used = {}
    recipes = dest / "DependencyRecipes"
    for name, path in sources.items():
        if path.is_relative_to(NATIVE):
            data = by_file[str(path.relative_to(NATIVE))]
            used.setdefault(data["name"], (data, []))[1].append(name)
    for name, (data, dylibs) in sorted(used.items()):
        package_dir = pathlib.Path(data["link"]["source"])
        license_dir = package_dir / "info/licenses"
        if license_dir.is_dir():
            shutil.copytree(license_dir, licenses / name)
        if (package_dir / "info/recipe").is_dir():
            recipes.mkdir(exist_ok=True)
            shutil.copytree(package_dir / "info/recipe", recipes / name)
        rows.append({
            "package": name, "version": data["version"], "build": data["build"],
            "license": data.get("license"), "binarySource": data["url"],
            "recipe": data.get("extra", {}).get("feedstock-name") or data.get("name"),
            "sha256": data.get("sha256"), "libraries": sorted(dylibs),
        })
    modules = call("go", "list", "-m", "-f", "{{.Path}}\t{{.Version}}\t{{.Dir}}", "all", capture=True)
    for row in modules.splitlines():
        name, version, directory = row.split("\t")
        if not version or not directory:
            continue
        src = pathlib.Path(directory)
        target = licenses / name.replace("/", "_")
        found = False
        for p in src.iterdir():
            if p.is_file() and p.name.upper().startswith(("LICENSE", "COPYING", "NOTICE", "PATENTS")):
                target.mkdir(exist_ok=True)
                shutil.copy2(p, target / p.name)
                found = True
        rows.append({"module": name, "version": version, "licenseTextIncluded": found})
    goroot = pathlib.Path(call("go", "env", "GOROOT", capture=True).strip())
    go_license = next(p for p in (goroot / "LICENSE", goroot.parent / "LICENSE") if p.is_file())
    shutil.copy2(go_license, licenses / "Go-LICENSE")
    (dest / "ThirdParty.json").write_text(json.dumps(rows, indent=2) + "\n")
    shutil.copy2(ROOT / "docs/THIRD_PARTY.md", dest / "THIRD_PARTY.md")


def verify_paths(binary, libs, dylib=False):
    prefix = "@loader_path/" if dylib else "@executable_path/../Frameworks/"
    for ref in dependencies(binary, dylib):
        if not ref.startswith(SYSTEM) and (not ref.startswith(prefix) or not (libs / ref[len(prefix):]).is_file()):
            raise RuntimeError(f"bundle has a non-local dependency: {binary.name} → {ref}")
    if paths(binary):
        raise RuntimeError(f"unexpected run path in {binary}")


def release_assets(app, dest, key, notes):
    dest.mkdir()
    archive = dest / f"mochipic-{VERSION}-{UPDATE_TARGET}.tar.gz"
    zip_path = dest / f"{BRAND}-{VERSION}-arm64.zip"
    manifest = dest / UPDATE_MANIFEST

    def archive_entry(entry):
        entry.uid = entry.gid = 0
        entry.uname = entry.gname = ""
        return entry

    # MyGo expects one complete, already signed .app at the archive root.
    with tarfile.open(archive, "w:gz") as tar:
        tar.add(app, arcname=app.name, filter=archive_entry)
    signature = call("go", "run", "./cmd/sign-update", key, PUBLIC_KEY, archive, capture=True).strip()
    manifest.write_text(json.dumps({
        "version": VERSION,
        "notes": notes,
        "date": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "url": f"{REPO}/releases/download/v{VERSION}/{archive.name}",
        "size": archive.stat().st_size,
        "signature": signature,
    }, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    call("ditto", "-c", "-k", "--sequesterRsrc", "--keepParent", app, zip_path)
    sums = []
    for file in (zip_path, archive, manifest):
        digest = hashlib.sha256()
        with file.open("rb") as stream:
            for chunk in iter(lambda: stream.read(1 << 20), b""):
                digest.update(chunk)
        sums.append(f"{digest.hexdigest()}  {file.name}\n")
    (dest / "SHA256SUMS").write_text("".join(sums), encoding="ascii")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--release-key", type=pathlib.Path, help="MyGo private key file outside the repository; create GitHub release assets")
    parser.add_argument("--notes", type=pathlib.Path, help="Markdown release notes for the update manifest")
    args = parser.parse_args()
    if args.notes and not args.release_key:
        parser.error("--notes requires --release-key")
    notes = args.notes.read_text(encoding="utf-8") if args.notes else ""
    if len(notes.encode("utf-8")) > 128 * 1024:
        parser.error("release notes must be at most 128 KiB")
    public_key = PUBLIC_KEY.read_text(encoding="ascii").strip()
    if len(base64.b64decode(public_key, validate=True)) != 32:
        parser.error("expected a MyGo Ed25519 public key")
    update_flags = (
        "-X github.com/egoist/mygo.production=1 "
        f"-X github.com/egoist/mygo.packageUpdateFeed={UPDATE_FEED} "
        f"-X github.com/egoist/mygo.packageUpdateKey={public_key}"
    )
    release = DIST / f"release-v{VERSION}"
    if args.release_key:
        args.release_key = args.release_key.resolve(strict=True)
        if args.release_key.is_relative_to(ROOT):
            parser.error("keep the update private key outside the repository")
        if release.exists() or release.is_symlink():
            parser.error(f"release assets already exist at {release}")

    DIST.mkdir(exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".imageflow-build-", dir=DIST) as stage:
        stage = pathlib.Path(stage)
        app = stage / f"{BRAND}.app"
        contents = app / "Contents"
        exe = contents / "MacOS" / BRAND
        libs = contents / "Frameworks"
        resources = contents / "Resources"
        for d in (exe.parent, libs, resources):
            d.mkdir(parents=True)
        call("bash", ROOT / "scripts/go.sh", "build", "-trimpath", "-ldflags", update_flags, "-o", exe, "./cmd/imageflow")
        sources, min_os = collect_libraries(exe, libs)
        iconset = stage / f"{BRAND}.iconset"
        call("xcrun", "swift", ROOT / "scripts/app-icon.swift", ARTWORK, iconset)
        call("iconutil", "-c", "icns", iconset, "-o", resources / f"{BRAND}.icns")
        shutil.copy2(ROOT / "internal/ui/assets/welcome/CREDITS.txt", resources / "PhotoCredits.txt")
        locales = sorted(CATALOGS.glob("*.json"))
        for catalog in locales:
            directory = resources / (catalog.stem + ".lproj")
            directory.mkdir()
            messages = json.loads(catalog.read_text(encoding="utf-8"))
            with (directory / "InfoPlist.strings").open("wb") as f:
                plistlib.dump({"CFBundleName": messages["app.name"], "CFBundleDisplayName": messages["app.name"],
                               IMAGE_DOCUMENT_KIND: messages["nav.images"]}, f)
        with (contents / "Info.plist").open("wb") as f:
            plistlib.dump({
                "CFBundlePackageType": "APPL", "CFBundleName": BRAND,
                "CFBundleDisplayName": BRAND, "CFBundleExecutable": BRAND,
                "CFBundleIconFile": f"{BRAND}.icns",
                "CFBundleIdentifier": BUNDLE_ID, "CFBundleShortVersionString": VERSION, "CFBundleVersion": VERSION.rsplit(".", 1)[1],
                "CFBundleDevelopmentRegion": "en", "CFBundleLocalizations": [p.stem for p in locales],
                "LSMinimumSystemVersion": min_os,
                "LSApplicationCategoryType": "public.app-category.graphics-design", "NSHighResolutionCapable": True,
                "CFBundleDocumentTypes": [{
                    "CFBundleTypeName": IMAGE_DOCUMENT_KIND, "CFBundleTypeRole": "Viewer", "LSHandlerRank": "Alternate",
                    "LSItemContentTypes": [uti for spec in json.loads(FORMATS.read_text()) for uti in spec["types"]],
                }],
            }, f)
        shutil.copy2(ROOT / "scripts/fonts.conf", resources / "fonts.conf")
        write_notices(resources, sources)
        verify_paths(exe, libs)
        for lib in sorted(libs.iterdir()):
            verify_paths(lib, libs, True)
            call("codesign", "--force", "--sign", "-", lib)
        call("codesign", "--force", "--sign", "-", app)
        call("codesign", "--verify", "--deep", "--strict", app)
        call("plutil", "-lint", contents / "Info.plist")
        assets = stage / "assets"
        if args.release_key:
            release_assets(app, assets, args.release_key, notes)
        final = DIST / app.name
        if final.exists():
            backup_root = ROOT / ".scratch"
            backup_root.mkdir(exist_ok=True)
            backup = pathlib.Path(tempfile.mkdtemp(prefix="previous-app-", dir=backup_root)) / app.name
            final.rename(backup)
            print(f"Previous bundle preserved at {backup}")
        app.rename(final)
        print(f"{final} ({len(sources)} private dylibs; Mach-O minimum macOS {min_os})")
        if args.release_key:
            assets.rename(release)
            print(f"GitHub release assets staged at {release}")


if __name__ == "__main__":
    main()
