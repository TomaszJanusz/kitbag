package plan

import (
	"encoding/json"
	"fmt"
	"strings"
)

// JSON renders a versioned plan with stable indentation and a final newline.
// It excludes output bytes and native merger implementation details.
func (p Plan) JSON() ([]byte, error) {
	if p.Operations == nil {
		p.Operations = []Operation{}
	}
	if p.Observations == nil {
		p.Observations = []Observation{}
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

// Text renders ordered actions. In-sync observations remain available to status
// consumers without adding noise to an empty action plan.
func (p Plan) Text() string {
	if len(p.Operations) == 0 {
		return "No changes.\n"
	}
	lines := make([]string, 0, len(p.Operations))
	for _, op := range p.Operations {
		line := fmt.Sprintf("%s %q [%s]", op.Kind, op.Path, op.Classification)
		if op.BackupPath != "" {
			line += fmt.Sprintf(" -> %q", op.BackupPath)
		}
		if op.Warning != "" {
			line += ": " + op.Warning
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n") + "\n"
}
