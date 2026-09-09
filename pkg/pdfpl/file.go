package pdfpl

import (
	"context"
	"fmt"
	"io"
	"os"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/fileutil"
	core "github.com/oliverpool/unipdf/v3/core"
	model "github.com/oliverpool/unipdf/v3/model"
)

// FileResult — результат замены плейсхолдеров в PDF-файле.
type FileResult struct {
	Pages    int // обработано страниц
	Streams  int // модифицировано контент-потоков
	Replaced int // заменено плейсхолдеров
	Skipped  int // пропущено (ключ не найден / значение шире)
}

// ReplaceFile читает PDF из input, заменяет плейсхолдеры {{.Key}} во всех
// контентных потоках страниц значениями из values и пишет результат в output.
//
// Путь output может совпадать с input: запись выполняется атомарно через
// временный файл (fileutil.WriteFileAtomic), поэтому исходник не повреждается.
// Контентные потоки читаются декодированными, заменяются на уровне текста
// (однорановые и межрановые плейсхолдеры) и кодируются обратно unipdf.
func ReplaceFile(ctx context.Context, input, output string, values map[string]string) (*FileResult, error) {
	if input == "" || output == "" {
		return nil, fmt.Errorf("pdfpl: пути input/output не могут быть пустыми")
	}

	//nolint:gosec // чтение PDF по пути из параметров вызова — ожидаемое поведение
	f, err := os.Open(input)
	if err != nil {
		return nil, fmt.Errorf("pdfpl: открыть входной PDF %s: %w", input, err)
	}
	defer func() {
		if err := f.Close(); err != nil {
			slog.Warn("pdfpl: закрыть входной PDF", slog.String("file", input), slog.String("err", err.Error()))
		}
	}()

	reader, err := model.NewPdfReader(f)
	if err != nil {
		return nil, fmt.Errorf("pdfpl: разобрать PDF %s: %w", input, err)
	}

	numPages, err := reader.GetNumPages()
	if err != nil {
		return nil, fmt.Errorf("pdfpl: число страниц %s: %w", input, err)
	}

	res := &FileResult{Pages: numPages}

	// Менеджер внедряемых шрифтов: TrueType-шрифты загружаются лениво,
	// по одному на семейство, сабсетятся после обработки всех страниц.
	fm := newFontManager()

	// 1) Заменяем плейсхолдеры на страницах.
	for p := 1; p <= numPages; p++ {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		page, err := reader.GetPage(p)
		if err != nil {
			return nil, fmt.Errorf("pdfpl: страница %d: %w", p, err)
		}
		if err := replacePageStreams(page, values, fm, res); err != nil {
			return nil, fmt.Errorf("pdfpl: страница %d: %w", p, err)
		}
	}

	if res.Replaced == 0 && res.Streams == 0 {
		// Нечего менять — перезаписываем выходной файл исходником.
		return res, copyFileAtomic(output, input)
	}

	// 2) Сужаем внедрённые шрифты до использованных глифов и собираем PDF.
	if err := fm.subsetAll(); err != nil {
		return res, err
	}
	return res, writeResult(ctx, reader, output)
}

// replacePageStreams заменяет плейсхолдеры во всех контентных потоках страницы.
// Раны, значение которых активный шрифт не покрывает, переключаются на
// внедрённый TrueType-шрифт (fm + per-page контекст ресурсов).
func replacePageStreams(page *model.PdfPage, values map[string]string, fm *fontManager, res *FileResult) error {
	streams, err := page.GetContentStreams()
	if err != nil {
		return fmt.Errorf("чтение контентных потоков: %w", err)
	}

	pfc := newPageFontContext(fm, page)
	changed := false
	for i, s := range streams {
		// В файловом слое заменяем даже значения шире плейсхолдера: отображение
		// переноса строки остаётся на совести шаблона; пропуск при этом оставил бы
		// в готовом PDF видимые {{.Key}} подстановки.
		out, err := Replace(&ReplaceParams{Stream: s, Values: values, AllowWider: true, Codec: pfc.provider()})
		if err != nil {
			return fmt.Errorf("поток %d: %w", i, err)
		}
		if out.Stream == s {
			continue
		}
		streams[i] = out.Stream
		changed = true
		res.Streams++
		res.Replaced += out.Replaced
		res.Skipped += out.Skipped
	}

	if !changed {
		return nil
	}
	if err := page.SetContentStreams(streams, core.NewFlateEncoder()); err != nil {
		return fmt.Errorf("запись контентных потоков: %w", err)
	}
	return nil
}

// writeResult собирает модифицированные страницы в новый PDF и пишет атомарно.
func writeResult(ctx context.Context, reader *model.PdfReader, output string) error {
	n, err := reader.GetNumPages()
	if err != nil {
		return fmt.Errorf("число страниц: %w", err)
	}
	pw := model.NewPdfWriter()
	for p := 1; p <= n; p++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		page, err := reader.GetPage(p)
		if err != nil {
			return fmt.Errorf("страница %d: %w", p, err)
		}
		if err := pw.AddPage(page); err != nil {
			return fmt.Errorf("добавить страницу %d: %w", p, err)
		}
	}
	return fileutil.WriteFileAtomic(output, func(tmpPath string) (retErr error) {
		//nolint:gosec // создание временного файла по служебному пути — ожидаемое поведение
		fo, err := os.Create(tmpPath)
		if err != nil {
			return err
		}
		defer func() {
			if err := fo.Close(); err != nil && retErr == nil {
				retErr = fmt.Errorf("закрыть выходной файл: %w", err)
			}
		}()
		return pw.Write(fo)
	})
}

// copyFileAtomic копирует input → output через временный файл (атомарно).
func copyFileAtomic(output, input string) error {
	return fileutil.WriteFileAtomic(output, func(tmpPath string) error {
		return copyFile(input, tmpPath)
	})
}

// copyFile копирует файл src в dst потоково (без загрузки в память).
func copyFile(src, dst string) (retErr error) {
	//nolint:gosec // копирование по явным параметрам вызова — ожидаемое поведение
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer func() {
		if err := s.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("закрыть источник: %w", err)
		}
	}()

	//nolint:gosec // создание результата по параметрам вызова — ожидаемое поведение
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		if err := d.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("закрыть результат: %w", err)
		}
	}()

	if _, err := io.Copy(d, s); err != nil {
		return err
	}
	return nil
}
