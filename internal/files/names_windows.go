package files

import (
	"path/filepath"
	"strings"
)

func platformComponent(name string) bool {
	return filepath.IsLocal(name) && !strings.ContainsAny(name, `<>"|?*`) && strings.TrimRight(name, ". ") == name
}
