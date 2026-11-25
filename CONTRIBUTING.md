# Contributing to MochiPic / 参与麻薯图图

## Add a language

1. Copy [`internal/l10n/locales/zh-Hans.json`](internal/l10n/locales/zh-Hans.json) to a new file named with a canonical BCP 47 tag, such as `de.json`. Set `@@locale` to the tag and `meta.languageName` to the language's own name, such as `Deutsch`.
2. Translate the values and keep the message IDs and argument names. The settings picker and macOS bundle discover languages from the catalog filenames. Keep the `lobe.*` and `mygo.*` entries: these are labels for controls and menus supplied by the frameworks.
3. Run `go test ./internal/l10n` to check catalog keys, ICU syntax, arguments and references from Go. Run `scripts/go.sh test ./...` with the [local dependencies](docs/DEVELOPMENT.md) installed.
4. Open the app and pick the language under Settings → Appearance → Language. Check the main pages, narrow windows (980 × 630), light and dark appearances, native menus, and a real failure message. You can start a dev build with `MYGO_LOCALE=de scripts/go.sh run ./cmd/imageflow` to preview a language for one run. Use a temporary `IMAGEFLOW_CONFIG_DIR` when testing preferences.

## Add or change a message

- Write all app copy in the locale files. Use a stable ID such as `watermark.textEmptyHint`. Update existing values for copy changes; keep the ID if the meaning is unchanged.
- Use [ICU MessageFormat](https://unicode-org.github.io/icu/userguide/format_parse/messages/) for dynamic text. Keep named arguments (such as `{count}` or `{name}`) in every locale. Translators can reorder arguments and choose plural forms. Quote literal braces with apostrophes in the catalog. User paths, preset names, watermark contents and rename variables such as `{original}` are data; pass them as arguments and leave them intact.
- In a native view, use `tr` for a label or `trf` for a message with arguments. When returning an app error, use `l10n.New`, `l10n.Error` or `l10n.Wrap`. The view translates errors as it displays them; wrapping preserves the cause. System and codec diagnostics may still appear in the language provided by those components.
- Simplified Chinese is the source and runtime fallback. Add the new message to all shipping catalogs before submitting a change.

## 中文速查

添加语言时，复制简体中文目录文件并使用规范的 BCP 47 文件名，修改语言代码、自称与各条译文。设置和应用包会读取新语言；拖拽安装盘还需在 `scripts/installer-copy.json` 增加对应语言的引导标题、文件名和帮助。修改文案时保留消息 ID、ICU 参数名和命名变量，使用 `go test ./internal/l10n` 检查，再到原生窗口测试显示和交互。
