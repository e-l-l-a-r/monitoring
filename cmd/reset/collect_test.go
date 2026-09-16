package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parseData разбирает исходник и возвращает doc-комментарий первого объявления.
func parseData(t *testing.T, src string) *ast.CommentGroup {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "src.go", "package p\n"+src, parser.ParseComments|parser.SkipObjectResolution)
	require.NoError(t, err)
	require.NotEmpty(t, file.Decls)
	decl, ok := file.Decls[0].(*ast.GenDecl)
	require.True(t, ok)
	return decl.Doc
}

func TestHasMarker(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want bool
	}{
		{
			name: "маркер отдельной строкой",
			src:  "// generate:reset\ntype S struct{}",
			want: true,
		},
		{
			name: "маркер после описания",
			src:  "// S описывает данные.\n//\n// generate:reset\ntype S struct{}",
			want: true,
		},
		{
			name: "маркер в блочном комментарии",
			src:  "/*\ngenerate:reset\n*/\ntype S struct{}",
			want: true,
		},
		{
			name: "маркер как часть предложения",
			src:  "// используем generate:reset здесь\ntype S struct{}",
			want: false,
		},
		{
			name: "комментария нет",
			src:  "type S struct{}",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, hasMarker(parseData(t, tt.src), resetMarker))
		})
	}
}

func TestCollectPackage(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		wantMarked   []string
		wantWarnings int
	}{
		{
			name:       "структура с маркером",
			src:        "package p\n\n// generate:reset\ntype A struct{ i int }\n\ntype B struct{ i int }\n",
			wantMarked: []string{"A"},
		},
		{
			name:       "маркер внутри блока type",
			src:        "package p\n\ntype (\n\t// generate:reset\n\tA struct{ i int }\n\n\tB struct{ i int }\n)\n",
			wantMarked: []string{"A"},
		},
		{
			name:         "маркер над не структурой",
			src:          "package p\n\n// generate:reset\ntype A int\n",
			wantMarked:   nil,
			wantWarnings: 1,
		},
		{
			name:         "маркер над псевдонимом",
			src:          "package p\n\ntype B struct{ i int }\n\n// generate:reset\ntype A = B\n",
			wantMarked:   nil,
			wantWarnings: 1,
		},
		{
			name:         "маркер над обобщённым типом",
			src:          "package p\n\n// generate:reset\ntype A[T any] struct{ v T }\n",
			wantMarked:   nil,
			wantWarnings: 1,
		},
		{
			name:         "метод Reset уже написан вручную",
			src:          "package p\n\n// generate:reset\ntype A struct{ i int }\n\nfunc (a *A) Reset() { a.i = 0 }\n",
			wantMarked:   nil,
			wantWarnings: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(dir, "types.go"), []byte(tt.src), 0o600))

			pkg, err := collectPackage(dir, resetMarker, outputFileName)
			require.NoError(t, err)
			require.NotNil(t, pkg)

			var got []string
			for _, ms := range pkg.Marked {
				got = append(got, ms.Name)
			}
			assert.Equal(t, tt.wantMarked, got)
			assert.Len(t, pkg.Warnings, tt.wantWarnings)
		})
	}
}

func TestCollectPackageSkipsNonGoFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "README.md"), []byte("# нет кода"), 0o600))

	pkg, err := collectPackage(dir, resetMarker, outputFileName)
	require.NoError(t, err)
	assert.Nil(t, pkg, "каталог без исходников Go должен пропускаться")
}

func TestCollectPackageIgnoresTestsAndGeneratedFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "types.go"),
		[]byte("package p\n\n// generate:reset\ntype A struct{ i int }\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "types_test.go"),
		[]byte("package p\n\n// generate:reset\ntype T struct{ i int }\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, outputFileName),
		[]byte("package p\n\nfunc (a *A) Reset() {}\n"), 0o600))

	pkg, err := collectPackage(dir, resetMarker, outputFileName)
	require.NoError(t, err)
	require.NotNil(t, pkg)
	require.Len(t, pkg.Marked, 1)
	assert.Equal(t, "A", pkg.Marked[0].Name)
	assert.Empty(t, pkg.Warnings, "метод из reset.gen.go не должен считаться написанным вручную")
}

func TestCollectPackageInvalidSource(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "broken.go"), []byte("package p\nfunc ("), 0o600))

	_, err := collectPackage(dir, resetMarker, outputFileName)
	assert.Error(t, err)
}
