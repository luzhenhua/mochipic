package certification

import (
	"cmp"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"imageflow/internal/l10n"
)

const MaxCustom = 24
const libraryName = "certification"

// Library lives next to the app's preferences, outside the source queue.
type Library string

func (l Library) open(create bool) (*os.Root, error) {
	if create {
		if err := os.MkdirAll(string(l), 0o700); err != nil {
			return nil, err
		}
	}
	parent, err := os.OpenRoot(string(l))
	if err != nil {
		return nil, err
	}
	defer parent.Close()
	if create {
		if err := parent.Mkdir(libraryName, 0o700); err != nil && !errors.Is(err, fs.ErrExist) {
			return nil, err
		}
	}
	info, err := parent.Lstat(libraryName)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, l10n.New("error.certificationLibrary")
	}
	return parent.OpenRoot(libraryName)
}

func listCustom(root *os.Root) ([]*Badge, error) {
	entries, err := fs.ReadDir(root.FS(), ".")
	if err != nil {
		return nil, err
	}
	var badges []*Badge
	for _, e := range entries {
		stem, png := strings.CutSuffix(e.Name(), ".png")
		hash, label, ok := strings.Cut(stem, "--")
		if !png || !ok || !e.Type().IsRegular() || len(hash) != sha256.Size*2 || label == "" {
			continue
		}
		if _, err := hex.DecodeString(hash); err != nil {
			continue
		}
		badges = append(badges, &Badge{name: label, path: filepath.Join(root.Name(), e.Name())})
	}
	slices.SortFunc(badges, func(a, b *Badge) int {
		return cmp.Or(strings.Compare(a.name, b.name), strings.Compare(a.path, b.path))
	})
	return badges, nil
}

func (l Library) List() ([]*Badge, error) {
	root, err := l.open(false)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer root.Close()
	badges, err := listCustom(root)
	return badges[:min(len(badges), MaxCustom)], err
}

func badgeName(path string) string {
	base := filepath.Base(path)
	label := strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || strings.ContainsRune("/\\:", r) {
			return -1
		}
		return r
	}, strings.TrimSuffix(base, filepath.Ext(base))))
	for i := range label {
		if i > 120 {
			label = strings.TrimSpace(label[:i])
			break
		}
	}
	if label == "" {
		return "PNG"
	}
	return label
}

// Save commits a new validated PNG without replacing any existing file. The
// content hash also makes importing the same asset again reuse its tile.
func (l Library) Save(source string, data []byte) (*Badge, error) {
	root, err := l.open(true)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	existing, err := listCustom(root)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(data)
	prefix := hex.EncodeToString(sum[:]) + "--"
	for _, b := range existing {
		if strings.HasPrefix(filepath.Base(b.path), prefix) {
			return b, nil
		}
	}
	if len(existing) >= MaxCustom {
		return nil, l10n.Error("error.certificationLibraryLimit", l10n.Args{"count": MaxCustom})
	}
	label := badgeName(source)
	name := prefix + label + ".png"
	tmp := ".import-" + rand.Text()
	f, err := root.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return nil, err
	}
	defer root.Remove(tmp)
	_, writeErr := f.Write(data)
	if err := errors.Join(writeErr, f.Close()); err != nil {
		return nil, err
	}
	if err := root.Link(tmp, name); err != nil {
		return nil, err
	}
	return &Badge{name: label, path: filepath.Join(root.Name(), name)}, nil
}
