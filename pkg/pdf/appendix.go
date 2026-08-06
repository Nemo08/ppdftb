package pdf

import (
	"log/slog"
	"strings"

	"github.com/Nemo08/ppdftb/pkg/appendixutil"
)

// AppendixLetter возвращает букву приложения по порядковому номеру (0-based).
// Делегирует в appendixutil — там же тесты и константы ГОСТ.
func AppendixLetter(n int) string {
	return appendixutil.Letter(n)
}

// isMarker проверяет что имя файла содержит маркер.
func isMarker(name, marker string) bool {
	ok := appendixutil.IsMarker(name, marker)
	if ok {
		slog.Debug("marker matched", slog.String("name", name), slog.String("marker", marker))
	}
	return ok
}

// FileKind — тип файла в контексте сборки тома.
type FileKind int

// Константы типа файла в контексте сборки тома.
const (
	KindNormal   FileKind = iota // обычный документ
	KindDivider                  // файл-разделитель (без расширения)
	KindAppendix                 // приложение (между маркерами)
)

// FileEntry — описание одного файла в папке PDF.
type FileEntry struct {
	FullPath  string // абсолютный путь (пустой для заглушек)
	Name      string // имя без расширения и без порядкового номера
	RawName   string // имя файла как есть (без расширения)
	Kind      FileKind
	Letter    string // буква приложения (только для KindAppendix)
	BookTitle string // заголовок для закладки PDF
}

// CollectEntries сканирует папку и возвращает упорядоченный список FileEntry.
// Если appendix=false — заглушки и приложения не распознаются, поведение как раньше.
func CollectEntries(dir string, appendix bool) ([]FileEntry, error) {
	raw, err := collectAllFiles(dir, appendix)
	if err != nil {
		return nil, err
	}
	return assignAppendixLetters(raw), nil
}

func collectAllFiles(dir string, appendix bool) ([]FileEntry, error) {
	entries, err := readDirNaturalSort(dir)
	if err != nil {
		return nil, err
	}

	var result []FileEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(extOf(e.Name()))
		base := stripExt(e.Name())
		cleanName := cleanFileName(base)

		slog.Debug("collectAllFiles",
			slog.String("file", e.Name()),
			slog.String("ext", ext),
			slog.String("base", base),
			slog.String("clean", cleanName),
			slog.Bool("appendix", appendix))

		switch ext {
		case ".pdf":
			result = append(result, FileEntry{
				FullPath: joinPath(dir, e.Name()),
				Name:     cleanName,
				RawName:  base,
				Kind:     KindNormal,
			})
		case "":
			if appendix {
				slog.Debug("detected divider", slog.String("name", e.Name()))
				result = append(result, FileEntry{
					FullPath: "",
					Name:     cleanName,
					RawName:  base,
					Kind:     KindDivider,
				})
			}
		default:
			slog.Debug("skipped file", slog.String("name", e.Name()))
		}
	}
	slog.Debug("collectAllFiles done", slog.Int("entries", len(result)))
	return result, nil
}

// assignAppendixLetters расставляет буквы приложений и заполняет BookTitle.
func assignAppendixLetters(entries []FileEntry) []FileEntry {
	inAppendix := false
	letterIdx := 0

	for i := range entries {
		e := &entries[i]
		switch e.Kind {
		case KindDivider:
			switch {
			case isMarker(e.RawName, appendixutil.MarkerAppendixBegin):
				slog.Debug("entering appendix section", slog.String("name", e.RawName))
				inAppendix = true
			case isMarker(e.RawName, appendixutil.MarkerAppendixEnd):
				slog.Debug("exiting appendix section", slog.String("name", e.RawName))
				inAppendix = false
			default:
				slog.Debug("divider not matching markers",
					slog.String("name", e.RawName),
					slog.String("beginMarker", appendixutil.MarkerAppendixBegin),
					slog.String("endMarker", appendixutil.MarkerAppendixEnd))
			}
			e.BookTitle = strings.ToUpper(e.Name)

		case KindNormal:
			if inAppendix {
				e.Kind = KindAppendix
				e.Letter = AppendixLetter(letterIdx)
				letterIdx++
				e.BookTitle = "Приложение " + e.Letter + ". " + e.Name
				slog.Debug("appendix entry",
					slog.String("name", e.Name),
					slog.String("letter", e.Letter))
			} else {
				e.BookTitle = e.Name
				slog.Debug("normal entry", slog.String("name", e.Name))
			}

		case KindAppendix:
			// Уже размеченное приложение: повторный вызов не сбрасывает Letter и BookTitle.
			slog.Debug("appendix entry already assigned",
				slog.String("name", e.Name),
				slog.String("letter", e.Letter))
		}
	}
	slog.Debug("assignAppendixLetters done", slog.Int("appendixCount", letterIdx))
	return entries
}
