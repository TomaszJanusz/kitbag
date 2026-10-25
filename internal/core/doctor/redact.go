package doctor

import (
	"regexp"
	"strings"
)

// Redact removes common secrets and personal identifiers from every free-text
// field before serialization. Reports never include environment/config dumps.
func (r Report) Redact(home string) Report {
	copyReport := Report{DoctorSchema: r.DoctorSchema, Checks: make([]Check, len(r.Checks))}
	for i, check := range r.Checks {
		check.ID = RedactText(check.ID, home)
		check.Message = RedactText(check.Message, home)
		check.Fix = RedactText(check.Fix, home)
		copyReport.Checks[i] = check
	}
	return copyReport
}

// RedactText also masks other users' Unix and Windows home paths. Apply patterns
// before escaping JSON so escaped slashes/backslashes cannot hide identifiers.
func RedactText(value, home string) string {
	for _, pattern := range []string{
		`(?i)(https?|ssh)://[^\s/@]+(?::[^\s/@]*)?@`,
		`(?i)\b(?:gh[pousr]_[A-Za-z0-9_]+|github_pat_[A-Za-z0-9_]+|sk-[A-Za-z0-9_-]+|glpat-[A-Za-z0-9_-]+|xox[baprs]-[A-Za-z0-9-]+)`,
		`(?i)\b(?:bearer|basic)\s+[A-Za-z0-9._~+/=-]+`,
		`\b(?:AKIA|ASIA)[A-Z0-9]{16}\b`,
		`\beyJ[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\b`,
		`(?i)\b(?:token|api[_-]?key|authorization|password|secret)\s*[:=]\s*[^\s,;]+`,
		`\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`,
	} {
		value = regexp.MustCompile(pattern).ReplaceAllString(value, "[REDACTED]")
	}
	if home != "" {
		value = strings.ReplaceAll(value, home, "~")
	}
	value = regexp.MustCompile(`/(?:home|Users)/[^/\s"'<>]+`).ReplaceAllString(value, "~")
	value = regexp.MustCompile(`(?i)[a-z]:[\\/]Users[\\/][^\\/\s"'<>]+`).ReplaceAllString(value, "~")
	return value
}
