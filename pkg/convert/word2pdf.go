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
func WordToPdf(ctx context.Context, fromWordFile, toPdfFile string) error {
	fromFile, err := filepath.Abs(fromWordFile)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	toFile, err := filepath.Abs(toPdfFile)
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

	word, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}
	defer word.Release()

	oleutil.PutProperty(word, "Visible", false)        //Отключаем видимость окна
	oleutil.PutProperty(word, "DisplayAlerts", 0)      //Отключаем вывод предупреждений
	oleutil.PutProperty(word, "ScreenUpdating", false) //Отключаем обновление экрана
	oleutil.PutProperty(word, "AutomationSecurity", 3) //Отключаем макросы в документе
	//oleutil.PutProperty(word, "CreateBookmarks", 2)    //Отключаем макросы в документе

	workspacev, err := oleutil.GetProperty(word, "Documents")
	workspace := workspacev.ToIDispatch()
	defer workspace.Release()

	//Открываем файл
	openArguments := []interface{}{fromFile}
	wordFilev, err := oleutil.CallMethod(workspace, "Open", openArguments...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error(), slog.String("file", fromFile))
		return err
	}
	wordFile := wordFilev.ToIDispatch()
	defer wordFile.Release()

	//Экспортируем файл
	//https://learn.microsoft.com/ru-ru/dotnet/api/microsoft.office.interop.word._document.exportasfixedformat?view=word-pia#microsoft-office-interop-word-document-exportasfixedformat(system-string-microsoft-office-interop-word-wdexportformat-system-boolean-microsoft-office-interop-word-wdexportoptimizefor-microsoft-office-interop-word-wdexportrange-system-int32-system-int32-microsoft-office-interop-word-wdexportitem-system-boolean-system-boolean-microsoft-office-interop-word-wdexportcreatebookmarks-system-boolean-system-boolean-system-boolean-system-object@)
	exportArguments := []interface{}{toFile, //OutputFileName
		17, //ExportFormat
		0,  //OpenAfterExport
		0,  //OptimizeFor
		0,  //Range
		0,  //From
		0,  //To
		0,  //Item
		0,  //IncludeDocProps
		0,  //KeepIRM
		1}  //CreateBookmarks

	_, err = oleutil.CallMethod(wordFile, "ExportAsFixedFormat", exportArguments...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error(), slog.String("file", fromFile))
		return err
	}

	//Закрываем файл
	closeArguments := []interface{}{}
	_, err = oleutil.CallMethod(wordFile, "Close", closeArguments...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	//Закрываем приложение
	quitArguments := []interface{}{}
	_, err = oleutil.CallMethod(word, "Quit", quitArguments...)
	if err != nil {
		slog.ErrorCtx(ctx, err.Error())
		return err
	}

	slog.DebugCtx(ctx, "Файл сконвертирован", slog.String("file", filepath.Base(toPdfFile)))

	return nil
}
