package ui

import (
	"imageflow/internal/config"
	"math"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	defaultWindowWidth  = 1120
	defaultWindowHeight = 720
	minWindowWidth      = 980
	minWindowHeight     = 630
)

func windowMinimum(area mygo.Rectangle) config.WindowSize {
	minimum := config.WindowSize{Width: minWindowWidth, Height: minWindowHeight}
	if area.Width > 0 {
		minimum.Width = min(minimum.Width, area.Width)
	}
	if area.Height > 0 {
		minimum.Height = min(minimum.Height, area.Height)
	}
	return minimum
}

func fitWindowFrame(frame, area mygo.Rectangle) mygo.Rectangle {
	return fitFrame(frame, area, windowMinimum(area))
}

func fitFrame(frame, area mygo.Rectangle, minimum config.WindowSize) mygo.Rectangle {
	frame.Width, frame.Height = max(minimum.Width, frame.Width), max(minimum.Height, frame.Height)
	if area.Width > 0 {
		frame.Width = min(frame.Width, area.Width)
		frame.X = min(max(area.X, frame.X), area.X+area.Width-frame.Width)
	}
	if area.Height > 0 {
		frame.Height = min(frame.Height, area.Height)
		frame.Y = min(max(area.Y, frame.Y), area.Y+area.Height-frame.Height)
	}
	return frame
}

func initialWindowFrame(saved config.WindowSize, area mygo.Rectangle) mygo.Rectangle {
	if saved.Width <= 0 || saved.Height <= 0 {
		saved = config.WindowSize{Width: defaultWindowWidth, Height: defaultWindowHeight}
	}
	frame := fitWindowFrame(mygo.Rectangle{Width: saved.Width, Height: saved.Height}, area)
	if area.Width > 0 && area.Height > 0 {
		frame.X = area.X + (area.Width-frame.Width)/2
		frame.Y = area.Y + (area.Height-frame.Height)/2
	}
	return frame
}

func frameSize(frame mygo.Rectangle) config.WindowSize {
	return config.WindowSize{Width: frame.Width, Height: frame.Height}
}

// The current normal size can be clamped by a display change. Remember the
// user's choice separately so a smaller display does not overwrite it.
type windowGeometry struct {
	win                             *mygo.Window
	workArea                        mygo.Rectangle
	current, preferred              config.WindowSize
	fitting, compact, transitioning bool
	generation                      uint64
	timer                           *time.Timer
}

func (d *desktop) openMainWindow() {
	area := mygo.Screen.PrimaryDisplay().WorkArea
	saved := d.model.Preferences.WindowSize
	compact := d.emptyBatch() && d.page != settings
	frame, minimum := modeFrame(compact, saved, area), modeMinimum(compact, area)
	w := mygo.NewWindow(mygo.WindowOptions{
		Title: d.t("app.name"), X: frame.X, Y: frame.Y, Width: frame.Width, Height: frame.Height,
		MinWidth: minimum.Width, MinHeight: minimum.Height,
		TitleBarStyle: mygo.TitleBarHidden, TrafficLightPosition: trafficLights(),
		Content: ui.View(d.view),
	})
	d.win = w
	g := windowGeometry{win: w, workArea: area, current: frameSize(frame), preferred: saved, compact: compact}
	d.geometry = &g
	w.OnResize(g.resized)
	checkDisplay := func() { g.fitToDisplay(false) }
	restore := func() { checkDisplay(); g.setCompact(d.page == importHome) }
	w.OnMove(checkDisplay)
	w.OnLeaveFullScreen(restore)
	w.OnUnmaximize(restore)
	w.OnRestore(restore)
	stopWatching := mygo.Screen.OnDisplaysChanged(func() { g.fitToDisplay(true) })
	w.OnClose(func(e *mygo.CloseEvent) {
		if !d.allowQuit() {
			e.PreventDefault()
			return
		}
		p := d.model.Preferences
		if g.preferred != p.WindowSize {
			p.WindowSize = g.preferred
			d.model.SetPreferences(p)
		}
	})
	w.OnClosed(func() {
		g.stopTransition()
		stopWatching()
		mygo.App.Quit()
	})
}

func (g *windowGeometry) normal() bool {
	w := g.win
	return !g.fitting && !w.IsDestroyed() && !w.IsFullScreen() && !w.IsMaximized() && !w.IsMinimized()
}

func (g *windowGeometry) resized() {
	if g.transitioning || !g.normal() || g.fitToDisplay(false) {
		return
	}
	if size := frameSize(g.win.Bounds()); size != g.current && size.Width > 0 && size.Height > 0 {
		g.current = size
		if !g.compact {
			g.preferred = size
		}
	}
}

// Clamp when the window crosses onto a smaller display or the desktop's
// displays change. Do not move a fitting window while the user drags it.
func (g *windowGeometry) fitToDisplay(displaysChanged bool) bool {
	if !g.normal() {
		return false
	}
	frame := g.win.Bounds()
	area := mygo.Screen.DisplayMatching(frame).WorkArea
	if area.Width <= 0 || area.Height <= 0 || area == g.workArea && !displaysChanged {
		return false
	}
	g.stopTransition()
	minimum := modeMinimum(g.compact, area)
	fitted := fitFrame(frame, area, minimum)
	g.workArea, g.fitting = area, true
	g.win.SetMinimumSize(minimum.Width, minimum.Height)
	if frameSize(fitted) != frameSize(frame) || displaysChanged && fitted != frame {
		g.win.SetBounds(fitted)
	}
	g.current = frameSize(g.win.Bounds())
	g.fitting = false
	return true
}

func modeMinimum(compact bool, area mygo.Rectangle) config.WindowSize {
	size := windowMinimum(area)
	if compact {
		size.Width = min(size.Width, 660)
		size.Height = min(size.Height, 390)
	}
	return size
}

func modeFrame(compact bool, saved config.WindowSize, area mygo.Rectangle) mygo.Rectangle {
	if !compact {
		return initialWindowFrame(saved, area)
	}
	size := modeMinimum(true, area)
	return mygo.Rectangle{X: area.X + (area.Width-size.Width)/2, Y: area.Y + (area.Height-size.Height)/2, Width: size.Width, Height: size.Height}
}

func (g *windowGeometry) stopTransition() {
	g.generation++
	if g.timer != nil {
		g.timer.Stop()
		g.timer = nil
	}
	g.transitioning = false
}

// Animate the existing window on the MyGo main thread. Intermediate sizes
// are presentation state and must never become the user's saved workspace size.
func (g *windowGeometry) setCompact(compact bool) {
	if g == nil || compact == g.compact || !g.normal() {
		return
	}
	g.stopTransition()
	start := g.win.Bounds()
	area := mygo.Screen.DisplayMatching(start).WorkArea
	dest := modeFrame(compact, g.preferred, area)
	minimum := modeMinimum(compact, area)
	dest.X = start.X + (start.Width-dest.Width)/2
	dest.Y = start.Y + (start.Height-dest.Height)/2
	dest = fitFrame(dest, area, minimum)
	g.compact, g.transitioning, g.workArea = compact, true, area
	small := modeMinimum(true, area)
	g.win.SetMinimumSize(small.Width, small.Height)
	began := time.Now()
	revision := g.generation
	var tick func()
	tick = func() {
		if revision != g.generation || g.win.IsDestroyed() {
			return
		}
		if !g.normal() {
			g.stopTransition()
			g.compact = !compact
			return
		}
		progress := min(1., float64(time.Since(began))/float64(250*time.Millisecond))
		if mygo.Theme.Preferences().ReduceMotion {
			progress = 1
		}
		mix := func(a, b int) int { return a + int(math.Round(float64(b-a)*progress*progress*(3-2*progress))) }
		g.fitting = true
		g.win.SetBounds(mygo.Rectangle{X: mix(start.X, dest.X), Y: mix(start.Y, dest.Y), Width: mix(start.Width, dest.Width), Height: mix(start.Height, dest.Height)})
		if progress == 1 {
			g.win.SetMinimumSize(minimum.Width, minimum.Height)
			g.transitioning = false
			g.timer = nil
		}
		g.current = frameSize(g.win.Bounds())
		g.fitting = false
		if g.transitioning {
			g.timer = time.AfterFunc(time.Second/60, func() { mygo.RunOnMain(tick) })
		} else {
			g.win.Invalidate()
		}
	}
	tick()
}
