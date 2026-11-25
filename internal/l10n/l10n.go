// Package l10n localizes ImageFlow messages without depending on the desktop UI.
package l10n

import (
	"embed"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"github.com/egoist/mygo/i18n"
)

const Default = "zh-Hans"

//go:embed locales/*.json
var catalogs embed.FS

var catalog = func() *i18n.Catalog {
	dir, err := fs.Sub(catalogs, "locales")
	if err != nil {
		panic(err)
	}
	c, err := i18n.NewCatalog(dir, Default)
	if err != nil {
		panic(err)
	}
	return c
}()

var tags = catalog.Languages()

type Language struct {
	Tag, Name string
}

// Register makes these catalogs available to MyGo and Lobe MyGo controls.
func Register() { i18n.Register(catalog) }

func Languages() []Language {
	out := make([]Language, len(tags))
	for i, tag := range tags {
		out[i] = Language{Tag: tag, Name: T(tag, "meta.languageName")}
	}
	return out
}

func Supported(tag string) bool { return slices.Contains(tags, tag) }

type Args = map[string]any

func T(tag, key string) string { return catalog.Format(tag, key, nil) }

func Format(tag, key string, args Args) string { return catalog.Format(tag, key, args) }

// messageError keeps the translated display message separate from its cause.
type messageError struct {
	key   string
	args  Args
	cause error
}

func New(key string) error { return Error(key, nil) }

func Error(key string, args Args) error {
	return &messageError{key: key, args: maps.Clone(args)}
}

// Wrap preserves errors.Is/As. Its message accepts a localized {cause}.
func Wrap(key string, cause error) error { return WrapWithArgs(key, cause, nil) }

func WrapWithArgs(key string, cause error, args Args) error {
	return &messageError{key: key, args: maps.Clone(args), cause: cause}
}

func (e *messageError) Error() string { return e.Localized(Default) }
func (e *messageError) Unwrap() error { return e.cause }

func (e *messageError) Localized(tag string) string {
	args := e.args
	if e.cause != nil {
		args = make(Args, len(e.args)+1)
		maps.Copy(args, e.args)
		args["cause"] = ErrorText(tag, e.cause)
	}
	return Format(tag, e.key, args)
}

// ErrorText translates app errors and preserves raw system or codec details.
func ErrorText(tag string, err error) string {
	if err == nil {
		return ""
	}
	if localized, ok := err.(interface{ Localized(string) string }); ok {
		return localized.Localized(tag)
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		text := make([]string, 0, len(joined.Unwrap()))
		for _, cause := range joined.Unwrap() {
			text = append(text, ErrorText(tag, cause))
		}
		return strings.Join(text, "\n")
	}
	return err.Error()
}
