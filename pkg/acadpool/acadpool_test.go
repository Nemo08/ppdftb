package acadpool

import "testing"

func TestStrReplace(t *testing.T) {
	replaces := map[string]string{
		"Name":   "Имя",
		"Number": "66955",
	}

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"no placeholders", "plain text", "plain text"},
		{"single placeholder", `\{\{Name\}\}`, "Имя"},
		{"multiple placeholders", `\{\{Name\}\} \{\{Number\}\}`, "Имя 66955"},
		{"partial match", `prefix \{\{Name\}\} suffix`, "prefix Имя suffix"},
		{"no closing braces", `\{\{Name`, `\{\{Name`},
		{"unknown placeholder", `\{\{Unknown\}\}`, `\{\{Unknown\}\}`},
		{"unescaped braces ignored", `{{Name}}`, `{{Name}}`},
		{"empty input", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StrReplace(tt.in, replaces)
			if got != tt.want {
				t.Errorf("StrReplace(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStrReplaceOverride(t *testing.T) {
	got := StrReplace(`\{\{Name\}\}`, map[string]string{"Name": "Other"})
	if got != "Other" {
		t.Errorf("StrReplace() = %q, want %q", got, "Other")
	}
}
