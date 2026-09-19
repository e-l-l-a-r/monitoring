package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

// markedStruct описывает структуру, помеченную комментарием-маркером.
type markedStruct struct {
	// Type — описание структуры из AST.
	Type *ast.StructType
	// Name — имя типа структуры.
	Name string
}

// packageInfo содержит сведения о разобранном пакете, необходимые для генерации.
type packageInfo struct {
	// Underlying хранит базовые типы всех именованных типов пакета.
	Underlying map[string]ast.Expr
	// Resetters содержит имена типов пакета, у которых есть метод Reset().
	Resetters map[string]bool
	// Handwritten содержит имена типов с методом Reset(), написанным вручную.
	Handwritten map[string]bool
	// Dir — каталог пакета.
	Dir string
	// Name — имя пакета.
	Name string
	// Marked — структуры, помеченные маркером, в порядке объявления.
	Marked []markedStruct
	// Warnings — сообщения о пропущенных объявлениях.
	Warnings []string
}

// collectPackage разбирает пакет в каталоге dir и находит структуры,
// помеченные комментарием marker. Файл output и тесты не разбираются.
// Если каталог не содержит исходников Go, возвращается nil.
func collectPackage(dir, marker, output string) (*packageInfo, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("чтение каталога: %w", err)
	}

	pkg := &packageInfo{
		Dir:         dir,
		Underlying:  make(map[string]ast.Expr),
		Resetters:   make(map[string]bool),
		Handwritten: make(map[string]bool),
	}
	fset := token.NewFileSet()
	found := false

	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") ||
			strings.HasSuffix(name, "_test.go") || name == output {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parser.ParseComments|parser.SkipObjectResolution)
		if err != nil {
			return nil, fmt.Errorf("разбор %s: %w", name, err)
		}
		if !found {
			pkg.Name = file.Name.Name
			found = true
		} else if pkg.Name != file.Name.Name {
			// В каталоге лежит второй пакет (например, x_test): пропускаем его.
			continue
		}
		pkg.collectFile(file, marker)
	}

	if !found {
		return nil, nil
	}
	pkg.dropHandwritten()
	return pkg, nil
}

// dropHandwritten исключает из генерации структуры, для которых метод Reset()
// уже написан вручную, чтобы не создавать повторное объявление.
func (p *packageInfo) dropHandwritten() {
	kept := p.Marked[:0]
	for _, ms := range p.Marked {
		if p.Handwritten[ms.Name] {
			p.Warnings = append(p.Warnings, fmt.Sprintf("тип %s пропущен: метод Reset() уже объявлен вручную", ms.Name))
			continue
		}
		kept = append(kept, ms)
	}
	p.Marked = kept
}

// collectFile собирает из файла объявления типов, методы Reset() и помеченные структуры.
func (p *packageInfo) collectFile(file *ast.File, marker string) {
	for _, decl := range file.Decls {
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if name, ok := resetReceiver(d); ok {
				p.Resetters[name] = true
				p.Handwritten[name] = true
			}
		case *ast.GenDecl:
			if d.Tok != token.TYPE {
				continue
			}
			for _, spec := range d.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				p.Underlying[ts.Name.Name] = ts.Type
				// Маркер может стоять как над отдельным типом внутри блока type (...),
				// так и над самим объявлением из одного типа.
				marked := hasMarker(ts.Doc, marker) || (len(d.Specs) == 1 && hasMarker(d.Doc, marker))
				if marked {
					p.addMarked(ts)
				}
			}
		}
	}
}

// addMarked добавляет помеченный тип в список генерации либо фиксирует предупреждение,
// если тип не поддерживается.
func (p *packageInfo) addMarked(ts *ast.TypeSpec) {
	st, ok := ts.Type.(*ast.StructType)
	if !ok {
		p.Warnings = append(p.Warnings, fmt.Sprintf("тип %s помечен маркером, но не является структурой", ts.Name.Name))
		return
	}
	if ts.Assign.IsValid() {
		p.Warnings = append(p.Warnings, fmt.Sprintf("тип %s помечен маркером, но является псевдонимом типа", ts.Name.Name))
		return
	}
	if ts.TypeParams != nil {
		p.Warnings = append(p.Warnings, fmt.Sprintf("тип %s помечен маркером, но обобщённые типы не поддерживаются", ts.Name.Name))
		return
	}
	p.Marked = append(p.Marked, markedStruct{Name: ts.Name.Name, Type: st})
	p.Resetters[ts.Name.Name] = true
}

// resetReceiver возвращает имя типа-получателя, если объявление является методом
// Reset() без параметров и результатов.
func resetReceiver(d *ast.FuncDecl) (string, bool) {
	if d.Name == nil || d.Name.Name != "Reset" || d.Recv == nil || len(d.Recv.List) != 1 {
		return "", false
	}
	if d.Type.Params != nil && len(d.Type.Params.List) > 0 {
		return "", false
	}
	if d.Type.Results != nil && len(d.Type.Results.List) > 0 {
		return "", false
	}
	return typeName(d.Recv.List[0].Type)
}

// typeName возвращает имя типа, отбрасывая указатели и параметры обобщённого типа.
func typeName(expr ast.Expr) (string, bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name, true
	case *ast.StarExpr:
		return typeName(t.X)
	case *ast.ParenExpr:
		return typeName(t.X)
	case *ast.SelectorExpr:
		return t.Sel.Name, true
	case *ast.IndexExpr:
		return typeName(t.X)
	case *ast.IndexListExpr:
		return typeName(t.X)
	}
	return "", false
}

// hasMarker сообщает, содержит ли группа комментариев отдельную строку с маркером.
func hasMarker(doc *ast.CommentGroup, marker string) bool {
	if doc == nil {
		return false
	}
	for _, comment := range doc.List {
		text := comment.Text
		switch {
		case strings.HasPrefix(text, "//"):
			text = text[2:]
		case strings.HasPrefix(text, "/*"):
			text = strings.TrimSuffix(text[2:], "*/")
		}
		for _, line := range strings.Split(text, "\n") {
			if strings.TrimSpace(line) == marker {
				return true
			}
		}
	}
	return false
}
