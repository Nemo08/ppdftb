package agte

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"io/ioutil"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"time"

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

var letterDigitRunes = []rune("abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ123456789")

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

		content, spts_im := saveCurleBracketsFromTemplate(t.beginPattern, t.endPattern, content, []string{"{", "}", "%"})

		normContent, err := normalizePlaceholders(content, t.beginPattern, t.endPattern)
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error(), "normalize")
			return err
		}

		normContent, spts := savePatternFromTemplate(t.beginPattern, t.endPattern, normContent, []string{"{", "}", "%"})

		ftpl, err := t.tpl.Parse(normContent)
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error(), path, "parse")
			return err
		}
		var buf bytes.Buffer

		err = ftpl.Execute(&buf, params)
		if err != nil {
			t.log.ErrorCtx(t.ctx, err.Error(), "exec")
			return err
		}

		normContent = buf.String()
		normContent = returnPatternFromTemplate(normContent, spts)
		normContent = returnPatternFromTemplate(normContent, spts_im)
		t.modifiedXmlFiles[path] = normContent
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
		//если нашли тег
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
		//если нашли конец плейсхолдера
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

func init() {
	rand.Seed(time.Now().UnixNano())
}

// спасаем всякие символы
func savePatternFromTemplate(beginPattern, endPattern, content string, toSave []string) (string, map[string]string) {
	replMap := make(map[string]string)
	bprep := randStringRunes(19)
	eprep := randStringRunes(19)
	content = strings.ReplaceAll(content, beginPattern, bprep)
	content = strings.ReplaceAll(content, endPattern, eprep)
	for _, v := range toSave {
		rep := randStringRunes(19)
		content = strings.ReplaceAll(content, v, rep)
		replMap[rep] = v
	}
	content = strings.ReplaceAll(content, bprep, beginPattern)
	content = strings.ReplaceAll(content, eprep, endPattern)
	return content, replMap
}

// спасаем фигурные скобки из тегов, относящиеся к картинкам
func saveCurleBracketsFromTemplate(beginPattern, endPattern, content string, toSave []string) (string, map[string]string) {
	var inTag bool
	var leftpos, rightpos []int
	replMap := make(map[string]string)
	for k, v := range content {
		if v == []rune("<")[0] {
			inTag = true
		}
		if v == []rune(">")[0] {
			inTag = false
		}
		if v == []rune("{")[0] && inTag == true {
			leftpos = append(leftpos, k)
		}
		if v == []rune("}")[0] && inTag == true {
			rightpos = append(rightpos, k)
		}
	}
	//fmt.Println("poses", leftpos, rightpos)

	bprep := randStringRunes(19)
	replMap[bprep] = "{"
	eprep := randStringRunes(19)
	replMap[eprep] = "}"
	//fmt.Println(bprep, eprep)

	for index := len(leftpos) - 1; index >= 0; index-- {
		content = content[:rightpos[index]] + eprep + content[rightpos[index]+1:]
		content = content[:leftpos[index]] + bprep + content[leftpos[index]+1:]
	}
	//fmt.Println(content)
	/*

		content = strings.ReplaceAll(content, beginPattern, bprep)
		content = strings.ReplaceAll(content, endPattern, eprep)
		for _, v := range toSave {
			rep := randStringRunes(19)
			content = strings.ReplaceAll(content, v, rep)
			replMap[rep] = v
		}
		content = strings.ReplaceAll(content, bprep, beginPattern)
		content = strings.ReplaceAll(content, eprep, endPattern)*/
	return content, replMap
}

func returnPatternFromTemplate(content string, toReturn map[string]string) string {
	for k, v := range toReturn {
		content = strings.ReplaceAll(content, k, v)
	}
	return content
}

func randStringRunes(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterDigitRunes[rand.Intn(len(letterDigitRunes))]
	}
	return string(b)
}
