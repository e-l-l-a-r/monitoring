// Package main предоставляет утилиту reset, генерирующую методы Reset()
// для структур, помеченных комментарием // generate:reset.
//
// Утилита рекурсивно обходит каталоги, начиная с корневого (флаг -dir),
// разбирает исходный код при помощи go/ast и для каждого пакета, в котором
// найдены помеченные структуры, создаёт файл reset.gen.go с методами Reset().
package main

import (
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

const (
	// resetMarker — комментарий-маркер, которым помечаются структуры.
	resetMarker = "generate:reset"
	// outputFileName — имя файла, в который записываются сгенерированные методы.
	outputFileName = "reset.gen.go"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "reset: %v\n", err)
	}
}

// reporter пишет диагностику и запоминает первую ошибку записи.
type reporter struct {
	w   io.Writer
	err error
}

// printf выводит сообщение; после первой ошибки запись прекращается.
func (r *reporter) printf(format string, args ...any) {
	if r.err != nil {
		return
	}
	_, r.err = fmt.Fprintf(r.w, format, args...)
}

// run разбирает аргументы командной строки, обходит каталоги и генерирует
// файлы с методами Reset(). Диагностика пишется в out.
func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("reset", flag.ContinueOnError)
	flags.SetOutput(out)
	root := flags.String("dir", ".", "корневой каталог, с которого начинается сканирование")
	if err := flags.Parse(args); err != nil {
		return err
	}

	dirs, err := goDirs(*root)
	if err != nil {
		return err
	}

	report := &reporter{w: out}

	for _, dir := range dirs {
		pkg, err := collectPackage(dir, resetMarker, outputFileName)
		if err != nil {
			return fmt.Errorf("пакет %s: %w", dir, err)
		}
		if pkg == nil {
			continue
		}
		for _, warning := range pkg.Warnings {
			report.printf("%s: предупреждение: %s\n", dir, warning)
		}
		if len(pkg.Marked) == 0 {
			continue
		}
		src, err := generatePackage(pkg)
		if err != nil {
			return fmt.Errorf("пакет %s: %w", dir, err)
		}
		path := filepath.Join(dir, outputFileName)
		if err := os.WriteFile(path, src, 0o644); err != nil {
			return fmt.Errorf("запись %s: %w", path, err)
		}
		report.printf("%s: сгенерировано методов — %d\n", path, len(pkg.Marked))
	}
	if report.err != nil {
		return fmt.Errorf("вывод диагностики: %w", report.err)
	}
	return nil
}

// goDirs возвращает список каталогов дерева root в лексикографическом порядке.
func goDirs(root string) ([]string, error) {
	var dirs []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		dirs = append(dirs, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("обход каталога %s: %w", root, err)
	}
	return dirs, nil
}
