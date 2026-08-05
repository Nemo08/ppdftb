package cache

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCwdStability(t *testing.T) {
	// Сбрасываем cachedCwd от предыдущих тестов
	cachedCwd = ""

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Создаём файл
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Загружаем кэш — cachedCwd = dir
	c := make(Cache)
	c.Set(testFile, FileEntry{Size: 4})

	// Проверяем что ключ — относительный путь от dir
	entry, ok := c.Get(testFile)
	if !ok {
		t.Fatal("expected entry to exist after Set+Get in same CWD")
	}
	if entry.Size != 4 {
		t.Errorf("Size = %d, want 4", entry.Size)
	}

	// Сохраняем кэш
	if err := SaveCache(c); err != nil {
		t.Fatal(err)
	}

	// Переходим в другую директорию и загружаем
	otherDir := filepath.Join(dir, "other")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadCache()
	if err != nil {
		t.Fatal(err)
	}

	// cachedCwd = otherDir, поэтому Get с путём от dir не найдёт запись
	otherFile := filepath.Join(otherDir, "test.txt")
	if _, ok := loaded.Get(otherFile); ok {
		t.Error("Get should not find file that doesn't exist in otherDir")
	}

	// А если загрузить кэш из той же директории dir — всё найдётся
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	loaded2, err := LoadCache()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded2.Get(testFile); !ok {
		t.Error("LoadCache from original dir should find the entry")
	}
}

func TestCachedCwdResetOnNewLoad(t *testing.T) {
	// Сбрасываем cachedCwd от предыдущих тестов
	cachedCwd = ""

	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Первая загрузка — LoadCache устанавливает cachedCwd = dir
	_, err = LoadCache()
	if err != nil {
		t.Fatal(err)
	}

	if cachedCwd != dir {
		t.Errorf("cachedCwd = %q, want %q", cachedCwd, dir)
	}

	// Переходим в другую директорию
	otherDir := filepath.Join(dir, "sub")
	if err := os.MkdirAll(otherDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(otherDir); err != nil {
		t.Fatal(err)
	}

	// Вторая загрузка — cachedCwd должен обновиться
	_, err = LoadCache()
	if err != nil {
		t.Fatal(err)
	}

	if cachedCwd != otherDir {
		t.Errorf("cachedCwd after second LoadCache = %q, want %q", cachedCwd, otherDir)
	}
}

func TestToRelWithCachedCwd(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	// Захватываем CWD
	cachedCwd = dir

	// Абсолютный путь под CWD
	absPath := filepath.Join(dir, "sub", "file.txt")
	rel := toRel(absPath)
	want := filepath.Join("sub", "file.txt")
	if rel != want {
		t.Errorf("toRel(%q) = %q, want %q", absPath, rel, want)
	}

	// Абсолютный путь вне CWD — должен вернуть исходный
	outsidePath := filepath.Join("G:", "other", "file.txt")
	rel2 := toRel(outsidePath)
	if rel2 != outsidePath {
		t.Errorf("toRel outside CWD = %q, want %q", rel2, outsidePath)
	}
}

func TestToAbsWithCachedCwd(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	dir := t.TempDir()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}

	cachedCwd = dir

	// Относительный путь → абсолютный
	relPath := filepath.Join("sub", "file.txt")
	abs := toAbs(relPath)
	want := filepath.Join(dir, "sub", "file.txt")
	if abs != want {
		t.Errorf("toAbs(%q) = %q, want %q", relPath, abs, want)
	}

	// Уже абсолютный — без изменений
	absPath := filepath.Join(dir, "other.txt")
	abs2 := toAbs(absPath)
	if abs2 != absPath {
		t.Errorf("toAbs absolute = %q, want %q", abs2, absPath)
	}
}
