package settings

import "github.com/TomaszJanusz/kitbag/internal/config"

// ChangeWorkingCopy applies [Service.Change] and reports the working copy that
// the change leaves unused: the effective one before a change of repo.url or
// repo.path, when it is an existing directory and no longer the effective
// one. It is never removed; it may hold unpushed work.
func (s Service) ChangeWorkingCopy(path, text string, unset bool) (string, error) {
	if path != "repo.url" && path != "repo.path" {
		return "", s.Change(path, text, unset)
	}
	before := s.workingCopy()
	if err := s.Change(path, text, unset); err != nil {
		return "", err
	}
	if before == "" || before == s.workingCopy() {
		return "", nil
	}
	// A missing or unreadable former copy needs no note.
	if info, err := s.Host.Stat(before); err == nil && info.IsDir() {
		return before, nil
	}
	return "", nil
}

// workingCopy is the effective working copy, or "" when it cannot be resolved.
func (s Service) workingCopy() string {
	url, err := s.Get("repo.url")
	if err != nil || url.Value == "" {
		return ""
	}
	path, err := s.Get("repo.path")
	if err != nil {
		return ""
	}
	cfg := config.Config{}
	cfg.Repo.URL, cfg.Repo.Path = url.Value, path.Value
	repo, err := cfg.WorkingCopy(s.Host)
	if err != nil {
		return ""
	}
	return repo
}
