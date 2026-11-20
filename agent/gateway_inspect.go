package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// GatewayReport is what InspectGateway learned about an OpenAI-compatible
// endpoint: whether its health route answers, which models it serves, and the
// context window it enforces.
type GatewayReport struct {
	HealthURL     string   `json:"health_url,omitempty"`
	Healthy       *bool    `json:"healthy"` // nil = the endpoint has no health route
	HealthStatus  int      `json:"health_status,omitempty"`
	Models        []string `json:"models"`
	ContextLimit  int      `json:"context_limit"`       // tokens; 0 = unknown
	SuggestedK    int      `json:"suggested_context_k"` // the profile setting for ContextLimit
	ContextSource string   `json:"context_source"`      // "models" | "probe" | ""
	Note          string   `json:"note,omitempty"`
}

// probeChars sizes the oversized-prompt probe: about 1M characters, which a
// gateway that pre-checks length (cf-llm-ctl) rejects without forwarding it.
const probeChars = 1 << 20

// InspectGateway asks an OpenAI-format endpoint for its health and context
// window. It only runs for openai and ollama configs. See the probe order in
// detectContext.
func InspectGateway(ctx context.Context, cfg Config) GatewayReport {
	var rep GatewayReport
	if cfg.Provider() != "openai" && cfg.Provider() != "ollama" {
		rep.Note = "context detection needs an OpenAI-format endpoint"
		return rep
	}
	transport, err := HTTPTransport(cfg.Proxy, cfg.CACert)
	if err != nil {
		rep.Note = err.Error()
		return rep
	}
	client := &http.Client{Transport: transport, Timeout: 60 * time.Second}
	base := strings.TrimRight(cfg.BaseURL, "/")
	rep.HealthURL, rep.Healthy, rep.HealthStatus = probeHealth(ctx, client, base)
	var listed int
	rep.Models, listed = listModelsWithLimit(ctx, client, base, cfg)
	if listed > 0 {
		rep.ContextLimit, rep.ContextSource = listed, "models"
	} else {
		rep.ContextLimit, rep.ContextSource, rep.Note = probeContext(ctx, client, base, cfg)
	}
	rep.SuggestedK = SuggestedContextK(rep.ContextLimit)
	return rep
}

// probeHealth tries /healthz then /health at the endpoint's origin.
func probeHealth(ctx context.Context, client *http.Client, base string) (string, *bool, int) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" {
		return "", nil, 0
	}
	origin := u.Scheme + "://" + u.Host
	for _, path := range []string{"/healthz", "/health"} {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, origin+path, nil)
		if err != nil {
			return "", nil, 0
		}
		resp, err := client.Do(req)
		if err != nil {
			down := false
			return origin + path, &down, 0
		}
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			continue
		}
		up := resp.StatusCode < 300
		return origin + path, &up, resp.StatusCode
	}
	return "", nil, 0
}

func authorize(req *http.Request, cfg Config) {
	if cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
	}
	if cfg.UserAgent != "" {
		req.Header.Set("User-Agent", cfg.UserAgent)
	}
}

// listModelsWithLimit reads /models and, when the configured model's entry
// carries one, its context window (vLLM: max_model_len).
func listModelsWithLimit(ctx context.Context, client *http.Client, base string, cfg Config) ([]string, int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, 0
	}
	authorize(req, cfg)
	resp, err := client.Do(req)
	if err != nil {
		return nil, 0
	}
	defer resp.Body.Close()
	var body struct {
		Data []map[string]any `json:"data"`
	}
	if resp.StatusCode != 200 || json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&body) != nil {
		return nil, 0
	}
	var names []string
	limit := 0
	for _, m := range body.Data {
		id, _ := m["id"].(string)
		names = append(names, id)
		if id != cfg.Model && len(body.Data) > 1 {
			continue
		}
		for _, key := range []string{"max_model_len", "context_length", "max_context_length", "context_window"} {
			if n, ok := m[key].(float64); ok && n > 0 {
				limit = int(n)
				break
			}
		}
	}
	return names, limit
}

// probeContext finds the limit from the endpoint's own rejection: first a tiny
// prompt with an impossible max_tokens (vLLM rejects it before any compute and
// names its window), then, only if that was accepted because the gateway
// clamps max_tokens, an oversized prompt that a length-checking gateway
// rejects before forwarding.
func probeContext(ctx context.Context, client *http.Client, base string, cfg Config) (int, string, string) {
	status, msg, err := chatProbe(ctx, client, base, cfg, "ok", 10_000_000)
	if err != nil {
		return 0, "", "context probe failed: " + err.Error()
	}
	if n := ParseContextLimit(msg); n > 0 {
		return n, "probe", ""
	}
	if status != 200 {
		return 0, "", fmt.Sprintf("context probe rejected without a limit (HTTP %d)", status)
	}
	status, msg, err = chatProbe(ctx, client, base, cfg, strings.Repeat("x ", probeChars/2), 1)
	if err != nil {
		return 0, "", "context probe failed: " + err.Error()
	}
	if n := ParseContextLimit(msg); n > 0 {
		return n, "probe", ""
	}
	if status == 200 {
		return 0, "", fmt.Sprintf("the model accepted a ~%dK-character prompt; its window is larger than the probe", probeChars/1000)
	}
	return 0, "", fmt.Sprintf("context probe rejected without a limit (HTTP %d)", status)
}

func chatProbe(ctx context.Context, client *http.Client, base string, cfg Config, content string, maxTokens int) (int, string, error) {
	payload, _ := json.Marshal(map[string]any{
		"model": cfg.Model, "max_tokens": maxTokens, "stream": false,
		"messages": []map[string]string{{"role": "user", "content": content}},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	authorize(req, cfg)
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return resp.StatusCode, string(body), nil
}
