package tui

import "github.com/TomaszJanusz/kitbag/internal/ui/model"

func intentName(intent model.Msg) string {
	switch intent := intent.(type) {
	case model.RequestServiceRefresh:
		return "h"
	case model.SyncNow:
		return keySync
	case model.Refresh:
		return keyRefresh
	case model.SaveSetting:
		return keySave
	case model.UnsetSetting:
		return keyUnset
	case model.RevertSetting:
		return keyEscape
	case model.ProbeAgents:
		return keyProbe
	case model.OpenAgentForm:
		return map[model.AgentOp]string{model.AgentAdd: keyAdd, model.AgentEdit: keyEdit, model.AgentRemove: keyRemove}[intent.Op]
	default:
		return ""
	}
}
