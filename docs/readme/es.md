<p align="center">
  <a href="../assets/workspace-crop.webp">
    <picture>
      <source media="(prefers-color-scheme: dark)" srcset="../assets/mochipic-cover-dark.webp">
      <img src="../assets/mochipic-cover.webp" width="840" alt="MochiPic · Recorte de la primera foto a 4:3 con las miniaturas de todo el lote visibles">
    </picture>
  </a>
</p>

<h1 align="center">Ajusta cada foto. Exporta todo el lote.</h1>

<p align="center">Editor nativo de imágenes por lotes para macOS y Windows. Tus imágenes se quedan en tu equipo.</p>

<p align="center">
  <a href="https://github.com/luzhenhua/mochipic/releases/download/v0.0.26/MochiPic-0.0.26-arm64-es.dmg"><img src="../assets/download-macos.webp" width="144" height="42" alt="Descargar para macOS"></a>
  &nbsp;
  <a href="https://github.com/luzhenhua/mochipic/releases/download/v0.0.26/MochiPic-0.0.26-windows-x64-setup.exe"><img src="../assets/download-windows.webp" width="144" height="42" alt="Descargar para Windows"></a>
  <br>
  <sub>macOS 13 o posterior · Apple Silicon &nbsp; / &nbsp; Windows 10/11 · x64</sub>
</p>

<p align="center">
  <a href="../../README.md">English</a> · <a href="zh-Hans.md">简体中文</a> · <a href="ja.md">日本語</a> · <strong>Español</strong> · <a href="fr.md">Français</a> · <a href="pt-BR.md">Português (Brasil)</a>
  <br>
  <sub><a href="https://mochipic.zhenhua.lu/">Sitio oficial</a> · <a href="https://github.com/luzhenhua/mochipic/releases/latest">Todas las descargas</a> · <a href="../USAGE.md">Guía de uso (简体中文)</a></sub>
</p>

## Funciones principales

### Edita las fotos que selecciones

Recorta o cambia el tamaño de una foto, crea un collage o añade una marca de agua a varias fotos a la vez. Cada foto conserva sus propios ajustes.

[![Marca de agua en las fotos seleccionadas](../assets/mochipic-watermark.webp)](../assets/workspace-watermark.webp)

### Exporta todo el lote

Convierte formatos, cambia los nombres de los archivos y exporta en un solo paso. Tus originales quedan intactos.

[![Conversión del lote a WebP](../assets/mochipic-webp.webp)](../assets/workspace-export.webp)

## Desarrollo y contribuciones

MochiPic usa Go, MyGo, Lobe MyGo y libvips. Ejecuta estos comandos para iniciar la versión de desarrollo en un Mac con Apple Silicon.

```sh
scripts/setup-native.sh
scripts/go.sh run ./cmd/imageflow
```

[Guía de compilación para macOS y Windows (简体中文)](../DEVELOPMENT.md) · [Arquitectura (简体中文)](../ARCHITECTURE.md)

[Notificar un problema](https://github.com/luzhenhua/mochipic/issues) · [Guía de contribución](../../CONTRIBUTING.md)

## Créditos

Diseño y desarrollo de [Zhenhua Lu](https://zhenhua.lu). Gracias a Lobe UI, Lobe MyGo, MyGo y los proyectos de código abierto que hacen posible MochiPic.

[Avisos de terceros](../THIRD_PARTY.md)
