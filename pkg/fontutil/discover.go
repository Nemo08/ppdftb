// Package fontutil — поиск, подбор и внедрение TrueType-шрифтов для PDF.
//
// Плейсхолдер-раны в целевых PDF (MS Word) сидят в простых ASCII-шрифтах
// (TrueType simple, WinAnsi), которые физически не содержат кириллических
// глифов, либо в CID-сабсетах (XOThames, Identity-H), чьи CID — номера глифов
// мини-сабсета, а не Unicode. Вставить произвольное русское значение такими
// «байтами» невозможно. Поэтому ран, значение которого не покрывается
// активным шрифтом, переключается на внедрённый TrueType-шрифт (Times/Arial
// с системного диска): переписывается оператор Tf (/F1 → /TF1), текст вставляется
// гекс-литералом <...> двухбайтовых CID, ширина берётся из реальных метрик.
//
// Пакет решает три задачи:
//   - discover.go — системные каталоги и обход файлов шрифтов;
//   - match.go — сопоставление логического имени шрифта с файлом на диске;
//   - coverage.go — проверка покрытия рун глифами шрифта;
//   - embed.go — менеджер внедрённых шрифтов и их финализация (ToUnicode, /W);
//   - estimate.go — эвристическая оценка ширины строки;
//   - codec.go — интерфейс пофайлового кодирования для шаблонизатора.
package fontutil

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

// fontFileRef — файл шрифта и имя верхнеуровневой записи каталога (для
// приоритета «буквально bold» по имени файла либо подкаталога-семейства).
type fontFileRef struct {
	name string
	path string
}

// fontFilesInDir собирает файлы шрифтов каталога: верхнеуровневые и внутри
// подкаталогов-семейств cloud-шрифтов (в порядке обхода каталога).
func fontFilesInDir(d string) []fontFileRef {
	var files []fontFileRef
	ents, err := os.ReadDir(d)
	if err != nil {
		slog.Debug("fontutil: каталог шрифтов недоступен", slog.String("dir", d), slog.String("err", err.Error()))
		return nil
	}
	for _, ent := range ents {
		if ent.IsDir() {
			raw, err := os.ReadDir(filepath.Join(d, ent.Name()))
			if err != nil {
				slog.Debug("fontutil: подкаталог шрифтов недоступен", slog.String("dir", filepath.Join(d, ent.Name())), slog.String("err", err.Error()))
				continue
			}
			for _, se := range raw {
				if fontFile(se) {
					files = append(files, fontFileRef{name: ent.Name(), path: filepath.Join(d, ent.Name(), se.Name())})
				}
			}
			continue
		}
		if fontFile(ent) {
			files = append(files, fontFileRef{name: ent.Name(), path: filepath.Join(d, ent.Name())})
		}
	}
	return files
}

// fontFile — является ли запись каталога файлом шрифта (.ttf/.otf).
func fontFile(ent os.DirEntry) bool {
	if ent.IsDir() {
		return false
	}
	low := strings.ToLower(ent.Name())
	return strings.HasSuffix(low, ".ttf") || strings.HasSuffix(low, ".otf")
}

// fontDirsOverride — тестовая подмена системных каталогов шрифтов. Когда не nil,
// systemFontDirs возвращает его целиком: тесты изолируют подбор от реальных
// каталогов машины, наполняя временный каталог копиями системных шрифтов.
var fontDirsOverride []string

// systemFontDirs — системные каталоги TrueType-шрифтов для текущей платформы:
// Windows (WINDIR\Fonts и пер-пользовательские %LOCALAPPDATA%\...\Fonts), а
// также стандартные каталоги Linux (Liberation/msttcorefonts) для
// кросс-платформенной сборки.
func systemFontDirs() []string {
	if fontDirsOverride != nil {
		return fontDirsOverride
	}
	var dirs []string
	if wd := os.Getenv("WINDIR"); wd != "" {
		dirs = append(dirs, filepath.Join(wd, "Fonts"))
	}
	if sr := os.Getenv("SystemRoot"); sr != "" && sr != os.Getenv("WINDIR") {
		dirs = append(dirs, filepath.Join(sr, "Fonts"))
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		dirs = append(dirs, filepath.Join(local, "Microsoft", "Windows", "Fonts"))
		// Cloud-шрифты Microsoft 365 (Aptos и др.): каталог семейств FontCache,
		// файлы начертаний лежат внутри подкаталогов с числовыми именами.
		dirs = append(dirs, filepath.Join(local, "Microsoft", "FontCache", "4", "CloudFonts"))
	}
	// Каталоги шрифтов Microsoft Office (устанавливаются в папке программы,
	// а не в Windows\Fonts).
	for _, key := range []string{"ProgramFiles", "ProgramFiles(x86)"} {
		if pf := os.Getenv(key); pf != "" {
			dirs = append(dirs, filepath.Join(pf, "Microsoft Office", "Fonts"))
			dirs = append(dirs, filepath.Join(pf, "Microsoft Office", "root", "Fonts"))
		}
	}
	dirs = append(dirs,
		"/usr/share/fonts/truetype/liberation",
		"/usr/share/fonts/truetype/msttcorefonts",
		"/usr/share/fonts/truetype/dejavu",
		"/usr/share/fonts",
	)
	return dirs
}

// fontCandidates собирает пути файлов шрифтов каталога, чьё имя (файла либо
// подкаталога-семейства cloud-шрифтов) намекает на искомое семейство — полное
// имя или его первые 5 символов ("timesbd" отыщет "TimesNewRomanPS").
func fontCandidates(dir, lfam, hint string) []string {
	ents, err := os.ReadDir(dir)
	if err != nil {
		slog.Debug("fontutil: каталог шрифтов недоступен", slog.String("dir", dir), slog.String("err", err.Error()))
		return nil
	}
	var paths []string
	for _, ent := range ents {
		fn := fontNorm(ent.Name())
		if !strings.Contains(fn, lfam) && !strings.Contains(fn, hint) {
			continue
		}
		if ent.IsDir() {
			sub, err := os.ReadDir(filepath.Join(dir, ent.Name()))
			if err != nil {
				slog.Debug("fontutil: подкаталог шрифтов недоступен", slog.String("dir", filepath.Join(dir, ent.Name())), slog.String("err", err.Error()))
				continue
			}
			for _, s := range sub {
				if fontFile(s) {
					paths = append(paths, filepath.Join(dir, ent.Name(), s.Name()))
				}
			}
			continue
		}
		if fontFile(ent) {
			paths = append(paths, filepath.Join(dir, ent.Name()))
		}
	}
	return paths
}
