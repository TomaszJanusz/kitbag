package agents

import "github.com/TomaszJanusz/kitbag/pkg/manifest"

// RestrictHint is the fix for a declared instance whose directory is missing
// on this machine: restrict it to the machines that have it (ADR 12). On a
// schema-1 manifest the upgrade comes first, since hosts needs schema 2.
func RestrictHint(schema int, target, name string) string {
	edit := "kitbag agent edit " + target + "/" + name + " --hosts <hosts>"
	if schema < manifest.SchemaLatest {
		return "if only some machines have it, upgrade the manifest (kitbag manifest upgrade), then restrict it: " + edit
	}
	return "if only some machines have it, restrict it: " + edit
}
