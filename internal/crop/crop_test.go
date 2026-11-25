package crop

import (
	"image"
	"testing"
)

func TestCornerAndMoveStayInsideOrientedSource(t *testing.T) {
	size := image.Pt(100, 60)
	box := image.Rect(10, 5, 74, 41)
	if got := ResizeCorner(box, size, 3, image.Pt(99, 99), Wide); got != image.Rect(10, 5, 90, 50) {
		t.Fatalf("corner must lock 16:9 and keep the opposite corner: %v", got)
	}
	if got := ResizeCorner(box, size, 0, image.Pt(500, 500), Free); got != image.Rect(73, 40, 74, 41) {
		t.Fatalf("corner crossed its anchor: %v", got)
	}
	if got := Move(box, size, image.Pt(-100, 100)); got != image.Rect(0, 24, 64, 60) {
		t.Fatalf("move must stop at the source edges: %v", got)
	}
}

func TestPerImageRuleAndMixedPageGeometry(t *testing.T) {
	for _, tc := range []struct {
		ratio Ratio
		size  image.Point
		want  image.Rectangle
	}{
		{Portrait, image.Pt(120, 60), image.Rect(36, 0, 84, 60)},
		{Portrait, image.Pt(60, 120), image.Rect(0, 45, 60, 120)},
		{FourThree, image.Pt(121, 62), image.Rect(20, 2, 100, 62)},
		{FourThree, image.Pt(62, 121), image.Rect(1, 76, 61, 121)},
	} {
		rule := Region{Ratio: tc.ratio, Alignment: Bottom}
		if got := rule.Resolve(tc.size); got != tc.want {
			t.Errorf("size %v: got %v, want %v", tc.size, got, tc.want)
		}
	}
	manual := Region{Box: image.Rect(36, 10, 68, 50), Reference: image.Pt(100, 60), Ratio: Portrait}
	if got := manual.Resolve(image.Pt(60, 100)); got.Empty() || got.Dx()*5 != got.Dy()*4 || !got.In(image.Rect(0, 0, 60, 100)) {
		t.Fatalf("different page geometry distorted the crop: %v", got)
	}
	manual.Box.Min.X = -1
	if !manual.Resolve(manual.Reference).Empty() {
		t.Fatal("invalid crop must be rejected")
	}
}
