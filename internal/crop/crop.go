// Package crop describes a non-destructive crop in oriented source pixels.
package crop

import (
	"image"
	"math"
)

type Ratio uint8

const (
	Free Ratio = iota
	Square
	Portrait
	FourThree
	Wide
)

func (r Ratio) Units() image.Point {
	switch r {
	case Square:
		return image.Pt(1, 1)
	case Portrait:
		return image.Pt(4, 5)
	case FourThree:
		return image.Pt(4, 3)
	case Wide:
		return image.Pt(16, 9)
	default:
		return image.Point{}
	}
}

type Alignment uint8

const (
	Center Alignment = iota
	Top
	Bottom
)

// A Region with Reference records a hand-drawn box. A Region with only a
// Ratio and Alignment fits a new box independently to each source.
type Region struct {
	Box       image.Rectangle
	Reference image.Point
	Ratio     Ratio
	Alignment Alignment
}

func (r Region) Active() bool { return r != (Region{}) }

func (r Region) Resolve(size image.Point) image.Rectangle {
	full := image.Rectangle{Max: size}
	if !r.Active() || full.Empty() {
		return full
	}
	if r.Reference == (image.Point{}) {
		b := Fit(full, r.Ratio)
		switch r.Alignment {
		case Top:
			b = b.Sub(image.Pt(0, b.Min.Y))
		case Bottom:
			b = b.Add(image.Pt(0, full.Max.Y-b.Max.Y))
		}
		return b
	}
	if r.Box.Empty() || !r.Box.In(image.Rectangle{Max: r.Reference}) {
		return image.Rectangle{}
	}
	if size == r.Reference {
		return r.Box
	}
	// Different TIFF pages may have different geometry. Keep the aspect
	// and relative center of the chosen box without stretching it.
	sx, sy := float64(size.X)/float64(r.Reference.X), float64(size.Y)/float64(r.Reference.Y)
	boxSize := r.Box.Size()
	capW, capH := max(1, round(float64(boxSize.X)*sx)), max(1, round(float64(boxSize.Y)*sy))
	units := r.Ratio.Units()
	if units == (image.Point{}) {
		units = boxSize
	}
	wh := fitSize(image.Pt(min(size.X, capW), min(size.Y, capH)), units)
	x := clamp(round(float64(r.Box.Min.X+r.Box.Max.X)*sx/2-float64(wh.X)/2), 0, size.X-wh.X)
	y := clamp(round(float64(r.Box.Min.Y+r.Box.Max.Y)*sy/2-float64(wh.Y)/2), 0, size.Y-wh.Y)
	return image.Rectangle{Min: image.Pt(x, y), Max: image.Pt(x, y).Add(wh)}
}

// Fit chooses the largest box of the given ratio centered in within.
func Fit(within image.Rectangle, r Ratio) image.Rectangle {
	if r == Free || within.Empty() {
		return within
	}
	wh := fitSize(within.Size(), r.Units())
	start := within.Min.Add(within.Size().Sub(wh).Div(2))
	return image.Rectangle{Min: start, Max: start.Add(wh)}
}

func fitSize(limit, units image.Point) image.Point {
	if units.X <= 0 || units.Y <= 0 {
		return limit
	}
	if k := min(limit.X/units.X, limit.Y/units.Y); k > 0 {
		return units.Mul(k)
	}
	scale := min(float64(limit.X)/float64(units.X), float64(limit.Y)/float64(units.Y))
	return image.Pt(clamp(round(float64(units.X)*scale), 1, limit.X), clamp(round(float64(units.Y)*scale), 1, limit.Y))
}

// Move retains the box size and stops at the source boundaries.
func Move(box image.Rectangle, size, delta image.Point) image.Rectangle {
	w, h := box.Dx(), box.Dy()
	x := clamp(box.Min.X+delta.X, 0, size.X-w)
	y := clamp(box.Min.Y+delta.Y, 0, size.Y-h)
	return image.Rect(x, y, x+w, y+h)
}

// ResizeCorner anchors the opposite corner. Corners are numbered in row order.
func ResizeCorner(box image.Rectangle, size image.Point, corner int, delta image.Point, ratio Ratio) image.Rectangle {
	anchor, moving := box.Min, box.Max
	sx, sy := 1, 1
	if corner%2 == 0 {
		anchor.X, moving.X, sx = box.Max.X, box.Min.X, -1
	}
	if corner < 2 {
		anchor.Y, moving.Y, sy = box.Max.Y, box.Min.Y, -1
	}
	capW, capH := size.X-anchor.X, size.Y-anchor.Y
	if sx < 0 {
		capW = anchor.X
	}
	if sy < 0 {
		capH = anchor.Y
	}
	w := max(1, sx*(moving.X+delta.X-anchor.X))
	h := max(1, sy*(moving.Y+delta.Y-anchor.Y))
	if u := ratio.Units(); u.X > 0 {
		if maxK := min(capW/u.X, capH/u.Y); maxK >= 1 {
			k := clamp(round(float64(w*u.X+h*u.Y)/float64(u.X*u.X+u.Y*u.Y)), 1, maxK)
			w, h = k*u.X, k*u.Y
		} else {
			wh := fitSize(image.Pt(capW, capH), u)
			w, h = wh.X, wh.Y
		}
	}
	return image.Rect(anchor.X, anchor.Y, anchor.X+sx*min(w, capW), anchor.Y+sy*min(h, capH))
}

func round(n float64) int     { return int(math.Round(n)) }
func clamp(n, lo, hi int) int { return min(max(n, lo), hi) }
