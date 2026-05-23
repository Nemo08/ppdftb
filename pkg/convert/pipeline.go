//go:build windows

package convert

import (
	"context"
	"fmt"
	"os"
	"strings"

	cache "github.com/Nemo08/ppdftb/pkg/cache"
	"github.com/Nemo08/ppdftb/pkg/wordpool"
)

// WconvPipeline — параметры полного цикла wconv.
type WconvPipeline struct {
	Src      string   // папка с исходными DOCX-шаблонами
	Out      string   // папка для готовых PDF
	Outd     string   // папка для промежуточных DOCX (если пуста — временная)
	DxFlags  []string // файлы данных XML (флаг -i)
	DxF      string   // папка с XML-данными (флаг -x)
	DxL      int      // уровень поиска XML (флаг -u)
	PicsDir  string   // папка с картинками для шаблонов (флаг -p)
	UseCache bool     // использовать инкрементальный кэш (флаг -c)
}

// RunWconvWithPool выполняет полный цикл wconv с переданным WordPool.
func RunWconvWithPool(ctx context.Context, pool *wordpool.WordPool, p *WconvPipeline) error {
	outDir := strings.TrimRight(p.Out, `/\`)
	outdDir := strings.TrimRight(p.Outd, `/\`)

	tempDir, err := resolveTempDir(outdDir)
	if err != nil {
		return err
	}
	if outdDir == "" {
		defer os.RemoveAll(tempDir)
	}

	var data [][]byte
	var xmlPaths []string

	if p.DxF != "" {
		xData, xPaths, err := FindXMLFiles(p.DxF, p.DxL)
		if err != nil {
			return fmt.Errorf("XML из папки: %w", err)
		}
		data = append(data, xData...)
		xmlPaths = append(xmlPaths, xPaths...)
	}

	if len(p.DxFlags) > 0 {
		iData, err := GetDataContent(ctx, p.DxFlags)
		if err != nil {
			return fmt.Errorf("файлы данных: %w", err)
		}
		data = append(data, iData...)
		xmlPaths = append(xmlPaths, p.DxFlags...)
	}

	var mergedData []byte
	if len(data) > 0 {
		mergedData, err = DataMerge(data)
		if err != nil {
			return fmt.Errorf("слияние данных: %w", err)
		}
	}

	sources := []string{p.Src}
	var toConvert []string
	if p.UseCache {
		toConvert, err = cache.FilesToConvert(sources, xmlPaths, outDir, false)
		if err != nil {
			return fmt.Errorf("проверка кэша: %w", err)
		}
	} else {
		toConvert, err = CollectWordFiles(sources)
		if err != nil {
			return fmt.Errorf("сбор файлов: %w", err)
		}
	}

	if len(toConvert) == 0 {
		return nil
	}

	if err := TplToDocxJJack3(ctx, toConvert, tempDir, mergedData, p.PicsDir); err != nil {
		return fmt.Errorf("шаблонизация: %w", err)
	}

	if err := FilesToPdfWithPool(ctx, pool, []string{tempDir}, outDir); err != nil {
		return fmt.Errorf("конвертация в PDF: %w", err)
	}

	if p.UseCache {
		if _, err := cache.CommitCache(sources, nil, false); err != nil {
			return fmt.Errorf("сохранение кэша: %w", err)
		}
	}
	return nil
}

func resolveTempDir(outd string) (string, error) {
	if outd != "" {
		return outd, nil
	}
	return os.MkdirTemp(os.TempDir(), "wconv")
}
