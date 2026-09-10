package capture

import (
	"net/http"
	"regexp"
)

const redacted = "[REDACTED]"

var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bsk-(?:ant-)?[a-z0-9_-]{16,}\b`),
	regexp.MustCompile(`(?i)\bBearer\s+[a-z0-9._~+/=-]{8,}`),
}

var sensitiveHeaders = map[string]struct{}{
	"Authorization": {},
	"Cookie":        {},
	"Set-Cookie":    {},
	"X-Api-Key":     {},
}

func RedactString(value string) string {
	for _, pattern := range secretPatterns {
		value = pattern.ReplaceAllString(value, redacted)
	}
	return value
}

// SanitizeHeaders returns a copy safe for persistence or diagnostics.
func SanitizeHeaders(headers http.Header) http.Header {
	sanitized := make(http.Header, len(headers))
	for key, values := range headers {
		canonicalKey := http.CanonicalHeaderKey(key)
		if _, sensitive := sensitiveHeaders[canonicalKey]; sensitive {
			continue
		}
		for _, value := range values {
			sanitized.Add(canonicalKey, RedactString(value))
		}
	}
	return sanitized
}
