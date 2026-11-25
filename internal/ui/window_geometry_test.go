package ui

import (
	"testing"

	"imageflow/internal/config"

	"github.com/egoist/mygo"
)

func TestWindowFitsTheCurrentDisplay(t *testing.T) {
	large := mygo.Rectangle{X: 20, Y: 24, Width: 1440, Height: 840}
	for _, tt := range []struct {
		name        string
		saved       config.WindowSize
		area, frame mygo.Rectangle
	}{
		{"new preferences", config.WindowSize{}, large, mygo.Rectangle{X: 180, Y: 84, Width: 1120, Height: 720}},
		{"restore smaller choice", config.WindowSize{Width: 1050, Height: 690}, large, mygo.Rectangle{X: 215, Y: 99, Width: 1050, Height: 690}},
		{"incomplete preference", config.WindowSize{Width: 1280}, large, mygo.Rectangle{X: 180, Y: 84, Width: 1120, Height: 720}},
		{"larger saved size on a laptop", config.WindowSize{Width: 1480, Height: 930}, mygo.Rectangle{Y: 24, Width: 1060, Height: 680}, mygo.Rectangle{Y: 24, Width: 1060, Height: 680}},
		{"display smaller than the normal minimum", config.WindowSize{}, mygo.Rectangle{X: -960, Y: 40, Width: 960, Height: 600}, mygo.Rectangle{X: -960, Y: 40, Width: 960, Height: 600}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := initialWindowFrame(tt.saved, tt.area); got != tt.frame {
				t.Fatalf("window frame = %+v; want %+v", got, tt.frame)
			}
		})
	}

	frame := mygo.Rectangle{X: -200, Y: -180, Width: 1250, Height: 680}
	area := mygo.Rectangle{Y: 24, Width: 1060, Height: 700}
	if got := fitWindowFrame(frame, area); got != (mygo.Rectangle{Y: 24, Width: 1060, Height: 680}) {
		t.Fatalf("normal window did not fit the new display: %+v", got)
	}
}
