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
		for op := range uint8(3) {
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
			allowed := []error{manifestedit.ErrLayout, manifestedit.ErrExists, manifestedit.ErrNotFound}
			if bytes.Contains(bytes.ToLower(data), []byte(".fuzz")) || m.Targets[target].Agent == "generic" {
				allowed = append(allowed, manifestedit.ErrVerify)
			}
			if !slices.ContainsFunc(allowed, func(e error) bool { return errors.Is(err, e) }) {
				t.Fatalf("unexpected refusal %v for %+v of\n%s", err, edit, data)
			}
			return
		}
		checkResult(t, data, out, edit)
	})
}

func fuzzEdit(target manifest.Target, name string, op, n uint8) manifestedit.Edit {
	dir := "~/.fuzz-" + strconv.Itoa(int(n))
	instances := slices.Sorted(maps.Keys(target.Instances))
	switch {
	case op%3 == 1 && len(instances) > 0:
		return manifestedit.Edit{Op: manifestedit.EditInstance, Target: name, Name: instances[0], Dir: dir, Settings: off()}
	case op%3 == 2 && len(instances) > 0:
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
