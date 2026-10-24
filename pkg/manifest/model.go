package manifest

import "io/fs"

// FileSystem is the read-only subset of internal/platform.FS used by validation.
// A platform.FS satisfies it; callers outside this module can supply their own.
type FileSystem interface {
	Lstat(string) (fs.FileInfo, error)
	ReadFile(string) ([]byte, error)
	ReadDir(string) ([]fs.DirEntry, error)
	ReadLink(string) (string, error)
}

// Environment is the subset of internal/platform.Env used for host selection.
type Environment interface {
	LookupEnv(string) (string, bool)
	Hostname() (string, error)
}

// Manifest is schema-v1 repository intent. Empty optional fields are unmanaged;
// nil Enabled and Settings switches mean true. No machine default mode is inferred.
type Manifest struct {
	Instructions   *Instructions     `toml:"instructions"`
	Skills         *Skills           `toml:"skills"`
	Commands       *Collection       `toml:"commands"`
	Agents         *Collection       `toml:"agents"`
	Hooks          *Collection       `toml:"hooks"`
	Rules          *Collection       `toml:"rules"`
	MCP            *MCP              `toml:"mcp"`
	Git            *Git              `toml:"git"`
	Targets        map[string]Target `toml:"targets"`
	Hosts          map[string]Host   `toml:"hosts"`
	Publish        *Publish          `toml:"publish"`
	doc            *document
	MinToolVersion string `toml:"min_tool_version"`
	Mode           string `toml:"mode"`
	Files          []File `toml:"files"`
	Schema         int    `toml:"schema"`
}

// Instructions declares shared, host and agent-specific instruction sources.
type Instructions struct {
	PerAgent map[string]string `toml:"per_agent"`
	Source   string            `toml:"source"`
	Hosts    string            `toml:"hosts"`
}

// Skills declares local skill directories and optional import intent.
type Skills struct {
	Imports     map[string]SkillImport `toml:"imports"`
	Interchange string                 `toml:"interchange"`
	Dirs        []string               `toml:"dirs"`
}

// SkillImport records a requested source and selector, never resolved provenance.
type SkillImport struct {
	Source string `toml:"source"`
	Path   string `toml:"path"`
	Ref    string `toml:"ref"`
	Dir    string `toml:"dir"`
}

// Collection declares native files shared across agents or specific to one kind.
type Collection struct {
	PerAgent map[string]string `toml:"per_agent"`
	Dir      string            `toml:"dir"`
}

// MCP declares server definitions and their merge policy (unset means merge).
type MCP struct {
	Servers map[string]Server `toml:"servers"`
	Policy  string            `toml:"policy"`
}

// Server is either a stdio command or a remote URL. Empty Transport means stdio
// with Command, or http with URL. Env and Headers contain committed literals.
type Server struct {
	Env       map[string]string `toml:"env"`
	Headers   map[string]string `toml:"headers"`
	Transport string            `toml:"transport"`
	Command   string            `toml:"command"`
	URL       string            `toml:"url"`
	Args      []string          `toml:"args"`
	SecretEnv []string          `toml:"secret_env"`
	Agents    []string          `toml:"agents"`
}

// File maps a repository source to a home-relative destination.
type File struct {
	Src   string   `toml:"src"`
	Dest  string   `toml:"dest"`
	Mode  string   `toml:"mode"`
	Hosts []string `toml:"hosts"`
}

// Git declares additive global Git includes.
type Git struct {
	Include []string `toml:"include"`
}

// Target selects an adapter and optional instances, settings and primitive policy.
type Target struct {
	Enabled    *bool                `toml:"enabled"`
	Primitives map[string]Primitive `toml:"primitives"`
	Instances  map[string]Instance  `toml:"instances"`
	Agent      string               `toml:"agent"`
	Mode       string               `toml:"mode"`
	Settings   string               `toml:"settings"`
}

// Primitive overrides one primitive. Render is valid only for instructions.
type Primitive struct {
	Enabled *bool  `toml:"enabled"`
	Mode    string `toml:"mode"`
	Render  string `toml:"render"`
}

// Instance declares an agent directory and optional primitive destination roots.
type Instance struct {
	Enabled    *bool                `toml:"enabled"`
	Settings   *bool                `toml:"settings"`
	Roots      map[string]string    `toml:"roots"`
	Primitives map[string]Primitive `toml:"primitives"`
	Dir        string               `toml:"dir"`
	Mode       string               `toml:"mode"`
}

// Host overrides manifest defaults for one host.
type Host struct {
	Publish *Publish              `toml:"publish"`
	Targets map[string]HostTarget `toml:"targets"`
	Mode    string                `toml:"mode"`
}

// HostTarget overrides a declared target. Instances replaces its entire set.
type HostTarget struct {
	Enabled   *bool               `toml:"enabled"`
	Instances map[string]Instance `toml:"instances"`
	Mode      string              `toml:"mode"`
}

// Publish declares publication policy. Empty Mode means direct.
type Publish struct {
	Mode   string `toml:"mode"`
	Branch string `toml:"branch"`
}
