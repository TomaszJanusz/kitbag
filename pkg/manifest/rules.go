package manifest

// rule mirrors only the schema-1 and schema-2 vocabulary. Keeping the schema interpreter out
// of production avoids a second public model and a JSON Schema runtime dependency.
type rule struct {
	properties map[string]*rule
	// refused maps a key that is not allowed here to the reason that replaces
	// the generic unknown-key message.
	refused  map[string]string
	item     *rule
	names    *rule
	kind     string
	pattern  string
	role     string
	required []string
	values   []string
	minimum  int
	maximum  int
	unique   bool
}

// schemaInstanceHosts is the first schema with instance hosts filters and
// host instance patches.
const schemaInstanceHosts = 2

const (
	maxPathLength  = 4096
	namePattern    = `^[a-z0-9][a-z0-9_.-]{0,63}$`
	hostPattern    = `^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`
	envPattern     = `^[A-Za-z_][A-Za-z0-9_]*$`
	versionPattern = `^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`
	refPattern     = `^[A-Za-z0-9][A-Za-z0-9/_.-]{0,254}$`
	repoPattern    = `^([^./\\~$][^/\\]*|\.[^./\\][^/\\]*|\.\.[^/\\]+)(/([^./\\][^/\\]*|\.[^./\\][^/\\]*|\.\.[^/\\]+))*$`
	homePattern    = `^~/([^./\\][^/\\]*|\.[^./\\][^/\\]*|\.\.[^/\\]+)(/([^./\\][^/\\]*|\.[^./\\][^/\\]*|\.\.[^/\\]+))*$`
)

func textRule(pattern string) *rule { return &rule{kind: "string", pattern: pattern} }
func enum(values ...string) *rule   { return &rule{kind: "string", values: values} }
func object(properties map[string]*rule, required ...string) *rule {
	return &rule{kind: "object", properties: properties, required: required}
}

func dictionary(names, item *rule, minimum int) *rule {
	return &rule{kind: "object", names: names, item: item, minimum: minimum}
}

func array(item *rule, minimum int, unique bool) *rule {
	return &rule{kind: "array", item: item, minimum: minimum, unique: unique}
}

func pathRule(role string) *rule {
	pattern := repoPattern
	if role == "home" {
		pattern = homePattern
	}
	return &rule{kind: "string", pattern: pattern, maximum: maxPathLength, role: role}
}

func agentRule() *rule {
	return enum("claude", "codex", "cursor", "gemini", "opencode", "copilot", "windsurf", "generic")
}

func primitiveRule() *rule {
	return enum("instructions", "skills", "commands", "agents", "hooks", "rules", "mcp")
}
func perAgent(role string) *rule { return dictionary(agentRule(), pathRule(role), 1) }
func collectionRule() *rule {
	r := object(map[string]*rule{"dir": pathRule("directory"), "per_agent": perAgent("directory")})
	r.minimum = 1
	return r
}

func primitivesRule() *rule {
	props := map[string]*rule{}
	for _, key := range primitiveRule().values {
		option := object(map[string]*rule{"enabled": {kind: "boolean"}, "mode": enum("link", "copy")})
		if key == "instructions" {
			option.properties["render"] = enum("import", "concat")
		}
		props[key] = option
	}
	r := object(props)
	r.minimum = 1
	return r
}

func instancesRule(schema int64) *rule {
	properties := map[string]*rule{
		"dir": pathRule("home"), "enabled": {kind: "boolean"}, "mode": enum("link", "copy"),
		"settings": {kind: "boolean"}, "roots": dictionary(primitiveRule(), pathRule("home"), 1),
		"primitives": primitivesRule(),
	}
	instance := object(properties, "dir")
	if schema >= schemaInstanceHosts {
		properties["hosts"] = hostsRule()
	} else {
		instance.refused = map[string]string{"hosts": "'hosts' on an instance needs manifest schema 2; preview the conversion with kitbag manifest upgrade --dry-run"}
	}
	return dictionary(textRule(namePattern), instance, 1)
}

func hostsRule() *rule { return array(textRule(hostPattern), 1, true) }

// instancePatchesRule is schema 2's [hosts.<h>.targets.<t>.instances.<n>]:
// it patches a target-level instance and never declares one.
func instancePatchesRule() *rule {
	patch := object(map[string]*rule{"enabled": {kind: "boolean"}, "mode": enum("link", "copy"), "settings": {kind: "boolean"}})
	patch.minimum = 1
	patch.refused = map[string]string{}
	for _, key := range []string{"dir", "roots", "primitives", "hosts"} {
		patch.refused[key] = "'" + key + "' is not allowed in a host instance patch, which sets only enabled, mode and settings; " +
			"declare it on the target's instance and limit that instance to hosts with its 'hosts' key"
	}
	return dictionary(textRule(namePattern), patch, 1)
}

func targetRule(schema int64) *rule {
	return object(map[string]*rule{
		"agent": agentRule(), "enabled": {kind: "boolean"}, "mode": enum("link", "copy"),
		"settings": pathRule("file"), "primitives": primitivesRule(), "instances": instancesRule(schema),
	}, "agent")
}

func publishRule() *rule {
	r := object(map[string]*rule{"mode": enum("direct", "pr"), "branch": textRule(refPattern)})
	r.minimum = 1
	return r
}

func hostRule(schema int64) *rule {
	instances := instancesRule(schema)
	if schema >= schemaInstanceHosts {
		instances = instancePatchesRule()
	}
	r := object(map[string]*rule{
		"mode": enum("link", "copy"), "publish": publishRule(),
		"targets": dictionary(textRule(namePattern), object(map[string]*rule{
			"enabled": {kind: "boolean"}, "mode": enum("link", "copy"), "instances": instances,
		}), 1),
	})
	r.minimum = 1
	return r
}

func skillsRule() *rule {
	source := textRule(`^[^\s]+$`)
	source.minimum = 1
	return object(map[string]*rule{
		"dirs": array(pathRule("directory"), 1, true), "interchange": pathRule("file"),
		"imports": dictionary(textRule(namePattern), object(map[string]*rule{
			"source": source, "path": pathRule("foreign"), "ref": textRule(refPattern), "dir": pathRule("directory"),
		}, "source"), 1),
	}, "dirs")
}

func serverRule() *rule {
	command := textRule("")
	command.minimum = 1
	return object(map[string]*rule{
		"transport": enum("stdio", "http", "sse"), "command": command,
		"args": array(textRule(""), 0, false), "env": dictionary(textRule(envPattern), textRule(""), 0),
		"url": textRule(`^https?://[^\s]+$`), "headers": dictionary(nil, textRule(""), 0),
		"secret_env": array(textRule(envPattern), 0, true), "agents": array(agentRule(), 1, true),
	})
}

// manifestRule returns the vocabulary of schema; any value other than 2
// selects schema 1, whose own check rejects unsupported values.
func manifestRule(schema int64) *rule {
	return object(map[string]*rule{
		"schema": {kind: "integer"}, "min_tool_version": textRule(versionPattern), "mode": enum("link", "copy"),
		"instructions": object(map[string]*rule{
			"source": pathRule("file"), "hosts": pathRule("template"), "per_agent": perAgent("file"),
		}, "source"),
		"skills": skillsRule(), "commands": collectionRule(), "agents": collectionRule(),
		"hooks": collectionRule(), "rules": collectionRule(),
		"mcp": object(map[string]*rule{
			"policy": enum("merge", "replace"), "servers": dictionary(textRule(namePattern), serverRule(), 1),
		}, "servers"),
		"files": array(object(map[string]*rule{
			"src": pathRule("source"), "dest": pathRule("home"), "mode": enum("link", "copy"),
			"hosts": hostsRule(),
		}, "src", "dest"), 0, false),
		"git":     object(map[string]*rule{"include": array(pathRule("file"), 1, true)}, "include"),
		"targets": dictionary(textRule(namePattern), targetRule(schema), 1),
		"hosts":   dictionary(textRule(hostPattern), hostRule(schema), 1), "publish": publishRule(),
	}, "schema")
}
