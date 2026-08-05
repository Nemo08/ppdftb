//go:build windows

package acadpool

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"log/slog"

	"github.com/Nemo08/ppdftb/pkg/olepool"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

var defaultReplaces = map[string]string{
	"Name":   "Имя",
	"Number": "66955",
}

// StrReplace заменяет плейсхолдеры вида {{Name}}/{{Number}} в строке.
func StrReplace(in string, replaces map[string]string) string {
	s := in
	for k, v := range replaces {
		s = strings.ReplaceAll(s, "\\{\\{"+k+"\\}\\}", v)
	}
	return s
}

var acadCfg = olepool.Config{
	AppName: "AutoCAD.Application",
}

type acadJob struct {
	fromFile string
	toDir    string
	replaces map[string]string
}

func (j *acadJob) Process(app *ole.IDispatch) error {
	slog.Debug("acadToPdf " + j.fromFile + " " + j.toDir)

	docsv, err := app.GetProperty("Documents")
	if err != nil {
		return err
	}
	docs := docsv.ToIDispatch()
	defer docs.Release()

	cadFilev, err := docs.CallMethod("Open", []any{j.fromFile, true}...)
	if err != nil {
		return err
	}
	if err := cadFilev.Clear(); err != nil {
		slog.Warn("не удалось освободить VARIANT документа", slog.String("err", err.Error()))
	}

	activeDocv, err := app.GetProperty("ActiveDocument")
	if err != nil {
		return err
	}
	activeDoc := activeDocv.ToIDispatch()
	defer activeDoc.Release()

	if err := replaceTextInSpaces(activeDoc, j.replaces); err != nil {
		return err
	}

	pconf, err := setupPlotConfig(activeDoc)
	if err != nil {
		return err
	}
	defer pconf.Release()

	pcount, err := pconf.GetProperty("Count")
	if err != nil {
		return err
	}

	bgp, err := saveAndDisableBackgroundPlot(activeDoc)
	if err != nil {
		return err
	}

	if err := plotAllConfigs(activeDoc, pconf, pcount, j.toDir); err != nil {
		return err
	}

	restoreBackgroundPlot(activeDoc, bgp)

	slog.Debug("Закрываем документ без сохранения")
	_, err = activeDoc.CallMethod("Close", []any{false}...)
	if err != nil {
		slog.Error(err.Error())
	}

	slog.Debug("Конец AcadToPdf")
	return nil
}

func replaceTextInSpaces(activeDoc *ole.IDispatch, replaces map[string]string) error {
	spaces := []string{"ModelSpace", "PaperSpace"}
	for _, spaceName := range spaces {
		slog.Debug(spaceName + " replaces begin")
		msv, err := activeDoc.GetProperty(spaceName)
		if err != nil {
			return err
		}
		ms := msv.ToIDispatch()

		msCount, err := ms.GetProperty("Count")
		if err != nil {
			ms.Release()
			return err
		}
		count, ok := msCount.Value().(int32)
		if !ok {
			ms.Release()
			return fmt.Errorf("count: неверный тип %T", msCount.Value())
		}
		for i := range count {
			itemv, err := ms.CallMethod("Item", []any{i}...)
			if err != nil {
				ms.Release()
				return err
			}
			item := itemv.ToIDispatch()

			ts, err := item.GetProperty("TextString")
			if err == nil {
				if err := item.PutProperty("TextString", []any{StrReplace(ts.ToString(), replaces)}...); err != nil {
					item.Release()
					ms.Release()
					return fmt.Errorf("замена текста в штампе: %w", err)
				}
			}
			item.Release()
		}
		ms.Release()
		slog.Debug(spaceName + " replaces end")
	}
	return nil
}

func setupPlotConfig(activeDoc *ole.IDispatch) (*ole.IDispatch, error) {
	slog.Debug("Получаем листы")
	layoutsv, err := activeDoc.GetProperty("Layouts")
	if err != nil {
		return nil, err
	}
	layouts := layoutsv.ToIDispatch()
	defer layouts.Release()

	slog.Debug("Переключаемся на первый лист")
	itemv, err := layouts.CallMethod("Item", []any{1}...)
	if err != nil {
		return nil, err
	}

	_, err = activeDoc.PutProperty("ActiveLayout", []any{itemv}...)
	if err != nil {
		return nil, err
	}

	slog.Debug("Получаем конфигурации печати")
	pconfv, err := activeDoc.GetProperty("PlotConfigurations")
	if err != nil {
		return nil, err
	}
	return pconfv.ToIDispatch(), nil
}

func saveAndDisableBackgroundPlot(activeDoc *ole.IDispatch) (*ole.VARIANT, error) {
	bgp, err := activeDoc.CallMethod("GetVariable", []any{"BACKGROUNDPLOT"}...)
	if err != nil {
		return nil, err
	}
	slog.Debug("BACKGROUNDPLOT is", slog.Int("value", int(bgp.Val)))

	slog.Debug("Устанавливаем BACKGROUNDPLOT в 0")
	_, err = activeDoc.CallMethod("SetVariable", []any{"BACKGROUNDPLOT", 0}...)
	if err != nil {
		return nil, err
	}
	return bgp, nil
}

// acadPlotDelay — задержка между командами AutoCAD COM.
// AutoCAD не предоставляет синхронного события завершения PlotToFile,
// поэтому минимальная пауза необходима. 100ms достаточно на большинстве машин;
// при нестабильной работе увеличьте через переменную окружения ACAD_PLOT_DELAY_MS.
const acadPlotDelay = 100 * time.Millisecond

func plotAllConfigs(activeDoc *ole.IDispatch, pconf *ole.IDispatch, pcount *ole.VARIANT, toDir string) error {
	pc, ok := pcount.Value().(int32)
	if !ok {
		return fmt.Errorf("PlotConfigurations.Count: неверный тип %T", pcount.Value())
	}

	plotv, err := activeDoc.GetProperty("Plot")
	if err != nil {
		return err
	}
	plot := plotv.ToIDispatch()
	defer plot.Release()

	activeLayoutv, err := activeDoc.GetProperty("ActiveLayout")
	if err != nil {
		return err
	}
	activeLayout := activeLayoutv.ToIDispatch()
	defer activeLayout.Release()

	time.Sleep(acadPlotDelay)

	slog.Debug("Получаем список конфигураций и печатаем их")
	for i := range pc {
		itemv, err := pconf.CallMethod("Item", []any{i}...)
		if err != nil {
			return err
		}
		item := itemv.ToIDispatch()

		itemName, err := item.GetProperty("Name")
		if err != nil {
			item.Release()
			return err
		}
		slog.Debug(itemName.ToString())

		_, err = activeLayout.CallMethod("CopyFrom", []any{item}...)
		time.Sleep(acadPlotDelay)
		item.Release()
		if err != nil {
			return err
		}

		plotArguments := []any{filepath.Join(toDir, itemName.ToString()+".pdf")}
		slog.Debug("plotArguments " + filepath.Join(toDir, itemName.ToString()+".pdf"))

		if _, err := oleutil.CallMethod(plot, "PlotToFile", plotArguments...); err != nil {
			return fmt.Errorf("PlotToFile: %w", err)
		}
		time.Sleep(acadPlotDelay)
	}
	return nil
}

func restoreBackgroundPlot(activeDoc *ole.IDispatch, bgp *ole.VARIANT) {
	bgpVal := bgp.Value()
	_, err := activeDoc.CallMethod("SetVariable", []any{"BACKGROUNDPLOT", bgpVal}...)
	if err != nil {
		slog.Error(err.Error())
	}
	if bgpInt, ok := bgpVal.(int16); ok {
		slog.Debug("BACKGROUNDPLOT restored", slog.Int("value", int(bgpInt)))
	}
}

// AcadPool — пул экземпляров AutoCAD для параллельной конвертации DWG/DXF в PDF.
type AcadPool struct {
	pool     *olepool.Pool
	replaces map[string]string
}

// NewAcadPool создаёт пул из size экземпляров AutoCAD.
func NewAcadPool(size int) *AcadPool {
	return &AcadPool{
		pool:     olepool.NewPool(size, acadCfg),
		replaces: defaultReplaces,
	}
}

// NewAcadPoolWithReplaces создаёт пул с таблицей подстановок.
func NewAcadPoolWithReplaces(size int, replaces map[string]string) *AcadPool {
	if replaces == nil {
		replaces = defaultReplaces
	}
	return &AcadPool{
		pool:     olepool.NewPool(size, acadCfg),
		replaces: replaces,
	}
}

// WaitReady блокируется пока хотя бы один воркер AutoCAD не будет готов.
func (p *AcadPool) WaitReady(ctx context.Context) error {
	return p.pool.WaitReady(ctx)
}

// AcadToPdf конвертирует один DWG/DXF через пул.
func (p *AcadPool) AcadToPdf(ctx context.Context, fromFile, toDir string) error {
	fromFile, err := filepath.Abs(strings.TrimSpace(fromFile))
	if err != nil {
		return err
	}
	toDir, err = filepath.Abs(strings.TrimSpace(toDir))
	if err != nil {
		return err
	}
	return p.pool.Submit(ctx, &acadJob{fromFile: fromFile, toDir: toDir, replaces: p.replaces})
}

// Close завершает все экземпляры AutoCAD и освобождает ресурсы.
func (p *AcadPool) Close() {
	p.pool.Close()
}
