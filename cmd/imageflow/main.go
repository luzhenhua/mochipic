package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"
	"time"

	"imageflow/internal/crop"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/series"
	workspace "imageflow/internal/ui"
	"imageflow/internal/winupdate"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/lobehub/lobe-mygo/pkg/action"
	"github.com/lobehub/lobe-mygo/pkg/theme"
)

func main() {
	imaging.ConfigureBundledFonts()
	if err := imaging.Start(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
	err := run()
	imaging.Shutdown()
	if errors.Is(err, winupdate.ErrRestart) {
		os.Exit(winupdate.RestartExit)
	}
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	if len(os.Args) == 2 && os.Args[1] == "--verify-runtime" {
		// main has already initialized the required libvips codecs.
		return nil
	}
	if len(os.Args) < 3 || os.Args[1] != "--verify-window" {
		return workspace.Run()
	}
	var label string
	opts := imaging.Defaults()
	opts.Resize.Enabled = true
	opts.Resize.Mode = imaging.LongEdge
	opts.Resize.Limit = 64
	opts.Text.Enabled, opts.Text.Text, opts.Text.Font, opts.Text.Size = true, "验收", "PingFang SC", 12
	ctx := context.Background()
	var collageInputs []imaging.CollageInput
	for _, path := range os.Args[2:] {
		parts, err := imaging.InspectSource(ctx, path, imaging.FormatOf(path))
		if err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(path), err)
		}
		for _, format := range []imaging.Format{imaging.WebP, imaging.AVIF} {
			opts.Format = format
			p, err := imaging.RenderPreviewPart(ctx, path, opts, true, true, max(1, parts.Count))
			if err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(path), err)
			}
			if p.Before == nil || p.After == nil || max(p.Output.Width, p.Output.Height) > 64 || p.Output.Bytes == 0 {
				return fmt.Errorf("unexpected %s result: %+v", format, p.Output)
			}
			label = fmt.Sprintf("libvips %s · %s → %s %d × %d · %d bytes",
				imaging.Version(), p.Input.Format, format, p.Output.Width, p.Output.Height, p.Output.Bytes)
			log.Print(label)
			if format == imaging.WebP && !parts.Multiple() && len(collageInputs) < 2 && p.Input.Width >= 2 {
				collageInputs = append(collageInputs, imaging.CollageInput{Path: path})
				if len(collageInputs) == 1 {
					size := image.Pt(p.Input.Width, p.Input.Height)
					collageInputs[0].Crop = crop.Region{Reference: size, Box: image.Rect(0, 0, size.X/2, size.Y)}
				}
			}
		}
		if parts.Multiple() {
			if err := verifySeries(ctx, path, opts, parts); err != nil {
				return fmt.Errorf("%s: %w", filepath.Base(path), err)
			}
		}
	}
	if len(collageInputs) == 2 {
		if err := verifyComposition(ctx, collageInputs); err != nil {
			return err
		}
	}
	rendered := false
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{Title: l10n.T(mygo.App.Locale(), "app.name"), Width: 560, Height: 320, Content: ui.View(func(c *ui.Context) {
			theme.Apply(c)
			ui.Column(c).Fill().Center().Gap(12).Background(theme.TokensOf(c).BgContainer).Children(func() {
				ui.Text(c, label)
				if action.Button(c, l10n.T(c.Locale(), "common.close"), action.ButtonProps{Variant: action.Primary}).Clicked() {
					mygo.App.Quit()
				}
			})
			if !rendered {
				rendered = true
				log.Print("MyGo native frame rendered with Lobe MyGo")
				time.AfterFunc(2*time.Second, mygo.App.Quit)
			}
		})})
	})
	if err := mygo.App.Run(); err != nil {
		return err
	}
	if !rendered {
		return fmt.Errorf("native frame was not rendered")
	}
	return nil
}

func verifyComposition(ctx context.Context, inputs []imaging.CollageInput) error {
	opts := imaging.Defaults()
	opts.Format, opts.Crop = imaging.PNG, inputs[0].Crop
	want := opts.Crop.Box.Size()
	var buf bytes.Buffer
	if _, err := imaging.Export(ctx, inputs[0].Path, opts, &buf); err != nil {
		return fmt.Errorf("verify crop: %w", err)
	}
	cfg, err := png.DecodeConfig(&buf)
	if err != nil || cfg.Width != want.X || cfg.Height != want.Y {
		return fmt.Errorf("cropped PNG dimensions: got %d × %d, want %v, error %v", cfg.Width, cfg.Height, want, err)
	}
	log.Printf("crop verified: %d × %d PNG", cfg.Width, cfg.Height)
	buf.Reset()
	layout := imaging.DefaultCollage()
	layout.Width, layout.Height, layout.Gap = 159, 101, 5
	if _, err := imaging.ExportCollage(ctx, inputs, layout, opts, &buf); err != nil {
		return fmt.Errorf("verify collage: %w", err)
	}
	cfg, err = png.DecodeConfig(&buf)
	if err != nil || cfg.Width != layout.Width || cfg.Height != layout.Height {
		return fmt.Errorf("collage PNG dimensions: got %d × %d, error %v", cfg.Width, cfg.Height, err)
	}
	log.Print("collage verified: 2 stills → 159 × 101 PNG")
	return nil
}

func verifySeries(ctx context.Context, path string, opts imaging.Options, info series.Info) error {
	format, ext := imaging.WebP, ".webp"
	if info.Kind == series.Pages {
		format, ext = imaging.TIFF, ".tiff"
	}
	out, err := os.CreateTemp("", "imageflow-check-*"+ext)
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	err = imaging.ExportSeries(ctx, path, opts, info, []int{1, info.Count}, out)
	closeErr := out.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	got, err := imaging.InspectSource(ctx, out.Name(), format)
	if err != nil || got != (series.Info{Kind: info.Kind, Count: 2}) {
		return fmt.Errorf("verification output is not a two-part %s: %+v, %v", format, got, err)
	}
	log.Printf("series verified: %s → %s (%d %s)", filepath.Base(path), format, got.Count, got.Kind)
	return nil
}
