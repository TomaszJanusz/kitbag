package fake

import (
	"errors"
	"io"
	"io/fs"

	"github.com/TomaszJanusz/kitbag/internal/platform/internal/check"
)

// WriteStage names a pre-commit failure in the simulated atomic writer.
type WriteStage int

// Failure stages leave the destination unchanged, just as Unix does before rename.
const (
	PartialWrite WriteStage = iota + 1
	BeforeRename
)

type writeFault struct {
	path  string
	skip  int
	stage WriteStage
}

// InterruptNextWrite injects a one-shot failure for path: a short write to the
// staged node, or a failed rename. The staged node is discarded; the destination
// remains intact. Other paths and Link are unaffected. A later call replaces
// a pending fault. This models interruption, not process death or directory sync.
func (h *Host) InterruptNextWrite(path string, stage WriteStage) error {
	return h.InterruptWriteAfter(path, stage, 0)
}

// InterruptWriteAfter skips successful matching writes before injecting the same
// one-shot failure as InterruptNextWrite. Other paths do not consume the count.
func (h *Host) InterruptWriteAfter(path string, stage WriteStage, skip int) error {
	if err := check.Path("interrupt-write", path); err != nil {
		return err
	}
	if skip < 0 || (stage != PartialWrite && stage != BeforeRename) {
		return fs.ErrInvalid
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.writeFault = &writeFault{path: path, stage: stage, skip: skip}
	return nil
}

// interruptWrite runs with h.mu held, after staging and before installation.
func (h *Host) interruptWrite(path string, staged *node) error {
	fault := h.writeFault
	if fault == nil || fault.path != path {
		return nil
	}
	if fault.skip > 0 {
		fault.skip--
		return nil
	}
	h.writeFault = nil
	switch fault.stage {
	case PartialWrite:
		const partialDivisor = 2
		staged.data = staged.data[:len(staged.data)/partialDivisor]
		return io.ErrUnexpectedEOF
	case BeforeRename:
		return errors.New("injected atomic rename failure")
	default:
		return fs.ErrInvalid
	}
}
