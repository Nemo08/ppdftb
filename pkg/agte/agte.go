package agte

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"golang.org/x/exp/slices"
	"golang.org/x/exp/slog"
)

var excludeXmlFiles = []string{
	"styles.xml",
	"fontTable.xml",
	"settings.xml",
	"app.xml",
	"theme1.xml",
	"webSettings.xml",
	"[Content_Types].xml",
}

type tplOpt func(t *template.Template)

var (
	IncorrectTagStructure     = errors.New("error with tag structure")
	IncorrectBracketStructure = errors.New("error with bracket structure")
	NestedBracketStructure    = errors.New("error with nested bracket structure")
	BracketCountNotEqual      = errors.New("error with brackets count")
)

type PatternPairPos struct {
	beginPos int
	endPos   int
}

type Template struct {
	path, name, ext          string
	ziprc                    *zip.ReadCloser
	zipw                     *zip.Writer
	xmlFiles                 map[string]string
	modifiedXmlFiles         map[string]string
	beginPattern, endPattern string
	ctx                      context.Context
	log                      *slog.Logger
	tpl                      *template.Template
}

// NewTemplate creates new template object
func NewTemplate(ctx context.Context, opts ...tplOpt) *Template {
	t := Template{}
	t.xmlFiles = make(map[string]string)
	t.modifiedXmlFiles = make(map[string]string)
	t.beginPattern = "{{"
	t.endPattern = "}}"
	t.ctx = ctx
	t.log = slog.New(slog.NewTextHandler(os.Stdout, nil))
	t.tpl = template.New("").Option("missingkey=zero")

	for _, v := range opts {
		v(t.tpl)
	}

	return &t
}

func (t *Template) Open(docPath string) error {
	var err error
	t.log.DebugContext(t.ctx, "open file", slog.String("file", docPath))
	//open file
	if t.ziprc, err = zip.OpenReader(docPath); err != nil {
		return err
	}
	defer t.ziprc.Close()

	t.path = docPath
	t.name = strings.TrimSuffix(filepath.Base(docPath), filepath.Ext(docPath))
	t.ext = filepath.Ext(docPath)

	for _, file := range t.ziprc.File {
		// add all xml files
		var extensionLength int
		var extension string

		if extensionLength = strings.LastIndex(file.FileInfo().Name(), "."); extensionLength > 0 {
			extension = file.FileInfo().Name()[extensionLength:]
		}

		if (extension == ".xml") && (!slices.Contains(excludeXmlFiles, file.FileInfo().Name())) {
			xfile, err := file.Open()
			if err != nil {
				t.log.ErrorCtx(t.ctx, err.Error())
				return err
			}
			defer xfile.Close()

			content, err := ioutil.ReadAll(xfile)
			if err != nil {
				t.log.ErrorCtx(t.ctx, err.Error())
				return err
			}

			t.xmlFiles[file.FileHeader.Name] = string(content)
			t.log.DebugContext(t.ctx, "add subfile", slog.String("subfile", file.FileHeader.Name))
		}
	}
	return nil
}

func (t *Template) Render(params map[string]string) error {
	for path, content := range t.xmlFiles {
		err := t.check(content)
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error())
			return err
		}

		normContent, err := normalizePlaceholders(content, t.beginPattern, t.endPattern)
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error())
			return err
		}

		ftpl, err := t.tpl.Parse(normContent)
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error())
			return err
		}
		var buf bytes.Buffer

		err = ftpl.Execute(&buf, params)
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error())
			return err
		}

		t.modifiedXmlFiles[path] = buf.String()
	}

	return nil
}

func cutBrackets(content string) (string, error) {
	if strings.Count(content, "<") != strings.Count(content, ">") {
		return "", IncorrectTagStructure
	}

	ebs := ""
	for {
		bb := strings.IndexRune(content, []rune("<")[0])
		bn := strings.IndexRune(content, []rune(">")[0])
		if (bb == -1) || (bn == -1) {
			break
		}
		if bb > bn {
			return "", IncorrectTagStructure
		}
		data := content[bb : bn+1]
		//check, where is in data other inner bracket
		if (strings.Count(data, "<") != 1) || (strings.Count(data, ">") != 1) {
			return "", IncorrectTagStructure
		}
		content = strings.ReplaceAll(content, data, ebs)
	}
	return content, nil
}

func cutRN(content []byte) []byte {
	return bytes.ReplaceAll(content, []byte("\r\n"), []byte(""))
}

func normalizePlaceholders(content string, beginPattern, endPattern string) (string, error) {
	out := content
	ppos, err := findPattern(content, beginPattern, endPattern)
	if err != nil {
		return out, err
	}

	for _, v := range ppos {
		o := moveTags(string(content[v.beginPos : v.endPos+len(endPattern)]))
		out = out[:v.beginPos] + o + out[v.endPos+len(endPattern):]
	}
	return out, nil
}

func findPattern(content string, beginPattern, endPattern string) ([]PatternPairPos, error) {
	var bArr, eArr []int
	strContent := content
	var out []PatternPairPos
	begCount := -1
	endCount := -1
	var begPos int
	var endPos int
	inTag := false
	_ = endPos

	for pos, r := range strContent {
		if (r == []rune(beginPattern)[0]) && begCount == -1 {
			begPos = pos
			begCount = 0
		}
		if begCount != -1 {
			if r == []rune("<")[0] {
				inTag = true
			}
			if r == []rune(">")[0] {
				inTag = false
			}

			if (r == []rune(beginPattern)[begCount]) && (inTag == false) {
				begCount += 1
			}
			if begCount == len(beginPattern) {
				begCount = -1
				bArr = append(bArr, begPos)
			}
		}

		if (r == []rune(endPattern)[0]) && endCount == -1 {
			endPos = pos
			endCount = 0
		}
		if endCount != -1 {
			if r == []rune("<")[0] {
				inTag = true
			}
			if r == []rune(">")[0] {
				inTag = false
			}

			if (r == []rune(endPattern)[endCount]) && (inTag == false) {
				endCount += 1
			}
			if endCount == len(endPattern) {
				endCount = -1
				eArr = append(eArr, pos-1)
			}
		}
	}

	if len(bArr) != len(eArr) {
		return out, BracketCountNotEqual
	}

	for pos, _ := range bArr {
		if bArr[pos] > eArr[pos] {
			return out, BracketCountNotEqual
		}
	}

	for pos, _ := range bArr {
		out = append(out, PatternPairPos{bArr[pos], eArr[pos]})
	}
	return out, nil
}

func moveTags(content string) string {
	var out, tags string
	var cut int
	for {
		b := strings.Index(content[cut:], "<")
		e := strings.Index(content[cut:], ">")

		if b != -1 && e != -1 {
			tag := content[b+cut : e+cut+len(">")]
			tags += tag
			out += content[cut : cut+b]
			cut += e + len(">")
		} else {
			out += content[cut+e+len(">"):] + tags
			break
		}

	}
	return out
}

func (t *Template) check(content string) error {
	if strings.Count(content, "<") != strings.Count(content, ">") {
		t.log.ErrorCtx(t.ctx, IncorrectTagStructure.Error())
		return IncorrectTagStructure
	}
	return nil
}

func (t *Template) SaveTo(path string) error {
	outFile, err := os.Create(path)
	if err != nil {
		t.log.ErrorCtx(t.ctx, err.Error())
		return err
	}
	defer outFile.Close()

	t.zipw = zip.NewWriter(outFile)
	defer t.zipw.Close()

	if t.ziprc, err = zip.OpenReader(t.path); err != nil {
		t.log.ErrorCtx(t.ctx, err.Error())
		return err
	}

	for _, zipItem := range t.ziprc.File {
		_, modified := t.modifiedXmlFiles[zipItem.FileHeader.Name]
		if !modified {
			zipItemReader, err := zipItem.OpenRaw()
			if err != nil {
				t.log.ErrorCtx(t.ctx, err.Error())
				return err
			}

			targetItem, err := t.zipw.CreateRaw(&zipItem.FileHeader)
			if err != nil {
				t.log.ErrorCtx(t.ctx, err.Error())
				return err
			}

			_, err = io.Copy(targetItem, zipItemReader)
			if err != nil {
				t.log.ErrorCtx(t.ctx, err.Error())
				return err
			}
		}
	}
	t.zipw.Flush()

	for path, content := range t.modifiedXmlFiles {
		file, err := t.zipw.Create(path)
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error())
			return err
		}
		_, err = file.Write([]byte(content))
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error())
			return err
		}
	}
	t.zipw.Flush()

	return nil
}
