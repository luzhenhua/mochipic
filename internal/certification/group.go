// Package certification describes per-image badge selections and their layout.
package certification

import (
	"embed"
	"image"
	"math"
	"slices"

	"imageflow/internal/placement"
)

const (
	MaxBadges = 10
	MinSize   = 1
	MaxSize   = 25
	MaxGap    = 72
)

// Badge is an immutable library entry. Groups hold stable references, so a
// planned batch and its retries can snapshot selections by value.
type Badge struct {
	name, builtin, path string
}

//go:embed badges/*.png
var artwork embed.FS

var builtins = [...]*Badge{
	{name: "ISO 9001", builtin: "iso-9001"},
	{name: "ISO 14001", builtin: "iso-14001"},
	{name: "ISO 45001", builtin: "iso-45001"},
	{name: "UKCA", builtin: "ukca"},
	{name: "EAC", builtin: "eac"},
	{name: "UL", builtin: "ul"},
	{name: "CSA", builtin: "csa"},
	{name: "ETL", builtin: "etl"},
	{name: "ATEX", builtin: "atex"},
	{name: "IECEx", builtin: "iecex"},
}

func Builtins() [MaxBadges]*Badge  { return builtins }
func (b *Badge) Name() string      { return b.name }
func (b *Badge) LocalPath() string { return b.path }
func (b *Badge) EmbeddedPNG() ([]byte, error) {
	return artwork.ReadFile("badges/" + b.builtin + ".png")
}

type Group struct {
	badges [MaxBadges]*Badge
	Size   int // Each badge's height as a percentage of the final width.
	Gap    int // In final output pixels.
	placement.Placement
}

func Defaults() Group {
	return Group{Size: 9, Gap: 34, Placement: placement.Placement{Anchor: 2, Opacity: 100}}
}

func (g Group) Count() int {
	if i := slices.Index(g.badges[:], nil); i >= 0 {
		return i
	}
	return len(g.badges)
}

func (g Group) Badge(i int) *Badge { return g.badges[i] }
func (g Group) Has(b *Badge) bool  { return b != nil && slices.Contains(g.badges[:], b) }
func (g Group) Valid(maxSide int) bool {
	return g.Count() == 0 || g.Size >= MinSize && g.Size <= MaxSize && g.Gap >= 0 && g.Gap <= MaxGap && g.Placement.Valid(maxSide)
}

// Toggle preserves the order in which the user added the badges.
func (g *Group) Toggle(b *Badge) bool {
	if b == nil {
		return false
	}
	if i := slices.Index(g.badges[:], b); i >= 0 {
		copy(g.badges[i:], g.badges[i+1:])
		g.badges[len(g.badges)-1] = nil
		return true
	}
	if i := g.Count(); i < len(g.badges) {
		g.badges[i] = b
		return true
	}
	return false
}

type Layout struct {
	Bounds image.Rectangle
	Badges [MaxBadges]image.Rectangle
}

// Arrange shares pixel coordinates between the editable canvas and export.
// Use the same height for every badge, keep aspect ratios, and fit the entire
// row before placing it. On small canvases reduce spacing before it dominates.
func (g Group) Arrange(canvas image.Point, sizes [MaxBadges]image.Point) (Layout, bool) {
	var out Layout
	n := g.Count()
	if n == 0 {
		return out, true
	}
	if canvas.X < n || canvas.Y < 1 {
		return out, false
	}
	p := g.Placement
	p.MarginX = min((canvas.X*6+50)/100, (canvas.X-n)/2)
	p.MarginY = min((canvas.Y*48+500)/1000, (canvas.Y-1)/2)
	w, h := canvas.X-2*p.MarginX, canvas.Y-2*p.MarginY
	gap := 0
	if n > 1 {
		gap = min(g.Gap, (w-n)/(n-1), w/(2*(n-1)))
	}
	var ratios [MaxBadges]float64
	for i, size := range sizes[:n] {
		if size.X <= 0 || size.Y <= 0 {
			return out, false
		}
		ratios[i] = float64(size.X) / float64(size.Y)
	}
	width := func(i int, height float64) int { return max(1, int(math.Round(height*ratios[i]))) }
	rowWidth := func(height float64) int {
		total := gap * (n - 1)
		for i := range n {
			total += width(i, height)
		}
		return total
	}
	height := float64(max(1, min(h, (canvas.X*g.Size+50)/100)))
	if rowWidth(height) > w {
		low, high := 0.0, height
		// A fractional height also fits extremely wide custom marks on a
		// small canvas; the final output still uses whole pixels.
		for range 24 {
			mid := (low + high) / 2
			if rowWidth(mid) > w {
				high = mid
			} else {
				low = mid
			}
		}
		height = low
	}
	group := image.Pt(rowWidth(height), max(1, int(math.Round(height))))
	start := p.Origin(canvas, group)
	out.Bounds = image.Rectangle{Min: start, Max: start.Add(group)}
	x := start.X
	for i := range n {
		dx := width(i, height)
		out.Badges[i] = image.Rect(x, start.Y, x+dx, out.Bounds.Max.Y)
		x += dx + gap
	}
	return out, true
}
