package utils

import (
	"fmt"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

func GeneratePuml(outputFileName string) error {
	var b strings.Builder
	b.WriteString("@startuml\n")
	b.WriteString("title Go Module Interactions\n")

	err := filepath.Walk(".", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if filepath.Ext(path) != ".go" {
			return nil
		}

		fset := token.NewFileSet()
		node, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}

		packageName := node.Name.Name
		for _, imp := range node.Imports {
			importPath := imp.Path.Value[1 : len(imp.Path.Value)-1]
			if _, err := fmt.Fprintf(&b, "[%s] --> [%s]\n", packageName, importPath); err != nil {
				return err
			}
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to walk directory: %w", err)
	}

	b.WriteString("@enduml\n")

	return os.WriteFile(outputFileName, []byte(b.String()), 0600)
}
