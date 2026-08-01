package tool

import "context"

// StubProvider is a no-op tool provider that returns "not available" for all tools.
// Useful as a fallback when users haven't registered real implementations.
type StubProvider struct {
	tool Tool
}

func NewStub(t Tool) *StubProvider { return &StubProvider{tool: t} }

func (s *StubProvider) Tool() Tool { return s.tool }

func (s *StubProvider) Execute(_ context.Context, args map[string]any) (string, error) {
	return NotAvailableMsg, nil
}

// BuiltinToolProvider implements tools that don't require external system access.
type BuiltinToolProvider struct {
	tool Tool
	fn   func(ctx context.Context, args map[string]any) (string, error)
}

func NewBuiltin(t Tool, fn func(ctx context.Context, args map[string]any) (string, error)) *BuiltinToolProvider {
	return &BuiltinToolProvider{tool: t, fn: fn}
}

func (b *BuiltinToolProvider) Tool() Tool { return b.tool }
func (b *BuiltinToolProvider) Execute(ctx context.Context, args map[string]any) (string, error) {
	return b.fn(ctx, args)
}
