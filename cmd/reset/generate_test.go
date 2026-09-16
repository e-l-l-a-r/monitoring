package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// generateFromSource собирает пакет из одного исходного файла и возвращает
// сгенерированный код.
func generateFromSource(t *testing.T, src string) string {
	t.Helper()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "types.go"), []byte(src), 0o600))

	pkg, err := collectPackage(dir, resetMarker, outputFileName)
	require.NoError(t, err)
	require.NotNil(t, pkg)

	out, err := generatePackage(pkg)
	require.NoError(t, err)
	return string(out)
}

func TestGenerateFieldRules(t *testing.T) {
	tests := []struct {
		name  string
		field string
		want  []string
	}{
		{
			name:  "целое число",
			field: "i int",
			want:  []string{"a.i = 0"},
		},
		{
			name:  "строка",
			field: "s string",
			want:  []string{`a.s = ""`},
		},
		{
			name:  "логическое значение",
			field: "b bool",
			want:  []string{"a.b = false"},
		},
		{
			name:  "слайс обрезается, но не зануляется",
			field: "s []string",
			want:  []string{"a.s = a.s[:0]"},
		},
		{
			name:  "мапа очищается",
			field: "m map[string]int",
			want:  []string{"clear(a.m)"},
		},
		{
			name:  "массив очищается",
			field: "arr [4]int",
			want:  []string{"clear(a.arr[:])"},
		},
		{
			name:  "указатель на примитив проверяется на nil",
			field: "p *string",
			want:  []string{"if a.p != nil {", `(*a.p) = ""`},
		},
		{
			name:  "интерфейс обнуляется",
			field: "v any",
			want:  []string{"a.v = nil"},
		},
		{
			name:  "канал обнуляется",
			field: "ch chan int",
			want:  []string{"a.ch = nil"},
		},
		{
			name:  "функция обнуляется",
			field: "fn func() error",
			want:  []string{"a.fn = nil"},
		},
		{
			name:  "анонимная структура раскрывается по полям",
			field: "inner struct{ n int }",
			want:  []string{"a.inner.n = 0"},
		},
		{
			name:  "тип из другого пакета проверяется на наличие Reset",
			field: "ts time.Time",
			want:  []string{"if resetter, ok := any(&a.ts).(interface{ Reset() }); ok {", "resetter.Reset()"},
		},
		{
			name:  "поле с пустым идентификатором пропускается",
			field: "_ int",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			src := "package p\n\nimport \"time\"\n\nvar _ time.Time\n\n// generate:reset\ntype A struct {\n\t" + tt.field + "\n}\n"
			got := generateFromSource(t, src)

			assert.Contains(t, got, "func (a *A) Reset() {")
			assert.Contains(t, got, "if a == nil {")
			for _, want := range tt.want {
				assert.Contains(t, got, want)
			}
			if tt.want == nil {
				// В теле метода должны остаться только защита от nil и закрывающая скобка.
				body := got[strings.Index(got, "func (a *A) Reset() {"):]
				assert.NotContains(t, body, "a._")
			}
		})
	}
}

func TestGenerateNestedStructs(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
	}{
		{
			name: "вложенная структура с Reset вызывает метод",
			src: "package p\n\ntype Inner struct{ n int }\n\nfunc (i *Inner) Reset() { i.n = 0 }\n\n" +
				"// generate:reset\ntype A struct{ inner Inner }\n",
			want: []string{"a.inner.Reset()"},
		},
		{
			name: "вложенная структура без Reset раскрывается по полям",
			src:  "package p\n\ntype Inner struct {\n\tn int\n\titems []string\n}\n\n// generate:reset\ntype A struct{ inner Inner }\n",
			want: []string{"a.inner.n = 0", "a.inner.items = a.inner.items[:0]"},
		},
		{
			name: "указатель на структуру без Reset раскрывается через разыменование",
			src:  "package p\n\ntype Inner struct{ n int }\n\n// generate:reset\ntype A struct{ inner *Inner }\n",
			want: []string{"if a.inner != nil {", "(*a.inner).n = 0"},
		},
		{
			name: "указатель на структуру с Reset вызывает метод после проверки nil",
			src: "package p\n\ntype Inner struct{ n int }\n\nfunc (i *Inner) Reset() { i.n = 0 }\n\n" +
				"// generate:reset\ntype A struct{ inner *Inner }\n",
			want: []string{"if a.inner != nil {", "a.inner.Reset()"},
		},
		{
			name: "рекурсивный указатель на себя вызывает сгенерированный Reset",
			src:  "package p\n\n// generate:reset\ntype A struct{ child *A }\n",
			want: []string{"if a.child != nil {", "a.child.Reset()"},
		},
		{
			name: "рекурсивный тип без Reset не разворачивается бесконечно",
			src:  "package p\n\ntype Node struct {\n\tn int\n\tnext []Node\n}\n\n// generate:reset\ntype A struct{ node Node }\n",
			want: []string{"a.node.n = 0", "a.node.next = a.node.next[:0]"},
		},
		{
			name: "встроенное поле сбрасывается по имени типа",
			src:  "package p\n\ntype Inner struct{ n int }\n\n// generate:reset\ntype A struct{ Inner }\n",
			want: []string{"a.Inner.n = 0"},
		},
		{
			name: "именованный тип с базовым примитивом сбрасывается нулём",
			src:  "package p\n\ntype ID int\n\n// generate:reset\ntype A struct{ id ID }\n",
			want: []string{"a.id = 0"},
		},
		{
			name: "именованный тип с базовой мапой очищается",
			src:  "package p\n\ntype Set map[string]struct{}\n\n// generate:reset\ntype A struct{ set Set }\n",
			want: []string{"clear(a.set)"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateFromSource(t, tt.src)
			for _, want := range tt.want {
				assert.Contains(t, got, want)
			}
		})
	}
}

func TestGenerateHeaderAndPackage(t *testing.T) {
	got := generateFromSource(t, "package mypkg\n\n// generate:reset\ntype A struct{ i int }\n")

	assert.True(t, strings.HasPrefix(got, generatedHeader), "файл должен начинаться с шапки о генерации")
	assert.Contains(t, got, "package mypkg")
	assert.Contains(t, got, "// Reset сбрасывает состояние A к начальным значениям.")
}

func TestZeroValue(t *testing.T) {
	tests := []struct {
		name     string
		typeName string
		want     string
		ok       bool
	}{
		{name: "bool", typeName: "bool", want: "false", ok: true},
		{name: "string", typeName: "string", want: `""`, ok: true},
		{name: "int64", typeName: "int64", want: "0", ok: true},
		{name: "float64", typeName: "float64", want: "0", ok: true},
		{name: "error", typeName: "error", want: "nil", ok: true},
		{name: "пользовательский тип", typeName: "Metrics", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := zeroValue(tt.typeName)
			assert.Equal(t, tt.ok, ok)
			if tt.ok {
				assert.Equal(t, tt.want, got)
			}
		})
	}
}
