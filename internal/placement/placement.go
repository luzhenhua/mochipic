// Package placement locates an overlay on the final-sized image.
package placement

import "image"

type Placement struct {
	// Anchor is in row-major order from top left to bottom right, 0–8.
	Anchor           int
	MarginX, MarginY int
	Opacity          int
	Free             FreePosition `json:",omitzero"`
}

// FreePosition is a fraction of the travel available to the whole overlay.
type FreePosition struct {
	Active bool
	X, Y   int
}

const positionUnits = 1_000_000

func (p *Placement) MoveTo(origin, canvas, mark image.Point) {
	axis := func(at, space int) int {
		if space <= 0 {
			return 0
		}
		at = max(0, min(at, space))
		return int((int64(at)*positionUnits + int64(space)/2) / int64(space))
	}
	p.Free = FreePosition{Active: true, X: axis(origin.X, canvas.X-mark.X), Y: axis(origin.Y, canvas.Y-mark.Y)}
}

func (p Placement) Valid(maxMargin int) bool {
	return p.Anchor >= 0 && p.Anchor <= 8 && p.Opacity >= 0 && p.Opacity <= 100 &&
		p.MarginX >= 0 && p.MarginY >= 0 && p.MarginX <= maxMargin && p.MarginY <= maxMargin &&
		(!p.Free.Active || p.Free.X >= 0 && p.Free.X <= positionUnits && p.Free.Y >= 0 && p.Free.Y <= positionUnits)
}

func (p Placement) Origin(canvas, mark image.Point) image.Point {
	space := canvas.Sub(mark)
	if p.Free.Active {
		axis := func(space, at int) int {
			return int((int64(max(0, space))*int64(at) + positionUnits/2) / positionUnits)
		}
		return image.Pt(axis(space.X, p.Free.X), axis(space.Y, p.Free.Y))
	}
	axis := func(space, side, margin int) int {
		switch side {
		case 0:
			return min(space, margin)
		case 1:
			return space / 2
		default:
			return max(0, space-margin)
		}
	}
	return image.Pt(axis(space.X, p.Anchor%3, p.MarginX), axis(space.Y, p.Anchor/3, p.MarginY))
}
