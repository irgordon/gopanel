package diagnostic

import (
	"encoding/json"
	"regexp"
	"strings"
)

var (
	authorizationCredential = regexp.MustCompile(`(?i)\bAuthorization\s*[:=]\s*(?:Bearer|Basic)\s+[^\s,;]+`)
	sensitiveAssignment     = regexp.MustCompile(`(?i)\b(password|passwd|pwd|access[_-]?token|refresh[_-]?token|api[_-]?key|set[_-]?cookie|session[_-]?id|token|secret|authorization|cookie)\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	urlCredential           = regexp.MustCompile(`(?i)(https?://)[^/@\s]+:[^/@\s]+@`)
)

func sanitizeDetail(detail string) string {
	if redacted, ok := redactJSON(detail); ok {
		return redacted
	}
	redacted := authorizationCredential.ReplaceAllString(detail, "Authorization: [REDACTED]")
	redacted = sensitiveAssignment.ReplaceAllString(redacted, `${1}${2}[REDACTED]`)
	return urlCredential.ReplaceAllString(redacted, `${1}[REDACTED]@`)
}

func redactJSON(detail string) (string, bool) {
	var value any
	if err := json.Unmarshal([]byte(detail), &value); err != nil {
		return "", false
	}
	encoded, err := json.Marshal(redactJSONValue(value))
	if err != nil {
		return "", false
	}
	return string(encoded), true
}

func redactJSONValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		redacted := make(map[string]any, len(typed))
		for key, child := range typed {
			if isSensitiveKey(key) {
				redacted[key] = "[REDACTED]"
				continue
			}
			redacted[key] = redactJSONValue(child)
		}
		return redacted
	case []any:
		redacted := make([]any, len(typed))
		for index, child := range typed {
			redacted[index] = redactJSONValue(child)
		}
		return redacted
	default:
		return value
	}
}

func isSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
	for _, marker := range []string{"password", "passwd", "pwd", "secret", "token", "credential", "authorization", "cookie"} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return normalized == "auth" || normalized == "key" || strings.HasPrefix(normalized, "auth_") || strings.HasSuffix(normalized, "_auth") || strings.HasSuffix(normalized, "_key")
}
