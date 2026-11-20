package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/db"
)

// Technical summary for the operators (the "Resumen" tab): what the task did,
// what it could not test, which credentials it used, coverage, findings and cost.
// It is computed on demand from the task's own data (intents, coverage ledger,
// credentials, findings, outcome); no LLM, always available. Distinct from the
// client-facing per-finding "report".

type summaryIntent struct {
	ID            int64  `json:"id"`
	Summary       string `json:"summary"`
	State         string `json:"state"`
	BlockedReason string `json:"blocked_reason,omitempty"`
}

type summaryCredential struct {
	Domain   string `json:"domain"`
	Username string `json:"username"`
	Source   string `json:"source"` // user | agent
	Note     string `json:"note,omitempty"`
	HasPass  bool   `json:"has_password"`
}

type taskSummaryDTO struct {
	Intents         []summaryIntent        `json:"intents"`
	IntentCounts    map[string]int         `json:"intent_counts"`
	FactCount       int                    `json:"fact_count"`
	Findings        db.TaskOutcomeFindings `json:"findings"`
	FindingsBySev   map[string]int         `json:"findings_by_severity"`
	Credentials     []summaryCredential    `json:"credentials"`
	Coverage        *db.CoverageLedger     `json:"coverage,omitempty"`
	CoverageEnabled bool                   `json:"coverage_enabled"`
	ToolCalls       int                    `json:"tool_calls"`
	SkillMisses     int                    `json:"skill_misses"`
	WallSeconds     int64                  `json:"wall_seconds"`
	Tokens          db.TaskOutcomeTokens   `json:"tokens"`
	Status          string                 `json:"status"`
}

// nodeSummaryField pulls the "summary" string out of an exploration node payload.
func nodeSummaryField(payload json.RawMessage) string {
	var p struct {
		Summary string `json:"summary"`
		Name    string `json:"name"`
	}
	_ = json.Unmarshal(payload, &p)
	s := strings.TrimSpace(p.Summary)
	if s == "" {
		s = strings.TrimSpace(p.Name)
	}
	if len(s) > 400 {
		s = s[:400] + "…"
	}
	return s
}

// GET /api/tasks/{id}/summary
func (s *Server) taskSummary(w http.ResponseWriter, r *http.Request) {
	t, ok := s.m.Task(r.PathValue("id"))
	if !ok {
		writeErr(w, 404, "task not found")
		return
	}
	taskID, _ := strconv.ParseInt(t.ID, 10, 64)
	out := taskSummaryDTO{
		IntentCounts:    map[string]int{},
		FindingsBySev:   map[string]int{},
		Credentials:     []summaryCredential{},
		Intents:         []summaryIntent{},
		CoverageEnabled: t.CoverageEnabled,
	}

	// Intents: what it did, what stalled (exhausted) and what was blocked.
	if intents, err := t.Store.ListByKind(db.KindIntent, 500); err == nil {
		for _, n := range intents {
			out.IntentCounts[n.State]++
			out.Intents = append(out.Intents, summaryIntent{
				ID: n.ID, Summary: nodeSummaryField(n.Payload), State: n.State, BlockedReason: n.BlockedReason,
			})
		}
	}
	if facts, err := t.Store.ListByKind(db.KindFact, 2000); err == nil {
		out.FactCount = len(facts)
	}
	if findings, err := t.Store.ListByKind(db.KindFinding, 1000); err == nil {
		for _, n := range findings {
			var p struct {
				Severity string `json:"severity"`
			}
			_ = json.Unmarshal(n.Payload, &p)
			sev := strings.ToLower(strings.TrimSpace(p.Severity))
			if sev == "" {
				sev = "unrated"
			}
			out.FindingsBySev[sev]++
		}
	}

	// Credentials used (password never leaves the server; only whether one exists).
	if s.m.PG() != nil {
		if creds, err := s.m.PG().TaskCredentials(taskID); err == nil {
			for _, c := range creds {
				out.Credentials = append(out.Credentials, summaryCredential{
					Domain: c.Domain, Username: c.Username, Source: c.Source, Note: c.Note,
					HasPass: strings.TrimSpace(c.Password) != "",
				})
			}
		}
		if o, err := s.m.PG().TaskOutcome(context.Background(), taskID); err == nil {
			out.Findings = o.Findings
			out.ToolCalls, out.SkillMisses, out.WallSeconds = o.ToolCalls, o.SkillMisses, o.WallSeconds
			out.Tokens, out.Status = o.Tokens, o.Status
		}
	}

	// Coverage ledger: the surfaces tested and the outcome per risk area, plus gaps.
	if t.CoverageEnabled && s.m.Assets() != nil {
		if ledger, err := s.m.Assets().CoverageLedger(taskID); err == nil {
			out.Coverage = ledger
		}
	}

	writeJSON(w, 200, out)
}
