package app

import (
	"testing"

	"imageflow/internal/batch"
	"imageflow/internal/files"
)

func TestRetryPreservesOtherFailures(t *testing.T) {
	item := func(path string, status batch.Status) batch.Result {
		return batch.Result{Entry: files.Entry{Source: files.Source{Path: path}}, Status: status}
	}
	previous := []Failure{
		{item("recovered.png", batch.Failed), "/previous"},
		{item("still-fails.png", batch.Failed), "/previous"},
		{item("not-selected.png", batch.Failed), "/previous"},
		{item("cancelled.png", batch.Failed), "/previous"},
	}
	report := batch.Report{Directory: "/retry", Results: []batch.Result{
		item("recovered.png", batch.Succeeded), item("still-fails.png", batch.Failed), item("cancelled.png", batch.Cancelled),
	}}
	got := reconcileFailures(previous, report, true)
	if len(got) != 3 || got[0].Entry.Source.Path != "not-selected.png" || got[1].Entry.Source.Path != "cancelled.png" ||
		got[2].Entry.Source.Path != "still-fails.png" || got[2].Directory != "/retry" {
		t.Fatalf("unexpected unresolved failures: %+v", got)
	}
}
