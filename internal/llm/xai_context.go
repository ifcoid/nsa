package llm

import "context"

// xaiContextKey is the unexported key type for storing XAI metadata in context.
type xaiContextKey struct{}

// XAIContext holds metadata used by the xAI logging wrapper to record audit entries.
type XAIContext struct {
	SessionID string
	Step      string
	AgentFunc string
}

// WithXAIContext returns a copy of ctx with the given xAI metadata attached.
func WithXAIContext(ctx context.Context, sessionID, step, agentFunc string) context.Context {
	return context.WithValue(ctx, xaiContextKey{}, XAIContext{
		SessionID: sessionID,
		Step:      step,
		AgentFunc: agentFunc,
	})
}

// XAIContextFrom extracts xAI metadata from ctx. Returns ok=false if not present.
func XAIContextFrom(ctx context.Context) (XAIContext, bool) {
	v, ok := ctx.Value(xaiContextKey{}).(XAIContext)
	return v, ok
}
