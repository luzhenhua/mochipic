package app

import (
	"math"

	"imageflow/internal/caption"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
)

// Each control patches only the fields it actually changed on the reference photo.
func applyField[T comparable](target *T, before, after T) {
	if before != after {
		*target = after
	}
}

func patchPlacement(dst, before, after imaging.Placement) imaging.Placement {
	applyField(&dst.MarginX, before.MarginX, after.MarginX)
	applyField(&dst.MarginY, before.MarginY, after.MarginY)
	applyField(&dst.Opacity, before.Opacity, after.Opacity)
	if before.Anchor != after.Anchor || before.Free != after.Free {
		dst.Anchor, dst.Free = after.Anchor, after.Free
	}
	return dst
}

func patchResize(dst, before, after imaging.Resize) imaging.Resize {
	applyField(&dst.Enabled, before.Enabled, after.Enabled)
	applyField(&dst.Mode, before.Mode, after.Mode)
	applyField(&dst.Fit, before.Fit, after.Fit)
	oldW, oldH := dst.Width, dst.Height
	applyField(&dst.Width, before.Width, after.Width)
	applyField(&dst.Height, before.Height, after.Height)
	if dst.LockRatio && dst.Mode == imaging.Exact && oldW > 0 && oldH > 0 {
		if before.Width != after.Width && before.Height == after.Height {
			dst.Height = max(1, min(imaging.MaxSide, int(math.Round(float64(dst.Width)*float64(oldH)/float64(oldW)))))
		} else if before.Height != after.Height && before.Width == after.Width {
			dst.Width = max(1, min(imaging.MaxSide, int(math.Round(float64(dst.Height)*float64(oldW)/float64(oldH)))))
		}
	}
	applyField(&dst.Percent, before.Percent, after.Percent)
	applyField(&dst.Limit, before.Limit, after.Limit)
	applyField(&dst.LockRatio, before.LockRatio, after.LockRatio)
	applyField(&dst.PadColor, before.PadColor, after.PadColor)
	applyField(&dst.Transparent, before.Transparent, after.Transparent)

	return dst
}

func patchText(dst, before, after imaging.TextMark) imaging.TextMark {
	applyField(&dst.Text, before.Text, after.Text)
	applyField(&dst.Font, before.Font, after.Font)
	applyField(&dst.Color, before.Color, after.Color)
	applyField(&dst.Size, before.Size, after.Size)
	applyField(&dst.Scale, before.Scale, after.Scale)
	applyField(&dst.Stroke, before.Stroke, after.Stroke)
	applyField(&dst.Shadow, before.Shadow, after.Shadow)
	dst.Placement = patchPlacement(dst.Placement, before.Placement, after.Placement)
	return dst
}

func patchLogo(dst, before, after imaging.LogoMark) imaging.LogoMark {
	applyField(&dst.Width, before.Width, after.Width)
	applyField(&dst.Height, before.Height, after.Height)
	applyField(&dst.KeepRatio, before.KeepRatio, after.KeepRatio)
	dst.Placement = patchPlacement(dst.Placement, before.Placement, after.Placement)
	return dst
}

func patchImage(dst, before, after imaging.Options) imaging.Options {
	applyField(&dst.Format, before.Format, after.Format)
	applyField(&dst.Quality, before.Quality, after.Quality)
	applyField(&dst.Lossless, before.Lossless, after.Lossless)
	applyField(&dst.FlattenAlpha, before.FlattenAlpha, after.FlattenAlpha)
	applyField(&dst.Background, before.Background, after.Background)
	dst.Text = patchText(dst.Text, before.Text, after.Text)
	dst.Resize = patchResize(dst.Resize, before.Resize, after.Resize)
	patchLogos(&dst, before, after)
	dst = withContentWatermarks(dst)
	return dst
}

func patchNaming(dst, before, after files.Rules) files.Rules {
	applyField(&dst.KeepOriginal, before.KeepOriginal, after.KeepOriginal)
	applyField(&dst.Prefix, before.Prefix, after.Prefix)
	applyField(&dst.Suffix, before.Suffix, after.Suffix)
	applyField(&dst.Template, before.Template, after.Template)
	applyField(&dst.Sequence, before.Sequence, after.Sequence)
	applyField(&dst.Start, before.Start, after.Start)
	applyField(&dst.Digits, before.Digits, after.Digits)

	return dst
}

func patchCaption(dst, before, after caption.Caption) caption.Caption {
	applyField(&dst.Text, before.Text, after.Text)
	applyField(&dst.Style.Background, before.Style.Background, after.Style.Background)
	applyField(&dst.Style.TextColor, before.Style.TextColor, after.Style.TextColor)
	applyField(&dst.Style.Opacity, before.Style.Opacity, after.Style.Opacity)
	applyField(&dst.Style.Size, before.Style.Size, after.Style.Size)
	applyField(&dst.Style.Height, before.Style.Height, after.Style.Height)
	applyField(&dst.Style.Alignment, before.Style.Alignment, after.Style.Alignment)

	return dst
}
