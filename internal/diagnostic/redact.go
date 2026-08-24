package diagnostic

import "regexp"

var (
	authorizationCredential = regexp.MustCompile(`(?i)\bAuthorization\s*[:=]\s*(?:Bearer|Basic)\s+[^\s,;]+`)
	sensitiveAssignment     = regexp.MustCompile(`(?i)\b(password|passwd|pwd|access[_-]?token|refresh[_-]?token|api[_-]?key|set[_-]?cookie|session[_-]?id|token|secret|authorization|cookie)\b(\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	urlCredential           = regexp.MustCompile(`(?i)(https?://)[^/@\s]+:[^/@\s]+@`)
)

func sanitizeDetail(detail string) string {
	redacted := authorizationCredential.ReplaceAllString(detail, "Authorization: [REDACTED]")
	redacted = sensitiveAssignment.ReplaceAllString(redacted, `${1}${2}[REDACTED]`)
	return urlCredential.ReplaceAllString(redacted, `${1}[REDACTED]@`)
}
