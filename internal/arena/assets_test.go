package arena

import (
	"strings"
	"testing"
)

func TestFrontendAssetsAvoidExecutableUserHTMLAndExternalRuntime(t *testing.T) {
	javascript, err := asset("app.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, unsafe := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write"} {
		if strings.Contains(string(javascript), unsafe) {
			t.Fatalf("frontend uses unsafe HTML sink %q", unsafe)
		}
	}
	page, err := renderPage("csrf-test")
	if err != nil {
		t.Fatal(err)
	}
	text := string(page)
	if !strings.Contains(text, "csrf-test") || !strings.Contains(text, "messages are sent to both configured model providers") {
		t.Fatal("rendered page is missing security or data-boundary context")
	}
	if !strings.Contains(text, `id="model-left"`) || !strings.Contains(text, `id="model-right"`) || !strings.Contains(string(javascript), "models: [modelLeft.value, modelRight.value]") {
		t.Fatal("frontend is missing constrained two-model selection")
	}
	if strings.Contains(text, "<script src=\"http") || strings.Contains(text, "<link rel=\"stylesheet\" href=\"http") {
		t.Fatal("frontend loads an external runtime asset")
	}
}
