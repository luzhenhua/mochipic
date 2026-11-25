package ui

import (
	"errors"
	"slices"
	"testing"
	"time"

	"imageflow/internal/app"
	"imageflow/internal/l10n"

	native "github.com/egoist/mygo/ui"
)

func TestAboutLinksOpenInBrowser(t *testing.T) {
	d := &desktop{model: &app.Model{}}
	w := native.NewTester(func(c *native.Context) {
		native.Column(c).Children(func() { d.aboutLinks(c) })
	}, 360, 120)
	w.SetLocale(l10n.Default)

	for _, name := range []string{"官方网站", "GitHub"} {
		if _, ok := w.AccessByRole(native.RoleLink, name); !ok {
			t.Fatalf("missing accessible link %q", name)
		}
	}
	if err := w.Click("官方网站"); err != nil {
		t.Fatal(err)
	}
	w.Key(0, native.KeyTab)
	w.Key(0, native.KeyEnter)
	want := []string{"https://mochipic.zhenhua.lu/", "https://github.com/luzhenhua/mochipic"}
	if got := w.OpenedURLs(); !slices.Equal(got, want) {
		t.Fatalf("opened %v, want %v", got, want)
	}

	cause := errors.New("default browser unavailable")
	w.FailOpenURL(cause)
	if err := w.Click("GitHub"); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(d.model.Error, cause) {
		t.Fatalf("browser failure was not surfaced: %v", d.model.Error)
	}
}

func TestAboutCreditsAndAuthor(t *testing.T) {
	d := &desktop{model: &app.Model{}}
	w := native.NewTester(func(c *native.Context) {
		native.Column(c).Fill().Children(func() {
			d.aboutCredits(c)
			d.creditsModal(c.Key("credits"))
		})
	}, 550, 460)
	w.SetLocale(l10n.Default)
	if _, ok := w.AccessByRole(native.RoleLink, "卢振华"); !ok {
		t.Fatal("author name is not an accessible link")
	}
	if err := w.Click("卢振华"); err != nil {
		t.Fatal(err)
	}
	if got := w.OpenedURLs(); !slices.Equal(got, []string{"https://zhenhua.lu"}) {
		t.Fatalf("opened %v", got)
	}

	if err := w.Click("开源鸣谢"); err != nil {
		t.Fatal(err)
	}
	w.Advance(400 * time.Millisecond)
	if !d.creditsOpen || !w.HasText("Lobe MyGo") || !w.HasText("跨平台桌面框架") {
		t.Fatal("credits did not open")
	}
	w.Scroll(290, 230, 0, 1600)
	w.Advance(400 * time.Millisecond)
	if last, ok := w.Access("Expat · libxml2 · libarchive"); !ok || last.Offscreen || last.Bounds.Y >= 460 {
		t.Fatalf("last credits row is not reachable by scrolling: %+v", last)
	}
	w.Key(0, native.KeyEscape)
	w.Advance(400 * time.Millisecond)
	if d.creditsOpen || w.HasText("Lobe MyGo") {
		t.Fatal("credits did not close")
	}
	w.SetLocale("en")
	if !w.HasText("MochiPic was designed and developed by") || !w.HasText("Open-source credits") {
		t.Fatal("about credits were not localized")
	}
}
