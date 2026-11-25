package app

import "imageflow/internal/batch"

// Failure tracks unresolved work across retries. The path belongs to the
// attempt that produced the error, which may use a different output root.
type Failure struct {
	batch.Result
	Directory string
}

func reconcileFailures(previous []Failure, report batch.Report, retry bool) []Failure {
	out := make([]Failure, 0, report.Failure+len(previous))
	if retry {
		retried := make(map[string]batch.Status, len(report.Results))
		for _, r := range report.Results {
			retried[r.Entry.Source.ID()] = r.Status
		}
		for _, f := range previous {
			if status, attempted := retried[f.Entry.Source.ID()]; !attempted || status == batch.Cancelled {
				out = append(out, f)
			}
		}
	}
	for _, r := range report.Results {
		if r.Status == batch.Failed {
			out = append(out, Failure{r, report.Directory})
		}
	}
	return out
}
