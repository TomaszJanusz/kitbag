package e2e

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rogpeppe/go-internal/testscript"
)

// Commands returns the harness's script commands:
//
//	agentdirs dir...           create directories relative to $HOME
//	gitremote bare [seed-dir]  create a bare repository on main, seeded with seed-dir
//	linktarget path [target]   assert path is a symlink (to exactly target)
//	content file text          assert file holds exactly text and a newline
//	filemode file perm         assert file's permission bits are octal perm
//	nooutput                   assert the last command wrote no stdout or stderr
//	hidecmd program...         drop PATH entries holding an executable program
//	homefixture dir            install $WORK/dir into $HOME (see cmdHomeFixture)
//	treesnap root file [skip...] record root's entries, contents and mtimes in file
//
// The assertions support negation with "!".
func Commands() map[string]func(ts *testscript.TestScript, neg bool, args []string) {
	return map[string]func(ts *testscript.TestScript, neg bool, args []string){
		"agentdirs":   cmdAgentDirs,
		"gitremote":   cmdGitRemote,
		"linktarget":  cmdLinkTarget,
		"content":     cmdContent,
		"filemode":    cmdFileMode,
		"nooutput":    cmdNoOutput,
		"hidecmd":     cmdHideCmd,
		"homefixture": cmdHomeFixture,
		"treesnap":    cmdTreeSnap,
	}
}

// cmdHomeFixture installs a recorded HOME: every file below $WORK/dir is copied
// to the same relative path below $HOME with "@HOME@" replaced by $HOME; each
// "relative -> target" line of $WORK/dir.links creates a symbolic link (target
// substituted the same way); $WORK/dir.gitconfig, if present, is appended to
// $GIT_CONFIG_GLOBAL. Writes are confined to $HOME through os.Root. Files get
// the modification time 2001-01-01T00:00:00Z.
func cmdHomeFixture(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) != 1 {
		ts.Fatalf("usage: homefixture dir")
	}
	realHomes, ok := ts.Value(realHomesKey{}).([]string)
	if !ok {
		ts.Fatalf("homefixture: the script was not prepared by e2e.Setup")
	}
	home := ts.Getenv("HOME")
	ts.Check(CheckHome(home, ts.Getenv("WORK"), realHomes))
	root, err := os.OpenRoot(home)
	ts.Check(err)
	defer func() { ts.Check(root.Close()) }()
	expand := func(text string) string { return strings.ReplaceAll(text, "@HOME@", home) }
	fixtureFiles(ts, root, ts.MkAbs(args[0]), expand)
	if text, exists := optionalFile(ts, args[0]+".links"); exists {
		fixtureLinks(ts, root, expand(text))
	}
	if text, exists := optionalFile(ts, args[0]+".gitconfig"); exists {
		appendGitConfig(ts, expand(text))
	}
}

// fixtureUnix is 2001-01-01T00:00:00Z.
const fixtureUnix = 978307200

// fixtureFiles copies every file below source to root, expanding its text.
func fixtureFiles(ts *testscript.TestScript, root *os.Root, source string, expand func(string) string) {
	fixtureTime := time.Unix(fixtureUnix, 0)
	ts.Check(filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if err = root.MkdirAll(filepath.Dir(relative), privateDir); err != nil {
			return err
		}
		if err = root.WriteFile(relative, []byte(expand(ts.ReadFile(path))), privateFile); err != nil {
			return err
		}
		// An old, fixed time makes any later rewrite visible to treesnap, even
		// on filesystems with a coarse clock.
		return root.Chtimes(relative, fixtureTime, fixtureTime)
	}))
}

func appendGitConfig(ts *testscript.TestScript, text string) {
	config, err := os.OpenFile(ts.Getenv("GIT_CONFIG_GLOBAL"), os.O_APPEND|os.O_WRONLY, privateFile)
	ts.Check(err)
	_, err = config.WriteString(text)
	ts.Check(errors.Join(err, config.Close()))
}

func fixtureLinks(ts *testscript.TestScript, root *os.Root, text string) {
	for line := range strings.SplitSeq(strings.TrimSpace(text), "\n") {
		relative, target, found := strings.Cut(line, " -> ")
		if !found {
			ts.Fatalf("homefixture: malformed link line %q", line)
		}
		ts.Check(root.MkdirAll(filepath.Dir(relative), privateDir))
		ts.Check(root.Symlink(target, relative))
	}
}

func optionalFile(ts *testscript.TestScript, name string) (string, bool) {
	if _, err := os.Lstat(ts.MkAbs(name)); errors.Is(err, fs.ErrNotExist) {
		return "", false
	}
	return ts.ReadFile(name), true
}

// cmdTreeSnap writes one line per entry below root, sorted: relative path,
// type and permissions, then a regular file's size, modification time in
// nanoseconds and SHA-256, or a link's exact target. Compare two snapshots with
// cmp to prove that nothing was created, removed, rewritten (even with equal
// bytes) or retargeted. Relative paths in skip, and everything below them, are
// left out (for example KitBag's own state directory).
func cmdTreeSnap(ts *testscript.TestScript, neg bool, args []string) {
	const minArgs = 2
	if neg || len(args) < minArgs {
		ts.Fatalf("usage: treesnap root file [skip...]")
	}
	root := ts.MkAbs(args[0])
	skip := args[2:]
	var lines []string
	ts.Check(filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		if slices.ContainsFunc(skip, func(s string) bool { return relative == s || strings.HasPrefix(relative, s+"/") }) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		line, err := snapshotLine(path, relative)
		lines = append(lines, line)
		return err
	}))
	slices.Sort(lines)
	ts.Check(os.WriteFile(ts.MkAbs(args[1]), []byte(strings.Join(lines, "\n")+"\n"), privateFile))
}

func snapshotLine(path, relative string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	line := fmt.Sprintf("%s %s", relative, info.Mode())
	switch {
	case info.Mode()&fs.ModeSymlink != 0:
		target, linkErr := os.Readlink(path)
		return line + " -> " + target, linkErr
	case info.Mode().IsRegular():
		data, readErr := os.ReadFile(path) //nolint:gosec // G304: the script names a path inside its own $WORK.
		return fmt.Sprintf("%s %d %d %x", line, info.Size(), info.ModTime().UnixNano(), sha256.Sum256(data)), readErr
	default:
		return line, nil
	}
}

func cmdAgentDirs(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) == 0 {
		ts.Fatalf("usage: agentdirs dir...")
	}
	realHomes, ok := ts.Value(realHomesKey{}).([]string)
	if !ok {
		ts.Fatalf("agentdirs: the script was not prepared by e2e.Setup")
	}
	home := ts.Getenv("HOME")
	ts.Check(CheckHome(home, ts.Getenv("WORK"), realHomes))
	for _, dir := range args {
		if !filepath.IsLocal(dir) {
			ts.Fatalf("agentdirs: %q is not a local path below $HOME", dir)
		}
		ts.Check(os.MkdirAll(filepath.Join(home, dir), privateDir))
	}
}

func cmdGitRemote(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) == 0 || len(args) > 2 {
		ts.Fatalf("usage: gitremote bare [seed-dir]")
	}
	bare := ts.MkAbs(args[0])
	if _, err := os.Lstat(bare); !errors.Is(err, fs.ErrNotExist) {
		ts.Fatalf("gitremote: %s already exists", args[0])
	}
	gitExec(ts, "init", "--quiet", "--bare", "--initial-branch=main", bare)
	if len(args) == 1 {
		return
	}

	// Commit the seed from a scratch repository so the seed directory stays
	// free of Git metadata, then push it like any client would.
	scratch, err := os.MkdirTemp(filepath.Join(ts.Getenv("WORK"), ".tmp"), "gitremote-")
	ts.Check(err)
	gitExec(ts, "init", "--quiet", "--bare", scratch)
	repo := []string{"--git-dir=" + scratch, "--work-tree=" + ts.MkAbs(args[1])}
	gitExec(ts, append(repo, "add", "--all")...)
	gitExec(ts, append(repo, "commit", "--quiet", "--message=Seed "+args[1])...)
	gitExec(ts, append(repo, "push", "--quiet", bare, "HEAD:refs/heads/main")...)
	ts.Check(os.RemoveAll(scratch))
}

func gitExec(ts *testscript.TestScript, args ...string) {
	if err := ts.Exec("git", args...); err != nil {
		ts.Fatalf("gitremote: git %v: %v", args, err)
	}
}

func cmdLinkTarget(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) == 0 || len(args) > 2 {
		ts.Fatalf("usage: linktarget path [target]")
	}
	got, err := os.Readlink(ts.MkAbs(args[0]))
	matched := err == nil && (len(args) == 1 || got == args[1])
	switch {
	case neg && matched:
		ts.Fatalf("%s is a symlink to %q", args[0], got)
	case neg:
		return
	case err != nil:
		ts.Fatalf("%s is not a symlink: %v", args[0], err)
	case !matched:
		ts.Fatalf("%s is a symlink to %q, want %q", args[0], got, args[1])
	}
}

func cmdContent(ts *testscript.TestScript, neg bool, args []string) {
	const fileAndText = 2
	if len(args) != fileAndText {
		ts.Fatalf("usage: content file text")
	}
	got := ts.ReadFile(args[0])
	want := args[1] + "\n"
	switch {
	case neg && got == want:
		ts.Fatalf("%s unexpectedly holds %q", args[0], got)
	case !neg && got != want:
		ts.Fatalf("%s holds %q, want %q", args[0], got, want)
	}
}

// cmdFileMode compares permission bits without following a final link. Windows
// does not have Unix modes; scripts using it skip Windows.
func cmdFileMode(ts *testscript.TestScript, neg bool, args []string) {
	const fileAndPerm = 2
	if len(args) != fileAndPerm {
		ts.Fatalf("usage: filemode file perm")
	}
	want, err := strconv.ParseUint(args[1], 8, 32)
	if err != nil || want&^uint64(fs.ModePerm) != 0 {
		ts.Fatalf("filemode: %q is not an octal permission", args[1])
	}
	info, err := os.Lstat(ts.MkAbs(args[0]))
	ts.Check(err)
	got := info.Mode().Perm()
	switch {
	case neg && uint64(got) == want:
		ts.Fatalf("%s unexpectedly has mode %#o", args[0], got)
	case !neg && uint64(got) != want:
		ts.Fatalf("%s has mode %#o, want %#o", args[0], got, want)
	}
}

func cmdNoOutput(ts *testscript.TestScript, neg bool, args []string) {
	if len(args) != 0 {
		ts.Fatalf("usage: nooutput")
	}
	stdout, stderr := ts.ReadFile("stdout"), ts.ReadFile("stderr")
	silent := stdout == "" && stderr == ""
	switch {
	case neg && silent:
		ts.Fatalf("the last command wrote no output")
	case !neg && stdout != "":
		ts.Fatalf("unexpected stdout: %q", stdout)
	case !neg && stderr != "":
		ts.Fatalf("unexpected stderr: %q", stderr)
	}
}

// cmdHideCmd removes from $PATH every directory that holds an executable
// named like one of the programs, so a script can run kitbag on a machine
// "without" them. Commands registered with testscript.Main stay available.
func cmdHideCmd(ts *testscript.TestScript, neg bool, args []string) {
	if neg || len(args) == 0 {
		ts.Fatalf("usage: hidecmd program...")
	}
	var kept []string
	for _, dir := range filepath.SplitList(ts.Getenv("PATH")) {
		holds := func(program string) bool { return isExecutable(filepath.Join(dir, program)) }
		if !slices.ContainsFunc(args, holds) {
			kept = append(kept, dir)
		}
	}
	ts.Setenv("PATH", strings.Join(kept, string(filepath.ListSeparator)))
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}
