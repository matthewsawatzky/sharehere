package server

import "testing"

func TestTemplateBasePath(t *testing.T) {
	tests := []struct {
		name     string
		basePath string
		want     string
	}{
		{name: "root", basePath: "/", want: ""},
		{name: "subpath", basePath: "/sharehere", want: "/sharehere"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &App{opts: Options{BasePath: tt.basePath}}
			got := app.templateBasePath()
			if got != tt.want {
				t.Fatalf("templateBasePath() = %q, want %q", got, tt.want)
			}
		})
	}
}
