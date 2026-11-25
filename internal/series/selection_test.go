package series

import (
	"slices"
	"testing"
)

func TestSelectionRequiresExplicitValidScope(t *testing.T) {
	info := Info{Kind: Frames, Count: 7}
	for _, s := range []Selection{{}, {Scope: Custom}, {Scope: Custom, Range: "0,1"}, {Scope: Custom, Range: "8"}, {Scope: Custom, Range: "2-1"}, {Scope: Custom, Range: "2, "}} {
		if _, err := s.Parts(info); err == nil {
			t.Fatalf("invalid selection accepted: %+v", s)
		}
	}
	if got, err := (Selection{Scope: Custom, Range: "5，3–4、1,5, 2-3"}).Parts(info); err != nil || !slices.Equal(got, []int{1, 2, 3, 4, 5}) {
		t.Fatalf("custom range: %v %v", got, err)
	}
	if got, err := (Selection{Scope: All}).Parts(info); err != nil || !slices.Equal(got, []int{1, 2, 3, 4, 5, 6, 7}) {
		t.Fatalf("all frames: %v %v", got, err)
	}
}
