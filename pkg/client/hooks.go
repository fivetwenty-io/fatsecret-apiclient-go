package client

// Hook and HookEvent are aliased from internal/http in client.go. This file
// documents their usage and provides hook constructor helpers.

// NewHook wraps fn in a Hook. It is provided as a named constructor so that
// generated code and examples can reference client.NewHook(fn) without a bare
// type conversion, improving readability when the caller has a named function.
func NewHook(fn func(HookEvent)) Hook {
	return Hook(fn)
}
