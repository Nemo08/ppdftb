package convert

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMergeMaps(t *testing.T) {
	tests := []struct {
		name string
		dst  map[string]any
		src  map[string]any
		want map[string]any
	}{
		{
			name: "add new key",
			dst:  map[string]any{"a": "1"},
			src:  map[string]any{"b": "2"},
			want: map[string]any{"a": "1", "b": "2"},
		},
		{
			name: "override scalar",
			dst:  map[string]any{"a": "old"},
			src:  map[string]any{"a": "new"},
			want: map[string]any{"a": "new"},
		},
		{
			name: "recursive merge",
			dst:  map[string]any{"n": map[string]any{"x": "1", "y": "2"}},
			src:  map[string]any{"n": map[string]any{"y": "3", "z": "4"}},
			want: map[string]any{"n": map[string]any{"x": "1", "y": "3", "z": "4"}},
		},
		{
			name: "empty src",
			dst:  map[string]any{"a": "1"},
			src:  map[string]any{},
			want: map[string]any{"a": "1"},
		},
		{
			name: "empty dst",
			dst:  map[string]any{},
			src:  map[string]any{"a": "1"},
			want: map[string]any{"a": "1"},
		},
		{
			name: "scalar overrides map",
			dst:  map[string]any{"k": map[string]any{"sub": "v"}},
			src:  map[string]any{"k": "flat"},
			want: map[string]any{"k": "flat"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mergeMaps(tt.dst, tt.src)
			got := tt.dst

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("mergeMaps() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestXmlToMap(t *testing.T) {
	tests := []struct {
		name    string
		xml     string
		want    map[string]any
		wantErr bool
	}{
		{
			name: "single text child",
			xml:  `<root><name>John</name></root>`,
			want: map[string]any{"name": "John"},
		},
		{
			name: "nested elements",
			xml:  `<root><a><b>deep</b></a></root>`,
			want: map[string]any{"a": map[string]any{"b": "deep"}},
		},
		{
			name: "duplicate siblings become array",
			xml:  `<root><item>a</item><item>b</item></root>`,
			want: map[string]any{"item": []any{"a", "b"}},
		},
		{
			name: "three duplicates",
			xml:  `<root><x>1</x><x>2</x><x>3</x></root>`,
			want: map[string]any{"x": []any{"1", "2", "3"}},
		},
		{
			name: "empty element",
			xml:  `<root><empty></empty></root>`,
			want: map[string]any{"empty": ""},
		},
		{
			name:    "invalid xml",
			xml:     `<root><unclosed>`,
			wantErr: true,
		},
		{
			name: "mixed content text ignored when children exist",
			xml:  `<root>before<tag>value</tag>after</root>`,
			want: map[string]any{"tag": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := xmlToMap(strings.NewReader(tt.xml))
			if (err != nil) != tt.wantErr {
				t.Fatalf("xmlToMap() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			gotJSON, _ := json.Marshal(got)
			wantJSON, _ := json.Marshal(tt.want)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("xmlToMap() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestDataMerge(t *testing.T) {
	tests := []struct {
		name    string
		data    [][]byte
		want    string
		wantErr bool
	}{
		{
			name: "single document",
			data: [][]byte{
				[]byte(`<root><name>Alice</name></root>`),
			},
			want: `{"name":"Alice"}`,
		},
		{
			name: "second overrides first",
			data: [][]byte{
				[]byte(`<root><name>Alice</name><age>30</age></root>`),
				[]byte(`<root><name>Bob</name></root>`),
			},
			want: `{"age":"30","name":"Bob"}`,
		},
		{
			name: "three docs merging",
			data: [][]byte{
				[]byte(`<root><a>1</a><b>2</b></root>`),
				[]byte(`<root><b>22</b><c>3</c></root>`),
				[]byte(`<root><d>4</d></root>`),
			},
			want: `{"a":"1","b":"22","c":"3","d":"4"}`,
		},
		{
			name:    "empty input",
			data:    [][]byte{},
			wantErr: true,
		},
		{
			name: "nested merge",
			data: [][]byte{
				[]byte(`<root><addr><city>NYC</city><zip>10001</zip></addr></root>`),
				[]byte(`<root><addr><city>LA</city><state>CA</state></addr></root>`),
			},
			want: `{"addr":{"city":"LA","state":"CA","zip":"10001"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DataMerge(tt.data)
			if (err != nil) != tt.wantErr {
				t.Fatalf("DataMerge() error = %v, wantErr = %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			var gotNorm, wantNorm any
			json.Unmarshal(got, &gotNorm)
			json.Unmarshal([]byte(tt.want), &wantNorm)

			gotJSON, _ := json.Marshal(gotNorm)
			wantJSON, _ := json.Marshal(wantNorm)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("DataMerge() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}

func TestHasExt(t *testing.T) {
	tests := []struct {
		name string
		ext  string
		exts []string
		want bool
	}{
		{"found", ".pdf", []string{".pdf", ".docx"}, true},
		{"not found", ".txt", []string{".pdf", ".docx"}, false},
		{"nil exts", ".pdf", nil, false},
		{"empty exts", ".pdf", []string{}, false},
		{"empty ext", "", []string{".pdf"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasExt(tt.ext, tt.exts); got != tt.want {
				t.Errorf("hasExt(%q, %v) = %v, want %v", tt.ext, tt.exts, got, tt.want)
			}
		})
	}
}

func TestHasAnyPrefix(t *testing.T) {
	tests := []struct {
		name     string
		s        string
		prefixes []string
		want     bool
	}{
		{"found", "~$test.docx", []string{"~$"}, true},
		{"not found", "test.docx", []string{"~$"}, false},
		{"nil prefixes", "test.docx", nil, false},
		{"empty prefixes", "test.docx", []string{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasAnyPrefix(tt.s, tt.prefixes); got != tt.want {
				t.Errorf("hasAnyPrefix(%q, %v) = %v, want %v", tt.s, tt.prefixes, got, tt.want)
			}
		})
	}
}

func TestCollectFiles(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "doc.docx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "sheet.xlsx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "~$temp.docx"), []byte{}, 0o644)
	os.WriteFile(filepath.Join(dir, "readme.txt"), []byte{}, 0o644)
	os.Mkdir(filepath.Join(dir, "sub"), 0o755)
	os.WriteFile(filepath.Join(dir, "sub", "nested.docx"), []byte{}, 0o644)

	t.Run("filter by .docx ext", func(t *testing.T) {
		files, err := CollectFiles([]string{dir}, []string{".docx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 2 {
			t.Errorf("CollectFiles() = %d files, want 2 (doc.docx, ~$temp.docx)", len(files))
		}
	})

	t.Run("filter by .xlsx ext", func(t *testing.T) {
		files, err := CollectFiles([]string{dir}, []string{".xlsx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Errorf("CollectFiles() = %d files, want 1 (sheet.xlsx)", len(files))
		}
	})

	t.Run("with skip prefix", func(t *testing.T) {
		files, err := CollectFiles([]string{dir}, []string{".docx", ".xlsx", ".txt"}, "~$")
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 3 {
			t.Errorf("CollectFiles() with skip prefix = %d files, want 3 (doc.docx, sheet.xlsx, readme.txt)", len(files))
		}
	})

	t.Run("single file source", func(t *testing.T) {
		files, err := CollectFiles([]string{filepath.Join(dir, "doc.docx")}, []string{".docx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 1 {
			t.Errorf("CollectFiles() = %d files, want 1", len(files))
		}
	})

	t.Run("non-existent source", func(t *testing.T) {
		_, err := CollectFiles([]string{filepath.Join(dir, "nope")}, []string{".docx"})
		if err == nil {
			t.Error("expected error for non-existent source")
		}
	})

	t.Run("empty sources", func(t *testing.T) {
		files, err := CollectFiles([]string{}, []string{".docx"})
		if err != nil {
			t.Fatal(err)
		}
		if len(files) != 0 {
			t.Errorf("CollectFiles() = %d files, want 0", len(files))
		}
	})
}
