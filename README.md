# ppdftb

Набор утилит командной строки для работы с PDF.
Конвертация Word/AutoCAD → PDF, объединение, нумерация страниц, оглавление.

**Требования:** Windows (COM-автоматизация для Word и AutoCAD).

## Сборка

```bat
build.bat              # сборка всех утилит в build/
build.bat <version>    # сборка с указанием версии
```

Сборка требует Go 1.22.5+. Если `go.mod` содержит replace на локальный
`go-template-docx`, этот модуль должен лежать рядом в `../go-template-docx`.

## Утилиты

### wconv — Word → PDF

Конвертирует `.doc`/`.docx`/`.rtf` в PDF через Microsoft Word.
Поддерживает шаблоны JJack с подстановкой данных из XML/JSON и встраиванием изображений.

```
wconv -s DOCFOLDER -o PDFFOLDER [-c] [-x XMLROOT -u 2] [-i file.xml] [-p PICFOLDER] [-l LEVEL]
```

### aconv — AutoCAD → PDF

Конвертирует чертежи `.dwg`/`.dxf` в PDF через AutoCAD (COM).
Выполняет подстановку плейсхолдеров `\{\{Name\}\}`/`\{\{Number\}\}` в текстовых объектах.

```
aconv -id CADFOLDER -od PDFFOLDER
aconv -if file.dwg -od PDFFOLDER
```

### mpdf — Merge PDF

Объединяет все PDF из папки в один. Сортировка natural sort.
Закладки (outline) из исходных файлов сохраняются как подзакладки.

```
mpdf -d PDFFOLDER -o All.pdf
```

### pnpdf — Page Number PDF

Добавляет нумерацию страниц в готовый PDF.
Номер ставится в правый нижний угол (горизонтальные A4 — с поворотом -90°).

```
pnpdf -if input.pdf -of output.pdf -pf 3 -nf 1
```

### toc — Table of Contents

Формирует оглавление в DOCX на основе шаблона docxplate и набора PDF-файлов.
Нумерация страниц подхватывается из реальных PDF.

```
toc "шаблон.docx" OUTDOCXDIR PDFDIR [page]
toc -tf "шаблон.docx" -td OUTDOCXDIR -pd PDFDIR [-tn 3]
```

### engine — единый сервер

TCP-сервер (:17321) с постоянными COM-пулами Word (4 экз.) и AutoCAD (1 экз.).
Утилиты запускаются через сервер, OLE-объекты не пересоздаются между вызовами.

**Оптимизации производительности:**
- **Конвейер шаблонизация → PDF**: шаблонизация и конвертация каждого файла выполняются
  в одной горутине — PDF начинает генерироваться сразу после обработки шаблона,
  без ожидания окончания шаблонизации всех файлов (`TplToPdfWithPool`).
- **Параллельный сбор страниц**: `collectPageCounts` читает PDF параллельно (до 8 горутин)
  для быстрого заполнения кэша страниц.
- **Кэш страниц**: toc использует кэш из wconv, повторное чтение PDF не требуется.

```
engine -serve                        # запуск сервера
engine wconv -s DIR -o DIR ...       # выполнить wconv через сервер
engine toc "шаблон" DIR DIR 3       # выполнить toc через сервер
engine mpdf -d DIR -o file.pdf      # выполнить mpdf через сервер
engine -shutdown                     # остановка сервера
```

## Полный цикл сборки тома

```bat
:: ---- без engine ----
wconv -s ШАБЛОН_ТОМА -o PDF -x КОРЕНЬ -d ДОКУМЕНТЫ_ТОМА -u 1
toc "Шаблоны\Содержание.docx" ШАБЛОН_ТОМА PDF 3
wconv -s ШАБЛОН_ТОМА\Содержание.docx -o PDF -x КОРЕНЬ
toc "Шаблоны\Содержание.docx" ШАБЛОН_ТОМА PDF 3
wconv -s ШАБЛОН_ТОМА\Содержание.docx -o PDF -x КОРЕНЬ
mpdf -d PDF -o temp.pdf
pnpdf -if temp.pdf -of ТОМ.pdf -pf 4 -nf 1
del temp.pdf

:: ---- через engine (быстрее: пулы Word/AutoCAD не пересоздаются) ----
engine -serve
engine wconv -s ШАБЛОН_ТОМА -o PDF -x КОРЕНЬ -d ДОКУМЕНТЫ_ТОМА -u 1
engine toc "Шаблоны\Содержание.docx" ШАБЛОН_ТОМА PDF 3
engine wconv -s ШАБЛОН_ТОМА\Содержание.docx -o PDF -x КОРЕНЬ
engine mpdf -d PDF -o temp.pdf
engine pnpdf -if temp.pdf -of ТОМ.pdf -pf 4 -nf 1
engine -shutdown
```

## Архитектура

```
cmd/
  wconv/     — конвертация Word → PDF (CLI)
  aconv/     — конвертация AutoCAD → PDF (CLI)
  mpdf/      — объединение PDF (CLI)
  pnpdf/     — нумерация страниц (CLI)
  toc/       — оглавление (CLI)
  engine/    — TCP-сервер с COM-пулами (map-диспетчер, DIP)
pkg/
  olepool/   — обобщённый COM-пул (Job Object, LockOSThread)
  wordpool/  — Word.Application поверх olepool + WordConverter
  acadpool/  — AutoCAD.Application поверх olepool + CadConverter + StrReplace
  convert/   — use-case слой: сбор файлов, XML→JSON, шаблонизация, пайплайн
               интерфейсы: WordConverter, CadConverter, ConvCache
               разбит: dataconv.go (XML→JSON), fileutil.go (файловые утилиты),
               convert_jack.go (шаблонизация JJack), aconv.go (CAD конвертация),
               pipeline.go (пайплайн wconv)
  cache/     — инкрементальный кэш (.filecache.json) + ConvCache
  pdf/       — merge, pagination, mm→pt (сжатие через SetOptimizer)
  toc/       — генерация DOCX-оглавления
  jobutil/   — Windows Job Object helpers
  slogutil/  — настройка slog
```

## Принципы

- **DIP**: `pkg/convert` не импортирует инфраструктурные пакеты напрямую —
  WordConverter/CadConverter/ConvCache определены как интерфейсы
  в `pkg/convert/interfaces.go` и реализованы в `pkg/wordpool`, `pkg/acadpool`, `pkg/cache`.
- **OCP**: `cmd/engine` использует `map[string]HandlerFunc` вместо switch.
- **DRY**: шаблонизация DOCX вынесена в общую `processOneFile`, используемую
  как `TplToPdfWithPool`, так и `TplToDocxJJack3`.
- **Clean Architecture**: `cmd/` — delivery, `pkg/convert` — use cases,
  остальные `pkg/` — infrastructure.

## Благодарности

- Microsoft за офисный пакет и AutoCAD
- Oliverpool за форк unipdf с поддержкой оптимизации
- FoxyUtils.com за библиотеку unipdf
- Автору библиотеки github.com/briiC/docxplate
