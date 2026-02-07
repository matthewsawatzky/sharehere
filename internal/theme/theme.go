package theme

import "fmt"

type Theme struct {
	Name         string            `json:"name"`
	Label        string            `json:"label"`
	Description  string            `json:"description"`
	CSSVariables map[string]string `json:"css_variables"`
}

type Overrides struct {
	Background   string `json:"background"`
	Text         string `json:"text"`
	Accent       string `json:"accent"`
	Radius       string `json:"radius"`
	Font         string `json:"font"`
	Surface      string `json:"surface"`
	SurfaceMuted string `json:"surface_muted"`
	Border       string `json:"border"`
}

func builtins() map[string]Theme {
	return map[string]Theme{
		"light": {
			Name:        "light",
			Label:       "Light",
			Description: "Clean neutral palette with bright surfaces",
			CSSVariables: map[string]string{
				"--bg":           "#f8fafc",
				"--bg-elevated":  "#ffffff",
				"--text":         "#0f172a",
				"--muted":        "#475569",
				"--accent":       "#0f766e",
				"--accent-strong": "#115e59",
				"--border":       "#cbd5e1",
				"--radius":       "14px",
				"--font":         "'Atkinson Hyperlegible', 'Segoe UI', sans-serif",
			},
		},
		"dark": {
			Name:        "dark",
			Label:       "Dark",
			Description: "High contrast slate palette for low-light environments",
			CSSVariables: map[string]string{
				"--bg":           "#020617",
				"--bg-elevated":  "#111827",
				"--text":         "#e2e8f0",
				"--muted":        "#94a3b8",
				"--accent":       "#14b8a6",
				"--accent-strong": "#2dd4bf",
				"--border":       "#334155",
				"--radius":       "14px",
				"--font":         "'Atkinson Hyperlegible', 'Segoe UI', sans-serif",
			},
		},
		"sunset": {
			Name:        "sunset",
			Label:       "Sunset",
			Description: "Bold warm theme with copper accent",
			CSSVariables: map[string]string{
				"--bg":           "#fff7ed",
				"--bg-elevated":  "#ffffff",
				"--text":         "#3f1d0f",
				"--muted":        "#7c3f1a",
				"--accent":       "#ea580c",
				"--accent-strong": "#c2410c",
				"--border":       "#fdba74",
				"--radius":       "18px",
				"--font":         "'IBM Plex Sans', 'Segoe UI', sans-serif",
			},
		},
	}
}

func List() []Theme {
	items := make([]Theme, 0, len(builtins()))
	for _, t := range builtins() {
		items = append(items, t)
	}
	return items
}

func Resolve(name string, o Overrides) (Theme, error) {
	t, ok := builtins()[name]
	if !ok {
		return Theme{}, fmt.Errorf("unknown theme %q", name)
	}
	apply := func(key, value string) {
		if value != "" {
			t.CSSVariables[key] = value
		}
	}
	apply("--bg", o.Background)
	apply("--text", o.Text)
	apply("--accent", o.Accent)
	apply("--radius", o.Radius)
	apply("--font", o.Font)
	apply("--bg-elevated", o.Surface)
	apply("--border", o.Border)
	if o.SurfaceMuted != "" {
		t.CSSVariables["--muted"] = o.SurfaceMuted
	}
	return t, nil
}
