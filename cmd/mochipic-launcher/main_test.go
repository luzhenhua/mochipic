package main

import (
	"strings"
	"testing"
)

func TestStartupLogKeepsBoundedErrorTail(t *testing.T) {
	var log startupLog
	for _, message := range []string{strings.Repeat("x", 4096), strings.Repeat("y", 320), "\nJPEG XL is unavailable"} {
		if n, err := log.Write([]byte(message)); err != nil || n != len(message) {
			t.Fatalf("partial stderr write: %d/%d %v", n, len(message), err)
		}
	}
	if got := string(log); len(got) != 2048 || !strings.HasSuffix(got, "\nJPEG XL is unavailable") {
		t.Fatalf("expected the bounded stderr tail with the startup error, got %q", got)
	}
}
