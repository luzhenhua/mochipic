// Package series defines frame and page choices shared by scanning, planning,
// preview and export. Positions are one-based and follow source order.
package series

import (
	"strconv"
	"strings"

	"imageflow/internal/l10n"
)

const (
	MaxParts   = 1000
	MaxOutputs = 10000
)

type Kind string

const (
	Frames Kind = "frames"
	Pages  Kind = "pages"
)

type Info struct {
	Kind  Kind
	Count int
}

func (i Info) Multiple() bool { return i.Count > 1 }

type Scope string

const (
	All    Scope = "all"
	Custom Scope = "custom"
)

type Selection struct {
	Scope    Scope
	Range    string
	Separate bool // Export numbered stills in the active format.
}

func (s Selection) Validate(i Info) error {
	if !i.Multiple() {
		return nil
	}
	if i.Count > MaxParts || (i.Kind != Frames && i.Kind != Pages) {
		return l10n.Error("error.seriesLimit", l10n.Args{"count": MaxParts})
	}
	switch s.Scope {
	case All:
		return nil
	case Custom:
		return ranges(s.Range, i.Count, nil)
	default:
		return l10n.New("error.seriesScope")
	}
}

func (s Selection) Parts(i Info) ([]int, error) {
	if !i.Multiple() {
		return nil, nil
	}
	if i.Count > MaxParts || (i.Kind != Frames && i.Kind != Pages) {
		return nil, l10n.Error("error.seriesLimit", l10n.Args{"count": MaxParts})
	}
	switch s.Scope {
	case All:
		parts := make([]int, i.Count)
		for n := range parts {
			parts[n] = n + 1
		}
		return parts, nil
	case Custom:
		var picked [MaxParts + 1]bool
		if err := ranges(s.Range, i.Count, func(a, b int) {
			for n := a; n <= b; n++ {
				picked[n] = true
			}
		}); err != nil {
			return nil, err
		}
		var parts []int
		for n, yes := range picked[:i.Count+1] {
			if yes {
				parts = append(parts, n)
			}
		}
		return parts, nil
	default:
		return nil, l10n.New("error.seriesScope")
	}
}

var rangePunctuation = strings.NewReplacer("，", ",", "、", ",", "－", "-", "–", "-", "—", "-")

func ranges(expression string, count int, add func(first, last int)) error {
	bad := l10n.Error("error.seriesRange", l10n.Args{"count": count})
	if len(expression) > 8192 {
		return bad
	}
	expression = rangePunctuation.Replace(expression)
	number := func(s string) (int, bool) {
		s = strings.TrimSpace(s)
		for _, r := range s {
			if r < '0' || r > '9' {
				return 0, false
			}
		}
		n, err := strconv.Atoi(s)
		return n, err == nil && n >= 1 && n <= count
	}
	for token := range strings.SplitSeq(expression, ",") {
		a, b, ranged := strings.Cut(token, "-")
		first, ok := number(a)
		if !ok {
			return bad
		}
		last := first
		if ranged {
			last, ok = number(b)
			if !ok || last < first {
				return bad
			}
		}
		if add != nil {
			add(first, last)
		}
	}
	return nil
}
