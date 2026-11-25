package ui

import (
	"strings"

	"imageflow/internal/l10n"

	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/feedback"
	"github.com/lobehub/lobe-mygo/pkg/lucide"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func (d *desktop) aboutLinks(c *ui.Context) {
	line(c)
	ui.Row(c).Wrap().Padding(8, 16).Gap(20).AlignItems(ui.Center).Children(func() {
		d.aboutLink(c.Key("website"), tr(c, "settings.officialWebsite"), "https://mochipic.zhenhua.lu/", lucide.Globe())
		d.aboutLink(c.Key("github"), "GitHub", "https://github.com/luzhenhua/mochipic", githubMark)
	})
}

func (d *desktop) aboutLink(c *ui.Context, label, target string, mark *ui.SVG) {
	link := ui.ButtonBase(c).Role(ui.RoleLink).Label(label).Tooltip(target).Shrink(0).AlignItems(ui.Center)
	action.StyleButton(c, link, "", action.ButtonProps{Variant: action.Link})
	tk := theme.TokensOf(c)
	fg := choose(link.Hovered() || link.Pressed(), tk.Text, tk.TextSecondary)
	link.Children(func() {
		icon(c, mark, 16, fg)
		ui.Text(c, label).TextColor(fg).SingleLine()
		icon(c, lucide.ArrowUpRight(), 12, fg)
	})
	if link.Clicked() {
		c.OpenURLThen(target, func(err error) {
			if err != nil {
				d.model.Error = l10n.Wrap("settings.openLinkFailed", err)
			}
		})
	}
}

func (d *desktop) aboutCredits(c *ui.Context) {
	const author, authorURL = "卢振华", "https://zhenhua.lu"
	tk := theme.TokensOf(c)
	byline := trf(c, "credits.byline", l10n.Args{"author": author})
	before, after, _ := strings.Cut(byline, author)
	line(c)
	ui.Row(c).Wrap().Padding(8, 16).Gap(12).AlignItems(ui.Center).Children(func() {
		ui.RichText(c).Wrap().Grow(1).MinWidth(195).TextColor(tk.TextSecondary).Children(func() {
			ui.Text(c, before)
			ui.Link(c, author, authorURL).Underline().TextColor(tk.Text).Tooltip(authorURL)
			ui.Text(c, " ↗"+after)
		})
		label := tr(c, "credits.title")
		button := ui.ButtonBase(c).Label(label).Shrink(0).AlignItems(ui.Center)
		action.StyleButton(c, button, "", action.ButtonProps{Variant: action.Link})
		fg := choose(button.Hovered() || button.Pressed(), tk.Text, tk.TextSecondary)
		button.Children(func() {
			ui.Text(c, label).TextColor(fg).SingleLine()
			icon(c, lucide.ChevronRight(), 14, fg)
		})
		if button.Clicked() {
			d.creditsOpen = true
		}
	})
}

type credit struct {
	projects string
	role     string
}

// Group related projects by their role across the two app packages. The full
// dependency versions and upstream licenses are recorded in the build manifests.
var credits = []struct {
	title string
	rows  []credit
}{
	{"credits.interface", []credit{
		{"Lobe UI", "credits.designSystem"},
		{"Lobe MyGo", "credits.nativeComponents"},
		{"MyGo", "credits.desktopFramework"},
		{"Lucide · GitHub Octicons", "credits.icons"},
	}},
	{"credits.images", []credit{
		{"libvips · govips", "credits.engine"},
		{"libwebp · libheif · libjxl", "credits.codecs"},
		{"libjpeg-turbo · MozJPEG · libpng · libtiff · OpenJPEG", "credits.codecs"},
		{"libnsgif · cgif · AOM · libde265 · libultrahdr", "credits.codecs"},
		{"librsvg · ImageMagick · LibRaw · OpenEXR · CFITSIO · NIfTI · matio", "credits.readers"},
		{"Little CMS · libexif · libimagequant · Lerc · GDK-PixBuf", "credits.imageTools"},
		{"libdeflate · zlib · zlib-ng · Zstandard · XZ Utils · Brotli", "credits.compression"},
	}},
	{"credits.fontsPackaging", []credit{
		{"FreeType · Fontconfig · HarfBuzz · Pango · FriBidi · Graphite2", "credits.fonts"},
		{"Cairo · Pixman · go-text/typesetting", "credits.rendering"},
		{"NSIS · dmgbuild · ds-store · mac-alias", "credits.installers"},
		{"conda-forge · LLVM/MinGW", "credits.buildTools"},
	}},
	{"credits.libraries", []credit{
		{"Go · purego · x/image · x/net · x/sys · x/text", "credits.goLibraries"},
		{"GLib · libffi · GNU libiconv · GNU gettext · PCRE2", "credits.systemLibraries"},
		{"libc++ · libunwind · Highway · FFTW", "credits.systemLibraries"},
		{"Expat · libxml2 · libarchive", "credits.systemLibraries"},
	}},
}

func (d *desktop) creditsModal(c *ui.Context) {
	feedback.Modal(c, &d.creditsOpen, feedback.ModalProps{
		Title: tr(c, "credits.title"), Width: 510, NoFooter: true,
	}, func() {
		tk := theme.TokensOf(c)
		line(c)
		ui.Column(c).PaddingY(12).Gap(18).Children(func() {
			for _, group := range credits {
				ui.Column(c.Key(group.title)).Gap(6).Children(func() {
					strong(c, tr(c, group.title)).FontSize(14)
					ui.Column(c).Children(func() {
						for i, entry := range group.rows {
							if i != 0 {
								line(c)
							}
							ui.Row(c.Key(entry.projects)).MinHeight(32).PaddingY(6).Gap(14).AlignItems(ui.Center).Children(func() {
								ui.Text(c, entry.projects).Wrap().Grow(3).Basis(0).MinWidth(0).TextColor(tk.Text)
								ui.Text(c, tr(c, entry.role)).Wrap().Grow(2).Basis(0).MinWidth(0).TextColor(tk.TextSecondary)
							})
						}
					})
				})
			}
		})
	})
}
