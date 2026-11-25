<p align="center">
  <a href="../assets/workspace-crop.webp">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="../assets/mochipic-cover-dark.webp">
      <img src="../assets/mochipic-cover.webp" width="840" alt="MochiPic · 下部に一括処理する写真のサムネイルを表示し、先頭の写真だけを 4:3 にトリミング">
    </picture>
  </a>
</p>

<h1 align="center">写真ごとに調整して、まとめて書き出す。</h1>

<p align="center">macOS と Windows 向けのネイティブ画像一括編集アプリ。画像の処理は端末内で完結します。</p>

<p align="center">
  <a href="https://github.com/luzhenhua/mochipic/releases/download/v0.0.26/MochiPic-0.0.26-arm64-ja.dmg"><img src="../assets/download-macos.webp" width="144" height="42" alt="macOS 版をダウンロード"></a>
  &nbsp;
  <a href="https://github.com/luzhenhua/mochipic/releases/download/v0.0.26/MochiPic-0.0.26-windows-x64-setup.exe"><img src="../assets/download-windows.webp" width="144" height="42" alt="Windows 版をダウンロード"></a>
  <br>
  <sub>macOS 13 以降 · Apple Silicon &nbsp; / &nbsp; Windows 10/11 · x64</sub>
</p>

<p align="center">
  <a href="../../README.md">English</a> · <a href="zh-Hans.md">简体中文</a> · <strong>日本語</strong> · <a href="es.md">Español</a> · <a href="fr.md">Français</a> · <a href="pt-BR.md">Português (Brasil)</a>
  <br>
  <sub><a href="https://mochipic.zhenhua.lu/">公式サイト</a> · <a href="https://github.com/luzhenhua/mochipic/releases/latest">すべてのダウンロード</a> · <a href="../USAGE.md">ユーザーガイド（简体中文）</a></sub>
</p>

## 主な機能

### 選んだ写真だけを編集

1 枚だけトリミングやリサイズをしたり、複数枚でコラージュを作ったり、まとめて透かしを入れたりできます。設定は写真ごとに保存されます。

[![選択した写真に透かしを追加](../assets/mochipic-watermark.webp)](../assets/workspace-watermark.webp)

### まとめて書き出す

形式変換やファイル名の変更を一括で行い、そのまま書き出せます。元の写真はそのまま残ります。

[![一括で WebP に変換](../assets/mochipic-webp.webp)](../assets/workspace-export.webp)

## 開発とコントリビュート

Go、MyGo、Lobe MyGo、libvips を使用しています。Apple Silicon Mac では次のコマンドで開発版を起動できます。

```sh
scripts/setup-native.sh
scripts/go.sh run ./cmd/imageflow
```

[macOS・Windows ビルドガイド（简体中文）](../DEVELOPMENT.md) · [アーキテクチャ（简体中文）](../ARCHITECTURE.md)

[Issue を作成](https://github.com/luzhenhua/mochipic/issues) · [コントリビューションガイド](../../CONTRIBUTING.md)

## ライセンス

MochiPic は個人の非商用利用に限り無料です。企業や団体での利用（社内業務を含む）および個人での商用利用には、書面による許可が必要です。

[個人利用ライセンス](../../LICENSE.md) · [商用ライセンスのお問い合わせ](https://zhenhua.lu/)

## クレジット

設計・開発は [Zhenhua Lu](https://zhenhua.lu)。 Lobe UI、Lobe MyGo、MyGo をはじめ、MochiPic を支えるオープンソースプロジェクトに感謝します。

[サードパーティのライセンス](../THIRD_PARTY.md)
