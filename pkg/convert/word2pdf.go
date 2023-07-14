package convert

import (
	"context"
	"path/filepath"
	"runtime"

	"golang.org/x/exp/slog"

	ole "github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
)

// WordConvertToPdf принимает путь fromFile к файлу word, конвертирует в pdf файл toFile
func WordToPdf(ctx context.Context, ff, tf string) error {

	fromFile, err := filepath.Abs(ff)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	toFile, err := filepath.Abs(tf)
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
	unknown, err := oleutil.CreateObject("Word.Application")
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	word := unknown.MustQueryInterface(ole.IID_IDispatch)
	defer word.Release()

	oleutil.PutProperty(word, "Visible", false)        //Отключаем видимость окна
	oleutil.PutProperty(word, "DisplayAlerts", 0)      //Отключаем вывод предупреждений
	oleutil.PutProperty(word, "ScreenUpdating", false) //Отключаем обновление экрана
	oleutil.PutProperty(word, "AutomationSecurity", 3) //Отключаем макросы в документе

	workspace := oleutil.MustGetProperty(word, "Documents").ToIDispatch()
	defer workspace.Release()

	//Открываем файл
	openArguments := []interface{}{fromFile}
	wordFile := oleutil.MustCallMethod(workspace, "Open", openArguments...).ToIDispatch()
	defer wordFile.Release()

	//Экспортируем файл
	exportArguments := []interface{}{toFile, 17}
	oleutil.MustCallMethod(wordFile, "ExportAsFixedFormat", exportArguments...)

	//Закрываем файл
	closeArguments := []interface{}{}
	oleutil.MustCallMethod(wordFile, "Close", closeArguments...)

	//Закрываем приложение
	quitArguments := []interface{}{}
	oleutil.MustCallMethod(word, "Quit", quitArguments...)

	slog.InfoCtx(ctx, "Файл сконвертирован", slog.String("file", filepath.Base(tf)))

	return nil
}
