package imaging

import (
	"slices"

	"imageflow/internal/l10n"
	"imageflow/internal/series"

	"github.com/davidbyttow/govips/v2/vips"
)

type frameMeta struct {
	series series.Info
	delay  int // milliseconds
	loop   int
}

func seriesLimit() error {
	return l10n.Error("error.seriesLimit", l10n.Args{"count": series.MaxParts})
}

func vipsSeries(im *vips.ImageRef, format Format, part int) (frameMeta, error) {
	var meta frameMeta
	count := im.Pages()
	if count <= 1 {
		return meta, nil
	}
	if count > series.MaxParts {
		return meta, seriesLimit()
	}
	if part > count {
		return meta, l10n.New("error.seriesChanged")
	}
	meta.series = series.Info{Kind: series.Pages, Count: count}
	if format == GIF || format == WebP || format == JXL {
		delay, _ := im.PageDelay()
		hasTiming := len(delay) == count && !slices.ContainsFunc(delay, func(ms int) bool { return ms < 0 })
		if format != JXL || hasTiming {
			meta.series.Kind = series.Frames
			meta.loop, meta.delay = im.Loop(), 100
			if hasTiming {
				meta.delay = delay[part-1]
			}
		}
	}
	return meta, nil
}
