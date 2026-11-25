# GitHub Releases 与应用内更新

当前公开试用版为 [v0.0.26](https://github.com/luzhenhua/mochipic/releases/tag/v0.0.26)。提供六种语言的 macOS 安装盘和应用 ZIP、Windows x64 安装程序和便携 ZIP，以及两端的应用内更新。已分别从 macOS 0.0.16 和 Windows 0.0.25（Wine）验证生产更新及重启。

本机开发包可以直接用 `python3 scripts/package-app.py` 构建。为 Apple Silicon 准备一次发布时，先递增 `internal/ui/window.go` 中的补丁版本、添加 `docs/release-notes/` 中的版本说明，再运行：

```sh
version=0.0.26 # 示例，准备下一版时替换为当次版本
mochipic_key="/path/to/the/release-signing-key"
python3 scripts/package-app.py --release-key "$mochipic_key" --notes "docs/release-notes/$version.md"
scripts/verify-bundle.sh
MOCHIPIC_VERIFY_RELEASE="$PWD/dist/release-v$version" scripts/go.sh test -v -count=1 ./internal/releaseqa
(cd "dist/release-v$version" && shasum -a 256 -c SHA256SUMS)
```

签发时复用本机为 MochiPic 创建的 MyGo 私钥。私钥保存在仓库之外，权限为 `0600`，请在密码管理器或其他安全位置备份。仓库只保留 `scripts/update-public-key.txt` 中的公钥；已安装客户端只接受这把钥匙签发的更新。不要替换它来解决发布失败，也不要把私钥上传为附件。只有建立**另一个独立的更新渠道**时，才需要为它生成新的钥匙和应用包。

在 [luzhenhua/mochipic 的 Releases](https://github.com/luzhenhua/mochipic/releases) 为当次版本创建发布草稿，指定发布仓库中用于该版本的标签或提交。收齐同一构建的以下附件并核对后，设为最新并公开：

- `MochiPic-<版本>-arm64-<语言>.dmg`：六种语言的拖拽安装盘。
- `MochiPic-<版本>-arm64.zip`：供手动安装及更新。
- `mochipic-<版本>-darwin-arm64.tar.gz`：MyGo 下载并校验的完整应用包。
- `update-darwin-arm64.json`：MyGo 读取的清单；不得改名。
- `MochiPic-<版本>-windows-x64-setup.exe` 和 `MochiPic-<版本>-windows-x64-portable.zip`：Windows x64 安装程序和便携包。
- `mochipic-<版本>-windows-amd64.tar.gz` 和 `update-windows-amd64.json`：Windows 的签名更新包和更新清单。
- `SHA256SUMS`：覆盖两个平台的安装包、ZIP、更新包、更新清单及源码归档。

发布前随包准备 `MochiPic-<版本>-third-party-source.tar.gz`，包含该构建实际携带的 LGPL 库及 MPL 组件的对应源码、构建配方、补丁和许可。0.0.3 的源码归档中有 18 个按配方或 Cargo.lock 验证过 SHA-256 的源码包。将该归档的 SHA-256 加入发布用清单。在 GitHub 草稿中核对各附件的大小与 SHA-256，传齐后设为最新并公开。

从 0.0.4 起，把同一签名应用制成六种语言的 DMG，方法见[拖拽安装盘](DEVELOPMENT.md#拖拽安装盘)。发给用户先试用；用户确认通过后，主动询问是否发布该版本。收到该版本的明确同意后再推送和发布，并把 DMG 的校验值并入发布用清单。

发布确认后保留用户试用过的签名应用。从这份 `MochiPic.app` 制作 ZIP 和签名更新归档；逐个只读挂载 DMG 核对应用内容，确保安装盘与应用内更新使用同一构建。

发布后可检查 [最新版本的清单](https://github.com/luzhenhua/mochipic/releases/latest/download/update-darwin-arm64.json)，再用一个可写目录中的旧版副本实际测试下载、安装、重启，确认「关于」显示新版本。只上传 ZIP、仅保存草稿或发布为预发布版本，都不会让稳定版收到更新。没有正式 Release 时，GitHub 返回 404。0.0.1 没有更新功能，需要手动安装一次。

## Windows 更新验收

Windows 使用独立的 `update-windows-amd64.json`。锁定版本的 MyGo 会整块替换运行资源目录，可能移除其中的用户文件。离线验收流程让 MyGo 在隔离目录验签解包，把验签后的文件安装到新的独立目录，最后切换当前版本记录。[离线更新验收包](WINDOWS_TEST.md#开发用离线更新验收)

自 0.0.20 起提供应用内检查和自动检查开关。自 0.0.21 起，安装包和签名更新包均携带运行文件归属清单。首次分发以 0.0.21 或更新的安装包建立支持安全更新与卸载的启动器。此前的试用安装目录缺少清单，卸载会保留这部分旧文件。[公开 Windows 清单](https://github.com/luzhenhua/mochipic/releases/latest/download/update-windows-amd64.json) 自 0.0.26 上线。试用版仍待 Windows 10 与 11 原生验收旧版安装、更新、重启及卸载。

对已经过试用的 Windows 安装版本，复用对应便携包制作发布附件。向 `python3 scripts/package-windows-update-qa.py` 传入 `--release-key`（现有、仓库外的 MyGo 私钥）及 `--notes`（版本说明文件），会在 `dist/release-v<版本>-windows-x64` 生成 Windows 签名归档和对应清单。该命令只生成本地文件。获得当次版本发布授权后，再加入 Windows 安装包、便携包、第三方源码包及校验值，核对 macOS 更新附件一并齐备。

同一 GitHub Release 同时提供两个平台时，核对两个清单和各自归档后再设为 Latest。仅测试其中一个平台时继续保留已发布的稳定更新源。

0.0.26 的 Windows 发布包使用重编的 libvips 8.18.7，关闭 FFTW 和 Poppler。签发前核对 Windows 运行目录内的 DLL 导入、安装版与便携版的运行文件及签名包内容。随发布提供单独的 Windows 第三方源码归档和构建配方。GitHub 首页包含两端的实测安装链接。官网从 GitHub Latest 读取当次的安装链接。

当前构建是本机试用包，使用临时签名。跨设备正式分发前，仍须在干净的 Mac 上验收，并完成 Developer ID 签名和公证。签名及公证会改变应用内容，应先完成，再归档并签发 MyGo 更新包。
