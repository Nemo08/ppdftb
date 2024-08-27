package convert

import (
	"context"
	"io/ioutil"
	"os"
	"path"
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
	slog.Debug("acadToPdf " + ff + " " + dir)
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

	//_, err = oleutil.PutProperty(acad, "Visible", false)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	wait()

	docsv, err := acad.GetProperty("Documents")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	docs := docsv.ToIDispatch()
	defer docs.Release()

	//Открываем чертеж
	openArguments := []interface{}{fromFile, true}
	cadFilev, err := docs.CallMethod("Open", openArguments...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	cadFile := cadFilev.ToIDispatch()
	defer cadFile.Release()

	activeDocv, err := acad.GetProperty("ActiveDocument")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	activeDoc := activeDocv.ToIDispatch()
	defer activeDoc.Release()

	var i int32

	//Modelspace replace
	var spaces = []string{"ModelSpace", "PaperSpace"}
	for _, v := range spaces {
		slog.Debug(v + " replaces begin")
		msv, err := activeDoc.GetProperty(v)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}
		ms := msv.ToIDispatch()
		defer ms.Release()

		msCount, err := ms.GetProperty("Count")
		for i = 0; i < msCount.Value().(int32); i++ {
			itemv, err := ms.CallMethod("Item", []interface{}{i}...)
			if err != nil {
				slog.ErrorCtx(ctx, err.Error())
				return err
			}
			item := itemv.ToIDispatch()
			defer item.Release()

			ts, err := item.GetProperty("TextString")
			if err == nil {
				item.PutProperty("TextString", []interface{}{StrReplace(ts.ToString())}...)
			}
		}
		slog.Debug(v + " replaces end")
	}

	//Получаем листы
	slog.Debug("Получаем листы")
	layoutsv, err := activeDoc.GetProperty("Layouts")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	layouts := layoutsv.ToIDispatch()
	defer layouts.Release()

	slog.Debug("Переключаемся на первый лист")
	itemv, err := layouts.CallMethod("Item", []interface{}{1}...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	_, err = activeDoc.PutProperty("ActiveLayout", []interface{}{itemv}...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	//Получаем конфигурации печати
	slog.Debug("Получаем конфигурации печати")
	pconfv, err := activeDoc.GetProperty("PlotConfigurations")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	pconf := pconfv.ToIDispatch()
	defer pconf.Release()

	pcount, err := pconf.GetProperty("Count")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	bgp, err := activeDoc.CallMethod("GetVariable", []interface{}{"BACKGROUNDPLOT"}...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	slog.Debug("BACKGROUNDPLOT is " + string(bgp.Val))

	//Устанавливаем BACKGROUNDPLOT в 0
	slog.Debug("Устанавливаем BACKGROUNDPLOT в 0")
	_, err = activeDoc.CallMethod("SetVariable", []interface{}{"BACKGROUNDPLOT", 0}...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	plotv, err := activeDoc.GetProperty("Plot")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	plot := plotv.ToIDispatch()
	defer plot.Release()

	activeLayoutv, err := activeDoc.GetProperty("ActiveLayout")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	activeLayout := activeLayoutv.ToIDispatch()
	defer activeLayout.Release()

	wait()
	//Получаем список конфигураций и печатаем их
	slog.Debug("Получаем список конфигураций и печатаем их")
	for i = 0; i < pcount.Value().(int32); i++ {
		itemv, err := pconf.CallMethod("Item", []interface{}{i}...)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}
		item := itemv.ToIDispatch()
		defer item.Release()

		itemName, err := item.GetProperty("Name")
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}
		slog.Debug(itemName.ToString())

		_, err = activeLayout.CallMethod("CopyFrom", []interface{}{item}...)
		wait()
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		} else {
			plotArguments := []interface{}{filepath.Join(toDir, itemName.ToString()+".pdf")}
			slog.Debug("plotArguments " + filepath.Join(toDir, itemName.ToString()+".pdf"))

			oleutil.MustCallMethod(plot, "PlotToFile", plotArguments...)
			wait()

			if err != nil {
				slog.ErrorCtx(ctx, err.Error())
				return err
			}
		}
	}

	//Устанавливаем BACKGROUNDPLOT обратно
	_, err = activeDoc.CallMethod("SetVariable", []interface{}{"BACKGROUNDPLOT", bgp.Value()}...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	slog.Debug("BACKGROUNDPLOT is " + string(bgp.Value().(int16)))

	//Закрываем документ без сохранения
	slog.Debug("Закрываем документ без сохранения")
	closeArguments := []interface{}{false}
	_, err = activeDoc.CallMethod("Close", closeArguments...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	//Закрываем приложение
	slog.Debug("Закрываем приложение")
	quitArguments := []interface{}{}
	_, err = acad.CallMethod("Quit", quitArguments...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	slog.Debug("Конец AcadToPdf")
	return nil
}

func A2pdf(ctx context.Context, sourceFile, sourceFolder, outputFolder string) error {
	var inputCadFiles []string
	files, err := ioutil.ReadDir(sourceFolder)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	//Если задан входной файл
	if sourceFile != "" {
		//Проверка наличия исходного файла
		if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
			slog.ErrorCtx(ctx, "Input file "+sourceFile+" does not exists")
		}

		ifn, err := filepath.Abs(sourceFile) //Полный путь входного файла
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}
		inputCadFiles = append(inputCadFiles, ifn)
	}

	//Ищем в папке файлы
	for _, file := range files {
		if !file.IsDir() {
			if (strings.ToLower(path.Ext(file.Name())) == ".dwg") || (strings.ToLower(path.Ext(file.Name())) == ".dxf") {
				ffn, err := filepath.Abs(filepath.Join(sourceFolder, file.Name())) //Полный путь входного файла
				if err != nil {
					slog.ErrorCtx(ctx, err.Error())
					return err
				}
				inputCadFiles = append(inputCadFiles, ffn)
			}
		}
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	for _, file := range inputCadFiles {
		outDir, err := os.MkdirTemp("", "aconv-")
		if err != nil {
			slog.ErrorCtx(ctx, err.Error(), slog.String("folder", outDir))
			return err
		}

		err = AcadToPdf(ctx, file, outDir)
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}

		cleanName := strings.TrimSuffix(file, filepath.Ext(file))
		slog.Debug(outputFolder, cleanName+".pdf")

		err = pdf.Merge(ctx, outDir, filepath.Join(outputFolder, filepath.Base(cleanName)+".pdf"))
		if err != nil {
			slog.ErrorCtx(ctx, err.Error())
			return err
		}
		err = os.RemoveAll(outDir)
		if err != nil {
			slog.InfoCtx(ctx, err.Error(), slog.String("folder", outDir))
		}

	}
	return nil
}
