package dataconv

import (
	"encoding/json"
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
			_ = json.Unmarshal(got, &gotNorm)
			_ = json.Unmarshal([]byte(tt.want), &wantNorm)

			gotJSON, _ := json.Marshal(gotNorm)
			wantJSON, _ := json.Marshal(wantNorm)
			if string(gotJSON) != string(wantJSON) {
				t.Errorf("DataMerge() = %s, want %s", gotJSON, wantJSON)
			}
		})
	}
}
