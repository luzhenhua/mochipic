#!/bin/bash
# Optional camera QA. Reuse a checked corpus, or download nine CC0 samples.
set -euo pipefail
cd "$(dirname "$0")/.."
mkdir -p .scratch
run="$(mktemp -d "$PWD/.scratch/raw-corpus.XXXXXX")"
samples="${1:-$run}"
samples="$(cd "$samples" && pwd -P)"
python3 - "$samples" "$run" <<'PY'
import concurrent.futures
import hashlib
import json
import pathlib
import re
import subprocess
import sys
import urllib.parse
import urllib.request

root, run = map(pathlib.Path, sys.argv[1:])
spec = json.loads(pathlib.Path("scripts/raw-fixtures.json").read_text())

def check(path, sha):
    digest = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1 << 20), b""):
            digest.update(chunk)
    if digest.hexdigest() != sha:
        raise ValueError(f"SHA-256 mismatch: {path}")

if root == run.resolve():
    with urllib.request.urlopen("https://raw.pixls.us/json/getrepository.php", timeout=25) as f:
        rows = json.load(f)["data"]
    urls = {}
    for row in rows:
        link = re.search(r"href='([^']+)'", row[7])
        if not link or "creativecommons.org/publicdomain/zero" not in row[5]:
            continue
        u = urllib.parse.urlsplit(link[1])
        if u.scheme == "https" and u.netloc == "raw.pixls.us":
            parts = u.path.split("/")
            if len(parts) >= 5 and parts[1] == "getfile.php":
                path = urllib.parse.quote(urllib.parse.unquote(u.path), safe="/")
                urls[parts[2]] = urllib.parse.urlunsplit(u._replace(path=path))

    def fetch(sample):
        key, (name, sha) = sample
        part = root / (name + ".part")
        subprocess.run(["curl", "-fsSL", "--retry", "2", "--max-time", "100",
                        "--max-filesize", "167772160", urls[key], "-o", str(part)], check=True)
        check(part, sha)
        part.rename(root / name)

    with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
        list(pool.map(fetch, spec.items()))
else:
    for name, sha in spec.values():
        check(root / name, sha)
print(f"Nine camera fixtures verified: {root}", flush=True)
PY
scripts/go.sh test -c -o "$run/imaging.test" ./internal/imaging
IMAGEFLOW_RAW_FIXTURES="$samples" FONTCONFIG_FILE="$PWD/scripts/fonts.conf" \
  /usr/bin/time -l "$run/imaging.test" -test.run '^TestCameraRawCorpus$' -test.count=1 -test.v
