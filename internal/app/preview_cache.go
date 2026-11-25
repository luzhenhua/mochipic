package app

import (
	"context"
	"image"
	"os"
	"slices"
	"time"

	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
)

const (
	previewCacheEntries = 12
	previewCacheBytes   = 64 << 20
)

type cachedBase struct {
	key   baseRequest
	base  imaging.PreviewBase
	stamp os.FileInfo
	bytes int
}

// The cache is owned by the UI thread. Only downsampled Go images are stored;
// each render worker releases its native decode graph before returning.
type baseCache struct {
	recent []cachedBase // Oldest first.
	bytes  int
}

func (c *baseCache) index(key baseRequest) int {
	return slices.IndexFunc(c.recent, func(b cachedBase) bool { return b.key == key })
}

func (c *baseCache) get(key baseRequest) (cachedBase, bool) {
	i := c.index(key)
	if i < 0 {
		return cachedBase{}, false
	}
	entry := c.recent[i]
	copy(c.recent[i:], c.recent[i+1:])
	c.recent[len(c.recent)-1] = entry
	return entry, true
}

func previewBytes(im image.Image) int {
	switch b := im.(type) {
	case nil:
		return 0
	case *image.RGBA:
		return len(b.Pix)
	case *image.NRGBA:
		return len(b.Pix)
	case *image.Gray:
		return len(b.Pix)
	default:
		return 8 * im.Bounds().Dx() * im.Bounds().Dy()
	}
}

func (c *baseCache) put(b cachedBase, speculative bool) {
	b.bytes = previewBytes(b.base.Original)
	if b.base.Original != b.base.Canvas {
		b.bytes += previewBytes(b.base.Canvas)
	}
	if b.stamp == nil || b.base.Original == nil || b.base.Canvas == nil || b.bytes > previewCacheBytes {
		return
	}
	if i := c.index(b.key); i >= 0 {
		if speculative {
			return
		}
		c.bytes -= c.recent[i].bytes
		c.recent = slices.Delete(c.recent, i, i+1)
	}
	if speculative && (len(c.recent) >= previewCacheEntries || c.bytes+b.bytes > previewCacheBytes) {
		return // Preparing the next photo must not evict a photo already viewed.
	}
	for len(c.recent) >= previewCacheEntries || c.bytes+b.bytes > previewCacheBytes {
		c.bytes -= c.recent[0].bytes
		c.recent = slices.Delete(c.recent, 0, 1)
	}
	c.bytes += b.bytes
	if speculative {
		c.recent = slices.Insert(c.recent, 0, b)
	} else {
		c.recent = append(c.recent, b)
	}
}

func (c *baseCache) removePaths(paths map[string]bool) {
	c.recent = slices.DeleteFunc(c.recent, func(b cachedBase) bool {
		if !paths[b.key.path] {
			return false
		}
		c.bytes -= b.bytes
		return true
	})
}

type baseLoad struct {
	key   baseRequest
	stamp os.FileInfo // When a cached preview is already on screen.
}

type baseResult struct {
	cachedBase
	checked, unchanged bool
}

func previewStamp(path string) (os.FileInfo, error) {
	st, err := os.Lstat(path)
	if err == nil && !st.Mode().IsRegular() {
		err = l10n.New("error.inputChanged")
	}
	return st, err
}

func samePreviewFile(a, b os.FileInfo) bool {
	return a != nil && b != nil && a.Size() == b.Size() && a.ModTime().Equal(b.ModTime()) && os.SameFile(a, b)
}

func loadBase(ctx context.Context, q baseLoad) (baseResult, error) {
	out := baseResult{cachedBase: cachedBase{key: q.key}, checked: q.stamp != nil}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	before, err := previewStamp(q.key.path)
	if err != nil {
		return out, &imaging.StageError{Stage: imaging.Decode, Err: err}
	}
	if samePreviewFile(before, q.stamp) {
		out.unchanged = true
		return out, nil
	}
	k := q.key
	out.base, err = imaging.RenderCroppedPreviewBase(ctx, k.path, k.resize, k.crop, k.part)
	if err == nil {
		after, statErr := previewStamp(k.path)
		if statErr != nil || !samePreviewFile(before, after) {
			out.base = imaging.PreviewBase{}
			return out, &imaging.StageError{Stage: imaging.Decode, Err: l10n.New("error.inputChanged")}
		}
		out.stamp = after
	}
	return out, err
}

type warmRequest [2]baseRequest

func (m *Model) initPreviewWarmup() {
	p := &m.preview
	p.warmWork = previewWorker(m, 100*time.Millisecond, func(ctx context.Context, q warmRequest) ([2]cachedBase, error) {
		var out [2]cachedBase
		for i, key := range q {
			if ctx.Err() != nil {
				return out, ctx.Err()
			}
			if key.path != "" {
				r, err := loadBase(ctx, baseLoad{key: key})
				if err == nil {
					out[i] = r.cachedBase
				}
			}
		}
		return out, nil
	}, func(entries [2]cachedBase, err error) {
		if err == nil {
			for _, b := range entries {
				p.cache.put(b, true)
			}
		}
	})
}

func (m *Model) scheduleWarmup() {
	p := &m.preview
	var want warmRequest
	if m.Mode == Before && !m.Working && !m.Scanning && m.OriginalReady() && !p.state.basePending &&
		slices.Contains(m.visible, m.Selected) &&
		len(p.cache.recent) < previewCacheEntries && p.cache.bytes < previewCacheBytes {
		current := m.sourceIndex()
		for i, index := range [2]int{current + 1, current - 1} {
			if current < 0 || index < 0 || index >= len(m.Sources) {
				continue
			}
			src := m.Sources[index]
			t := m.thumbs[src.Path]
			// Stay off the single slot for large or system-decoded inputs.
			// Heavy photos still get a cached preview once explicitly opened.
			if t.Image == nil || src.Series.Multiple() || src.Size > 12<<20 ||
				t.Info.Width <= 0 || t.Info.Height <= 0 || t.Info.Width*t.Info.Height > 12_000_000 {
				continue
			}
			switch t.Info.Format {
			case imaging.JPEG, imaging.PNG, imaging.WebP:
				key := baseRequest{path: src.Path, part: 1}
				if p.cache.index(key) < 0 {
					want[i] = key
				}
			}
		}
	}
	if want == p.warmRequest {
		return
	}
	p.warmRequest = want
	if want == (warmRequest{}) {
		p.warmWork.Cancel()
	} else {
		p.warmWork.Submit(want)
	}
}
