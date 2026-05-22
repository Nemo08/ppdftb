//go:build windows

package acadpool

import "testing"

func TestStrReplace(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "replace Name",
			in:   `\{\{Name\}\}`,
			want: "Имя",
		},
		{
			name: "replace Number",
			in:   `\{\{Number\}\}`,
			want: "66955",
		},
		{
			name: "no placeholders",
			in:   "plain text",
			want: "plain text",
		},
		{
			name: "partial match no replace",
			in:   `\{\{Unknown\}\}`,
			want: `\{\{Unknown\}\}`,
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StrReplace(tt.in); got != tt.want {
				t.Errorf("StrReplace() = %q, want %q", got, tt.want)
			}
		})
	}
}
