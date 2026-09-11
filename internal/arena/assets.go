package arena

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
)

//go:embed assets/*
var assets embed.FS

func renderPage(csrfToken string) ([]byte, error) {
	source, err := assets.ReadFile("assets/index.html")
	if err != nil {
		return nil, fmt.Errorf("read arena page: %w", err)
	}
	tmpl, err := template.New("arena").Parse(string(source))
	if err != nil {
		return nil, fmt.Errorf("parse arena page: %w", err)
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, struct{ CSRF string }{CSRF: csrfToken}); err != nil {
		return nil, fmt.Errorf("render arena page: %w", err)
	}
	return output.Bytes(), nil
}

func asset(name string) ([]byte, error) {
	if name != "app.css" && name != "app.js" {
		return nil, fmt.Errorf("unknown arena asset %q", name)
	}
	return assets.ReadFile("assets/" + name)
}
