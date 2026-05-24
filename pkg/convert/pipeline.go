//go:build windows

package convert

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/Nemo08/ppdftb/pkg/dataconv"
	"github.com/Nemo08/ppdftb/pkg/fileutil"
)

// WconvPipeline — параметры полного цикла wconv.
type WconvPipeline struct {
	Src      string    // папка с исходными DOCX-шаблонами
	Out      string    // папка для готовых PDF
	Outd     string    // папка для промежуточных DOCX (если пуста — временная)
	DxFlags  []string  // файлы данных XML (флаг -i)
	DxF      string    // папка с XML-данными (флаг -x)
	DxL      int       // уровень поиска XML (флаг -u)
	PicsDir  string    // папка с картинками для шаблонов (флаг -p)
	UseCache bool      // использовать инкрементальный кэш (флаг -c)
	Cache    ConvCache // реализация кэша (nil = кэш отключён)
}

// RunWconvWithPool выполняет полный цикл wconv с переданным WordPool.
func RunWconvWithPool(ctx context.Context, pool WordConverter, p *WconvPipeline) error {
	outDir := strings.TrimRight(p.Out, `/\`)
	outdDir := strings.TrimRight(p.Outd, `/\`)

	tempDir, err := resolveTempDir(outdDir)
	if err != nil {
		return err
	}
	if outdDir == "" {
		defer os.RemoveAll(tempDir)
	}

	mergedData, xmlPaths, err := collectAndMergeXML(ctx, p)
	if err != nil {
		return err
	}

	sources := []string{p.Src}
	toConvert, err := resolveFilesToConvert(p, sources, xmlPaths, outDir)
	if err != nil {
		return err
	}
	if len(toConvert) == 0 {
		return nil
	}

	if err := TplToPdfWithPool(ctx, pool, toConvert, tempDir, outDir, mergedData, p.PicsDir); err != nil {
		return fmt.Errorf("шаблонизация → PDF: %w", err)
	}
	return commitCache(p, sources)
}

func collectAndMergeXML(ctx context.Context, p *WconvPipeline) (mergedData []byte, xmlPaths []string, err error) {
	var data [][]byte

	if p.DxF != "" {
		xData, xPaths, err := fileutil.FindXMLFiles(p.DxF, p.DxL)
		if err != nil {
			return nil, nil, fmt.Errorf("XML из папки: %w", err)
		}
		data = append(data, xData...)
		xmlPaths = append(xmlPaths, xPaths...)
	}

	if len(p.DxFlags) > 0 {
		iData, err := fileutil.GetDataContent(ctx, p.DxFlags)
		if err != nil {
			return nil, nil, fmt.Errorf("файлы данных: %w", err)
		}
		data = append(data, iData...)
		xmlPaths = append(xmlPaths, p.DxFlags...)
	}

	if len(data) > 0 {
		merged, err := dataconv.DataMerge(data)
		if err != nil {
			return nil, nil, fmt.Errorf("слияние данных: %w", err)
		}
		return merged, xmlPaths, nil
	}
	return nil, xmlPaths, nil
}

func resolveFilesToConvert(p *WconvPipeline, sources, xmlPaths []string, outDir string) ([]string, error) {
	switch {
	case p.UseCache && p.Cache != nil:
		toConvert, err := p.Cache.FilesToConvert(sources, xmlPaths, outDir, false)
		if err != nil {
			return nil, fmt.Errorf("проверка кэша: %w", err)
		}
		return toConvert, nil
	case p.UseCache && p.Cache == nil:
		return nil, fmt.Errorf("cache включён (-c), но реализация не предоставлена")
	default:
		toConvert, err := fileutil.CollectWordFiles(sources)
		if err != nil {
			return nil, fmt.Errorf("сбор файлов: %w", err)
		}
		return toConvert, nil
	}
}

func commitCache(p *WconvPipeline, sources []string) error {
	if !p.UseCache || p.Cache == nil {
		return nil
	}
	if err := p.Cache.CommitCache(sources, nil, false); err != nil {
		return fmt.Errorf("сохранение кэша: %w", err)
	}
	return nil
}

func resolveTempDir(outd string) (string, error) {
	if outd != "" {
		return outd, nil
	}
	return os.MkdirTemp(os.TempDir(), "wconv")
}
