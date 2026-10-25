package claude

import (
	"errors"
	"strings"
)

func mergeHooks(current, want any) (object, error) {
	kept, err := hookEvents(current, false)
	if err != nil {
		return nil, err
	}
	wanted, err := hookEvents(want, true)
	if err != nil {
		return nil, err
	}
	result := object{}
	for _, event := range kept {
		groups := asArray(event.value)
		retained := []any{}
		for _, group := range groups {
			if !managedGroup(asObject(group)) {
				retained = append(retained, group)
			}
		}
		result.set(event.key, retained)
	}
	for _, event := range wanted {
		groups := asArray(result.get(event.key))
		for _, group := range asArray(event.value) {
			groups = append(groups, markGroup(asObject(group)))
		}
		result.set(event.key, groups)
	}
	nonempty := object{}
	for _, event := range result {
		if len(asArray(event.value)) > 0 {
			nonempty = append(nonempty, event)
		}
	}
	return nonempty, nil
}

func hookEvents(value any, strict bool) (object, error) {
	if value == nil || value == false {
		return object{}, nil
	}
	events, ok := value.(object)
	if !ok {
		return nil, errors.New("settings: hooks must be an object")
	}
	for _, event := range events {
		groups, valid := event.value.([]any)
		if !valid {
			return nil, errors.New("settings: hook event must be an array")
		}
		for _, value := range groups {
			group, valid := value.(object)
			if !valid {
				return nil, errors.New("settings: hook group must be an object")
			}
			if err := validateGroup(group, strict); err != nil {
				return nil, err
			}
		}
	}
	return events, nil
}

func validateGroup(group object, strict bool) error {
	value := group.get("hooks")
	if value == nil && !strict {
		return nil
	}
	hooks, ok := value.([]any)
	if !ok {
		return errors.New("settings: group hooks must be an array")
	}
	for _, value := range hooks {
		if err := validateHook(value, strict); err != nil {
			return err
		}
	}
	return nil
}

func managedGroup(group object) bool {
	hooks := asArray(group.get("hooks"))
	for _, value := range hooks {
		hook := asObject(value)
		command := asString(hook.get("command"))
		if strings.HasSuffix(command, HookMarker) || strings.HasSuffix(command, legacyHookMarker) {
			return true
		}
	}
	return false
}

func markGroup(group object) object {
	hooks := asArray(group.get("hooks"))
	for _, value := range hooks {
		hook := asObject(value)
		command := asString(hook.get("command"))
		command = unmark(command)
		hook.set("command", command+" "+HookMarker)
	}
	return group
}

func validateHook(value any, strict bool) error {
	hook, valid := value.(object)
	if !valid {
		return errors.New("settings: hook must be an object")
	}
	command := hook.get("command")
	if command != nil {
		if _, valid = command.(string); !valid {
			return errors.New("settings: hook command must be a string")
		}
	}
	if strict && (command == nil || (hook.get("type") != nil && hook.get("type") != "command")) {
		return errors.New("settings: only command hooks are supported")
	}
	return nil
}

func unmark(command string) string {
	for _, marker := range []string{legacyHookMarker, HookMarker} {
		if strings.HasSuffix(command, marker) {
			return strings.TrimSuffix(command, " "+marker)
		}
	}
	return command
}
