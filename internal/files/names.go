// Package files plans and publishes exports without replacing any existing file.
package files

import (
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"imageflow/internal/imageformat"
	"imageflow/internal/l10n"

	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

type Rules struct {
	KeepOriginal             bool
	Prefix, Suffix, Template string
	Sequence                 bool
	Start, Digits            int
}

func DefaultRules() Rules { return Rules{KeepOriginal: true, Sequence: true, Start: 1, Digits: 3} }

func (r Rules) Pattern() string {
	if r.Template != "" {
		return r.Template
	}
	p := "{prefix}{original}{suffix}"
	if r.Sequence {
		p += "_{index}"
	}
	return p
}

func (r Rules) Name(original, extension string, ordinal int, date time.Time) (string, error) {
	if extension != ".tiff" && !imageformat.SupportsOutputExtension(extension) {
		return "", l10n.New("error.unsupportedExtension")
	}
	base := strings.TrimSuffix(filepath.Base(original), filepath.Ext(original))
	if !r.KeepOriginal {
		p := r.Pattern()
		if len(p) > 1024 {
			return "", l10n.New("error.nameTemplateTooLong")
		}
		index := ""
		if r.Sequence {
			if r.Start < 0 || r.Start > 999_999_999 || r.Digits < 1 || r.Digits > 9 || ordinal < 0 || ordinal > 999_999_999 {
				return "", l10n.New("error.invalidSequence")
			}
			index = strconv.Itoa(r.Start + ordinal)
			index = strings.Repeat("0", max(0, r.Digits-len(index))) + index
		}
		v := map[string]string{"original": base, "prefix": r.Prefix, "suffix": r.Suffix, "index": index, "date": date.Format("20060102")}
		var out strings.Builder
		for len(p) > 0 {
			i := strings.IndexAny(p, "{}")
			if i < 0 {
				out.WriteString(p)
				break
			}
			if p[i] == '}' {
				return "", l10n.New("error.unmatchedTemplateBraces")
			}
			out.WriteString(p[:i])
			p = p[i+1:]
			key, rest, ok := strings.Cut(p, "}")
			if !ok {
				return "", l10n.New("error.unmatchedTemplateBraces")
			}
			value, ok := v[key]
			if !ok {
				return "", l10n.Error("error.unknownNameVariable", l10n.Args{"variable": key})
			}
			out.WriteString(value)
			p = rest
		}
		base = out.String()
	}
	name := norm.NFC.String(base) + extension
	if !validComponent(base) || !validComponent(name) || len(name) > 240 {
		return "", l10n.New("error.invalidOutputName")
	}
	return name, nil
}

func validComponent(s string) bool {
	return s != "" && s != "." && s != ".." && utf8.ValidString(s) && !strings.ContainsAny(s, "/\\:") &&
		strings.IndexFunc(s, unicode.IsControl) < 0 && platformComponent(s)
}

func validateRelativeDir(s string) error {
	if s == "" || s == "." {
		return nil
	}
	if filepath.IsAbs(s) || filepath.Clean(s) != s {
		return l10n.New("error.unsafeSubfolder")
	}
	for _, part := range strings.Split(s, string(filepath.Separator)) {
		if !validComponent(part) {
			return l10n.New("error.unsafeSubfolder")
		}
	}
	return nil
}

var foldCase = cases.Fold()

func fold(s string) string { return norm.NFD.String(foldCase.String(s)) }

// Within tests path equality or a descendant on a conservative Unicode and
// case-folded basis, also on case-sensitive volumes.
func Within(path, root string) bool {
	if root == "" {
		return false
	}
	p, r := fold(filepath.Clean(path)), fold(filepath.Clean(root))
	return p == r || strings.HasPrefix(p, strings.TrimRight(r, string(filepath.Separator))+string(filepath.Separator))
}
