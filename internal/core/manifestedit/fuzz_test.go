package manifestedit_test

import (
	"bytes"
	"errors"
	"maps"
	"slices"
	"strconv"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/manifestedit"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// FuzzApply asserts "refuse, or reparse equals intent" over arbitrary
// manifests: an accepted edit decodes to the intended instance, and an add
// keeps every original byte in order around one insertion.
func FuzzApply(f *testing.F) {
	seed, err := testdata.ReadFile("testdata/agent-config.toml")
	if err != nil {
		f.Fatal(err)
	}
	for _, s := range []string{
		string(seed),
		"schema=1\n[targets.c]\nagent='claude'\n",
		"schema=1\r\n# c\r\n[targets.c]\r\nagent='claude'\r\n[targets.c.instances.a]\r\ndir='~/a'\r\n",
		"schema=1\n[targets.c]\nagent='claude'\ninstances = { a = { dir = '~/a' } }\n",
		"schema=1\n[targets.c]\nagent='claude'\n[targets.c.instances.a]\ndir='~/a' # x\nsettings=true\n[targets.c.instances.a.roots]\nskills='~/s'\n# tail",
	} {
		for op := range uint8(4) {
			f.Add([]byte(s), op, uint8(1))
		}
	}
	f.Fuzz(func(t *testing.T, data []byte, op, n uint8) {
		m, err := manifest.Parse("kitbag.toml", data)
		if err != nil || len(m.Targets) == 0 {
			return
		}
		target := slices.Sorted(maps.Keys(m.Targets))[0]
		edit := fuzzEdit(m.Targets[target], target, op, n)
		out, err := manifestedit.Apply("kitbag.toml", data, edit)
		if err != nil {
			allowed := allowedRefusals(data, m, target)
			if !slices.ContainsFunc(allowed, func(e error) bool { return errors.Is(err, e) }) {
				t.Fatalf("unexpected refusal %v for %+v of\n%s", err, edit, data)
			}
			return
		}
		if edit.Op == manifestedit.SetInstanceHosts {
			checkHosts(t, out, edit)
			return
		}
		checkResult(t, data, out, edit)
	})
}

// allowedRefusals lists the errors an edit of target in m may end with.
func allowedRefusals(data []byte, m *manifest.Manifest, target string) []error {
	allowed := []error{manifestedit.ErrLayout, manifestedit.ErrExists, manifestedit.ErrNotFound}
	if bytes.Contains(bytes.ToLower(data), []byte(".fuzz")) || m.Targets[target].Agent == "generic" || replaced(m, target) {
		allowed = append(allowed, manifestedit.ErrVerify)
	}
	return allowed
}

func checkHosts(t *testing.T, after []byte, edit manifestedit.Edit) {
	t.Helper()
	m, err := manifest.Parse("kitbag.toml", after)
	if err != nil {
		t.Fatalf("accepted result does not parse: %v", err)
	}
	instance, ok := m.Targets[edit.Target].Instances[edit.Name]
	if !ok || !slices.Equal(instance.Hosts, edit.Hosts) {
		t.Fatalf("result does not match %+v:\n%s", edit, after)
	}
}

func fuzzEdit(target manifest.Target, name string, op, n uint8) manifestedit.Edit {
	dir := "~/.fuzz-" + strconv.Itoa(int(n))
	instances := slices.Sorted(maps.Keys(target.Instances))
	switch {
	case op%4 == 3 && len(instances) > 0:
		var hosts []string
		if n%2 == 1 {
			hosts = []string{"h" + strconv.Itoa(int(n)), "mac"}
		}
		return manifestedit.Edit{Op: manifestedit.SetInstanceHosts, Target: name, Name: instances[0], Hosts: hosts}
	case op%4 == 1 && len(instances) > 0:
		return manifestedit.Edit{Op: manifestedit.EditInstance, Target: name, Name: instances[0], Dir: dir, Settings: off()}
	case op%4 == 2 && len(instances) > 0:
		return manifestedit.Edit{Op: manifestedit.RemoveInstance, Target: name, Name: instances[0]}
	}
	return manifestedit.Edit{Op: manifestedit.AddInstance, Target: name, Name: "fuzz" + strconv.Itoa(int(n)), Dir: dir}
}

func checkResult(t *testing.T, before, after []byte, edit manifestedit.Edit) {
	t.Helper()
	m, err := manifest.Parse("kitbag.toml", after)
	if err != nil {
		t.Fatalf("accepted result does not parse: %v", err)
	}
	instance, ok := m.Targets[edit.Target].Instances[edit.Name]
	if ok == (edit.Op == manifestedit.RemoveInstance) || (ok && instance.Dir != edit.Dir) {
		t.Fatalf("result does not match %+v:\n%s", edit, after)
	}
	if edit.Op != manifestedit.AddInstance {
		return
	}
	prefix := 0
	for prefix < len(before) && before[prefix] == after[prefix] {
		prefix++
	}
	if !bytes.HasSuffix(after, before[prefix:]) {
		t.Fatalf("add changed bytes outside one insertion:\n%q\n%q", before, after)
	}
}

// replaced reports whether a host's replacement set declares target, which
// the manifest refuses together with an instance hosts list.
func replaced(m *manifest.Manifest, target string) bool {
	for _, host := range m.Hosts {
		if host.Targets[target].Instances != nil {
			return true
		}
	}
	return false
}
