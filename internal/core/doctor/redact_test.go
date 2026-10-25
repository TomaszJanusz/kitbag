package doctor

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReportRedaction(t *testing.T) {
	t.Parallel()
	secrets := []string{
		"ghp_FAKE_doctor_token_1234567890", "github_pat_FAKE_doctor_token", "sk-FAKE_doctor_secret", // gitleaks:allow
		"glpat-FAKE_doctor_secret", "xoxb-FAKE-doctor-secret", "person@example.invalid",
		"https://username:password@example.invalid/path", "token=opaque-secret", "api_key=opaque-key",
		"Bearer opaque-authorization", "Basic b3BhcXVlLXNlY3JldA==",
		"/private/tmp/custom-home", "/home/somebody", "/Users/another", `C:\Users\someone`,
	}
	input := strings.Join(secrets, " ")
	r := Report{DoctorSchema: 1, Checks: []Check{{ID: input, Status: Warn, Message: input, Fix: input}}}
	redacted := r.Redact("/private/tmp/custom-home")
	data, err := json.Marshal(redacted)
	must(t, err)
	output := string(data)
	for _, secret := range append(secrets, "opaque-authorization", "b3BhcXVlLXNlY3JldA==") {
		encoded, err := json.Marshal(secret)
		must(t, err)
		if strings.Contains(output, string(encoded[1:len(encoded)-1])) {
			t.Errorf("report leaked %q", secret)
		}
	}
	if !strings.Contains(output, "[REDACTED]") || !strings.Contains(output, "~/path") && !strings.Contains(output, "~") {
		t.Fatal("redactor is a no-op", output)
	}
	if r.Checks[0].Message != input {
		t.Fatal("redaction mutated original report")
	}
}
