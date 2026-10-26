//go:build darwin || linux

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/TomaszJanusz/kitbag/internal/api"
	"github.com/TomaszJanusz/kitbag/internal/api/client"
	"github.com/TomaszJanusz/kitbag/internal/platform"
	"github.com/TomaszJanusz/kitbag/internal/platform/unix"
	"github.com/TomaszJanusz/kitbag/internal/vcs/embedded"
)

type apiFixture struct {
	host   platform.Host
	events *api.Events
	token  string
	path   string
}

func newAPIFixture(t *testing.T) apiFixture {
	t.Helper()
	// Short paths also fit Darwin's 104-byte sockaddr_un.sun_path.
	runtime, err := os.MkdirTemp("/tmp", "kitbag-api-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if removeErr := os.RemoveAll(runtime); removeErr != nil {
			t.Error(removeErr)
		}
	})
	// Resolve macOS's /var -> /private/var before the planner checks parents.
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	env := map[string]string{"HOME": home, "XDG_RUNTIME_DIR": runtime, "GIT_CONFIG_GLOBAL": filepath.Join(home, ".gitconfig"), "GIT_CONFIG_NOSYSTEM": "1"}
	host := unix.New(unix.WithEnv(func(key string) (string, bool) { value, ok := env[key]; return value, ok }))
	repo := filepath.Join(home, "repo")
	if err = host.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	backend := embedded.New(embedded.Options{Env: host})
	if err = backend.Init(t.Context(), repo); err != nil {
		t.Fatal(err)
	}
	writeAPIFixture(t, host, repo)
	token, err := api.EnsureToken(host)
	if err != nil {
		t.Fatal(err)
	}
	path, err := api.SocketPath(host)
	if err != nil {
		t.Fatal(err)
	}
	events := api.NewEvents()
	cli := &commandLine{host: host, stdout: io.Discard, stderr: io.Discard}
	startAPIServer(t, host, path, cli.apiHandlers(token, events))
	return apiFixture{host: host, token: token, path: path, events: events}
}

func writeAPIFixture(t *testing.T, host platform.Host, repo string) {
	t.Helper()
	dirs, err := host.Dirs()
	if err != nil {
		t.Fatal(err)
	}
	if err = host.MkdirAll(dirs.Config, 0o700); err != nil {
		t.Fatal(err)
	}
	home, err := host.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{".claude", ".claude_2"} {
		if err = host.MkdirAll(filepath.Join(home, dir), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		filepath.Join(dirs.Config, "config.toml"): "schema=1\n[repo]\nurl=\"https://example.invalid/config.git\"\npath=\"~/repo\"\n[git]\nbackend=\"embedded\"\n",
		filepath.Join(repo, "kitbag.toml"):        "schema=1\n[[files]]\nsrc=\"source\"\ndest=\"~/output\"\n[targets.claude]\nagent=\"claude\"\n[targets.claude.instances.main]\ndir=\"~/.claude\"\n[targets.claude.instances.second]\ndir=\"~/.claude_2\"\n",
		filepath.Join(repo, "source"):             "api contract bytes\n",
	} {
		if err = host.WriteAtomic(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func startAPIServer(t *testing.T, host platform.Host, path string, handlers api.Handlers) {
	t.Helper()
	listener, err := host.ListenLocal(path)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- api.Serve(ctx, listener, handlers) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("API failed to stop")
		}
	})
}

func testAPIContract(t *testing.T, method, valid, invalid string, code int, requestDef, resultDef string) {
	t.Helper()
	fixture := newAPIFixture(t)
	good := apiRequest(t, method, valid, fixture.token)
	validateAPIValue(t, requestDef, good, true)
	conn := fixture.connect(t)
	response := fixture.exchange(t, conn, good)
	if response.Error != nil || response.Result == nil {
		t.Fatalf("%s: %+v", method, response)
	}
	validateAPIValue(t, resultDef, response.Result.Data, true)
	bad := apiRequest(t, method, invalid, fixture.token)
	validateAPIValue(t, requestDef, bad, false)
	response = fixture.exchange(t, conn, bad)
	if response.Error == nil || response.Error.Code != code {
		t.Fatalf("invalid %s: %+v", method, response)
	}
}

func apiRequest(t *testing.T, method, arguments, token string) []byte {
	t.Helper()
	var params map[string]any
	if err := json.Unmarshal([]byte(arguments), &params); err != nil {
		t.Fatal(err)
	}
	if _, ok := params["token"]; !ok {
		params["token"] = token
	}
	if _, ok := params["api_version"]; !ok {
		params["api_version"] = api.Version
	}
	data, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (f apiFixture) connect(t *testing.T) net.Conn {
	t.Helper()
	conn, err := f.host.DialLocal(t.Context(), f.path)
	if err != nil {
		t.Fatal(err)
	}
	if err = conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	})
	return conn
}

func (f apiFixture) exchange(t *testing.T, conn net.Conn, request []byte) api.Response {
	t.Helper()
	if _, err := conn.Write(append(request, '\n')); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(conn)
	for {
		var frame json.RawMessage
		if err := decoder.Decode(&frame); err != nil {
			t.Fatal(err)
		}
		var notification api.Notification
		if err := json.Unmarshal(frame, &notification); err != nil {
			t.Fatal(err)
		}
		if notification.Method != "" {
			validateAPIValue(t, "notification", frame, true)
			continue
		}
		validateAPIValue(t, "response", frame, true)
		var response api.Response
		if err := json.Unmarshal(frame, &response); err != nil {
			t.Fatal(err)
		}
		return response
	}
}

func validateAPIValue(t *testing.T, definition string, data []byte, wantValid bool) {
	t.Helper()
	compiler := apiSchema(t)
	schema, err := compiler.Compile("https://kitbag.sh/docs/api/api-v1.schema.json#/$defs/" + definition)
	if err != nil {
		t.Fatal(err)
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if err = schema.Validate(doc); (err == nil) != wantValid {
		t.Fatalf("%s validity=%t: %s: %v", definition, wantValid, data, err)
	}
}

func apiSchema(t *testing.T) *jsonschema.Compiler {
	t.Helper()
	compiler := jsonschema.NewCompiler()
	for url, path := range map[string]string{
		"https://kitbag.sh/docs/api/api-v1.schema.json": "../../docs/api/api-v1.schema.json",
		"https://kitbag.sh/schemas/plan.schema.json":    "../../schemas/plan.schema.json",
	} {
		data, err := os.ReadFile(path) //nolint:gosec // G304: fixed repository schema paths above.
		if err != nil {
			t.Fatal(err)
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		if err = compiler.AddResource(url, doc); err != nil {
			t.Fatal(err)
		}
	}
	return compiler
}

func TestAPINewerMinorClient(t *testing.T) {
	t.Parallel()
	fixture := newAPIFixture(t)
	var info api.VersionInfo
	if err := (client.Client{Host: fixture.host, APIVersion: "1.99"}).Call(t.Context(), "version.get", api.Params{}, &info, nil); err != nil {
		t.Fatal(err)
	}
	if info.MaxVersion != "1.0" {
		t.Fatalf("negotiated version=%+v", info)
	}
	conn := fixture.connect(t)
	request := apiRequest(t, "version.get", `{"optional_future_field":{"value":true}}`, fixture.token)
	request = bytes.Replace(request, []byte(`"api_version":"1.0"`), []byte(`"api_version":"1.99"`), 1)
	response := fixture.exchange(t, conn, request)
	if response.Error != nil || response.Result == nil || response.Result.APIVersion != "1.0" {
		t.Fatalf("minor negotiation: %+v", response)
	}
	request = bytes.Replace(request, []byte(`"version.get"`), []byte(`"future.optional"`), 1)
	response = fixture.exchange(t, conn, request)
	if response.Error == nil || response.Error.Code != api.MethodNotFound {
		t.Fatalf("unknown extension: %+v", response)
	}
	// The same connection still serves a supported method after the error.
	response = fixture.exchange(t, conn, apiRequest(t, "version.get", "{}", fixture.token))
	if response.Error != nil {
		t.Fatal(response.Error)
	}
}

func TestAPISyncChangesAndConfigValidation(t *testing.T) {
	t.Parallel()
	fixture := newAPIFixture(t)
	conn := fixture.connect(t)
	response := fixture.exchange(t, conn, apiRequest(t, "sync.run", `{"no_pull":true}`, fixture.token))
	if response.Error != nil {
		t.Fatal(response.Error)
	}
	home, err := fixture.host.HomeDir()
	if err != nil {
		t.Fatal(err)
	}
	got, err := fixture.host.ReadFile(filepath.Join(home, "output"))
	if err != nil || string(got) != "api contract bytes\n" {
		t.Fatalf("sync output %q, %v", got, err)
	}
	response = fixture.exchange(t, conn, apiRequest(t, "config.set", `{"config":"schema=1\n[repo]\nurl=\"secret-marker\"\n[sync]\nmode=\"INVALID\""}`, fixture.token))
	if response.Error == nil || response.Error.Data.ExitCode != 2 || strings.Contains(response.Error.Error(), "secret-marker") {
		t.Fatalf("config error: %+v", response)
	}
	response = fixture.exchange(t, conn, apiRequest(t, "config.get", "{}", fixture.token))
	if response.Error != nil || bytes.Contains(response.Result.Data, []byte("secret-marker")) {
		t.Fatalf("invalid config replaced file: %+v", response)
	}
}
