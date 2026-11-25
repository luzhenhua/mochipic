package l10n

import (
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/egoist/mygo/i18n"
)

func catalogFile(t *testing.T, tag string) map[string]string {
	t.Helper()
	data, err := catalogs.ReadFile("locales/" + tag + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var messages map[string]string
	if err := json.Unmarshal(data, &messages); err != nil {
		t.Fatal(err)
	}
	return messages
}

func TestCatalogsUseTheSameMessagesAndArguments(t *testing.T) {
	source := catalogFile(t, Default)
	files, err := fs.Glob(catalogs, "locales/*.json")
	if err != nil || len(files) != len(Languages()) {
		t.Fatalf("invalid locale filename: found %v, loaded %v (%v)", files, tags, err)
	}
	for _, lang := range Languages() {
		t.Run(lang.Tag, func(t *testing.T) {
			messages := catalogFile(t, lang.Tag)
			if messages["@@locale"] != lang.Tag || lang.Name == "" || lang.Name == "meta.languageName" {
				t.Errorf("invalid locale metadata: %+v", lang)
			}
			for key, value := range messages {
				if _, ok := source[key]; !ok {
					t.Errorf("unknown message %q", key)
				}
				if strings.TrimSpace(value) == "" {
					t.Errorf("empty message %q", key)
				}
			}
			for key := range source {
				if strings.HasPrefix(key, "@") {
					continue
				}
				msg, found, ok := catalog.Message(lang.Tag, key)
				if !ok || found != lang.Tag {
					t.Errorf("%q is missing or invalid", key)
					continue
				}
				base, err := i18n.ParseMessage(source[key])
				if err != nil {
					t.Errorf("%q: %v", key, err)
					continue
				}
				names := func(m *i18n.Message) []string {
					var names []string
					for _, a := range m.Args() {
						names = append(names, a.Name)
					}
					slices.Sort(names)
					return names
				}
				if !slices.Equal(names(base), names(msg)) {
					t.Errorf("%q arguments: want %v, got %v", key, names(base), names(msg))
				}
			}
		})
	}
}

func TestMessageReferencesExist(t *testing.T) {
	source := catalogFile(t, Default)
	set := token.NewFileSet()
	root := filepath.Join("..", "..")
	for _, dir := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			f, err := parser.ParseFile(set, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				var index int
				switch fn := call.Fun.(type) {
				case *ast.Ident:
					if fn.Name != "tr" && fn.Name != "trf" {
						return true
					}
					index = 1
				case *ast.SelectorExpr:
					receiver, ok := fn.X.(*ast.Ident)
					if !ok {
						return true
					}
					switch receiver.Name + "." + fn.Sel.Name {
					case "l10n.T", "l10n.Format":
						index = 1
					case "d.t", "l10n.New", "l10n.Error", "l10n.Wrap", "l10n.WrapWithArgs":
						index = 0
					default:
						return true
					}
				default:
					return true
				}
				if index >= len(call.Args) {
					return true
				}
				if lit, ok := call.Args[index].(*ast.BasicLit); ok && lit.Kind == token.STRING {
					key, _ := strconv.Unquote(lit.Value)
					if _, ok := source[key]; !ok {
						t.Errorf("%s: no message %q", set.Position(lit.Pos()), key)
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestPluralizationAndFallback(t *testing.T) {
	if one, many := Format("en", "image.count", Args{"count": 1}), Format("en", "image.count", Args{"count": 2}); one != "1 image" || many != "2 images" {
		t.Fatalf("unexpected plural forms: %q, %q", one, many)
	}
	if got := T("xx", "nav.settings"); got != "设置" {
		t.Fatalf("unsupported locale should fall back to Chinese: %q", got)
	}
	if got := Format("ja", "error.unknownNameVariable", Args{"variable": "serial"}); got != "不明な命名変数 {serial}" {
		t.Fatalf("naming token was altered: %q", got)
	}
}

func TestNestedErrorsKeepCausesAndUserText(t *testing.T) {
	const name = "零件{count}.png"
	sentinel := errors.New("file not found")
	err := WrapWithArgs("error.presetDetails", Wrap("error.readLogo", sentinel), Args{"name": name})
	if !errors.Is(err, sentinel) {
		t.Fatal("lost the underlying error")
	}
	want := "Préréglage “" + name + "” : Lecture du logo : file not found"
	if got := ErrorText("fr", err); got != want {
		t.Fatalf("want %q, got %q", want, got)
	}
}
