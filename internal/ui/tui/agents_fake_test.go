package tui

import (
	"context"

	"github.com/TomaszJanusz/kitbag/internal/ui/model"
)

// The terminal renderer has no agents view yet; these satisfy model.Client.

func (*fakeClient) Agents(context.Context, bool) (model.AgentsSnapshot, error) {
	return model.AgentsSnapshot{}, nil
}

func (*fakeClient) PreviewAgentChange(context.Context, model.AgentChange) (model.AgentPreview, error) {
	return model.AgentPreview{}, nil
}

func (*fakeClient) ApplyAgentChange(context.Context, model.AgentChange, string) (model.AgentApplied, error) {
	return model.AgentApplied{}, nil
}
