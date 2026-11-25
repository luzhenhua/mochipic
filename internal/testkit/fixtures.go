// Package testkit generates synthetic images for the automated verification.
package testkit

import (
	"crypto/sha256"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"testing"
)

func Solid(w, h int, c color.NRGBA) *image.NRGBA {
	img := image.NewNRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(c), image.Point{}, draw.Src)
	return img
}
func PNG(t testing.TB, path string, im image.Image) {
	t.Helper()
	Write(t, path, func(w io.Writer) error { return png.Encode(w, im) })
}
func JPEG(t testing.TB, path string, im image.Image) {
	t.Helper()
	Write(t, path, func(w io.Writer) error { return jpeg.Encode(w, im, &jpeg.Options{Quality: 90}) })
}
func Write(t testing.TB, path string, fn func(io.Writer) error) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	err = fn(f)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		t.Fatal(err)
	}
}
func Hash(t testing.TB, path string) [32]byte {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	h := sha256.New()
	if _, err = io.Copy(h, f); err != nil {
		t.Fatal(err)
	}
	var sum [32]byte
	copy(sum[:], h.Sum(nil))
	return sum
}
