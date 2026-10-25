package e2e

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/rogpeppe/go-internal/testscript"
)

// Commands returns the harness's script commands:
//
//	agentdirs dir...           create directories relative to $HOME
//	gitremote bare [seed-dir]  create a bare repository on main, seeded with seed-dir
//	linktarget path [target]   assert path is a symlink (to exactly target)
//	content file text          assert file holds exactly text and a newline
//	nooutput                   assert the last command wrote no stdout or stderr
//	hidecmd program...         drop PATH entries holding an executable program
//
// The assertions support negation with "!".
func Commands() map[string]func(ts *testscript.TestScript, neg bool, args []string) {
	return map[string]func(ts *testscript.TestScript, neg bool, args []string){
		"agentdirs":  cmdAgentDirs,
		"gitremote":  cmdGitRemote,
		"linktarget": cmdLinkTarget,
		"content":    cmdContent,
		"nooutput":   cmdNoOutput,
		"hidecmd":    cmdHideCmd,
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
