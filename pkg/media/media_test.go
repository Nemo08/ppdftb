package media

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeImage создаёт файл с произвольным содержимым (картинкой не является,
// но LoadMedia работает только с байтами, валидность PNG не проверяет).
func writeImage(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

// mustJSON сериализует значение в JSON (для безопасной вставки путей Windows).
func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func collectStatic(m *MediaLoader) map[string][]byte {
	out := make(map[string][]byte)
	m.RangeStatic(func(k string, v []byte) bool {
		out[k] = v
		return true
	})
	return out
}

func collectMapped(m *MediaLoader) map[string][]byte {
	out := make(map[string][]byte)
	m.RangeMapped(func(k string, v []byte) bool {
		out[k] = v
		return true
	})
	return out
}

func TestLoadMediaEmpty(t *testing.T) {
	m := LoadMedia("", nil)
	if len(collectStatic(m)) != 0 {
		t.Error("при пустой папке Static должна быть пустой")
	}
	if len(collectMapped(m)) != 0 {
		t.Error("при пустых данных Mapped должна быть пустой")
	}
}

func TestLoadMediaStaticFromDir(t *testing.T) {
	dir := t.TempDir()
	pics := []struct {
		name    string
		content []byte
	}{
		{"a.png", []byte("png-a")},
		{"b.jpg", []byte("jpg-b")},
		{"c.jpeg", []byte("jpeg-c")},
		{"d.txt", []byte("txt-d")},   // не картинка — должна игнорироваться
		{"e.DOCX", []byte("docx-e")}, // чужое расширение — игнор
	}
	for _, p := range pics {
		writeImage(t, filepath.Join(dir, p.name), p.content)
	}
	// Подпапка не сканируется.
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeImage(t, filepath.Join(dir, "sub", "inner.png"), []byte("png-inner"))

	m := LoadMedia(dir, nil)
	st := collectStatic(m)

	if len(st) != 3 {
		t.Fatalf("Static: ожидалось 3 картинки, получено %d (%v)", len(st), keysOf(st))
	}
	for name, want := range map[string]string{"a.png": "png-a", "b.jpg": "jpg-b", "c.jpeg": "jpeg-c"} {
		got, ok := st[name]
		if !ok {
			t.Errorf("Static: картинка %s не загружена", name)
			continue
		}
		if string(got) != want {
			t.Errorf("Static[%s] = %q, want %q", name, got, want)
		}
	}
	if len(collectMapped(m)) != 0 {
		t.Error("без данных Mapped должна быть пустой")
	}
}

func TestLoadStaticImagesNonexistentDir(t *testing.T) {
	m := LoadMedia(filepath.Join(t.TempDir(), "no-such-dir"), nil)
	if len(collectStatic(m)) != 0 {
		t.Error("несуществующая папка не должна давать Static-картинок")
	}
}

func TestLoadMediaMappedFromData(t *testing.T) {
	dir := t.TempDir()
	sig := filepath.Join(dir, "sig.png")
	writeImage(t, sig, []byte("sig-content"))

	data := mustJSON(t, map[string]any{
		"stamp": map[string]any{
			"develop": sig,
			"checked": "replaced/nowhere.png",
		},
	})
	m := LoadMedia(dir, data)

	mp := collectMapped(m)
	if got := string(mp["sig.png"]); got != "sig-content" {
		t.Errorf(`Mapped["sig.png"] = %q, want "sig-content"`, got)
	}
	if got := string(mp[sig]); got != "sig-content" {
		t.Errorf("Mapped[полный путь] = %q, want содержимое sig.png", got)
	}
	// Нормализованный (слеши) ключ тоже должен присутствовать.
	norm := strings.ReplaceAll(sig, `\`, "/")
	if got := string(mp[norm]); got != "sig-content" {
		t.Errorf("Mapped[нормализованный] = %q, want содержимое sig.png", got)
	}
	// Отсутствующий файл не порождает записи.
	if _, ok := mp["nowhere.png"]; ok {
		t.Error("Mapped не должна содержать отсутствующий файл")
	}
}

func TestLoadMediaMappedWindowsBackslash(t *testing.T) {
	dir := t.TempDir()
	writeImage(t, filepath.Join(dir, "win.png"), []byte("win-content"))

	// Путь с обратными слешами (как из XML-данных).
	data := mustJSON(t, map[string]any{"withBackslash": dir + `\win.png`})
	m := LoadMedia(dir, data)

	mp := collectMapped(m)
	if got := string(mp["win.png"]); got != "win-content" {
		t.Errorf("Mapped[win.png] = %q, want win-content (путь с обратными слешами)", got)
	}
}

func TestLoadMediaMappedStillDataPriority(t *testing.T) {
	dir := t.TempDir()
	writeImage(t, filepath.Join(dir, "shared.png"), []byte("from-pics"))

	// Файл с тем же именем есть в Static (загружен из picsDir),
	// mapped-загрузчик должен переиспользовать Static без повторного ReadFile.
	data := mustJSON(t, map[string]any{"stamp": map[string]any{"podp": dir + `\shared.png`}})
	m := LoadMedia(dir, data)

	mp := collectMapped(m)
	if got := string(mp["shared.png"]); got != "from-pics" {
		t.Errorf("Mapped[shared.png] = %q, want from-pics (переиспользование Static)", got)
	}
	if len(mp) != 3 {
		t.Errorf("Mapped: ожидалось 3 ключа (base/raw/normalized), получено %d", len(mp))
	}
}

func TestLoadMediaMappedInvalidJSON(t *testing.T) {
	// Некорректный JSON не должен паниковать и давать пустую Mapped.
	m := LoadMedia("", []byte("{not-json"))
	if len(collectMapped(m)) != 0 {
		t.Error("битый JSON не должен заполнять Mapped")
	}
}

func TestLoadMediaMappedNestedAndArrays(t *testing.T) {
	dir := t.TempDir()
	writeImage(t, filepath.Join(dir, "one.png"), []byte("one"))
	writeImage(t, filepath.Join(dir, "two.png"), []byte("two"))

	data := mustJSON(t, map[string]any{"deep": map[string]any{
		"arr": []string{filepath.Join(dir, "one.png"), filepath.Join(dir, "two.png")},
	}})
	m := LoadMedia(dir, data)

	mp := collectMapped(m)
	if got := string(mp["one.png"]); got != "one" {
		t.Errorf("Mapped[one.png] = %q, want one", got)
	}
	if got := string(mp["two.png"]); got != "two" {
		t.Errorf("Mapped[two.png] = %q, want two", got)
	}
}

func TestLoadMediaIgnoresNonImagePaths(t *testing.T) {
	data := mustJSON(t, map[string]any{
		"text":  "README.txt",
		"noExt": `C:\temp\archive`,
		"img":   "x.png",
	})
	m := LoadMedia("", data)
	// x.png не существует — mapped остаётся пустой, без паники и крикливого лога.
	if len(collectMapped(m)) != 0 {
		t.Error("пути без расширений картинок не должны обрабатываться")
	}
}

// keysOf возвращает отсортированные ключи карты (для стабильных сообщений теста).
func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

func TestRangeFunctions(t *testing.T) {
	dir := t.TempDir()
	writeImage(t, filepath.Join(dir, "a.png"), []byte("a"))
	writeImage(t, filepath.Join(dir, "b.png"), []byte("b"))
	m := LoadMedia(dir, nil)

	// RangeStatic с ранним выходом (false).
	count := 0
	m.RangeStatic(func(_ string, _ []byte) bool {
		count++
		return false
	})
	if count != 1 {
		t.Errorf("RangeStatic: ранний выход остановился после %d вызовов, want 1", count)
	}

	// RangeStatic обходит все записи при true.
	seen := map[string]bool{}
	m.RangeStatic(func(k string, _ []byte) bool {
		seen[k] = true
		return true
	})
	for _, name := range []string{"a.png", "b.png"} {
		if !seen[name] {
			t.Errorf("RangeStatic не обошёл %s", name)
		}
	}

	// RangeMapped на пустых данных — без вызовов.
	calls := 0
	m.RangeMapped(func(_ string, _ []byte) bool {
		calls++
		return true
	})
	if calls != 0 {
		t.Errorf("RangeMapped: на пустой Mapped вызовов %d, want 0", calls)
	}
}

func TestLoadMediaStaticContentIsSlices(t *testing.T) {
	// Проверяем, что байты картинок не мутируют между Range-обходами
	// (общее содержимое, одинаковое для Static и Mapped при переиспользовании).
	dir := t.TempDir()
	writeImage(t, filepath.Join(dir, "s.png"), bytes.Repeat([]byte{0xAB}, 32))

	data := mustJSON(t, map[string]any{"podp": filepath.Join(dir, "s.png")})
	m := LoadMedia(dir, data)

	want := bytes.Repeat([]byte{0xAB}, 32)
	for _, mm := range []map[string][]byte{collectStatic(m), collectMapped(m)} {
		for k, v := range mm {
			if !bytes.Equal(v, want) {
				t.Errorf("%s: содержимое = %x, want %x", k, v, want)
			}
		}
	}
}
