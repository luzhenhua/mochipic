"""Read the selected immutable runtime from a Windows portable package."""

import base64
import re


UPDATE_FEED = 'https://github.com/luzhenhua/mochipic/releases/latest/download/update-windows-amd64.json'
QA_FEED = 'http://127.0.0.1:47026/manifest'


def update_flags(public_key, feed):
    if len(base64.b64decode(public_key, validate=True)) != 32:
        raise ValueError('expected a MyGo Ed25519 public key')
    return (
        '-X github.com/egoist/mygo.production=1 '
        f'-X github.com/egoist/mygo.packageUpdateFeed={feed} '
        f'-X github.com/egoist/mygo.packageUpdateKey={public_key}'
    )


def active_runtime(installation):
    runtime_name = (installation / 'current-runtime').read_text(encoding='ascii').strip()
    if re.fullmatch(r'runtime-[A-Z2-7]{26,100}', runtime_name) is None:
        raise ValueError(f'invalid Windows runtime name: {runtime_name!r}')
    runtime = installation / runtime_name
    if not (runtime / 'MochiPic.exe').is_file():
        raise ValueError(f'active Windows executable is missing from {runtime}')
    return runtime
