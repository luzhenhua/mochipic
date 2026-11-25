package app

import (
	"context"
	"errors"
	"image"
	"io"
	"path/filepath"
	"slices"
	"time"

	"imageflow/internal/crop"
	"imageflow/internal/files"
	"imageflow/internal/imaging"
	"imageflow/internal/l10n"
	"imageflow/internal/preview"
)

type Collage struct {
	Options                        imaging.CollageOptions
	Items                          []files.Source
	Tiles                          map[string]imaging.CollageTile
	Layers                         []imaging.PreviewLayer
	Loading, Exporting, Cancelling bool
	Err, LayerErr                  error
	LastOutput                     string
	Cancelled                      bool
}

type collageMarks struct {
	size image.Point
	opts imaging.Options
}

type collageSession struct {
	tiles  *preview.Latest[[]imaging.CollageInput, previewResult[map[string]imaging.CollageTile]]
	marks  *preview.Latest[collageMarks, previewResult[[]imaging.PreviewLayer]]
	cancel context.CancelFunc
	draft  collageDraft
	loaded []imaging.CollageInput
}

type collageDraft struct {
	paths     [imaging.MaxCollageImages]string
	count     int
	options   imaging.CollageOptions
	recommend bool
}

func (m *Model) initCollage() {
	m.collage.tiles = previewWorker(m, 0, func(ctx context.Context, inputs []imaging.CollageInput) (map[string]imaging.CollageTile, error) {
		out := make(map[string]imaging.CollageTile, len(inputs))
		for _, input := range inputs {
			tile, err := imaging.LoadCollageTile(ctx, input)
			if err != nil {
				return out, l10n.WrapWithArgs("error.collageMaterial", err, l10n.Args{"file": filepath.Base(input.Path)})
			}
			out[input.Path] = tile
		}
		return out, nil
	}, func(tiles map[string]imaging.CollageTile, err error) {
		m.Collage.Tiles, m.Collage.Err, m.Collage.Loading = tiles, err, false
		m.syncCollage()
	})
	m.collage.marks = previewWorker(m, 50*time.Millisecond, func(ctx context.Context, q collageMarks) ([]imaging.PreviewLayer, error) {
		var text imaging.TextPreview
		var err error
		if q.opts.Text.Enabled {
			text, err = imaging.RenderPreviewText(ctx, q.size, q.opts.Text)
			if err != nil {
				return nil, err
			}
		}
		logos := make(map[string]imaging.LogoPreview)
		for _, mark := range q.opts.LogoMarks() {
			logo, err := imaging.RenderPreviewLogo(ctx, mark.Path)
			if err != nil {
				return nil, err
			}
			logos[mark.Path] = logo
		}
		return imaging.PreviewLayers(q.size, text, logos, q.opts), nil
	}, func(layers []imaging.PreviewLayer, err error) {
		m.Collage.Layers, m.Collage.LayerErr = layers, err
	})
}

func collageInputs(items []files.Source) []imaging.CollageInput {
	inputs := make([]imaging.CollageInput, len(items))
	for i, s := range items {
		inputs[i] = imaging.CollageInput{Path: s.Path, Crop: s.Crop}
	}
	return inputs
}

func (m *Model) BeginCollage(items []files.Source) error {
	if m.Working || m.Scanning || m.LogoPending || m.Collage.Exporting {
		return l10n.New("error.waitForOperation")
	}
	spec := imaging.DefaultCollage()
	if _, err := spec.Geometry(len(items)); err != nil {
		return err
	}
	for _, item := range items {
		if item.Series.Multiple() {
			return l10n.WrapWithArgs("error.collageMaterial", l10n.New("error.collageStillOnly"), l10n.Args{"file": filepath.Base(item.Path)})
		}
	}
	var draft collageDraft
	draft.count, draft.options, draft.recommend = len(items), spec, true
	for i, s := range items {
		draft.paths[i] = s.Path
	}
	before := m.collage.draft
	// The collage belongs to the batch; leaving its view does not discard it.
	// Reopen the same selection without resetting its order or canvas settings.
	if sameCollageSelection(before, draft) {
		m.syncCollage()
		return nil
	}
	recordValue(m, HistoryCollageCreate, 0, before, draft, setCollageDraft)
	setCollageDraft(m, draft)
	return nil
}

func sameCollageSelection(a, b collageDraft) bool {
	if a.count != b.count {
		return false
	}
	for _, path := range b.paths[:b.count] {
		if !slices.Contains(a.paths[:a.count], path) {
			return false
		}
	}
	return true
}

func (m *Model) HasCollage() bool { return len(m.Collage.Items) >= 2 }

func setCollageDraft(m *Model, draft collageDraft) {
	m.collage.draft = draft
	m.syncCollage()
}

func (m *Model) EndCollage() {
	if m.Collage.Exporting {
		return
	}
	m.collage.tiles.Cancel()
	m.collage.marks.Cancel()
	m.collage.draft, m.collage.loaded = collageDraft{}, nil
	m.Collage = Collage{}
}

// A removed member suspends the draft; restoring the queue makes it usable
// again. Tile data is only held for the current draft, never in undo history.
func (m *Model) syncCollage() {
	draft := m.collage.draft
	items := make([]files.Source, 0, draft.count)
	for _, path := range draft.paths[:draft.count] {
		if i := m.sourceIndexFor(path); i >= 0 {
			items = append(items, m.Sources[i])
		} else {
			items = nil
			break
		}
	}
	s := &m.Collage
	before := s.Options
	s.Options, s.Items = draft.options, items
	if len(items) < 2 {
		if m.collage.loaded != nil {
			m.collage.tiles.Cancel()
			m.collage.marks.Cancel()
			m.collage.loaded = nil
			s.Tiles, s.Layers, s.Loading, s.Err, s.LayerErr = nil, nil, false, nil, nil
		}
		return
	}
	inputs := collageInputs(items)
	match := len(inputs) == len(m.collage.loaded)
	for _, input := range inputs {
		match = match && slices.Contains(m.collage.loaded, input)
	}
	if !match {
		m.collage.loaded = inputs
		s.Tiles, s.Err, s.Loading = nil, nil, true
		m.collage.tiles.Submit(inputs)
	}
	if draft.recommend && !s.Loading && s.Err == nil {
		sizes := make([]image.Point, len(items))
		for i, item := range items {
			sizes[i] = s.Tiles[item.Path].Size
		}
		size := imaging.RecommendedCollage(sizes, s.Options)
		s.Options.Width, s.Options.Height = size.X, size.Y
	}
	_, previousErr := before.Geometry(len(items))
	_, currentErr := s.Options.Geometry(len(items))
	if !match || before.Width != s.Options.Width || before.Height != s.Options.Height || (previousErr == nil) != (currentErr == nil) {
		m.requestCollageLayers()
	}
}

func (m *Model) requestCollageLayers() {
	s := &m.Collage
	s.Layers, s.LayerErr = nil, nil
	o := m.Image.PreviewOptions()
	g, err := s.Options.Geometry(len(s.Items))
	if err != nil || !o.Text.Enabled && len(o.LogoMarks()) == 0 {
		m.collage.marks.Cancel()
		return
	}
	m.collage.marks.Submit(collageMarks{g.Size, o})
}

func (m *Model) SetCollageOptions(o imaging.CollageOptions) {
	m.EditCollageOptions(o, 0)
}

func (m *Model) EditCollageOptions(o imaging.CollageOptions, group HistoryGroup) {
	s := &m.Collage
	if s.Exporting || !m.HasCollage() || s.Options == o {
		return
	}
	before := m.collage.draft
	after := before
	if o.Layout == imaging.CollageCustom && s.Options.Layout != o.Layout {
		if s.Loading || s.Err != nil {
			return
		}
		g, err := s.Options.Geometry(len(s.Items))
		if err != nil {
			return
		}
		for i, item := range s.Items {
			if _, ok := o.Photo(item.Path); !ok {
				size := s.Tiles[item.Path].Size
				p := s.Options.Place(g.Cells[i], size)
				o.PutPhoto(imaging.NewCollagePhoto(item.Path, g.Size, size, p))
			}
		}
	}
	if o.Width != s.Options.Width || o.Height != s.Options.Height || o.Layout == imaging.CollageCustom {
		after.recommend = false
	}
	after.options = o
	recordValue(m, HistoryCollageLayout, group, before, after, setCollageDraft)
	setCollageDraft(m, after)
}

func (m *Model) MoveCollage(source, target string) {
	s := &m.Collage
	if s.Exporting || source == target {
		return
	}
	from := slices.IndexFunc(s.Items, func(i files.Source) bool { return i.Path == source })
	to := slices.IndexFunc(s.Items, func(i files.Source) bool { return i.Path == target })
	if from < 0 || to < 0 {
		return
	}
	before := m.collage.draft.paths
	after := before
	item := after[from]
	if from < to {
		copy(after[from:to], after[from+1:to+1])
	} else {
		copy(after[to+1:from+1], after[to:from])
	}
	after[to] = item
	set := func(m *Model, v [imaging.MaxCollageImages]string) {
		m.collage.draft.paths = v
		m.syncCollage()
	}
	recordValue(m, HistoryCollageOrder, 0, before, after, set)
	set(m, after)
}

func (m *Model) StartCollageExport() error {
	s := &m.Collage
	if m.Working || m.Scanning || m.LogoPending || s.Loading || s.Exporting {
		return l10n.New("error.waitForOperation")
	}
	if s.Err != nil {
		return s.Err
	}
	if _, err := s.Options.Geometry(len(s.Items)); err != nil {
		return err
	}
	opts := m.Image
	opts.Resize, opts.Crop = imaging.Resize{}, crop.Region{}
	if err := opts.Validate(); err != nil {
		return err
	}
	output := m.Output
	if output.Directory == "" {
		return l10n.New("error.chooseOutputFolder")
	}
	layout, inputs, originals := s.Options, collageInputs(s.Items), slices.Clone(m.Sources)
	ctx, cancel := context.WithCancel(m.ctx)
	m.collage.cancel = cancel
	s.Exporting, s.Cancelled, s.LastOutput = true, false, ""
	m.wg.Go(func() {
		defer cancel()
		path, directory, err := exportCollage(ctx, originals, inputs, layout, opts, output)
		m.onUI(func() {
			m.collage.cancel = nil
			s.Exporting, s.Cancelling = false, false
			if errors.Is(err, context.Canceled) {
				s.Cancelled = true
			} else if err != nil {
				m.Error = l10n.Wrap("error.collageExport", err)
			} else {
				s.LastOutput = path
				if !slices.Contains(m.knownOutputs, directory) {
					m.knownOutputs = append(m.knownOutputs, directory)
				}
			}
		})
	})
	return nil
}

func exportCollage(ctx context.Context, originals []files.Source, inputs []imaging.CollageInput, layout imaging.CollageOptions, opts imaging.Options, output files.Settings) (path, directory string, err error) {
	p, err := files.BuildCollagePlan(ctx, originals, output, opts.Format.Extension(), time.Now())
	if err != nil {
		return "", "", err
	}
	dst, err := files.OpenDestination(p)
	if err != nil {
		return "", "", err
	}
	defer dst.Close()
	path, err = dst.Commit(ctx, p.Entries[0], func(w io.Writer) error {
		_, err := imaging.ExportCollage(ctx, inputs, layout, opts, w)
		return err
	})
	return path, p.Directory, err
}

func (m *Model) CancelCollageExport() {
	if m.collage.cancel != nil {
		m.Collage.Cancelling = true
		m.collage.cancel()
	}
}
