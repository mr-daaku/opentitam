package agent

import (
	"context"

	"github.com/opentitam-ai/opentitam/internal/history"
	"github.com/opentitam-ai/opentitam/internal/llm/tools"
	"github.com/opentitam-ai/opentitam/internal/lsp"
	"github.com/opentitam-ai/opentitam/internal/message"
	"github.com/opentitam-ai/opentitam/internal/permission"
	"github.com/opentitam-ai/opentitam/internal/session"
)

func CoderAgentTools(
	permissions permission.Service,
	sessions session.Service,
	messages message.Service,
	history history.Service,
	lspClients map[string]*lsp.Client,
) []tools.BaseTool {
	ctx := context.Background()
	otherTools := GetMcpTools(ctx, permissions)
	if len(lspClients) > 0 {
		otherTools = append(otherTools, tools.NewDiagnosticsTool(lspClients))
	}
	return append(
		[]tools.BaseTool{
			tools.NewBashTool(permissions),
			tools.NewEditTool(lspClients, permissions, history),
			tools.NewGlobTool(),
			tools.NewGrepTool(),
			tools.NewLsTool(),
			tools.NewViewTool(lspClients),
			tools.NewPatchTool(lspClients, permissions, history),
			tools.NewWriteTool(lspClients, permissions, history),
			NewAgentTool(sessions, messages, lspClients),
		}, otherTools...,
	)
}

func TaskAgentTools(lspClients map[string]*lsp.Client) []tools.BaseTool {
	return []tools.BaseTool{
		tools.NewGlobTool(),
		tools.NewGrepTool(),
		tools.NewLsTool(),
		tools.NewViewTool(lspClients),
	}
}
