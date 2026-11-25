# 相机 RAW 导入

ImageFlow 通过 macOS Core Image 在本机读取 RAW 数据并生成 sRGB 成片，保留拍摄方向。它使用系统已有的 RAW 解码器，不下载解码模型，也不会回退到内嵌的 JPEG 缩略图。支持情况还取决于 macOS 版本、相机型号和拍摄模式。

下列来自 [raw.pixls.us 样片库](https://raw.pixls.us/) 的 CC0 文件，在 Apple Silicon、macOS 27.3 的 0.5.0 上通过了扫描、原图预览和缩小导出。它们不是这些厂商所有型号及模式的保证。

| 后缀 | 实测相机 |
| --- | --- |
| CR2 | Canon EOS 5D Mark III |
| CR3 | Canon EOS R6 |
| NEF | Nikon D750 |
| ARW | Sony α7 III |
| RAF | Fujifilm X-T3 |
| RW2 | Panasonic DC-G9 |
| ORF | Olympus E-M1 Mark II |
| PEF | Pentax K-5 |
| DNG | Apple iPhone 12 Pro，竖拍 |

RAW 沿用输入限制：160 MiB、5,000 万像素、单边 12,000 px；输出最多 2,400 万像素。需缩小时在「尺寸」设置长边或目标画布。输出为常用位图，可继续加水印和转换格式。

维护者可运行 `scripts/raw-corpus.sh` 从公开样片库下载并校验这九个文件，或传入已下载的目录离线复测。原始照片不进入项目源码或安装包。样片 ID 和 SHA-256 见 [raw-fixtures.json](../scripts/raw-fixtures.json)。Apple API 依据为 [Core Image RAW 数据解码](https://developer.apple.com/documentation/coreimage/cirawfilter/init(imagedata:identifierhint:))。
