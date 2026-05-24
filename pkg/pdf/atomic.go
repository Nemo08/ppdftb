package pdf

import "github.com/Nemo08/ppdftb/pkg/fileutil"

func WriteFileAtomic(path string, fn func(tmpPath string) error) error {
	return fileutil.WriteFileAtomic(path, fn)
}
