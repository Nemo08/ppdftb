package acadpool

import (
	"testing"
)

func TestStrReplace(t *testing.T) {
	replacesMu.Lock()
	replaces = map[string]string{
		"Name":   "Имя",
		"Number": "66955",
	}
	replacesMu.Unlock()

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
			got := StrReplace(tt.in)
			if got != tt.want {
				t.Errorf("StrReplace(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestStrReplaceConcurrent(t *testing.T) {
	t.Parallel()
	done := make(chan struct{}, 2)
	go func() {
		replacesMu.Lock()
		replaces["Name"] = "TestName"
		replacesMu.Unlock()
		done <- struct{}{}
	}()
	go func() {
		StrReplace(`\{\{Name\}\}`)
		done <- struct{}{}
	}()
	<-done
	<-done
}
