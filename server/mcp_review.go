package server

import (
	"context"
	"fmt"
	"slices"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/bhanuprakaash/reef/store"
)

// _reviewTools are the only tools a memory-review token may use.
var _reviewTools = []string{"recall", "read_memory", "remember", "retire_memory"}

// restrictToReview lists and runs only _reviewTools: the token's review flag, not the prompt,
// keeps a memory review from posting or changing anything but the Bot's own memory.
func restrictToReview(srv *mcp.Server) {
	srv.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			if method == "tools/call" {
				if p, ok := req.GetParams().(*mcp.CallToolParamsRaw); !ok || !slices.Contains(_reviewTools, p.Name) {
					return nil, fmt.Errorf("%w: a memory review may only use %v", store.ErrInvalid, _reviewTools)
				}
			}
			res, err := next(ctx, method, req)
			if l, ok := res.(*mcp.ListToolsResult); ok && err == nil {
				l.Tools = slices.DeleteFunc(l.Tools, func(t *mcp.Tool) bool {
					return !slices.Contains(_reviewTools, t.Name)
				})
			}
			return res, err
		}
	})
}
