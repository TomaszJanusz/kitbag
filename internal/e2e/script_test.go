package e2e

import (
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/rogpeppe/go-internal/testscript"
)

func TestMain(m *testing.M) {
	testscript.Main(m, map[string]func(){
		"probe":    Guarded(func() { os.Exit(probe(os.Args[1:], os.Stdout, os.Stderr)) }),
		"stubsync": Guarded(func() { os.Exit(stubSync(os.Args[1:], os.Stdout, os.Stderr)) }),
	})
}

func TestScripts(t *testing.T) {
	t.Parallel()

	testscript.Run(t, params(t, "testdata/script"))
}

// TestFailingByDesign proves each script under testdata/fail fails, with the
// expected message, so the harness's assertions and guards cannot pass vacuously.
func TestFailingByDesign(t *testing.T) {
	t.Parallel()

	want := map[string]string{
		"agentdirs_escape.txtar":       `"\.\./outside" is not a local path below \$HOME`,
		"agentdirs_real_home.txtar":    `refusing to run with HOME=.*: it is the real home directory`,
		"content_mismatch.txtar":       `file holds "hello\\n", want "goodbye\\n"`,
		"content_negated.txtar":        `file unexpectedly holds "hello\\n"`,
		"gitremote_exists.txtar":       `gitremote: remote\.git already exists`,
		"hidecmd_usage.txtar":          `usage: hidecmd program\.\.\.`,
		"homefixture_escape.txtar":     `path escapes from parent`,
		"linktarget_negated.txtar":     `link is a symlink to "target"`,
		"linktarget_not_symlink.txtar": `file is not a symlink`,
		"linktarget_wrong.txtar":       `link is a symlink to "target", want "other"`,
		"nooutput_negated.txtar":       `the last command wrote no output`,
		"nooutput_stderr.txtar":        `unexpected stderr: "oops\\n"`,
		"nooutput_stdout.txtar":        `unexpected stdout: "hello\\n"`,
	}
	files, err := filepath.Glob("testdata/fail/*.txtar")
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(files))
	for _, file := range files {
		names = append(names, filepath.Base(file))
	}
	if got := slices.Sorted(maps.Keys(want)); !slices.Equal(got, names) {
		t.Fatalf("expectations %q do not match scripts %q", got, names)
	}

	for name, pattern := range want {
		t.Run(name, func(t *testing.T) {
			t.Parallel()

			assertScriptFails(t, filepath.Join("testdata", "fail", name), pattern)
		})
	}
}

func assertScriptFails(t *testing.T, file, pattern string) {
	t.Helper()

	p := params(t, "")
	p.Files = []string{file}
	var rec recorder
	testscript.RunT(&rec, p)
	failed, log := rec.result()
	if !failed {
		t.Fatalf("%s passed, want a failure; log:\n%s", file, log)
	}
	if !regexp.MustCompile(pattern).MatchString(log) {
		t.Fatalf("%s failed without matching %q; log:\n%s", file, pattern, log)
	}
}

// params extends the harness parameters for the harness's own scripts:
// $REAL_HOME is the account's real home, which only guarded commands that
// write nothing receive, and $WORK/real is registered as a further real home
// so commands that write can be shown to refuse one safely.
func params(t *testing.T, dir string) testscript.Params {
	t.Helper()

	homes := accountHome()
	if len(homes) == 0 {
		t.Fatal("no account home directory found to test the guards against")
	}
	p := Params(dir)
	p.Setup = func(env *testscript.Env) error {
		if err := setup(env, append(RealHomes(), filepath.Join(env.WorkDir, "real"))); err != nil {
			return err
		}
		env.Setenv("REAL_HOME", homes[0])
		return nil
	}
	return p
}

// recorder is a testscript.T that records a failure instead of failing the test.
type recorder struct {
	log    []string
	mu     sync.Mutex
	failed bool
}

func (r *recorder) Skip(args ...any) {
	r.Log(append([]any{"SKIP: "}, args...)...)
	runtime.Goexit()
}

func (r *recorder) Fatal(args ...any) {
	r.Log(args...)
	r.FailNow()
}

func (*recorder) Parallel() {}

func (r *recorder) Log(args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.log = append(r.log, fmt.Sprint(args...))
}

func (r *recorder) FailNow() {
	r.mu.Lock()
	r.failed = true
	r.mu.Unlock()
	runtime.Goexit()
}

// Run runs f on its own goroutine, as FailNow and Skip end the calling goroutine.
func (r *recorder) Run(_ string, f func(testscript.T)) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		f(r)
	}()
	<-done
}

func (*recorder) Verbose() bool { return false }

func (r *recorder) result() (failed bool, log string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.failed, strings.Join(r.log, "\n")
}

// probe writes its arguments as one line to stdout, or to stderr after
// -stderr, and nothing at all without arguments.
func probe(args []string, stdout, stderr io.Writer) int {
	out := stdout
	if len(args) > 0 && args[0] == "-stderr" {
		out, args = stderr, args[1:]
	}
	if len(args) == 0 {
		return 0
	}
	if _, err := fmt.Fprintln(out, strings.Join(args, " ")); err != nil {
		return 1
	}
	return 0
}

// stubSync stands in for a synchronization command so the mandatory
// scenarios template runs today: stubsync source-dir instance-dir... links
// every file of source-dir into each instance directory, reports only the
// links it creates and fails, after syncing the rest, if an instance is missing.
func stubSync(args []string, stdout, stderr io.Writer) int {
	if len(args) < 2 {
		return report(stderr, 2, "usage: stubsync source-dir instance-dir...")
	}
	entries, err := os.ReadDir(args[0])
	if err != nil {
		return report(stderr, 1, "stubsync:", err)
	}
	code := 0
	for _, instance := range args[1:] {
		// A root confines every link to its instance directory.
		root, err := os.OpenRoot(instance)
		if err != nil {
			code = report(stderr, 1, "stubsync: instance", instance, "is missing")
			continue
		}
		err = errors.Join(linkAll(root, args[0], entries, stdout), root.Close())
		if err != nil {
			return report(stderr, 1, "stubsync:", err)
		}
	}
	return code
}

// linkAll links each entry of source into root unless it already is, and
// reports every link it creates.
func linkAll(root *os.Root, source string, entries []os.DirEntry, stdout io.Writer) error {
	for _, entry := range entries {
		target := filepath.Join(source, entry.Name())
		if got, err := root.Readlink(entry.Name()); err == nil && got == target {
			continue
		}
		if err := root.Symlink(target, entry.Name()); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(stdout, "linked", filepath.Join(root.Name(), entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

// report writes a diagnostic line and returns code, or 1 if writing failed.
func report(w io.Writer, code int, args ...any) int {
	if _, err := fmt.Fprintln(w, args...); err != nil {
		return 1
	}
	return code
}
