package apply

import (
	"path/filepath"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/plan"
	"github.com/TomaszJanusz/kitbag/internal/core/state"
)

func validateStorage(session *state.Session, p plan.Plan) error {
	directory, err := session.Directory()
	if err != nil {
		return err
	}
	for _, op := range p.Operations {
		if contains(directory, op.Path) || contains(op.Path, directory) {
			return errUsage()
		}
	}
	return nil
}

func contains(parent, path string) bool {
	for {
		if strings.EqualFold(parent, path) {
			return true
		}
		next := filepath.Dir(path)
		if next == path {
			return false
		}
		path = next
	}
}
