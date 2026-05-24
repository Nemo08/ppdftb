//go:build windows

package convert

import (
	"context"

	"github.com/Nemo08/ppdftb/pkg/fileutil"
)

// GetDataContent читает файлы из source и возвращает их содержимое как [][]byte.
func GetDataContent(ctx context.Context, source []string) ([][]byte, error) {
	return fileutil.GetDataContent(ctx, source)
}

// FindXMLFiles поднимается вверх по ФС от startDir на steps шагов,
// собирает XML-файлы из каждой директории в алфавитном порядке
// и возвращает их содержимое от верхнего уровня к startDir.
func FindXMLFiles(startDir string, steps int) ([][]byte, []string, error) {
	return fileutil.FindXMLFiles(startDir, steps)
}

// CollectFiles собирает файлы с указанными расширениями из списка источников.
func CollectFiles(sources []string, exts []string, skipPrefix ...string) ([]string, error) {
	return fileutil.CollectFiles(sources, exts, skipPrefix...)
}

// CollectWordFiles собирает все *.doc/*.docx/*.rtf из списка файлов и папок.
func CollectWordFiles(sources []string) ([]string, error) {
	return fileutil.CollectWordFiles(sources)
}

// CollectCadFiles собирает DWG/DXF из файла или папки.
func CollectCadFiles(sourceFile, sourceFolder string) ([]string, error) {
	return fileutil.CollectCadFiles(sourceFile, sourceFolder)
}
