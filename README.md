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
wconv -s DOCFOLDER -o PDFFOLDER [-c] [-x XMLROOT -u 2] [-i file.xml] [-p PICFOLDER] [-l LEVEL]
```

| Флаг | Описание |
|------|----------|
| `-s` | Файл или папка для конвертации |
| `-o` | Папка для PDF (обязательно) |
| `-d` | Папка для промежуточных DOCX |
| `-c` | Использовать кэш (пропуск неизменившихся) |
| `-i` | XML-файл данных для шаблона (повторяемый) |
| `-x` | Корневая папка с XML |
| `-u` | Уровней вверх для поиска XML |
| `-p` | Папка с картинками для подстановки |
| `-l` | Уровень лога: debug, info, warn, error (по умолч.) |
| `-v` | Версия программы |

### aconv — AutoCAD → PDF

Конвертирует чертежи `.dwg`/`.dxf` в PDF через AutoCAD.

```
aconv -id CADFOLDER -od PDFFOLDER
aconv -if file.dwg -od PDFFOLDER
```

| Флаг | Описание |
|------|----------|
| `-if` | Конкретный DWG/DXF файл |
| `-id` | Папка с DWG/DXF файлами |
| `-od` | Папка для PDF (обязательно) |
| `-log` | Уровень лога |
| `-v` | Версия программы |

### mpdf — Merge PDF

Объединяет все PDF из папки в один. Сортировка natural sort.
Закладки (outline) из исходных файлов сохраняются как подзакладки.

```
mpdf -d PDFFOLDER -o All.pdf
```

| Флаг | Описание |
|------|----------|
| `-d` | Папка с PDF (обязательно) |
| `-o` | Выходной файл (по умолч. `out.pdf`) |
| `-l` | Уровень лога |
| `-v` | Версия программы |

### pnpdf — Page Number PDF

Добавляет нумерацию страниц в готовый PDF.

```
pnpdf -if input.pdf -of output.pdf -pf 3 -nf 1
```

| Флаг | Описание |
|------|----------|
| `-if` | Входной PDF (обязательно) |
| `-of` | Выходной PDF (обязательно) |
| `-pf` | С какой физической страницы нумеровать (по умолч. 1) |
| `-nf` | С какого номера начинать (по умолч. 1) |
| `-l` | Уровень лога |
| `-v` | Версия программы |

### toc — Table of Contents

Формирует оглавление в DOCX на основе шаблона и набора PDF-файлов.
Нумерация страниц подхватывается из реальных PDF.

```
toc "шаблон.docx" OUTDOCXDIR PDFDIR [page]
toc -tf "шаблон.docx" -td OUTDOCXDIR -pd PDFDIR [-tn 3]
```

| Аргумент / Флаг | Описание |
|-----------------|----------|
| `source` | Файл шаблона `.docx` (позиционный или `-tf`) |
| `output` | Папка для результата (позиционный или `-td`) |
| `pdf` | Папка с PDF (позиционный или `-pd`) |
| `page` / `-tn` | Номер страницы содержания (по умолч. 3) |
| `-l` | Уровень лога |
| `-v` | Версия программы |

### engine — единый сервер

Общий TCP-сервер (:17321) с постоянными COM-пулами Word (4 экз.) и AutoCAD (1 экз.).
Утилиты wconv/aconv/toc/mpdf/pnpdf запускаются через него без пересоздания OLE-объектов.

```
engine -serve                        # запуск сервера
engine wconv -s DIR -o DIR ...       # выполнить wconv через сервер
engine toc "шаблон" DIR DIR 3       # выполнить toc через сервер
engine mpdf -d DIR -o file.pdf      # выполнить mpdf через сервер
engine -shutdown                     # остановка сервера
```

| Флаг | Описание |
|------|----------|
| `-serve` | Режим сервера |
| `-shutdown` | Остановить сервер |
| `-port` | Порт TCP (по умолч. 17321) |
| `-l` | Уровень лога |
| `-v` | Версия |

---

## Примеры сценариев

```bat
:: 1. Конвертация всех Word-файлов в PDF
wconv -s DOCFOLDER -o PDFFOLDER

:: 2. Конвертация с шаблонами и картинками
wconv -s DOCFOLDER -o PDFFOLDER -x XMLROOT -u 2 -p PICFOLDER

:: 3. Конвертация с кэшем (повторный запуск — только изменённые)
wconv -s DOCFOLDER -o PDFFOLDER -c -x XMLROOT -u 2

:: 4. Конвертация чертежей AutoCAD
aconv -id CADFOLDER -od PDFFOLDER

:: 5. Предварительное оглавление
toc "шаблон.docx" TMPDIR PDFDIR 3

:: 6. PDF из оглавления
wconv -s TMPDIR -o PDFFOLDER

:: 7. Финальное оглавление
toc "шаблон.docx" TMPDIR PDFDIR 3

:: 8. Сборка в один PDF
mpdf -d PDFFOLDER -o All.pdf

:: ---- полный цикл через engine ----
engine -serve
engine wconv -s DOCFOLDER -o PDFFOLDER -d TMPDIR -x XMLROOT -u 2
engine toc "шаблон.docx" TMPDIR PDFFOLDER 3
engine wconv -s TMPDIR -o PDFFOLDER -x XMLROOT
engine mpdf -d PDFFOLDER -o All.pdf
engine pnpdf -if All.pdf -of Final.pdf -pf 4 -nf 1
engine -shutdown
```

---

## Архитектура

Общий OLE-пул (`pkg/olepool`) — обобщённый COM-пул с Job Object, PID force-kill,
`LockOSThread`, каналами. WordPool (`pkg/wordpool`) и AcadPool (`pkg/acadpool`) —
тонкие обёртки над ним.

Единый пайплайн `WconvPipeline` (`pkg/convert/pipeline.go`) — сбор файлов,
шаблонизация, конвертация через WordPool. Используется и CLI, и engine-сервером.

```
CLI (wconv --serve …) ──► общий пайплайн ──► WordPool ──► COM
CLI (wconv -s …)      ──► свой WordPool ──► COM
engine -serve          ──► общий WordPool+AcadPool
```

### Пакеты

| Пакет | Назначение |
|-------|------------|
| `pkg/olepool` | Generic OLE pool (Job, Config, Pool, Submit, Close) |
| `pkg/wordpool` | WordPool — обёртка над olepool для Word.Application |
| `pkg/acadpool` | AcadPool — обёртка над olepool для AutoCAD.Application |
| `pkg/jobutil` | Job Object helpers (экспортированные) |
| `pkg/convert` | Логика конвертации: сбор файлов, шаблоны, пайплайн |
| `pkg/cache` | Кэш для повторных запусков |
| `pkg/pdf` | Pagination, merge |
| `pkg/toc` | Table of contents |
| `pkg/slogutil` | Настройка логгера |

---

## Благодарности

- Microsoft за офисный пакет
- FoxyUtils.com за библиотеку unipdf
- Автору библиотеки github.com/briiC/docxplate
