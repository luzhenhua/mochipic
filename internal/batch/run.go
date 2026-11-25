package batch

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"time"

	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
)

const MaxWorkers = 4

const (
	StagePlan   = "stage.plan"
	StageCommit = "stage.commit"
	StageWrite  = "stage.write"
)

type Status string

const (
	Waiting   Status = "waiting"
	Running   Status = "running"
	Succeeded Status = "succeeded"
	Failed    Status = "failed"
	Skipped   Status = "skipped"
	Cancelled Status = "cancelled"
)

type Result struct {
	Entry    files.Entry
	Status   Status
	Output   string
	Stage    string
	Err      error
	Finished time.Time
}

type Progress struct {
	Total, Success, Failure, Skipped, Cancelled, Running, Remaining int
	Current                                                         []files.Source
	Recent                                                          []Result
	Started, Finished                                               time.Time
}

func (p Progress) Done() int { return p.Success + p.Failure + p.Skipped + p.Cancelled }
func (p Progress) Percent() int {
	if p.Total == 0 {
		return 0
	}
	return 100 * p.Done() / p.Total
}

type Report struct {
	Progress
	Directory string
	Results   []Result
}

func (r Report) FailedSources() []files.Source {
	var src []files.Source
	for _, res := range r.Results {
		if res.Status == Failed {
			src = append(src, res.Entry.Source)
		}
	}
	return src
}

type queue struct {
	mu      sync.Mutex
	next    int
	entries []files.Entry
	results []Result
	running map[int]files.Source
	p       Progress
}

func (q *queue) take(ctx context.Context) (int, files.Entry, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if ctx.Err() != nil || q.next >= len(q.entries) {
		return 0, files.Entry{}, false
	}
	i := q.next
	e := q.entries[i]
	q.next++
	q.p.Remaining--
	q.p.Running++
	q.running[i] = e.Source
	return i, e, true
}

func (q *queue) record(i int, result Result) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.results[i] = result
	delete(q.running, i)
	q.p.Running--
	switch result.Status {
	case Succeeded:
		q.p.Success++
	case Failed:
		q.p.Failure++
	case Skipped:
		q.p.Skipped++
	case Cancelled:
		q.p.Cancelled++
	}
	if len(q.p.Recent) == 6 {
		q.p.Recent = slices.Delete(q.p.Recent, 0, 1)
	}
	q.p.Recent = append(q.p.Recent, result)
}

func (q *queue) snapshot() Progress {
	q.mu.Lock()
	defer q.mu.Unlock()
	p := q.p
	p.Recent = slices.Clone(p.Recent)
	for _, src := range q.running {
		p.Current = append(p.Current, src)
	}
	slices.SortFunc(p.Current, func(a, b files.Source) int { return a.Ordinal - b.Ordinal })
	return p
}

type encodeFunc func(context.Context, string, imaging.Options, io.Writer) (imaging.Info, error)

func Run(ctx context.Context, p files.Plan, opts imaging.Options, workers int, notify func(Progress)) (Report, error) {
	return run(ctx, p, opts, workers, notify, nil)
}

func run(ctx context.Context, p files.Plan, opts imaging.Options, workers int, notify func(Progress), encode encodeFunc) (Report, error) {
	if len(p.Entries) == 0 {
		return Report{}, l10n.New("error.noImagesToProcess")
	}
	if err := opts.Validate(); err != nil {
		return Report{}, err
	}
	if err := ctx.Err(); err != nil {
		return Report{}, err
	}
	dst, err := files.OpenDestination(p)
	if err != nil {
		return Report{}, err
	}
	defer dst.Close()
	var encoder *imaging.BatchEncoder
	if encode == nil {
		encoder = imaging.NewBatchEncoder(ctx, opts)
		defer encoder.Close()
		encode = encoder.Export
	}
	q := queue{
		entries: p.Entries, results: make([]Result, len(p.Entries)), running: make(map[int]files.Source),
		p: Progress{Started: time.Now(), Total: len(p.Entries), Remaining: len(p.Entries)},
	}
	emit := func(p Progress) {
		if notify != nil {
			notify(p)
		}
	}
	emit(q.snapshot())
	var group sync.WaitGroup
	for range min(len(p.Entries), max(1, min(workers, MaxWorkers))) {
		group.Go(func() {
			for {
				i, e, ok := q.take(ctx)
				if !ok {
					return
				}
				result := execute(ctx, dst, e, opts, encode, encoder)
				q.record(i, result)
			}
		})
	}
	done := make(chan struct{})
	go func() { group.Wait(); close(done) }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			emit(q.snapshot())
		case <-done:
			now := time.Now()
			for i := q.next; i < len(q.entries); i++ {
				q.results[i] = Result{Entry: q.entries[i], Status: Cancelled, Finished: now, Err: ctx.Err()}
				q.p.Cancelled++
				q.p.Remaining--
			}
			q.p.Finished = now
			pg := q.snapshot()
			emit(pg)
			return Report{Progress: pg, Directory: p.Directory, Results: q.results}, nil
		}
	}
}

func execute(ctx context.Context, dst *files.Destination, e files.Entry, opts imaging.Options, encode encodeFunc, encoder *imaging.BatchEncoder) (out Result) {
	out.Entry = e
	defer func() { out.Finished = time.Now() }()
	switch {
	case e.Issue != nil:
		out.Status, out.Stage, out.Err = Failed, StagePlan, e.Issue
	case e.Skip:
		out.Status, out.Stage, out.Err = Skipped, StagePlan, files.ErrCollision
	default:
		path, err := dst.Commit(ctx, e, func(w io.Writer) error {
			s := e.Source
			if s.Settings != nil {
				opts = s.Settings.Image
			}
			opts.Crop = s.Crop
			opts.Certification = s.Certification
			opts.Caption = s.Caption
			if s.Part > 0 {
				if encoder != nil {
					_, err := encoder.ExportPart(ctx, s.Path, opts, s.Part, s.Series, w)
					return err
				}
				_, err := imaging.ExportPart(ctx, s.Path, opts, s.Part, s.Series, w)
				return err
			}
			if s.Series.Multiple() {
				parts, err := s.Selection.Parts(s.Series)
				if err != nil {
					return err
				}
				if encoder != nil {
					return encoder.ExportSeries(ctx, s.Path, opts, s.Series, parts, w)
				}
				return imaging.ExportSeries(ctx, s.Path, opts, s.Series, parts, w)
			}
			_, err := encode(ctx, s.Path, opts, w)
			return err
		})
		out.Output, out.Err = path, err
		switch {
		case err == nil:
			out.Status = Succeeded
		case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
			out.Status = Cancelled
		case errors.Is(err, files.ErrCollision):
			out.Status, out.Stage = Skipped, StageCommit
		default:
			out.Status, out.Stage = Failed, StageWrite
			if imageErr := (*imaging.StageError)(nil); errors.As(err, &imageErr) {
				out.Stage = string(imageErr.Stage)
			}
		}
	}
	return out
}
