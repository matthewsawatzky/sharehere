package config

import "testing"

func TestNormalizeBasePath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "empty", in: "", want: "/"},
		{name: "root", in: "/", want: "/"},
		{name: "segment", in: "files", want: "/files"},
		{name: "leading and trailing slash", in: "/files/", want: "/files"},
		{name: "scheme relative input", in: "//static", want: "/static"},
		{name: "multiple slashes", in: "///files//", want: "/files"},
		{name: "absolute url with path", in: "https://example.test/sharehere/", want: "/sharehere"},
		{name: "absolute url with no path", in: "https://example.test", want: "/"},
		{name: "path with query and fragment", in: "/files/?q=1#top", want: "/files"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NormalizeBasePath(tt.in)
			if got != tt.want {
				t.Fatalf("NormalizeBasePath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}
