//go:build darwin || linux

package platform_test

import (
	"context"
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
)

func TestUnixServiceContract(t *testing.T) {
	t.Parallel()
	active, enabled, loaded := false, false, false
	home := t.TempDir()
	h := unix.New(unix.WithEnv(func(key string) (string, bool) {
		value, ok := map[string]string{"HOME": home, "USER": "test"}[key]
		return value, ok
	}), unix.WithServiceRunner(func(ctx context.Context, name string, args, env []string) (string, int, error) {
		if err := ctx.Err(); err != nil {
			return "", 0, err
		}
		if !strings.Contains(strings.Join(env, "\n"), "LC_ALL=C") {
			t.Fatal("locale not explicit")
		}
		for _, arg := range args {
			switch arg {
			case "enable":
				enabled = true
				loaded = true
			case "start", "bootstrap", "kickstart":
				active = true
				loaded = true
			case "stop", "bootout":
				active = false
				loaded = false
			case "disable":
				enabled = false
			}
		}
		return serviceContractReply(name, args, active, enabled, loaded)
	}))
	serviceContract(t, h)
}

func serviceContractReply(name string, args []string, active, enabled, loaded bool) (string, int, error) {
	if name == "systemctl" {
		return "LoadState=" + map[bool]string{true: "loaded", false: "not-found"}[loaded] + "\nActiveState=" + map[bool]string{true: "active", false: "inactive"}[active] + "\nUnitFileState=" + map[bool]string{true: "enabled", false: "disabled"}[enabled], 0, nil
	}
	if args[0] == "print" && !loaded {
		return "", 113, nil
	}
	if args[0] == "print-disabled" {
		return `"dev.kitbag.agent" => ` + map[bool]string{true: "false", false: "true"}[enabled], 0, nil
	}
	return "state = running\nlast exit code = 0", 0, nil
}
