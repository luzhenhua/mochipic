package imaging

import "path/filepath"

func DefaultLogo() LogoMark {
	return LogoMark{KeepRatio: true, Width: 18, Height: 18, Placement: Placement{MarginX: 40, MarginY: 40, Opacity: 82}}
}

func (l LogoMark) valid() bool {
	return filepath.IsAbs(l.Path) && FormatOf(l.Path) == PNG && l.Width >= 1 && l.Width <= 100 &&
		(l.KeepRatio || l.Height >= 1 && l.Height <= 100) && l.Placement.Valid(MaxSide)
}

// LogoMarks is in back-to-front order. The first slot also stores the
// empty draft and keeps single-Logo preferences compatible.
func (o Options) LogoMarks() []LogoMark {
	var marks []LogoMark
	if o.Logo.Enabled {
		marks = append(marks, o.Logo)
	}
	for _, l := range o.AdditionalLogos {
		if l.Enabled {
			marks = append(marks, l)
		}
	}
	return marks
}

func (o *Options) SetLogoMarks(marks []LogoMark) {
	clear(o.AdditionalLogos[:])
	if len(marks) == 0 {
		o.Logo.Path, o.Logo.Enabled = "", false
		return
	}
	o.Logo = marks[0]
	copy(o.AdditionalLogos[:], marks[1:])
}

func (o *Options) LogoByPath(path string) *LogoMark {
	if path == "" {
		return nil
	}
	if o.Logo.Enabled && o.Logo.Path == path {
		return &o.Logo
	}
	for i := range o.AdditionalLogos {
		if l := &o.AdditionalLogos[i]; l.Enabled && l.Path == path {
			return l
		}
	}
	return nil
}

// The old LogoOnTop flag still defines the two ends of the stack.
// LogosBelowText lets text sit between Logos without moving either one.
func (o Options) TextLayerPosition(logoCount int) int {
	if !o.LogoOnTop {
		return logoCount
	}
	return min(logoCount, max(0, o.LogosBelowText))
}

func (o *Options) SetTextLayerPosition(below, logoCount int) {
	o.LogoOnTop = below < logoCount
	o.LogosBelowText = 0
	if o.LogoOnTop {
		o.LogosBelowText = max(0, below)
	}
}
