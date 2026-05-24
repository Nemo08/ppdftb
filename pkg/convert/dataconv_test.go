package convert

import (
	"encoding/json"
	"testing"
)

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
