package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"runtime/debug"
	"sync"

	"github.com/Autumn-27/norma/llm"
	actool "github.com/Autumn-27/norma/tool"
)

// DeferredInfo carries the deferred-tools wiring an agent needs to build its
// Options: the MCP tool names whose schemas are withheld, the subset listed in the
// global system-prompt block (non-skill-gated), and the shared session unlock set.
// UnlockSkill unlocks a named skill's MCPs — hosts call it to rebuild the unlock set
// from history on a resumed session (design doc C2).
type DeferredInfo struct {
	FindingGuidance string            // derived from the final permitted tools, including DB overrides
	Deferred        []string          // all MCP tool names (schema withheld)
	GlobalNames     []string          // MCP names to list in the system-prompt block
	Unlock          *actool.UnlockSet // shared call-gate; nil when no MCP tools
	UnlockSkill     func(skillName string)
}

// ToolAugmentFunc is the ToolAugment hook. If set, it returns the EXTRA tools
// an agent should see beyond its built-in base set — the agent's visible skills (packed into one Skill meta-tool)
// and visible MCP servers (expanded to mcp__server__tool). It also returns the
// DeferredInfo describing how those MCP tools are deferred/gated. The server wires
// it to the PG agent_visibility table. cleanup releases any spawned MCP clients.
//
// When nil, agents run with only their built-in tools — behavior is unchanged
// until a user assigns a skill/MCP to the agent in the UI.
type ToolAugmentFunc func(ctx context.Context, agentKey string) (extra []actool.CoreTool, def DeferredInfo, cleanup func())

// The hook is set by server wiring while restored tasks may already be
// assembling tools on other goroutines, so every access goes through the lock.
var (
	toolAugmentMu sync.RWMutex
	toolAugment   ToolAugmentFunc
)

// SetToolAugment installs (or, with nil, removes) the ToolAugment hook.
func SetToolAugment(f ToolAugmentFunc) {
	toolAugmentMu.Lock()
	toolAugment = f
	toolAugmentMu.Unlock()
}

// ToolAugment returns the current ToolAugment hook, or nil when none is set.
func ToolAugment() ToolAugmentFunc {
	toolAugmentMu.RLock()
	defer toolAugmentMu.RUnlock()
	return toolAugment
}

// AugmentTools returns base plus the agent's visible skill/MCP tools, the
// DeferredInfo, and a cleanup func the caller must defer (closes MCP clients).
// Built-in base tools are kept as-is — never filtered (built-in tools stay at the code layer, no visibility filtering).
func AugmentTools(ctx context.Context, agentKey string, base []actool.CoreTool) ([]actool.CoreTool, DeferredInfo, func()) {
	var (
		def     DeferredInfo
		cleanup = func() {}
		out     = base
	)
	// Copy each hook once under its lock, then call it outside the lock.
	if augment := ToolAugment(); augment != nil {
		var extra []actool.CoreTool
		var cl func()
		extra, def, cl = augment(ctx, agentKey)
		if cl != nil {
			cleanup = cl
		}
		if len(extra) > 0 {
			out = append(append([]actool.CoreTool{}, base...), extra...)
		}
	}
	// DB tools table has the final say on the built-in tools: drop the ones this
	// agent isn't bound to (or that are disabled) and swap in overridden
	// descriptions/schemas + default injection. MCP/skill/host tools have no row
	// and pass through untouched, so deferred/unlock wiring stays consistent.
	if resolve := ToolResolve(); resolve != nil {
		out = resolve(ctx, agentKey, out)
	}
	// Tool RAG (off by default): lexical projection of the bound list; only removes.
	out = applyToolRAG(ctx, agentKey, out, def.Deferred, bm25Score)
	out, def.FindingGuidance = findingWorkflowTools(agentKey, out)
	// Strip schema patterns a grammar-constrained model server (vLLM) may fail to
	// compile; one such schema fails every request the agent makes.
	out = sanitizeTools(out)
	for i, t := range out {
		out[i] = guardPanic(t)
	}
	return out, def, cleanup
}

// guardPanic turns a panicking tool handler into an ordinary tool error. The
// harness runs each tool on its own goroutine, so a panic inside a handler can't
// be recovered by the caller that started the run — it takes the whole process
// down, and on restart the agent replays the same call and crashes again. Applied
// last, so it covers every tool the agent can reach: domain, SDK, MCP and skill.
func guardPanic(t actool.CoreTool) actool.CoreTool { return &guardedTool{CoreTool: t} }

// Loop breaker thresholds. A worker that resends the exact same tool call learns
// nothing new; a validation/permission error that it does not fix repeats forever.
// These stop the two degenerate loops seen in production (a grep repeated 119x; a
// record_fact with an empty summary resent 112x) without touching legitimate use,
// which rarely issues the byte-identical call many times in a row.
const (
	loopErrorStop = 3 // 3rd consecutive identical call once the prior ones errored -> intercept
	loopHardStop  = 6 // 6th consecutive identical call of any kind -> intercept
)

// guardedTool wraps a tool with panic recovery and a per-run circuit breaker that
// intercepts a call repeated identically too many times (especially one that keeps
// erroring), returning an actionable notice instead of running it again. State is
// per instance; AugmentTools builds fresh wrappers per agent run, so it is
// naturally scoped to one run and needs no session key.
type guardedTool struct {
	actool.CoreTool
	mu        sync.Mutex
	lastSig   string
	repeat    int // consecutive identical calls, including the current one
	errStreak int // consecutive identical calls that returned an error
}

func (g *guardedTool) sig(in json.RawMessage) string {
	h := sha256.Sum256(in)
	return hex.EncodeToString(h[:8])
}

// loopBreak returns the intercept notice for a degenerate loop, or ("",false) when
// the call should proceed. It advances the repeat counter under the lock.
func (g *guardedTool) loopBreak(in json.RawMessage) (actool.Result, bool) {
	sig := g.sig(in)
	g.mu.Lock()
	defer g.mu.Unlock()
	if sig != g.lastSig {
		g.lastSig, g.repeat, g.errStreak = sig, 0, 0
	}
	g.repeat++
	errLoop := g.repeat >= loopErrorStop && g.errStreak >= loopErrorStop-1
	hardLoop := g.repeat >= loopHardStop
	if !errLoop && !hardLoop {
		return actool.Result{}, false
	}
	var msg string
	if errLoop {
		msg = fmt.Sprintf("Loop breaker: you have called %s with these exact arguments %d times and the last %d failed with the same error. Resending it will not change the result — read the error, fix the specific field it names, or abandon this approach and try a different one. This call was not executed.", g.Name(), g.repeat, g.errStreak)
	} else {
		msg = fmt.Sprintf("Loop breaker: you have called %s with these exact arguments %d times in a row with no new outcome. An unchanged or empty result is information, not a reason to repeat — change the input or the approach, or move on. This call was not executed.", g.Name(), g.repeat)
	}
	return actool.Result{Content: []llm.ContentBlock{llm.TextBlock(msg)}, IsError: true}, true
}

func (g *guardedTool) Call(ctx context.Context, in json.RawMessage, tc *actool.ToolContext) (res actool.Result, err error) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[tools] %s panic: %v\n%s", g.Name(), r, debug.Stack())
			res, err = actool.Errorf(fmt.Sprintf("internal error in tool %s: %v (this call has failed; try different arguments or another tool)", g.Name(), r)), nil
		}
	}()
	if notice, broke := g.loopBreak(in); broke {
		log.Printf("[loop-breaker] %s intercepted after %d identical calls", g.Name(), g.repeat)
		return notice, nil
	}
	res, err = g.CoreTool.Call(ctx, in, tc)
	g.mu.Lock()
	if err != nil || res.IsError {
		g.errStreak++
	} else {
		g.errStreak = 0
	}
	g.mu.Unlock()
	return res, err
}
