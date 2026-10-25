package claude

import "strings"

// HookMarker identifies command hook groups owned by KitBag. The legacy marker
// is accepted for migration, but every newly written command uses HookMarker.
const (
	HookMarker       = "# kitbag-managed"
	legacyHookMarker = "# agent-config-managed"
)

// Merger merges a Claude settings fragment without host access. Repository is
// the absolute working copy path supplied by the caller for placeholder expansion.
// Inputs are never mutated. Invalid/non-object JSON or unsafe hook shapes fail
// closed; the planner reports a warning and leaves the destination alone.
type Merger struct{ Repository string }

// Merge ports agent-config-sync's recursive merge and hook-group ownership.
// An absent/empty current file is an empty object. Both {repo} and legacy @REPO@
// string values expand literally; object keys and current local strings do not.
func (m Merger) Merge(current, fragment []byte) ([]byte, error) {
	cur, err := parseObject(current, true)
	if err != nil {
		return nil, err
	}
	want, err := parseObject(fragment, false)
	if err != nil {
		return nil, err
	}
	want = asObject(substitute(want, m.Repository))
	hooks, err := mergeHooks(cur.get("hooks"), want.get("hooks"))
	if err != nil {
		return nil, err
	}
	result := asObject(mergeValues(cur, want.without("hooks")))
	if len(hooks) == 0 {
		result = result.without("hooks")
	} else {
		result.set("hooks", hooks)
	}
	return formatJSON(result)
}

func substitute(value any, repository string) any {
	switch v := value.(type) {
	case string:
		return strings.NewReplacer("{repo}", repository, "@REPO@", repository).Replace(v)
	case object:
		for i := range v {
			v[i].value = substitute(v[i].value, repository)
		}
	case []any:
		for i := range v {
			v[i] = substitute(v[i], repository)
		}
	}
	return value
}

func mergeValues(current, want any) any {
	switch w := want.(type) {
	case object:
		if c, ok := current.(object); ok {
			for _, entry := range w {
				c.set(entry.key, mergeValues(c.get(entry.key), entry.value))
			}
			return c
		}
	case []any:
		if c, ok := current.([]any); ok {
			for _, entry := range w {
				if !contains(c, entry) {
					c = append(c, entry)
				}
			}
			return c
		}
	}
	return want
}

func contains(values []any, wanted any) bool {
	for _, value := range values {
		if equal(value, wanted) {
			return true
		}
	}
	return false
}
