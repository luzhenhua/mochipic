package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"testing"

	"imageflow/internal/app"
	"imageflow/internal/testkit"
)

func TestNativePhotoSwitchPaintsTheRightPhotoBeforeDecodeAndReusesWarmPreview(t *testing.T) {
	d, w, wait := testWorkspace(t)
	m := d.model
	dir := t.TempDir()
	colors := []color.NRGBA{{R: 218, A: 255}, {G: 184, A: 255}, {R: 185, G: 120, A: 255}, {R: 190, B: 125, A: 255}, {B: 205, A: 255}}
	var paths []string
	for i, ink := range colors {
		path := filepath.Join(dir, fmt.Sprintf("%02d.png", i+1))
		testkit.PNG(t, path, testkit.Solid(440, 286, ink))
		paths = append(paths, path)
	}
	firstHash := testkit.Hash(t, paths[0])
	d.importPaths(paths)
	wait("first photo and filmstrip ready", func() bool {
		return !m.Scanning && len(m.Sources) == len(paths) && m.OriginalReady() && m.Thumbnail(m.Sources[4].Path).Image != nil
	})
	first, last := m.Sources[0].Path, m.Sources[4].Path
	firstPreview := m.Preview.Before
	assertColor := func(ink color.NRGBA) {
		t.Helper()
		got := w.Image().RGBAAt(482, 367)
		if got.R != ink.R || got.G != ink.G || got.B != ink.B {
			t.Fatalf("canvas painted %v, want %v", got, ink)
		}
	}
	assertColor(colors[0])

	// Jump past the photos eligible for warmup. Worker completions are
	// queued by testWorkspace until wait is called, as on a UI event turn.
	clickWorkspace(t, w, filepath.Base(last))
	f := m.BeforeFrame()
	if m.Selected != last || !f.Thumbnail || f.Image != m.Thumbnail(last).Image || m.OriginalReady() {
		t.Fatal("cold click did not synchronously show its own thumbnail")
	}
	assertColor(colors[4])
	fit := d.fit
	wait("clear preview of last photo", m.OriginalReady)
	lastPreview := m.Preview.Before
	if lastPreview == f.Image || d.fit != fit {
		t.Fatal("thumbnail did not resolve at the same apparent size")
	}
	for range 8 {
		clickWorkspace(t, w, filepath.Base(first))
		if m.Rendering || m.Preview.Before != firstPreview || m.Mode != app.Before {
			t.Fatal("return to first photo queued another decode")
		}
		assertColor(colors[0])
		clickWorkspace(t, w, filepath.Base(last))
		if m.Rendering || m.Preview.Before != lastPreview {
			t.Fatal("return to last photo queued another decode")
		}
		assertColor(colors[4])
	}

	// Simulate another app replacing one of our own test inputs. A cached
	// preview still responds immediately, then updates from the new file.
	clickWorkspace(t, w, filepath.Base(first))
	replacement := filepath.Join(dir, "replacement.png")
	newColor := color.NRGBA{R: 35, G: 190, B: 65, A: 255}
	testkit.PNG(t, replacement, testkit.Solid(448, 290, newColor))
	if err := os.Rename(replacement, last); err != nil {
		t.Fatal(err)
	}
	clickWorkspace(t, w, filepath.Base(last))
	wait("changed photo updated", func() bool {
		return m.OriginalReady() && m.Preview.Input.Width == 448 && m.Preview.Before != lastPreview
	})
	assertColor(newColor)
	if testkit.Hash(t, first) != firstHash {
		t.Fatal("preview modified the original")
	}
}
