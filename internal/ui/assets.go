package ui

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
)

//go:embed assets/*
var embeddedAssets embed.FS

type pageTemplateData struct {
	Standalone bool
	CSS        template.CSS
	JS         template.JS
	Data       template.JS
}

func RenderPage(data any, standalone bool) ([]byte, error) {
	page, err := embeddedAssets.ReadFile("assets/index.html")
	if err != nil {
		return nil, fmt.Errorf("read embedded page: %w", err)
	}
	tmpl, err := template.New("report").Parse(string(page))
	if err != nil {
		return nil, fmt.Errorf("parse embedded page: %w", err)
	}
	view := pageTemplateData{Standalone: standalone}
	if standalone {
		css, cssErr := embeddedAssets.ReadFile("assets/app.css")
		if cssErr != nil {
			return nil, cssErr
		}
		javascript, jsErr := embeddedAssets.ReadFile("assets/app.js")
		if jsErr != nil {
			return nil, jsErr
		}
		serialized, jsonErr := json.Marshal(data)
		if jsonErr != nil {
			return nil, fmt.Errorf("encode report data: %w", jsonErr)
		}
		// CSS and JS are trusted embedded build assets. encoding/json escapes HTML
		// delimiters in untrusted report data before it is placed in a script block.
		view.CSS = template.CSS(css)
		view.JS = template.JS(javascript)
		view.Data = template.JS(serialized)
	}
	var output bytes.Buffer
	if err := tmpl.Execute(&output, view); err != nil {
		return nil, fmt.Errorf("render report page: %w", err)
	}
	return output.Bytes(), nil
}

func Asset(name string) ([]byte, error) {
	if name != "app.css" && name != "app.js" {
		return nil, fmt.Errorf("unknown UI asset %q", name)
	}
	return embeddedAssets.ReadFile("assets/" + name)
}
