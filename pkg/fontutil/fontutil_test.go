package fontutil

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	model "github.com/oliverpool/unipdf/v3/model"
)

func TestLogicalFor(t *testing.T) {
	cases := map[string]string{
		"TimesNewRomanPSMT":      "TimesNewRomanPSMT",
		"TimesNewRomanPS-BoldMT": "TimesNewRomanPS-BoldMT",
		"Helvetica":              "Helvetica",
		"DECAFE+SomeFont-Bold":   "SomeFont-Bold",
		"AB12C+MyFont-Regular":   "MyFont-Regular",
		"":                       "",
	}
	for base, want := range cases {
		if got := logicalFor(base); got != want {
			t.Errorf("logicalFor(%q) = %q, want %q", base, got, want)
		}
	}
}

func TestFontNorm(t *testing.T) {
	cases := map[string]string{
		"XO_Thames_Bu.ttf": "xothamesbuttf",
		"XOThames-Bold":    "xothamesbold",
		"Times  New!":      "timesnew",
	}
	for in, want := range cases {
		if got := fontNorm(in); got != want {
			t.Errorf("fontNorm(%q) = %q, want %q", in, got, want)
		}
	}
}

// pickSystemFont возвращает путь к любому системному TrueType или пропускает
// тест, если шрифтов на машине нет (CI без них).
func pickSystemFont(t *testing.T) string {
	t.Helper()
	for _, d := range systemFontDirs() {
		ents, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		for _, ent := range ents {
			if ent.IsDir() {
				continue
			}
			low := strings.ToLower(ent.Name())
			if strings.HasSuffix(low, ".ttf") || strings.HasSuffix(low, ".otf") {
				return filepath.Join(d, ent.Name())
			}
		}
	}
	t.Skip("системные TrueType-шрифты не найдены")
	return ""
}

func TestSystemFontDirs(t *testing.T) {
	t.Logf("каталогов шрифтов: %d", len(systemFontDirs()))
}

func TestCoveredByCompositeFont(t *testing.T) {
	ttf := pickSystemFont(t)
	fn, err := model.NewCompositePdfFontFromTTFFile(ttf)
	if err != nil {
		t.Fatalf("NewCompositePdfFontFromTTFFile(%s): %v", ttf, err)
	}
	if !isComposite(fn) {
		t.Error("isComposite(fn) = false, want true")
	}
	// Внедрённый полный TTF даёт cmap со всеми глифами Тима и кириллицы —
	// составной шрифт покрывает оба текста. Пропуск, если у шрифта нет
	// FontFile2 и parseFontCmap вернул nil — тогда покрытия нет.
	if cov := fontCmap(fn); cov == nil {
		t.Logf("пропуск: внедрённой программы шрифта нет (%s)", ttf)
		return
	}
	for _, s := range []string{"Hello", "Привет"} {
		if !coveredBy(fn, s) {
			t.Errorf("coveredBy(%q) = false, want true (все руны есть в cmap)", s)
		}
	}
}

func TestCoveredBySimpleFont(t *testing.T) {
	// WinAnsi-шрифт (Times-Roman стандарт 14) — как в плейсхолдер-ранах PDF:
	// ASCII покрыт, кириллица — нет.
	fn, err := model.NewStandard14Font(model.TimesRomanName)
	if err != nil {
		t.Fatalf("NewStandard14Font: %v", err)
	}
	if !coveredBy(fn, "Hello 123") {
		t.Error("coveredBy(\"Hello 123\") = false, want true")
	}
	if coveredBy(fn, "Привет") {
		t.Error("coveredBy(\"Привет\") = true, want false (WinAnsi не знает кириллицу)")
	}
	if isComposite(fn) {
		t.Error("isComposite(Times-Roman) = true, want false")
	}
}

func TestIsSubsetFontName(t *testing.T) {
	cases := map[string]bool{
		"":                        false,
		"TimesNewRomanPSMT":       false,
		"BCDEEE+XOThames-Regular": true,
		"BCDFEE+XOThames-Bold":    true,
		"ABC+Times-Roman":         true,
	}
	for base, want := range cases {
		if got := isSubsetFontName(base); got != want {
			t.Errorf("isSubsetFontName(%q) = %v, want %v", base, got, want)
		}
	}
}

func TestEmbeddedCodecThroughManager(t *testing.T) {
	m := NewFontManager()
	_, embed, err := m.FontForLogical("TimesNewRomanPSMT", "Привет")
	if err != nil {
		t.Skipf("системный шрифт не найден: %v", err)
	}

	orig, err := model.NewStandard14Font(model.TimesRomanName)
	if err != nil {
		t.Fatalf("orig font: %v", err)
	}
	c := &embeddedCodec{orig: orig, embed: embed, fam: "TimesNewRomanPSMT", m: m, key: "/TF1"}

	if !c.Hex() {
		t.Error("Hex() = false, want true (Identity-H)")
	}
	enc := c.Encode("Привет")
	if enc == "" {
		t.Error("Encode(\"Привет\") пустой")
	}
	t.Logf("Encode(\"Привет\") = %d байт CIDs", len(enc))

	// Width() — метрики ИСХОДНОГО шрифта; применимы к плейсхолдер-тексту (ASCII).
	if w := c.Width("Name"); w <= 0 {
		t.Errorf("Width(\"Name\") = %g, want > 0", w)
	}
	if w := c.NewWidth("Привет"); w <= 0 {
		t.Errorf("NewWidth(\"Привет\") = %g, want > 0", w)
	}
	if c.FontName() != "/TF1" {
		t.Errorf("FontName() = %q, want /TF1", c.FontName())
	}
}

// clearFontCaches сбрасывает все уровни кэша Resolve (текст, семейство, файл).
// Кэши глобальные для прогона, поэтому без сброса тесты с подменёнными
// каталогами зависели бы от результатов предыдущих тестов.
func clearFontCaches() {
	clear := func(m *sync.Map) {
		m.Range(func(k, _ any) bool {
			m.Delete(k)
			return true
		})
	}
	clear(&resolveCache)
	clear(&familyCache)
	clear(&fontFileCache)
}

// withFontDirs временно подменяет системные каталоги шрифтов на заданные и
// восстанавливает прежние по завершении теста (обычно — автоопределение).
// Кэши Resolve сбрасываются: тесты проверяют поведение при заданных каталогах,
// а не унаследованные от прошлых тестов результаты подбора.
func withFontDirs(t *testing.T, dirs []string) {
	t.Helper()
	clearFontCaches()
	prev := fontDirsOverride
	fontDirsOverride = dirs
	t.Cleanup(func() { fontDirsOverride = prev })
}

// pickLoadableFont возвращает путь и BaseFont системного TrueType из короткого
// списка вероятных системных имён (Windows/Linux). Перебор же ВСЕХ шрифтов с
// дорогой загрузкой unipdf (NewCompositePdfFontFromTTFFile сжимает файл) на
// большом каталоге Windows слишком медленный. bold=true требует жирного файла.
// Пропускает тест, если ни один не загрузился.
func pickLoadableFont(t *testing.T, bold bool) (src, base string) {
	t.Helper()
	known := []struct {
		name string
		bold bool
	}{
		{"arial.ttf", false},
		{"arialbd.ttf", true},
		{"times.ttf", false},
		{"timesbd.ttf", true},
		{"cour.ttf", false},
		{"courbd.ttf", true},
		{"segoeui.ttf", false},
		{"segoeuib.ttf", true},
		{"calibri.ttf", false},
		{"DejaVuSans.ttf", false},
		{"DejaVuSans-Bold.ttf", true},
		{"LiberationSans-Regular.ttf", false},
		{"LiberationSans-Bold.ttf", true},
	}
	for _, k := range known {
		if k.bold != bold {
			continue
		}
		for _, d := range systemFontDirs() {
			p := filepath.Join(d, k.name)
			f, err := model.NewCompositePdfFontFromTTFFile(p)
			if err != nil || f.BaseFont() == "" {
				continue
			}
			return p, f.BaseFont()
		}
	}
	t.Skip("подходящий системный TrueType не найден")
	return "", ""
}

// writeFontCopy копирует файл шрифта src в dst (копии системных шрифтов во
// временных каталогах детерминируют тесты подбора).
func writeFontCopy(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("ReadFile(%s): %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		t.Fatalf("WriteFile(%s): %v", dst, err)
	}
}

func TestFindFontByNameExactMatch(t *testing.T) {
	src, base := pickLoadableFont(t, false)
	dir := t.TempDir()
	// Имя копии = логическое имя шрифта: в подменённом каталоге кандидат один,
	// и подбор обязан вернуть именно его.
	dst := filepath.Join(dir, logicalFor(base)+filepath.Ext(src))
	writeFontCopy(t, src, dst)
	withFontDirs(t, []string{dir})

	if got := findFontByName(logicalFor(base)); got != dst {
		t.Errorf("findFontByName(%q) = %q, want %q", logicalFor(base), got, dst)
	}
}

func TestFindFontByNameCloudDir(t *testing.T) {
	src, base := pickLoadableFont(t, false)
	fam := fontFamilyOf(base)
	dir := t.TempDir()
	// Каталог-семейство (как Aptos в CloudFonts) с числовым именем файла —
	// подсказки в имени файла нет, решает имя подкаталога и BaseFont.
	sub := filepath.Join(dir, fam)
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(sub, "123456"+filepath.Ext(src))
	writeFontCopy(t, src, dst)
	withFontDirs(t, []string{dir})

	if got := findFontByName(fam); got != dst {
		t.Errorf("findFontByName(%q) = %q, want %q (cloud-каталог)", fam, got, dst)
	}
}

func TestFindFontByNameBoldName(t *testing.T) {
	src, base := pickLoadableFont(t, true)
	if !fontBoldish(base) {
		t.Skip("системный жирный TrueType не найден")
	}
	fam := fontFamilyOf(base)
	dir := t.TempDir()
	// Имя файла с «bd»-суффиксом (нестандартное сокращение) + жирное логическое
	// имя: семейство совпадает через patch-подсказку и вес — через токен fontBoldish.
	dst := filepath.Join(dir, fam+"bd"+filepath.Ext(src))
	writeFontCopy(t, src, dst)
	withFontDirs(t, []string{dir})

	logical := fam + "-BoldMT"
	if got := findFontByName(logical); got != dst {
		t.Errorf("findFontByName(%q) = %q, want %q", logical, got, dst)
	}
}

func TestFindFontByNameMissing(t *testing.T) {
	withFontDirs(t, []string{t.TempDir()})

	if got := findFontByName(""); got != "" {
		t.Errorf("findFontByName(\"\") = %q, want пусто", got)
	}
	if got := findFontByName("SuchFontDoesNotExist"); got != "" {
		t.Errorf("findFontByName() = %q, want пусто (шрифта нет)", got)
	}
}

func TestResolveEmptyFamily(t *testing.T) {
	if _, _, err := Resolve("", "12"); err == nil {
		t.Error("Resolve(\"\", ...) = nil err, want ошибка пустого имени")
	}
}

// TestResolveCoversCyrillic — основной контракт Resolve для нумерации:
// запрошенный шрифт покрывает кириллицу, ошибки нет.
func TestResolveCoversCyrillic(t *testing.T) {
	src, base := pickLoadableFont(t, false)
	withFontDirs(t, []string{filepath.Dir(src)})

	const text = "Приложение А 12"
	f, file, err := Resolve(base, text)
	if err != nil {
		t.Fatalf("Resolve(%q, %q): %v", base, text, err)
	}
	if f == nil {
		t.Fatal("Resolve вернул nil-шрифт")
	}
	if file == "" {
		t.Error("Resolve не вернул путь к файлу-источнику")
	}
	if !fontCovers(f, text) {
		t.Errorf("шрифт %q не покрывает %q", f.BaseFont(), text)
	}
}

// TestResolveFallbackWhenFamilyAbsent — запрошенного семейства в каталоге нет:
// Resolve обязан найти любой покрывающий шрифт и НЕ вернуть ошибку.
func TestResolveFallbackWhenFamilyAbsent(t *testing.T) {
	src, _ := pickLoadableFont(t, false)
	withFontDirs(t, []string{filepath.Dir(src)})

	const text = "Приложение А 12"
	f, file, err := Resolve("NoSuchFontFamily-XYZ", text)
	if err != nil {
		t.Fatalf("Resolve с отсутствующим семейством вернул ошибку: %v", err)
	}
	if f == nil || file == "" {
		t.Fatal("Resolve не подобрал заменяющий шрифт")
	}
	if !fontCovers(f, text) {
		t.Errorf("заменяющий шрифт %q не покрывает %q", f.BaseFont(), text)
	}
}

// TestResolveNoCoveringFont — пустой каталог: покрыть текст нечем, ошибка
// обязательна (вызывающий код решает, фатально это или нет).
func TestResolveNoCoveringFont(t *testing.T) {
	withFontDirs(t, []string{t.TempDir()})

	if _, _, err := Resolve("Arial", "Приложение А 12"); err == nil {
		t.Error("Resolve в пустом каталоге = nil err, want ошибка")
	}
}

// TestResolveMemoizedByRuneSet — два вызова с одинаковым набором рун (порядок
// и повторы символов не важны) обязаны вернуть тот же экземпляр шрифта: без
// повторного сканирования системных каталогов.
func TestResolveMemoizedByRuneSet(t *testing.T) {
	src, base := pickLoadableFont(t, false)
	withFontDirs(t, []string{filepath.Dir(src)})

	f1, _, err := Resolve(base, "12")
	if err != nil {
		t.Skipf("системный шрифт не найден: %v", err)
	}
	f2, _, err := Resolve(base, "21")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if f1 != f2 {
		t.Error("Resolve не переиспользовал шрифт для одинакового набора рун")
	}
}

// TestResolveReusesLoadedFontFile — наборы рун разные (разные цифры номера
// страницы), но файл один: кэш загрузки TTF обязан вернуть тот же экземпляр,
// иначе каждая цифра номера читала бы и сжимала системный TTF заново.
func TestResolveReusesLoadedFontFile(t *testing.T) {
	src, base := pickLoadableFont(t, false)
	withFontDirs(t, []string{filepath.Dir(src)})

	f1, _, err := Resolve(base, "12")
	if err != nil {
		t.Skipf("системный шрифт не найден: %v", err)
	}
	f2, _, err := Resolve(base, "98")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if f1 != f2 {
		t.Error("Resolve перечитал TTF для другого набора рун вместо кэша по файлу")
	}
}

// TestResolveFamilyCacheSkipsDirScan — после первого удачного подбора семейства
// повторный вызов с ДРУГИМ набором рун (другой номер страницы) обязан работать
// без системных каталогов: каталог удаляется, а Resolve всё равно возвращает
// тот же экземпляр шрифта. Раньше каждый новый набор рун заново сканировал
// каталоги и разбирал файлы-кандидаты (у Arial их девять ≈ 8 МБ), из-за чего
// нумерация на томе стоила около 0.4 с на страницу.
func TestResolveFamilyCacheSkipsDirScan(t *testing.T) {
	src, base := pickLoadableFont(t, false)
	dir := t.TempDir()
	// Имя копии = логическое имя семейства, чтобы findFontByName нашёл её.
	dst := filepath.Join(dir, logicalFor(base)+filepath.Ext(src))
	writeFontCopy(t, src, dst)
	withFontDirs(t, []string{dir})

	f1, file1, err := Resolve(base, "12")
	if err != nil {
		t.Fatalf("Resolve(%q, 12): %v", base, err)
	}
	if file1 != dst {
		t.Fatalf("Resolve взял %q, want копию в тестовом каталоге %q", file1, dst)
	}

	if err := os.RemoveAll(dir); err != nil {
		t.Fatalf("RemoveAll(%s): %v", dir, err)
	}

	f2, _, err := Resolve(base, "98")
	if err != nil {
		t.Fatalf("Resolve после удаления каталога: %v (семейство должно отдаваться из кэша)", err)
	}
	if f1 != f2 {
		t.Error("Resolve не переиспользовал шрифт из кэша семейства")
	}
}
