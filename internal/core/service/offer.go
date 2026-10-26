package service

import (
	"context"
	"path/filepath"
)

// Offer is what a setup command (init, adopt) does about the background
// service. Without a flag a noninteractive run never installs silently.
type Offer int

const (
	// OfferHint prints how to install the service later.
	OfferHint Offer = iota
	// OfferAsk asks on the terminal; the default answer is yes.
	OfferAsk
	// OfferInstall installs the service (--service).
	OfferInstall
	// OfferDecline skips the service on request (--no-service).
	OfferDecline
)

// Decide maps the --service and --no-service flags and terminal availability
// to an Offer. Callers reject both flags together.
func Decide(install, decline, interactive bool) Offer {
	switch {
	case install:
		return OfferInstall
	case decline:
		return OfferDecline
	case interactive:
		return OfferAsk
	default:
		return OfferHint
	}
}

// Installation lists the files an installation wrote, or found up to date.
type Installation struct {
	DaemonVersion, BinaryVersion string
	Mode                         string
	Paths                        []string
	Changed                      bool
	Restarted, Started, Deferred bool
}

// InstallDefault installs and starts the daemon-mode service, exactly like
// kitbag service install without flags.
func (s Service) InstallDefault(ctx context.Context) (Installation, error) {
	options, err := s.Current("daemon", DefaultInterval)
	if err != nil {
		return Installation{}, err
	}
	changed, err := s.Install(ctx, options, false)
	if err != nil {
		return Installation{}, err
	}
	paths, err := s.Paths(options)
	return Installation{Mode: options.Mode, Paths: paths, Changed: changed}, err
}

// Paths returns the definition files and the saved installation inputs that
// Install writes for options, in a stable order.
func (s Service) Paths(options Options) ([]string, error) {
	layout, err := s.Host.ServiceLayout()
	if err != nil {
		return nil, err
	}
	files, err := Generate(layout.Manager, options)
	if err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(files)+1)
	for _, name := range sortedKeys(files) {
		paths = append(paths, filepath.Join(layout.Directory, name))
	}
	metadata, err := s.metadata()
	return append(paths, metadata), err
}
