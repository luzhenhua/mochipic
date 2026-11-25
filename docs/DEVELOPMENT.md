# 开发与打包

- 已验证：Apple Silicon Mac17,3、macOS 27.3、Go 1.27.3、CGO=1、Apple Clang 21。
- MyGo CLI 0.8.3 可用于 `mygo doctor`；应用实际使用 Lobe MyGo 钉住的 MyGo Go 模块 `v0.8.4-0.20261108082913-9233663736ca`，由 `go.mod` 和 `go.sum` 固定。应用不需要 CLI 或前端工具。
- `scripts/setup-native.sh` 以 `native-osx-arm64.lock` 安装项目内编译依赖，校验 libvips 8.18.7 源包，并使用 Xcode SDK 在 `.deps/vips` 构建。启用 JPEG、PNG、WebP、TIFF、GIF 读取、libheif、libjxl、OpenJPEG、librsvg、EXIF、LCMS、Pango、Fontconfig；不启用动态模块。更新锁文件后重跑脚本会刷新已有项目内环境。BMP、ICO、PSD 和原始 J2K 码流通过 Image I/O 解码，相机 RAW 使用 Core Image。
- 若 `pkg-config` 找不到 vips，先执行上述脚本，再用 `scripts/go.sh` 调用 Go。后者将私有 `.pc`、SDK、库路径和 Fontconfig 传给该次进程。不修改 shell 配置或系统工具链。
- 直接开发：`scripts/go.sh run ./cmd/imageflow`。测试偏好设置可用 `IMAGEFLOW_CONFIG_DIR` 指向独立目录。生产偏好设置使用 macOS 的 Application Support；读取失败时保留原文件并在设置页显示错误。

## 复现验证

```sh
scripts/go.sh test -count=1 ./...
scripts/go.sh vet ./...
scripts/go.sh build ./...
scripts/go.sh test -race -count=1 ./...
MYGO_E2E=1 scripts/go.sh test -count=1 -v github.com/egoist/mygo/internal/e2e -run '^TestContentWindowFileDrop$'
```

压力测试不随默认测试运行，避免每次修改都生成 1,000 个文件：

```sh
mkdir -p .scratch
scripts/go.sh test -c -o .scratch/imageflow-batch.test ./internal/batch
IMAGEFLOW_STRESS=1 FONTCONFIG_FILE="$PWD/scripts/fonts.conf" \
  /usr/bin/time -l .scratch/imageflow-batch.test -test.run '^TestStress1000$' -test.count=1 -test.v
```

`scripts/visual-qa.sh` 用单独的构建标签和临时偏好设置启动原生窗口，捕获工作台各状态、帧页范围、裁剪框、拼图换序、导出结果以及取消结果。不给路径时输出到新建的 `.scratch/` 子目录。默认使用有[来源说明](../internal/ui/assets/welcome/CREDITS.txt)的欢迎页照片，也可用 `IMAGEFLOW_QA_ASSETS` 指定本机样例目录。脚本验证合并输出、选中重试、取消和原图 SHA-256。MyGo `CapturePage` 保存原生内容。裁剪和拼图的指针拖动由 `internal/ui` 的原生视图测试验证。

`scripts/format-corpus.sh` 在默认测试之外复测 govips/libvips 随附样片（含灰度 ICC JPEG XL、带方向的 JP2 和分层 PSD）和本机生成的 48MP HEIC、APNG、动画 AVIF。该可选脚本用已安装的 FFmpeg（含 SVT-AV1）生成动图样本；默认测试和试用包无需 FFmpeg。`scripts/raw-corpus.sh` 用公开 CC0 样片核对九种 RAW，传入已有样片目录可离线复测。可将单个样片的绝对路径作为 `IMAGEFLOW_QA_RAW` 传给原生验收脚本，或作为 `IMAGEFLOW_VERIFY_RAW` 传给打包验收脚本。

新增格式时更新 `internal/imageformat/formats.json`、对应解码器及样片测试。系统选择器、Finder 文件关联、扫描及输出名称使用同一份格式登记。RAW 的后缀顺序与对应 UTI 顺序一一对应。

拖放手动复核：在 Finder 同时选一张 JPG 和含 PNG 的文件夹，拖到 ImageFlow 图片区，检查新增两张、再次拖入计为重复、两份原图均未变动。

预览渲染基准使用固定的 720 px PNG 和 1200 万像素 JPEG；样图生成不计入计时。它测量从读图到清晰位图的时间，切图首帧和缓存由原生视图测试覆盖。

```sh
scripts/go.sh test -run '^$' -bench '^BenchmarkOriginalPreview$' -benchtime 600ms -count 3 ./internal/imaging
```

## 本机包

`python3 scripts/package-app.py` 将 `go build` 的 arm64 二进制、图标、私有 Fontconfig 配置和第三方许可封入 `dist/MochiPic.app`。脚本用 `otool -L` 计算动态库闭包，复制到 `Contents/Frameworks`，把引用改为相对位置，删除构建机运行路径；在暂存目录用 `codesign --sign -` 临时签名，通过验证才替换本地产物。旧的同名包移到 `.scratch/`。声明的 Mach-O 最低系统版本来自可执行文件和动态库，仅在 macOS 27.3 上运行验证过。

新品牌版本从 `0.0.1` 开始，后续每次改动把 `internal/ui/window.go` 中的补丁版本加一。包和 ZIP 使用同一版本。标题、应用菜单、关于和 Finder 的显示名来自语言目录；应用内部标识和本机偏好目录继续兼容 ImageFlow 的已有设置。

企鹅原图存于 `internal/ui/assets/mochipic.png`。`scripts/app-icon.swift` 为 macOS 图标和透明 PNG 共用圆角比例。更新窗口和「关于」内嵌同一份 `mochipic-128.png`，无需系统另加圆角。未来的 Windows 图标可使用 [256 px 透明 PNG](../design/brand/mochipic-rounded-256.png) 生成多尺寸 ICO。改原图后重新生成两张 PNG：

```sh
xcrun swift scripts/app-icon.swift internal/ui/assets/mochipic.png internal/ui/assets/mochipic-128.png 128
xcrun swift scripts/app-icon.swift internal/ui/assets/mochipic.png design/brand/mochipic-rounded-256.png 256
```

`scripts/verify-bundle.sh` 把应用复制到带空格的另一路径，在没有开发环境变量的进程中启动，验证静态格式、水印、裁剪、拼图和 WebP／AVIF 导出，也将动图及多页 TIFF 的首尾画面实际合并、重新导入并校验数量。dyld 日志确认 libvips、libwebpmux 等非系统库从包内载入。该验证仍发生在开发 Mac 上。发给同事前需在另一台未安装开发依赖的 Apple Silicon Mac 上走一次导入、中文水印和导出；通过 Gatekeeper 的跨设备分发还需 Developer ID 签名与公证。

包含更新清单的 GitHub Release 试用包，见[发布指南](RELEASING.md)。

## 拖拽安装盘

已有签名完整的 `dist/MochiPic.app` 后，使用项目内虚拟环境构建 DMG：

```sh
python3 -m venv .deps/installer-venv
.deps/installer-venv/bin/python -m pip install --require-hashes --only-binary=:all: -r scripts/installer-requirements.txt
.deps/installer-venv/bin/python scripts/package-dmg.py
```

输出为 `dist/installers-v<版本>/` 下的六个 Apple Silicon DMG 和 `SHA256SUMS`。开发时可用 `--locale zh-Hans --output .scratch/installer-preview` 构建单份；目标目录须为空缺。构建使用 Finder 的实际应用图标、指向 `/Applications` 的链接及安装帮助文本。Swift 生成背景的普通与 Retina 图，窗口预留 Finder 标题栏与路径栏的高度。打包脚本逐一挂载验证镜像、帮助文案、布局及应用签名；发布前仍应在 Finder 双击目视检查。安装帮助里记录的终端命令不会被打包程序或安装盘执行。

安装器文案位于 `scripts/installer-copy.json`；键须覆盖所有应用语言。新增语言时同时补齐此处的标题、文件名与帮助。

## 多语言开发

每份 `internal/l10n/locales/<tag>.json` 是一个独立的 ICU 文案目录。应用在构建时嵌入目录；macOS 打包时生成相同的 `CFBundleLocalizations` 和本地化文件类型说明。先运行 `go test ./internal/l10n` 检查新增或变更的译文，具体格式和实机检查见 [翻译贡献指南](../CONTRIBUTING.md)。

设置中的语言会保存到本机偏好，并通过 MyGo 更新视图、组件和应用菜单。调试可设置 `MYGO_LOCALE`，并用单独的 `IMAGEFLOW_CONFIG_DIR` 隔离偏好；原生截图脚本用这种方式保留用户的实际设置。

## 逐图编辑验收

`scripts/go.sh test ./internal/ui -run 'TestNativeFourPhotoEditScopeAndActualExports|TestScopedLogoImportAndRetryKeepTheOriginalPhotoSettings' -count=1` 通过原生控件与真实编码器验证多选编辑、撤销、整批导出和失败重试。

复拍时，在临时目录的 `images/` 放四张本机样片，另外创建空的 `export/`、`config/` 和截图目录。样片按文件名排序，首张使用横向照片。以 `scripts/go.sh build -tags imageflowqa -o <临时程序> ./cmd/imageflow` 构建，运行时设置 `IMAGEFLOW_VISUAL_SCENARIO=edit-scope`、`IMAGEFLOW_VISUAL_FIXTURES=<临时目录的绝对路径>`、`IMAGEFLOW_VISUAL_CAPTURE=<截图目录的绝对路径>`、`IMAGEFLOW_CONFIG_DIR=<config 的绝对路径>`、`FONTCONFIG_FILE=<项目绝对路径>/scripts/fonts.conf` 及 `MYGO_LOCALE=zh-Hans`。程序捕获五个编辑场景和任务结果，并写出四份 WebP。截图驱动只在带 `imageflowqa` 标签的构建中启用。

## Windows x64 交叉构建

在 Apple Silicon Mac 上使用 `scripts/setup-windows-sdk.sh` 准备 LLVM-MinGW、libvips 8.18.7、WebP 和 TIFF 头文件及 NSIS。脚本校验固定版本的归档，依赖保存在 `.deps/windows-cross`。需要本机已有 Go、`pkg-config`、Python 和支持 zstd 的 `tar`。然后用 `scripts/build-windows-codecs.sh` 构建 HEIC／AVIF 解码运行库。这个脚本使用 uv、Ninja 和 AOM 3.15 的头文件，默认查找已有的 macOS SDK。最后运行 `python3 scripts/package-windows.py`，生成测试用安装程序、便携 ZIP 和 SHA-256。

系统支持目标为 Windows 10 x64 及以上。交叉编译或静态核对 DLL 不能验证启动、拖放、显示缩放或文件系统行为。实机操作见 [Windows 验收](WINDOWS_TEST.md)。

安装包生成后可运行 `python3 scripts/package-windows-qa.py`。该命令从同版本便携包复制程序和 DLL，加上交叉编译的文件、图像、批处理与 UI 测试，以及仓库自带的样图。Windows 侧使用 `Run-Checks.cmd` 执行。

已有九个相机样片时，运行 `python3 scripts/package-windows-qa.py --raw-fixtures /绝对路径/RAW样片`，生成独立的离线格式验收包。可用 `scripts/raw-corpus.sh` 获取带校验的公开样片。Windows 侧用 `Run-Format-Checks.cmd` 测相机 RAW、动图与多页 TIFF。RAW 在 Windows 使用 LibRaw，在 macOS 使用 Core Image，相机传感器边缘的处理会导致同一张样片的实际像素尺寸略有差别。

`python3 scripts/package-windows-install-qa.py --previous 0.0.24` 从两份已经打包的发行物中核对并复制原版 NSIS 安装器，交付干净 Windows 安装、覆盖升级及实际卸载验收包。该工具只在指定的 Windows 测试账户中修改安装项和快捷方式。[运行方式](WINDOWS_TEST.md#原版安装包验收)

Windows 安装目录根部的 `MochiPic.exe` 是纯 Go 启动器。它从 `current-runtime` 读取当前运行目录并在那里启动 MyGo 程序。应用、DLL 和配置文件共存于同一个版本目录，安装器先写好完整运行目录，再切换记录。

Windows SDK 的 libvips 8.18.7 从 `libvips-42.dll` 所在目录查找 `vips-modules-8.18`。保持 SDK 中 DLL 和插件目录的相对位置。启动器在失败弹窗中保留主程序最后 2 KiB 的标准错误。`--verify-runtime` 和 `--verify-window` 用退出码和标准错误供自动验收使用。[上游 Windows 打包补丁](https://github.com/libvips/build-win64-mxe/blob/v8.18.7/build/plugins/all-deps/patches/vips-8-dlldir-as-libdir.patch)

自 0.0.21 起，每个运行目录都有 `runtime-files.json`，记录随包文件的相对路径和 SHA-256。Windows 卸载通过启动器清理，持有启动及更新的排他锁。只清理同时满足允许的运行文件名和相同哈希的文件，Windows 在同一个文件句柄上完成核对和删除。不同内容、用户新增文件、目录联接和没有清单的旧目录都会保留。

`python3 scripts/package-windows-update-qa.py` 从同一份便携包制作 `-safe-update-qa.zip`。该脚本让 `mygo keygen` 在系统临时目录里生成一次性密钥，使用项目的 `cmd/sign-update` 签发测试归档，只把公钥放入验收包。它从正式启动器的同一份源代码编译回环更新版。Windows 侧使用 `Run-Update-Checks.cmd` 运行，试用安装包则内嵌 GitHub 更新源与项目公钥。

Windows 的更新按钮用 MyGo 检查版本。用户确认后，启动器在系统临时目录中运行独立的验签子进程。它校验已签名归档里的 `runtime-version`、预先启动新程序检查 libvips 及插件，然后用 `internal/winupdate` 安装到新目录。准备退出的工作台会先保存偏好，再由启动器重新读取 `current-runtime` 并启动新版。若仍在处理图片，等下次启动再启用新版。
