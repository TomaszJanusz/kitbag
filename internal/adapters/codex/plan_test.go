package codex_test

import (
	"strings"
	"testing"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
)

func operations(p plan.Plan) map[string]plan.Operation {
	ops := map[string]plan.Operation{}
	for _, op := range p.Operations {
		ops[op.Path] = op
	}
	return ops
}

// TestForeignSkillDirectoryIsLeftAlone: a real directory already at a skill's
// shared destination is an unmanaged obstacle; it is skipped with a warning
// and the other skills are still materialized.
func TestForeignSkillDirectoryIsLeftAlone(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.write(t, sharedSkills+"/alpha/SKILL.md", "Foreign alpha.\n")
	ops := operations(f.build(t, everything))
	foreign := ops[sharedSkills+"/alpha"]
	if foreign.Kind != plan.Skip || foreign.Classification != plan.UnmanagedObstacle || foreign.Warning == "" {
		t.Fatalf("foreign directory: %+v", foreign)
	}
	if ops[sharedSkills+"/beta"].Kind != plan.Materialize || ops[sharedSkills+"/gamma"].Kind != plan.Materialize {
		t.Fatalf("other skills not materialized: %+v", ops)
	}
	for path, op := range ops {
		if strings.HasPrefix(path, sharedSkills+"/alpha") && op.Kind != plan.Skip {
			t.Fatalf("operation on foreign directory: %+v", op)
		}
	}
}

// TestSharedSkillsAcrossInstances: two instances share one operation each,
// and an instance's copy mode wins for the shared root.
func TestSharedSkillsAcrossInstances(t *testing.T) {
	t.Parallel()
	text := everything + "[targets.codex.instances.main]\ndir = \"~/.codex\"\n[targets.codex.instances.work]\ndir = \"~/.codex-work\"\nmode = \"copy\"\n"
	f := newFixture(t, nil)
	f.write(t, codexHome+"/keep", "")
	f.write(t, home+"/.codex-work/keep", "")
	p := f.build(t, text)
	op := operations(p)[sharedSkills+"/alpha"]
	if op.Kind != plan.Materialize || len(op.Desired.Owners) != 2 || op.Desired.Mode != "copy" {
		t.Fatalf("shared skill: %+v", op)
	}
}

// TestMissingListedInstance protects a listed instance that does not exist
// while the shared skills root is still applied, as agent-config-sync did.
func TestMissingListedInstance(t *testing.T) {
	t.Parallel()
	text := everything + "[targets.codex.instances.main]\ndir = \"~/.codex\"\n"
	ops := operations(newFixture(t, nil).build(t, text))
	if op := ops[codexHome+"/AGENTS.md"]; op.Kind != plan.Skip || !strings.Contains(op.Warning, "instance directory is missing") {
		t.Fatalf("missing instance AGENTS.md: %+v", op)
	}
	if ops[sharedSkills+"/alpha"].Kind != plan.Materialize {
		t.Fatalf("shared skills skipped: %+v", ops)
	}
}

// TestDefaultInstanceIsCreated: the unlisted default home may be absent.
func TestDefaultInstanceIsCreated(t *testing.T) {
	t.Parallel()
	ops := operations(newFixture(t, nil).build(t, everything))
	for _, path := range []string{codexHome + "/AGENTS.md", codexHome + "/prompts/review.md"} {
		if ops[path].Kind != plan.Write && ops[path].Kind != plan.Materialize {
			t.Fatalf("%s: %+v", path, ops[path])
		}
	}
}

// TestUnmanagedAgentsFileIsBackedUpOnce: an existing hand-written AGENTS.md.
func TestUnmanagedAgentsFileIsBackedUpOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t, nil)
	f.write(t, codexHome+"/AGENTS.md", "Hand-written.\n")
	var kinds []plan.Kind
	for _, op := range f.build(t, everything).Operations {
		if op.Path == codexHome+"/AGENTS.md" {
			kinds = append(kinds, op.Kind)
		}
	}
	if len(kinds) != 2 || kinds[0] != plan.Backup || kinds[1] != plan.Write {
		t.Fatalf("unmanaged AGENTS.md: %v", kinds)
	}
}

// TestDisabledSkillsDoNotChangeSharedMode keeps an opted-out instance from
// changing the representation requested by instances that actually own skills.
func TestDisabledSkillsDoNotChangeSharedMode(t *testing.T) {
	t.Parallel()
	text := everything + `[targets.codex.instances.main]
dir = "~/.codex"
mode = "link"
[targets.codex.instances.work]
dir = "~/.codex-work"
mode = "copy"
primitives.skills.enabled = false
`
	p := newFixture(t, nil).build(t, text)
	op := operations(p)[sharedSkills+"/alpha"]
	if op.Kind != plan.Materialize || op.Desired.Mode != "link" || len(op.Desired.Owners) != 1 {
		t.Fatalf("shared skill: %+v", op.Desired)
	}
}
