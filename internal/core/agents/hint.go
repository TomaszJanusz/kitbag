package agents

// RestrictHint is the fix for a declared instance whose directory is missing
// on this machine: limit it to the machines that have it (ADR 12).
func RestrictHint(target, name string) string {
	return "if only some machines have it, restrict it: kitbag agent edit " + target + "/" + name + " --hosts <hosts>"
}
