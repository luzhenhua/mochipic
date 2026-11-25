<p align="center">
  <a href="../assets/workspace-crop.webp">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="../assets/mochipic-cover-dark.webp">
      <img src="../assets/mochipic-cover.webp" width="840" alt="麻薯图图 · MochiPic · 在工作台中单独把第一张照片裁成 4:3，底部保留整批缩略图">
    </picture>
  </a>
</p>

<h1 align="center">单张细调，整批导出。</h1>

<p align="center">macOS 和 Windows 上的原生图片批处理工具。图片始终留在本机。</p>

<p align="center">
  <a href="https://github.com/luzhenhua/mochipic/releases/download/v0.0.26/MochiPic-0.0.26-arm64-zh-Hans.dmg"><img src="../assets/download-macos.webp" width="144" height="42" alt="下载 macOS 版"></a>
  &nbsp;
  <a href="https://github.com/luzhenhua/mochipic/releases/download/v0.0.26/MochiPic-0.0.26-windows-x64-setup.exe"><img src="../assets/download-windows.webp" width="144" height="42" alt="下载 Windows 版"></a>
  <br>
  <sub>macOS 13+ · Apple Silicon &nbsp; / &nbsp; Windows 10/11 · x64</sub>
</p>

<p align="center">
  <a href="../../README.md">English</a> · <strong>简体中文</strong> · <a href="ja.md">日本語</a> · <a href="es.md">Español</a> · <a href="fr.md">Français</a> · <a href="pt-BR.md">Português (Brasil)</a>
  <br>
  <sub><a href="https://mochipic.zhenhua.lu/">官方网站</a> · <a href="https://github.com/luzhenhua/mochipic/releases/latest">全部下载</a> · <a href="../USAGE.md">使用指南</a></sub>
</p>

## 主要功能

### 选中哪几张，就编辑哪几张

单张可以单独裁剪和调整尺寸，选中多张可以一起加水印或组成拼图。每张照片都能保留自己的设置。

[![给选中的照片加水印](../assets/mochipic-watermark.webp)](../assets/workspace-watermark.webp)

### 把整批照片一次导出

统一转换格式、批量重命名，再一次导出。原图保持原样。

[![整批转为 WebP](../assets/mochipic-webp.webp)](../assets/workspace-export.webp)

## 开发与贡献

使用 Go、MyGo、Lobe MyGo 和 libvips。在 Apple Silicon Mac 上启动开发版可使用以下命令。

```sh
scripts/setup-native.sh
scripts/go.sh run ./cmd/imageflow
```

[macOS 与 Windows 构建指南](../DEVELOPMENT.md) · [架构说明](../ARCHITECTURE.md)

[提交问题](https://github.com/luzhenhua/mochipic/issues) · [贡献指南](../../CONTRIBUTING.md)

## 使用许可

麻薯图图供个人非商业用途免费使用。公司或其他组织使用，以及个人用于经营活动，须取得书面授权。公司内部使用也需要授权。

[个人使用许可](../../LICENSE.md) · [联系获取商业授权](https://zhenhua.lu/)

## 鸣谢

麻薯图图由[卢振华](https://zhenhua.lu)开发设计。 感谢 Lobe UI、Lobe MyGo、MyGo，以及其他为麻薯图图提供支持的开源项目。

[第三方许可](../THIRD_PARTY.md)
