package app

import (
	"fmt"
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"imageflow/internal/imaging"
	"imageflow/internal/testkit"
)

func TestPreviewWarmupPreparesOnlyTheAdjacentPhoto(t *testing.T) {
	if err := imaging.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(imaging.Shutdown)
	t.Setenv("IMAGEFLOW_CONFIG_DIR", t.TempDir())
	events := make(chan func(), 32)
	m := New(func(fn func()) { events <- fn }, func() {})
	t.Cleanup(func() { _ = m.Close() })
	dir := t.TempDir()
	var inputs []string
	for i := range 4 {
		path := filepath.Join(dir, fmt.Sprintf("%d.png", i))
		testkit.PNG(t, path, testkit.Solid(380, 240, color.NRGBA{R: uint8(40 + i*50), A: 255}))
		inputs = append(inputs, path)
	}
	wait := func(cond func() bool) {
		t.Helper()
		timeout := time.NewTimer(4 * time.Second)
		defer timeout.Stop()
		for !cond() {
			select {
			case event := <-events:
				event()
			case <-timeout.C:
				t.Fatalf("preview did not arrive: %v", m.PreviewErr)
			}
		}
	}
	m.Import(inputs)
	wait(func() bool { return len(m.Sources) == 4 && m.OriginalReady() })
	paths := make([]string, 0, len(m.Sources))
	for _, s := range m.Sources {
		paths = append(paths, s.Path)
	}
	m.ShowThumbnails(paths)
	key := baseRequest{path: paths[1], part: 1}
	wait(func() bool { return m.preview.cache.index(key) >= 0 })
	if len(m.preview.cache.recent) != 2 {
		t.Fatal("warmup loaded more than the one neighbor")
	}
	warm, _ := m.preview.cache.get(key)
	m.Select(paths[1])
	if !m.OriginalReady() || m.Preview.Before != warm.base.Original || m.Rendering {
		t.Fatal("neighbor was not ready on the selection's own event")
	}
	m.NewBatch()
	if len(m.preview.cache.recent) != 0 || m.Preview.Before != nil {
		t.Fatal("new batch kept previous preview data")
	}
}

func TestPreviewWarmupCannotPushViewedPhotosOutOfThePixelBudget(t *testing.T) {
	f, err := os.Stat(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	bitmap := image.NewRGBA(image.Rect(0, 0, 1400, 1400))
	entry := func(i int) cachedBase {
		return cachedBase{
			key:   baseRequest{path: fmt.Sprintf("%d.png", i), part: 1},
			base:  imaging.PreviewBase{Original: bitmap, Canvas: bitmap},
			stamp: f,
		}
	}
	var c baseCache
	for i := range 8 {
		c.put(entry(i), false)
	}
	c.get(entry(0).key) // Keep the first photo recent when a ninth is opened.
	c.put(entry(8), true)
	if len(c.recent) != 8 || c.index(entry(8).key) >= 0 {
		t.Fatal("speculative work evicted a viewed photo")
	}
	c.put(entry(8), false)
	if c.bytes > previewCacheBytes || c.index(entry(0).key) < 0 || c.index(entry(1).key) >= 0 || c.index(entry(8).key) < 0 {
		t.Fatal("viewed photos are not retained by recency within the byte budget")
	}
}
