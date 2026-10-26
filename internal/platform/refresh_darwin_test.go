package platform_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

func TestDetachedRefreshUsesSeparateProcessGroup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	result := filepath.Join(dir, "result")
	binary := filepath.Join(dir, "helper")
	quoted := "'" + strings.ReplaceAll(result, "'", "'\"'\"'") + "'"
	body := "#!/bin/sh\nprintf '%s\\n' \"$$\" \"$*\" > " + quoted + "\n/bin/ps -o pgid= -p $$ >> " + quoted + "\n"
	if err := os.WriteFile(binary, []byte(body), 0o700); err != nil { //nolint:gosec // G306: executable test helper in a private temporary directory.
		t.Fatal(err)
	}
	if err := platform.DetachServiceRefresh(binary, "/config with spaces.toml", dir); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		data, err := os.ReadFile(result) //nolint:gosec // G304: fixed result file inside this test's private temporary directory.
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if err == nil && len(lines) == 3 {
			pid, pidErr := strconv.Atoi(lines[0])
			group, groupErr := strconv.Atoi(strings.TrimSpace(lines[2]))
			if pidErr != nil || groupErr != nil || pid != group || lines[1] != "--config /config with spaces.toml service refresh" {
				t.Fatal(string(data), pidErr, groupErr)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("detached helper did not report its process group")
}
