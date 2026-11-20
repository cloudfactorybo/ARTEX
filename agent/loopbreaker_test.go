package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
)

// fakeTool returns a fixed result (optionally an error) and counts executions.
type fakeTool struct {
	name  string
	err   bool
	calls int
}

func (f *fakeTool) Name() string                           { return f.name }
func (f *fakeTool) Description() string                    { return "fake" }
func (f *fakeTool) Prompt() string                         { return "" }
func (f *fakeTool) InputSchema() map[string]any            { return map[string]any{"type": "object"} }
func (f *fakeTool) IsReadOnly(json.RawMessage) bool        { return true }
func (f *fakeTool) IsConcurrencySafe(json.RawMessage) bool { return true }
func (f *fakeTool) CheckPermissions(context.Context, json.RawMessage, permission.Context) permission.Decision {
	return permission.Decision{}
}
func (f *fakeTool) Call(_ context.Context, _ json.RawMessage, _ *actool.ToolContext) (actool.Result, error) {
	f.calls++
	return actool.Result{Content: nil, IsError: f.err}, nil
}

func cbText(bs []llm.ContentBlock) string {
	var b strings.Builder
	for _, x := range bs {
		b.WriteString(x.Text)
	}
	return b.String()
}

func TestLoopBreakerStopsErrorLoop(t *testing.T) {
	ft := &fakeTool{name: "record_fact", err: true}
	g := guardPanic(ft)
	// 10 identical erroring calls: the breaker must stop executing the tool well before 10.
	for i := 0; i < 10; i++ {
		g.Call(context.Background(), json.RawMessage(`{"a":1}`), &actool.ToolContext{})
	}
	if ft.calls >= 10 {
		t.Fatalf("error loop not broken: tool executed %d/10 times", ft.calls)
	}
	if ft.calls > loopErrorStop {
		t.Fatalf("error loop broken too late: executed %d times, want <= %d", ft.calls, loopErrorStop)
	}
}

func TestLoopBreakerStopsIdenticalSuccessLoop(t *testing.T) {
	ft := &fakeTool{name: "Bash", err: false}
	g := guardPanic(ft)
	for i := 0; i < 12; i++ {
		g.Call(context.Background(), json.RawMessage(`{"command":"grep x y"}`), &actool.ToolContext{})
	}
	if ft.calls >= 12 {
		t.Fatalf("identical-success loop not broken: executed %d/12", ft.calls)
	}
	if ft.calls > loopHardStop {
		t.Fatalf("hard loop broken too late: executed %d, want <= %d", ft.calls, loopHardStop)
	}
}

func TestLoopBreakerResetsOnDifferentArgs(t *testing.T) {
	ft := &fakeTool{name: "Bash", err: false}
	g := guardPanic(ft)
	// Alternating distinct args must never trip the breaker.
	for i := 0; i < 20; i++ {
		g.Call(context.Background(), json.RawMessage(fmt.Sprintf(`{"command":"cmd %d"}`, i)), &actool.ToolContext{})
	}
	if ft.calls != 20 {
		t.Fatalf("distinct calls were wrongly intercepted: executed %d/20", ft.calls)
	}
}
