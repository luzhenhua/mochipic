# 第三方组件与内部试用

此应用动态链接 libvips（LGPL-2.1-or-later）和用于图像编解码、文字绘制及色彩转换的库。`Contents/Resources/ThirdParty.json` 记录实际打包的 dylib、版本、许可与包来源；`Licenses/` 保存上游许可文本。Go、MyGo、Lobe MyGo 和 govips 的组件许可随包保留。

「设置 → 关于 → 开源鸣谢」向原生组件、设计系统、图像库、字体和构建工具致谢。Windows 随包的 `ThirdParty/runtime-dlls.txt` 记录实际打包的 DLL。`ThirdParty/versions.json` 记录上游 SDK 的组件版本，`ThirdParty/codec-build.txt` 记录替换过的编解码库版本。应用内的 GitHub 图标来自 [GitHub Octicons](https://github.com/primer/octicons)，许可文本与嵌入的图标一同保留。

- [libvips 8.18.7 源码](https://github.com/libvips/libvips/releases/tag/v8.18.7)；本项目用 `scripts/setup-native.sh` 重现构建选项，源码未打补丁。
- 静态 GIF 读取使用 libvips 源包内附的 libnsgif，随包保留其许可文本。HEIC 和 AVIF 由锁定的 libheif LGPL 构建及其编解码库提供；没有加入 HEVC 编码器。BMP 和 ICO 使用 macOS 自带框架。
- JPEG XL、JP2 和 SVG 分别通过锁定的 libjxl、OpenJPEG 和 librsvg 导入，包内保留实际动态库及许可清单。PSD 和相机 RAW 使用 macOS 自带框架；公开测试照片不随试用包分发。
- 其余 dylib 来自 [conda-forge](https://conda-forge.org/)。锁文件记录源包 URL 与 SHA-256。发布时应审查各依赖的对应源码和许可义务，尤其是 LGPL/MPL 双许可或单独许可的动态库。应保留替换或重新链接这些库的途径。

本地构建使用临时签名验证装载；未使用 Developer ID 或 Apple 公证。当前产物用于内部验收。对外分发前需验证对应源码的提供方式及签名、干净 Mac 启动。
