//go:build windows

package acadpool

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"github.com/Nemo08/ppdftb/pkg/olepool"
	"log/slog"
)

var (
	replacesMu sync.RWMutex
	replaces   = map[string]string{
		"Name":   "Имя",
		"Number": "66955",
	}
)

// StrReplace заменяет плейсхолдеры вида {{Name}}/{{Number}} в строке.
func StrReplace(in string) string {
	replacesMu.RLock()
	defer replacesMu.RUnlock()
	s := in
	for k, v := range replaces {
		if strings.Contains(s, "\\{\\{"+k+"\\}\\}") {
			s = strings.ReplaceAll(s, "\\{\\{"+k+"\\}\\}", v)
		}
	}
	return s
}

var acadCfg = olepool.Config{
	AppName: "AutoCAD.Application",
}

type acadJob struct {
	fromFile string
	toDir    string
}

func (j *acadJob) Process(app *ole.IDispatch) error {
	slog.Debug("acadToPdf " + j.fromFile + " " + j.toDir)

	docsv, err := app.GetProperty("Documents")
	if err != nil {
		return err
	}
	docs := docsv.ToIDispatch()
	defer docs.Release()

	cadFilev, err := docs.CallMethod("Open", []interface{}{j.fromFile, true}...)
	if err != nil {
		return err
	}
	cadFile := cadFilev.ToIDispatch()
	defer cadFile.Release()

	activeDocv, err := app.GetProperty("ActiveDocument")
	if err != nil {
		return err
	}
	activeDoc := activeDocv.ToIDispatch()
	defer activeDoc.Release()

	spaces := []string{"ModelSpace", "PaperSpace"}
	for _, spaceName := range spaces {
		slog.Debug(spaceName + " replaces begin")
		msv, err := activeDoc.GetProperty(spaceName)
		if err != nil {
			return err
		}
		ms := msv.ToIDispatch()
		defer ms.Release()

		msCount, err := ms.GetProperty("Count")
		if err != nil {
			return err
		}
		for i := int32(0); i < msCount.Value().(int32); i++ {
			itemv, err := ms.CallMethod("Item", []interface{}{i}...)
			if err != nil {
				return err
			}
			item := itemv.ToIDispatch()

			ts, err := item.GetProperty("TextString")
			if err == nil {
				item.PutProperty("TextString", []interface{}{StrReplace(ts.ToString())}...)
			}
			item.Release()
		}
		slog.Debug(spaceName + " replaces end")
	}

	slog.Debug("Получаем листы")
	layoutsv, err := activeDoc.GetProperty("Layouts")
	if err != nil {
		return err
	}
	layouts := layoutsv.ToIDispatch()
	defer layouts.Release()

	slog.Debug("Переключаемся на первый лист")
	itemv, err := layouts.CallMethod("Item", []interface{}{1}...)
	if err != nil {
		return err
	}

	_, err = activeDoc.PutProperty("ActiveLayout", []interface{}{itemv}...)
	if err != nil {
		return err
	}

	slog.Debug("Получаем конфигурации печати")
	pconfv, err := activeDoc.GetProperty("PlotConfigurations")
	if err != nil {
		return err
	}
	pconf := pconfv.ToIDispatch()
	defer pconf.Release()

	pcount, err := pconf.GetProperty("Count")
	if err != nil {
		return err
	}

	bgp, err := activeDoc.CallMethod("GetVariable", []interface{}{"BACKGROUNDPLOT"}...)
	if err != nil {
		return err
	}
	slog.Debug("BACKGROUNDPLOT is", slog.Int("value", int(bgp.Val)))

	slog.Debug("Устанавливаем BACKGROUNDPLOT в 0")
	_, err = activeDoc.CallMethod("SetVariable", []interface{}{"BACKGROUNDPLOT", 0}...)
	if err != nil {
		return err
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

	time.Sleep(time.Millisecond * 300)

	slog.Debug("Получаем список конфигураций и печатаем их")
	for i := int32(0); i < pcount.Value().(int32); i++ {
		itemv, err := pconf.CallMethod("Item", []interface{}{i}...)
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

		_, err = activeLayout.CallMethod("CopyFrom", []interface{}{item}...)
		time.Sleep(time.Millisecond * 300)
		item.Release()
		if err != nil {
			return err
		}

		plotArguments := []interface{}{filepath.Join(j.toDir, itemName.ToString()+".pdf")}
		slog.Debug("plotArguments " + filepath.Join(j.toDir, itemName.ToString()+".pdf"))

		oleutil.MustCallMethod(plot, "PlotToFile", plotArguments...)
		time.Sleep(time.Millisecond * 300)
	}

	_, err = activeDoc.CallMethod("SetVariable", []interface{}{"BACKGROUNDPLOT", bgp.Value()}...)
	if err != nil {
		slog.Error(err.Error())
	}
	slog.Debug("BACKGROUNDPLOT restored", slog.Int("value", int(bgp.Value().(int16))))

	slog.Debug("Закрываем документ без сохранения")
	_, err = activeDoc.CallMethod("Close", []interface{}{false}...)
	if err != nil {
		slog.Error(err.Error())
	}

	slog.Debug("Конец AcadToPdf")
	return nil
}

// AcadPool — пул экземпляров AutoCAD для параллельной конвертации DWG/DXF в PDF.
type AcadPool struct {
	pool *olepool.Pool
}

// NewAcadPool создаёт пул из size экземпляров AutoCAD.
func NewAcadPool(size int) *AcadPool {
	return &AcadPool{pool: olepool.NewPool(size, acadCfg)}
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
	return p.pool.Submit(ctx, &acadJob{fromFile: fromFile, toDir: toDir})
}

// Close завершает все экземпляры AutoCAD и освобождает ресурсы.
func (p *AcadPool) Close() {
	p.pool.Close()
}
