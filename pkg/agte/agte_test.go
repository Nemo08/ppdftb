package agte

import (
	//	"context"
	"errors"
	//"path/filepath"
	"testing"
)

const testPath = "test_data"

type testBytesPair struct {
	name      string
	input     string
	want      string
	wantError error
}

var testFiles = []string{"test1.docx"}

/*
func TestNewTemplate(t *testing.T) {
	_ = NewTemplate(context.Background())
}

func TestOpen(t *testing.T) {
	tpl := NewTemplate(context.Background())
	for _, v := range testFiles {
		err := tpl.Open(filepath.Join(testPath, v))
		if err != nil {
			t.Error("error open file with " + err.Error())
		}
		t.Logf("read file %s", filepath.Join(testPath, v))
		if tpl.path != filepath.Join(testPath, v) {
			t.Error("Paths not equal")
		}

		for path, _ := range tpl.xmlFiles {
			t.Logf("read xml subfile %s", path)
			if filepath.Ext(path) != ".xml" {
				t.Errorf("file list in template have non-xml file %s", path)
			}
		}
	}
}

func TestCutBrackets(t *testing.T) {
	var tests = []testBytesPair{
		{"ok 1", "<rtre>", "", nil},
		{"ok 2", "<fdg><fg><hg>", "", nil},
		{"ok 3", "", "", nil},
		{"ok 4", "<fdg/><fg></hg>", "", nil},
		{"ok 5", "<fdg>hgh<fg><hg>", "hgh", nil},
		{"error 1", "<", "", IncorrectTagStructure},
		{"error 2", "<fdg><fgr>><hg>", "", IncorrectTagStructure},
		{"error 3", "<fg><gf><", "", IncorrectTagStructure},
		{"error 4", "<<fg><gf><", "", IncorrectTagStructure},
		{"error 5", "><", "", IncorrectTagStructure},
		{"error 6", "<<ghfhg>>", "", IncorrectTagStructure},
		{"error 7", "<<hgj>dfsdf>", "", IncorrectTagStructure},
	}

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			res, err := cutBrackets(v.input)

			if !errors.Is(err, v.wantError) {
				t.Errorf("got error '%s', want '%s'", err.Error(), v.wantError)
			}

			if string(res) != string(v.want) {
				t.Errorf("got '%s', want '%s'", string(res), string(v.want))
			}

		})
	}
}
*/

func TestNormalizePlaceholders(t *testing.T) {
	var tests = []testBytesPair{

		//{"ok 1", "ff", "ff", nil},
		{"ok 22", "ddf {{fdfdsfdsf}{}{}} ttr", "ddf {{fdfdsfdsf}}{}{} ttr", nil},
		/*	{"ok 2", "ddf {{fdfdsfdsf}} ttr", "ddf {{fdfdsfdsf}} ttr", nil},
			{"ok 3", "ddf <dd> {{<xdf>fdfds</bbc>fdsf}} ttr", "ddf <dd> {{fdfdsfdsf}}<xdf></bbc> ttr", nil},
			{"ok 4", "ddf {{fdfdsfdsf}}gg{{ddd}} ttr", "ddf {{fdfdsfdsf}}gg{{ddd}} ttr", nil},
			{"ok 17", "xxx<br>{<br>{sdas}}yyy {<br>{eeee}<d>}dd", "xxx<br>{{sdas}}<br>yyy {{eeee}}<br><d>dd", nil},

			{"ok 4", "{{dfds}} dsfsd {{dd}}", "{{dfds}} dsfsd {{dd}}", nil},

			{"ok 3", "ddf{{fdfdsfdsf}}jhjh{{}}", "ddf{{fdfdsfdsf}}jhjh{{}}", nil},
			{"ok 5", "{", "{", nil},
			{"ok 6", "}{", "}{", nil},
			{"ok 7", "dsadsa {{sdas<br>}} dsfs", "dsadsa {{sdas}}<br> dsfs", nil},
			{"ok 8", "dsadsa {{</br>sdas<br>}} dsfs", "dsadsa {{sdas}}</br><br> dsfs", nil},
			{"ok 9", "dsadsa {{</br> sdas <br>}} dsfs", "dsadsa {{ sdas }}</br><br> dsfs", nil},
			{"ok 10", "dsadsa {{</br> sdas <br>}} ds <s> fs", "dsadsa {{ sdas }}</br><br> ds <s> fs", nil},
			{"ok 11", "dsa<ls>dsa {{</br> sdas <br>}} ds <s> fs", "dsa<ls>dsa {{ sdas }}</br><br> ds <s> fs", nil},
			{"ok 12", "dsadsa {{sdas<br>}} ds {{ss}} fs", "dsadsa {{sdas}}<br> ds {{ss}} fs", nil},
			{"ok 13", "dsadsa {{sdas<br>}} ds {{<dem>s</t>s}} fs", "dsadsa {{sdas}}<br> ds {{ss}}<dem></t> fs", nil},
			{"ok 14", "{<br>{sdas}}", "{{sdas}}<br>", nil},
			{"ok 15", "dsadsa {<r>{sdas<br>}<r>} ds <m>{{ss}} fs", "dsadsa {{sdas}}<r><br><r> ds <m>{{ss}} fs", nil},
			{"ok 16", "{{sdas}<br>}", "{{sdas}}<br>", nil},

			{"error 1", "{{}", "", BracketCountNotEqual},
			{"error 2", "{}}", "", BracketCountNotEqual},
			{"error 3", "{}{}{}}", "", BracketCountNotEqual},*/
	}
	beginPattern := "{{"
	endPattern := "}}"

	for _, v := range tests {
		t.Run(v.name, func(t *testing.T) {
			res, err := normalizePlaceholders(v.input, beginPattern, endPattern)
			if err != nil {
				if !errors.Is(err, v.wantError) {
					t.Errorf("got error '%s', want '%s'", err.Error(), v.wantError)
				}
			} else {
				if string(res) != string(v.want) {
					t.Errorf("got '%s', want '%s'", string(res), string(v.want))
				}
			}
		})
	}
}

/*
func TestRender(t *testing.T) {
	for _, v := range testFiles {
		tpl := NewTemplate(context.Background())
		err := tpl.Open(filepath.Join(testPath, v))
		if err != nil {
			t.Error("error open file with " + err.Error())
		}
		t.Logf("read file %s", filepath.Join(testPath, v))
		if tpl.path != filepath.Join(testPath, v) {
			t.Error("Paths not equal")
		}

		for xmlPath, _ := range tpl.xmlFiles {
			t.Logf("read xml subfile %s", xmlPath)
			if filepath.Ext(xmlPath) != ".xml" {
				t.Errorf("file list in template have non-xml file %s", xmlPath)
			}
		}

		data := map[string]string{
			"One":  "3711",
			"Two":  "2fgdfgdfg138",
			"Four": "44444",
			"adg":  "912",
		}

		err = tpl.Render(data)
		tpl.SaveTo("C:\\Users\\dormi\\YandexDisk\\Dev\\doctpl\\test_data\\" + v + ".docx")
	}
}
*/
