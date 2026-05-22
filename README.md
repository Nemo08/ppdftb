# ppdftb

Набор утилит командной строки для работы с PDF файлами.
Конвертация Word/AutoCAD → PDF, объединение, нумерация страниц, оглавление.

**Требования:** Windows (COM-автоматизация для Word и AutoCAD).

---

## Утилиты

### wconv — Word → PDF

Конвертирует `.doc`/`.docx`/`.rtf` в PDF через Microsoft Word.
Поддерживает шаблоны JJack с подстановкой данных из XML/JSON и встраиванием изображений.

```
wconv --source=DOCFOLDER --output=PDFFOLDER [--cache] [--xmlf=XMLROOT --up=2] [--xml=file.xml] [--pics=PICFOLDER] [--log=LEVEL]
```

| Флаг | Сокращение | Описание |
|------|-----------|----------|
| `--source` | `-s` | Файл или папка для конвертации |
| `--output` | `-o` | Папка для PDF (обязательно) |
| `--outputd` | `-d` | Папка для промежуточных DOCX |
| `--cache` | `-c` | Использовать кэш (пропуск неизменившихся) |
| `--xml` | `-i` | XML-файл данных для шаблона |
| `--xmlf` | `-x` | Корневая папка с XML |
| `--up` | `-u` | Уровней вверх для поиска XML |
| `--pics` | `-p` | Папка с картинками для подстановки |
| `--log` | `-l` | Уровень лога: debug, info, warn, error (по умолч.) |
| `--version` | `-v` | Версия программы |

### aconv — AutoCAD → PDF

Конвертирует чертежи `.dwg`/`.dxf` в PDF через AutoCAD.

```
aconv --id=CADFOLDER --od=PDFFOLDER
aconv --if=file.dwg --od=PDFFOLDER
```

| Флаг | Сокращение | Описание |
|------|-----------|----------|
| `--if` | `-i` | Конкретный DWG/DXF файл |
| `--id` | `-d` | Папка с DWG/DXF файлами |
| `--od` | `-o` | Папка для PDF (обязательно) |
| `--log` | `-l` | Уровень лога |

### mpdf — Merge PDF

Объединяет все PDF из папки в один. Сортировка natural sort.
Закладки (outline) из исходных файлов сохраняются как подзакладки.

```
mpdf --dir=PDFFOLDER --out=All.pdf
```

| Флаг | Сокращение | Описание |
|------|-----------|----------|
| `--dir` | `-d` | Папка с PDF (обязательно) |
| `--out` | `-o` | Выходной файл (по умолч. `out.pdf`) |
| `--log` | `-l` | Уровень лога |

### pnpdf — Page Number PDF

Добавляет нумерацию страниц в готовый PDF.

```
pnpdf --if=input.pdf --of=output.pdf --pf=3 --nf=1
```

| Флаг | Сокращение | Описание |
|------|-----------|----------|
| `--if` | `-i` | Входной PDF (обязательно) |
| `--of` | `-o` | Выходной PDF (обязательно) |
| `--pf` | `-p` | С какой физической страницы нумеровать (по умолч. 1) |
| `--nf` | `-n` | С какого номера начинать (по умолч. 1) |
| `--log` | `-l` | Уровень лога |

### toc — Table of Contents

Формирует оглавление в DOCX на основе шаблона и набора PDF-файлов.
Нумерация страниц подхватывается из реальных PDF.

```
toc "шаблон.docx" OUTDOCXDIR PDFDIR --page=3
```

| Аргумент | Описание |
|----------|----------|
| `source file` | Файл шаблона `.docx` (позиционный) |
| `output file` | Папка для результата (позиционный) |
| `pdf folder` | Папка с PDF (позиционный) |
| `--page` / `-n` | Номер страницы содержания в собранном файле (по умолч. 3) |
| `--log` / `-l` | Уровень лога |

---

## Примеры сценариев

```bat
:: 1. Конвертация всех Word-файлов в PDF
wconv --source=DOCFOLDER --output=PDFFOLDER

:: 2. Конвертация с шаблонами и картинками
wconv --source=DOCFOLDER --output=PDFFOLDER --xmlf=XMLROOT --up=2 --pics=PICFOLDER

:: 3. Конвертация с кэшем (повторный запуск — только изменённые)
wconv --source=DOCFOLDER --output=PDFFOLDER --cache --xmlf=XMLROOT --up=2

:: 4. Конвертация чертежей AutoCAD
aconv --id=CADFOLDER --od=PDFFOLDER

:: 5. Предварительное оглавление
toc "шаблон.docx" TMPDIR PDFDIR --page=3

:: 6. PDF из оглавления
wconv --source=TMPDIR --output=PDFFOLDER

:: 7. Финальное оглавление
toc "шаблон.docx" TMPDIR PDFDIR --page=3

:: 8. Сборка в один PDF
mpdf --dir=PDFFOLDER --out=All.pdf
```

---

## Благодарности

- Microsoft за офисный пакет
- FoxyUtils.com за библиотеку unipdf
- Автору библиотеки github.com/briiC/docxplate
