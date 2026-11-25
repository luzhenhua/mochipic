package imaging

import (
	"image"
	"math"
)

type CollageCorner int

const (
	CornerSquare CollageCorner = iota
	CornerRound
	CornerBevel
)

// Frame is the complete placed photo; Visible is the rectangular crop on it.
// Content is expressed in oriented source pixels, after the individual photo crop.
type CollagePhoto struct {
	Path                    string
	Canvas, Original        image.Point
	Frame, Visible, Content image.Rectangle
	Corner                  CollageCorner
	Radius                  int
}

func (o CollageOptions) Photo(path string) (CollagePhoto, bool) {
	for _, p := range o.Photos {
		if p.Path == path && path != "" {
			return p, true
		}
	}
	return CollagePhoto{}, false
}
func (o *CollageOptions) PutPhoto(p CollagePhoto) {
	for i, v := range o.Photos {
		if v.Path == p.Path || v.Path == "" {
			o.Photos[i] = p
			return
		}
	}
}
func NewCollagePhoto(path string, canvas, source image.Point, p CollagePlacement) CollagePhoto {
	return CollagePhoto{Path: path, Canvas: canvas, Original: source, Frame: p.Target, Visible: p.Target, Content: p.Source, Radius: 24}
}
func mapBox(r, from, to image.Rectangle) image.Rectangle {
	coord := func(x, a, b, n, m int) int { return b + int(math.Round(float64(x-a)*float64(m)/float64(n))) }
	x := func(v int) int { return coord(v, from.Min.X, to.Min.X, from.Dx(), to.Dx()) }
	y := func(v int) int { return coord(v, from.Min.Y, to.Min.Y, from.Dy(), to.Dy()) }
	left, top := x(r.Min.X), y(r.Min.Y)
	return image.Rect(left, top, max(left+1, x(r.Max.X)), max(top+1, y(r.Max.Y)))
}

// fitInside keeps even subpixel slices addressable as a single output pixel.
func fitInside(r, bounds image.Rectangle) image.Rectangle {
	x := clampInt(r.Min.X, bounds.Min.X, bounds.Max.X-1)
	y := clampInt(r.Min.Y, bounds.Min.Y, bounds.Max.Y-1)
	return image.Rect(x, y, clampInt(r.Max.X, x+1, bounds.Max.X), clampInt(r.Max.Y, y+1, bounds.Max.Y))
}
func (p CollagePhoto) OnCanvas(size image.Point) CollagePhoto {
	if p.Canvas != size {
		// Resize the canvas without changing product proportions. Keep each
		// photo at its relative position within the new canvas.
		scale := min(float64(size.X)/float64(p.Canvas.X), float64(size.Y)/float64(p.Canvas.Y))
		v := p.Visible
		extent := image.Pt(max(1, int(math.Round(float64(v.Dx())*scale))), max(1, int(math.Round(float64(v.Dy())*scale))))
		center := func(lo, hi, old, new, span int) int {
			return clampInt(int(math.Round(float64(lo+hi)*.5*float64(new)/float64(old)-float64(span)*.5)), 0, new-span)
		}
		origin := image.Pt(center(v.Min.X, v.Max.X, p.Canvas.X, size.X, extent.X), center(v.Min.Y, v.Max.Y, p.Canvas.Y, size.Y, extent.Y))
		next := image.Rectangle{Min: origin, Max: origin.Add(extent)}
		p.Frame = mapBox(p.Frame, v, next).Union(next)
		p.Visible = next
		p.Canvas = size
	}
	return p
}
func (p CollagePhoto) Placement(src image.Point) CollagePlacement {
	content := fitInside(mapBox(p.Content, image.Rectangle{Max: p.Original}, image.Rectangle{Max: src}), image.Rectangle{Max: src})
	return CollagePlacement{Source: fitInside(mapBox(p.Visible, p.Frame, content), content), Target: p.Visible}
}
func clampInt(n, low, high int) int { return max(low, min(high, n)) }

// Adjust takes the total gesture delta in output pixels, relative to the press.
// A corner keeps its opposite corner fixed. The image itself never scales in crop mode.
func (p CollagePhoto) Adjust(part int, delta image.Point, stretch bool) CollagePhoto {
	v := p.Visible
	if part < 0 {
		delta.X = clampInt(delta.X, -v.Min.X, p.Canvas.X-v.Max.X)
		delta.Y = clampInt(delta.Y, -v.Min.Y, p.Canvas.Y-v.Max.Y)
		p.Frame = p.Frame.Add(delta)
		p.Visible = v.Add(delta)
		return p
	}
	left, top := part%2 == 0, part < 2
	if stretch && part >= 4 {
		left, top = part == 4, part == 6
		if part < 6 {
			delta.Y = 0
		} else {
			delta.X = 0
		}
	}
	bounds := image.Rectangle{Max: p.Canvas}
	if !stretch {
		bounds = bounds.Intersect(p.Frame)
	}
	mx, my := bounds.Max.X-v.Min.X, bounds.Max.Y-v.Min.Y
	if left {
		delta.X = -delta.X
		mx = v.Max.X - bounds.Min.X
	}
	if top {
		delta.Y = -delta.Y
		my = v.Max.Y - bounds.Min.Y
	}
	w, h := float64(v.Dx()), float64(v.Dy())
	nw, nh := clampInt(v.Dx()+delta.X, 1, mx), clampInt(v.Dy()+delta.Y, 1, my)
	if !stretch {
		scale := 1 + (w*float64(delta.X)+h*float64(delta.Y))/(w*w+h*h)
		scale = min(float64(mx)/w, float64(my)/h, max(1/min(w, h), scale))
		nw = max(1, int(math.Round(w*scale)))
		nh = max(1, int(math.Round(h*scale)))
	}
	if left {
		v.Min.X = v.Max.X - nw
	} else {
		v.Max.X = v.Min.X + nw
	}
	if top {
		v.Min.Y = v.Max.Y - nh
	} else {
		v.Max.Y = v.Min.Y + nh
	}
	if stretch {
		p.Content = p.Placement(p.Original).Source
		p.Frame = v
	}
	p.Visible = v
	return p
}
func (p CollagePhoto) EffectiveRadius() int {
	return max(0, min(p.Radius, p.Visible.Dx()/2, p.Visible.Dy()/2))
}

// A grid whose cell dimensions do not exceed any of the available originals.
func RecommendedCollage(sizes []image.Point, o CollageOptions) image.Point {
	g, err := o.Geometry(len(sizes))
	if err != nil {
		return image.Pt(o.Width, o.Height)
	}
	w, h := MaxSide, MaxSide
	for _, s := range sizes {
		if s.X < 1 || s.Y < 1 {
			return image.Pt(o.Width, o.Height)
		}
		w = min(w, s.X)
		h = min(h, s.Y)
	}
	gx, gy := (g.Columns+1)*o.Gap, (g.Rows+1)*o.Gap
	scale := min(1., float64(MaxSide-gx)/float64(w*g.Columns), float64(MaxSide-gy)/float64(h*g.Rows))
	for {
		size := image.Pt(max(1, int(float64(w)*scale))*g.Columns+gx, max(1, int(float64(h)*scale))*g.Rows+gy)
		if size.X*size.Y <= MaxPixels {
			return size
		}
		scale *= .95
	}
}

func maskCollageCorners(rgba []byte, size image.Point, p CollagePhoto) {
	if p.Corner == CornerSquare {
		return
	}
	r := p.EffectiveRadius()
	for y := 0; y < r; y++ {
		for x := 0; x < r; x++ {
			dx, dy := float64(r-x)-.5, float64(r-y)-.5
			dist := (dx + dy - float64(r)) / math.Sqrt2
			if p.Corner == CornerRound {
				dist = math.Hypot(dx, dy) - float64(r)
			}
			coverage := max(0., min(1., .5-dist))
			if coverage == 1 {
				continue
			}
			for _, pt := range [4]image.Point{{x, y}, {size.X - 1 - x, y}, {x, size.Y - 1 - y}, {size.X - 1 - x, size.Y - 1 - y}} {
				index := (pt.Y*size.X+pt.X)*4 + 3
				rgba[index] = byte(math.Round(float64(rgba[index]) * coverage))
			}
		}
	}
}
