// Package service generates and manages user service definitions through platform contracts.
package service

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/TomaszJanusz/kitbag/internal/platform"
)

// Options are deterministic, non-secret service inputs. Environment is a
// deliberately small allow-list; credentials are never copied into unit files.
type Options struct {
	Environment map[string]string `json:"environment"`
	Binary      string            `json:"binary"`
	Config      string            `json:"config"`
	Mode        string            `json:"mode"`
	LogDir      string            `json:"log_dir"`
	Interval    time.Duration     `json:"interval"`
}

// DefaultInterval is the timer's default cadence.
const DefaultInterval = 15 * time.Minute

func (o Options) validate() error {
	for _, path := range []string{o.Binary, o.Config, o.LogDir} {
		if !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
			return fmt.Errorf("service paths must be absolute without control characters: %w", fs.ErrInvalid)
		}
	}
	if o.Mode != "daemon" && o.Mode != "timer" {
		return fmt.Errorf("service mode must be daemon or timer: %w", fs.ErrInvalid)
	}
	if o.Interval < time.Minute || o.Interval > 24*time.Hour || o.Interval%time.Second != 0 {
		return fmt.Errorf("service interval must be whole seconds between 1m and 24h: %w", fs.ErrInvalid)
	}
	return o.validateEnvironment()
}

func (o Options) validateEnvironment() error {
	for key, value := range o.Environment {
		if !slices.Contains(environmentKeys(), key) || strings.ContainsAny(value, "\x00\r\n") {
			return fmt.Errorf("unsupported service environment: %w", fs.ErrInvalid)
		}
	}
	return nil
}

func environmentKeys() []string {
	return []string{"HOME", "PATH", "XDG_CONFIG_HOME", "XDG_DATA_HOME", "XDG_STATE_HOME", "XDG_CACHE_HOME", "XDG_RUNTIME_DIR", "CLAUDE_CONFIG_DIR", "CODEX_HOME", "GEMINI_HOME", "OPENCODE_CONFIG_DIR"}
}

// Generate returns basename-to-content definitions; it performs no host operations.
func Generate(manager string, o Options) (map[string]string, error) {
	if err := o.validate(); err != nil {
		return nil, err
	}
	switch manager {
	case "systemd":
		return systemd(o), nil
	case "launchd":
		return map[string]string{platform.ServiceLabel + ".plist": launchd(o)}, nil
	default:
		return nil, platform.ErrUnsupported
	}
}

func arguments(o Options) []string {
	command := "daemon"
	if o.Mode == "timer" {
		command = "sync"
	}
	return []string{o.Binary, "--config", o.Config, command}
}

func systemdQuote(value string) string {
	value = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "%", "%%", "\t", "\\t").Replace(value)
	return `"` + value + `"`
}

func systemd(o Options) map[string]string {
	args := arguments(o)
	for i := range args {
		args[i] = systemdQuote(strings.ReplaceAll(args[i], "$", "$$"))
	}
	text := "[Unit]\nDescription=KitBag configuration synchronization\n\n[Service]\n"
	if o.Mode == "daemon" {
		text += "Type=simple\nRestart=on-failure\nRestartSec=5s\n"
	} else {
		text += "Type=oneshot\n"
	}
	text += "NoNewPrivileges=true\nExecStart=" + strings.Join(args, " ") + "\n"
	var environment strings.Builder
	for _, key := range sortedKeys(o.Environment) {
		writeString(&environment, "Environment="+systemdQuote(key+"="+o.Environment[key])+"\n")
	}
	text += environment.String()
	text += "\n[Install]\nWantedBy=default.target\n"
	files := map[string]string{platform.ServiceLabel + ".service": text}
	if o.Mode == "timer" {
		files[platform.ServiceLabel+".timer"] = "[Unit]\nDescription=KitBag synchronization timer\n\n[Timer]\nOnBootSec=1min\nOnUnitActiveSec=" + strconv.FormatInt(int64(o.Interval/time.Second), 10) + "s\nRandomizedDelaySec=30s\nUnit=" + platform.ServiceLabel + ".service\n\n[Install]\nWantedBy=timers.target\n"
	}
	return files
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func xmlString(value string) string {
	var out strings.Builder
	// strings.Builder.Write never fails.
	if err := xml.EscapeText(&out, []byte(value)); err != nil {
		panic(err)
	}
	return "<string>" + out.String() + "</string>"
}

func launchd(o Options) string {
	text := "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n<key>Label</key>" + xmlString(platform.ServiceLabel) + "\n<key>ProgramArguments</key><array>\n"
	var args strings.Builder
	for _, arg := range arguments(o) {
		writeString(&args, xmlString(arg)+"\n")
	}
	text += args.String()
	text += "</array>\n<key>RunAtLoad</key><true/>\n"
	if o.Mode == "daemon" {
		text += "<key>KeepAlive</key><true/>\n"
	} else {
		text += "<key>StartInterval</key><integer>" + strconv.FormatInt(int64(o.Interval/time.Second), 10) + "</integer>\n"
	}
	text += "<key>ProcessType</key><string>Background</string>\n<key>StandardOutPath</key>" + xmlString(filepath.Join(o.LogDir, "service.stdout.log")) + "\n<key>StandardErrorPath</key>" + xmlString(filepath.Join(o.LogDir, "service.stderr.log")) + "\n<key>EnvironmentVariables</key><dict>\n"
	var environment strings.Builder
	for _, key := range sortedKeys(o.Environment) {
		writeString(&environment, "<key>"+key+"</key>"+xmlString(o.Environment[key])+"\n")
	}
	text += environment.String()
	return text + "</dict>\n</dict></plist>\n"
}

func writeString(builder *strings.Builder, value string) {
	if _, err := builder.WriteString(value); err != nil {
		panic(err)
	}
}
