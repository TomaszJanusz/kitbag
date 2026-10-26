package api

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/TomaszJanusz/kitbag/internal/core/agents"
	"github.com/TomaszJanusz/kitbag/internal/core/apply"
	"github.com/TomaszJanusz/kitbag/internal/core/workflow"
	"github.com/TomaszJanusz/kitbag/internal/platform/overlay"
	"github.com/TomaszJanusz/kitbag/pkg/manifest"
)

// agents serves agents.list with the caller's relocation variables, and
// agents.add/edit/remove through the same core workflow as the CLI.
func (h Handlers) agents(ctx context.Context, method string, p Params, id json.RawMessage) (any, *Error) {
	service, err := h.Service(ctx, p.Options)
	if err != nil {
		return nil, operationError(err)
	}
	if method == "agents.list" {
		if p.Env != nil {
			service.Host = overlay.Environment(service.Host, service.AgentVariables(), p.Env)
		}
		result, listErr := service.Agents(p.Probe)
		return result, agentError(listErr)
	}
	if p.Agent == nil {
		return nil, rpcError(InvalidParams, "invalid params: agent is required", apply.Usage)
	}
	change := *p.Agent
	change.Op = agents.Op(strings.TrimPrefix(method, "agents."))
	result, err := service.ChangeAgent(ctx, change, workflow.AgentOptions{DryRun: p.DryRun, Revision: p.Revision})
	if result.Sync != nil {
		h.Events.Publish(Event{Kind: "sync.finished", RequestID: id, ExitCode: int(result.Sync.ExitCode), Changed: result.Sync.Changed})
	}
	if err != nil {
		return nil, agentError(err)
	}
	return result, nil
}

// agentError adds the public detail of a refused agent change: validation
// messages name only kinds, targets, instances and paths the caller chose.
func agentError(err error) *Error {
	problem := operationError(err)
	if problem == nil || problem.Code == Busy {
		return problem
	}
	var invalid *manifest.Error
	if errors.Is(err, agents.ErrInvalid) || errors.Is(err, agents.ErrUnsupported) || errors.Is(err, apply.ErrConflict) || errors.As(err, &invalid) {
		problem.Data.Detail = err.Error()
	}
	return problem
}
