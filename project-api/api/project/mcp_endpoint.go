package project

import (
	"test.com/project-api/pkg/mcp"

	"github.com/gin-gonic/gin"
)

// mcpEndpoint exposes the same tool registry used by the in-process AI
// orchestrator. It remains behind the normal project TokenVerify middleware,
// so an external MCP host can only see data belonging to the authenticated
// member.
func (p *HandlerProject) mcpEndpoint(c *gin.Context) {
	ctx := mcp.WithCaller(c.Request.Context(), mcp.Caller{
		MemberID:         c.GetInt64("memberId"),
		MemberName:       c.GetString("memberName"),
		OrganizationCode: c.GetString("organizationCode"),
	})
	projectMCPServer.ServeHTTP(c.Writer, c.Request.WithContext(ctx))
}
