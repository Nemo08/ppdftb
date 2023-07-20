package convert

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"golang.org/x/exp/slog"

	pdf "github.com/Nemo08/ppdftb/pkg/pdf"
	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

var replaces = map[string]string{
	"Name":   "Имя",
	"Number": "66955",
}

func StrReplace(in string) string {
	s := in
	for k, v := range replaces {
		if strings.Contains(s, "\\{\\{"+k+"\\}\\}") {
			s = strings.ReplaceAll(s, "\\{\\{"+k+"\\}\\}", v)
		}
	}
	return s
}

func wait() {
	time.Sleep(time.Millisecond * 300)
}

// Печатает из настроенных конфигураций печати в указанную папку
func AcadToPdf(ctx context.Context, ff, dir string) error {

	fromFile, err := filepath.Abs(ff)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	toDir, err := filepath.Abs(dir)

	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	//Инициализируем Ole
	err = ole.CoInitializeEx(0, ole.COINIT_DISABLE_OLE1DDE|ole.COINIT_APARTMENTTHREADED|ole.COINIT_SPEED_OVER_MEMORY|ole.COINIT_MULTITHREADED)
	if err != nil {
		ole.CoUninitialize()
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("AutoCAD.Application")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	acad := unknown.MustQueryInterface(ole.IID_IDispatch)
	defer acad.Release()

	_, err = oleutil.PutProperty(acad, "Visible", false)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	wait()

	docsv, err := oleutil.GetProperty(acad, "Documents")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	docs := docsv.ToIDispatch()
	defer docs.Release()

	//Открываем чертеж
	openArguments := []interface{}{fromFile, true}
	cadFile := oleutil.MustCallMethod(docs, "Open", openArguments...).ToIDispatch()
	defer cadFile.Release()

	activeDoc := oleutil.MustGetProperty(acad, "ActiveDocument").ToIDispatch()
	defer activeDoc.Release()

	var i int32

	//Modelspace replace
	msv, err := activeDoc.GetProperty("ModelSpace")
	ms := msv.ToIDispatch()
	defer ms.Release()

	msCount, err := ms.GetProperty("Count")

	for i = 0; i < msCount.Value().(int32); i++ {
		itemArguments := []interface{}{i}
		itemv := oleutil.MustCallMethod(ms, "Item", itemArguments...)
		item := itemv.ToIDispatch()
		ts, err := oleutil.GetProperty(item, "TextString")
		if err == nil {
			oleutil.PutProperty(item, "TextString", []interface{}{StrReplace(ts.ToString())}...)
		}

		defer item.Release()
	}

	//Paperspace replace
	ps := oleutil.MustGetProperty(activeDoc, "PaperSpace").ToIDispatch()
	defer ps.Release()

	psCount, err := ps.GetProperty("Count")

	for i = 0; i < psCount.Value().(int32); i++ {
		itemArguments := []interface{}{i}

		itemv, err := ps.CallMethod("Item", itemArguments...)
		item := itemv.ToIDispatch()

		ts, err := item.GetProperty("TextString")
		if err == nil {
			item.PutProperty("TextString", []interface{}{StrReplace(ts.ToString())}...)
		}

		defer item.Release()
	}

	activeLayoutv, err := activeDoc.GetProperty("ActiveLayout")

	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	activeLayout := activeLayoutv.ToIDispatch()

	//Получаем листы
	layoutsv, err := activeDoc.GetProperty("Layouts")
	layouts := layoutsv.ToIDispatch()
	defer layouts.Release()

	itemArguments := []interface{}{1}
	itemv := oleutil.MustCallMethod(layouts, "Item", itemArguments...)

	addArguments := []interface{}{itemv}
	_, err = oleutil.PutProperty(activeDoc, "ActiveLayout", addArguments...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	//Получаем конфигурации печати
	pconfv, err := oleutil.GetProperty(activeDoc, "PlotConfigurations")
	pconf := pconfv.ToIDispatch()
	defer pconf.Release()

	pcount, err := oleutil.GetProperty(pconf, "Count")

	plotv, err := oleutil.GetProperty(activeDoc, "Plot")
	plot := plotv.ToIDispatch()

	defer plot.Release()

	//Получаем список конфигураций и печатаем их

	for i = 0; i < pcount.Value().(int32); i++ {
		itemArguments := []interface{}{i}
		wait()
		itemv := oleutil.MustCallMethod(pconf, "Item", itemArguments...)
		item := itemv.ToIDispatch()

		defer item.Release()

		itemName, err := oleutil.GetProperty(item, "Name")
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}

		addArguments := []interface{}{item}

		_, err = oleutil.CallMethod(activeLayout, "CopyFrom", addArguments...)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		} else {
			plotArguments := []interface{}{filepath.Join(toDir, itemName.ToString()+".pdf")}
			_, err = oleutil.CallMethod(plot, "PlotToFile", plotArguments...)
			if err != nil {
				slog.ErrorCtx(ctx, err.Error())
				return err
			}
		}
	}

	//Закрываем документ без сохранения
	closeArguments := []interface{}{false}
	oleutil.MustCallMethod(activeDoc, "Close", closeArguments...)

	//Закрываем приложение
	quitArguments := []interface{}{}
	oleutil.MustCallMethod(acad, "Quit", quitArguments...)

	return nil
}

func A2pdf(ctx context.Context, ff, dir string) error {
	outDir, err := os.MkdirTemp(dir, "temp-")
	defer os.RemoveAll(outDir)

	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	err = AcadToPdf(ctx, ff, outDir)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	cleanName := strings.TrimSuffix(ff, filepath.Ext(ff))

	err = pdf.Merge(ctx, outDir, filepath.Join(dir, cleanName+".pdf"))
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	return nil
}
