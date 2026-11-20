package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

const legacyWS = "\t\n\v\f\r \u0085   -     　"

func TestSanitizeToolSchemaDropsOnlyUnsafePatterns(t *testing.T) {
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"bad":     map[string]any{"type": "string", "pattern": "[^" + legacyWS + "]", "minLength": 1},
			"escaped": map[string]any{"type": "string", "pattern": `^ *x$`},
			"good":    map[string]any{"type": "string", "pattern": `^[a-z0-9-]+$`},
			"list":    map[string]any{"type": "array", "items": []any{map[string]any{"pattern": "　"}}},
		},
		"patternProperties": map[string]any{"^é": map[string]any{}, "^x_": map[string]any{}},
	}
	issues := SchemaGrammarIssues(schema)
	if len(issues) != 4 {
		t.Fatalf("issues = %+v, want 4 (bad, escaped, list item, patternProperties key)", issues)
	}
	clean, changed := SanitizeToolSchema(schema)
	if !changed || len(SchemaGrammarIssues(clean)) != 0 {
		t.Fatalf("sanitized schema still has issues: %+v", SchemaGrammarIssues(clean))
	}
	props := clean["properties"].(map[string]any)
	if props["good"].(map[string]any)["pattern"] != `^[a-z0-9-]+$` {
		t.Fatal("a safe pattern was removed")
	}
	if props["bad"].(map[string]any)["minLength"] != 1 {
		t.Fatal("sibling keys of a removed pattern were lost")
	}
	if _, ok := clean["patternProperties"].(map[string]any)["^x_"]; !ok {
		t.Fatal("a safe patternProperties key was removed")
	}
	if _, ok := schema["properties"].(map[string]any)["bad"].(map[string]any)["pattern"]; !ok {
		t.Fatal("SanitizeToolSchema mutated its input")
	}
	if same, changed := SanitizeToolSchema(map[string]any{"pattern": "^ok$"}); changed || same["pattern"] != "^ok$" {
		t.Fatal("a clean schema must come back unchanged")
	}
}

func TestErrorClassification(t *testing.T) {
	grammar := errors.New(`openai: status 400: {"error":{"message":"{\"error\": {\"message\": \"Invalid grammar specification: 'triggers'\"}}"}}`)
	gatewayOverflow := errors.New(`openai: status 400: {"error":{"message":"el prompt (~297151 tokens estimados) supera el contexto máximo de 150000 tokens; reduzca o compacte la conversación","type":"invalid_request_error","code":"context_length_exceeded"}}`)
	vllmOverflow := errors.New(`openai: status 400: This model's maximum context length is 32768 tokens. However, you requested 40000 tokens`)
	auth := errors.New("openai: status 401: invalid key")
	reset := errors.New("read tcp: connection reset by peer")

	if !IsToolGrammarError(grammar) || IsToolGrammarError(gatewayOverflow) {
		t.Fatal("grammar classification")
	}
	for _, err := range []error{gatewayOverflow, vllmOverflow} {
		if !IsContextOverflowError(err) {
			t.Errorf("not an overflow: %v", err)
		}
		if IsDeterministicRequestError(err) {
			t.Errorf("an overflow must stay rerunnable: %v", err)
		}
	}
	if !IsDeterministicRequestError(grammar) || !IsDeterministicRequestError(auth) || IsDeterministicRequestError(reset) {
		t.Fatal("deterministic classification")
	}
	if n := ParseContextLimit(gatewayOverflow.Error()); n != 150000 {
		t.Fatalf("cf-llm-ctl limit = %d, want 150000", n)
	}
	if n := ParseContextLimit(vllmOverflow.Error()); n != 32768 {
		t.Fatalf("vLLM limit = %d, want 32768", n)
	}
	if k := SuggestedContextK(150000); k != 127 {
		t.Fatalf("SuggestedContextK(150000) = %d, want 127", k)
	}
}

func TestClarifyErrorNamesSuspectTools(t *testing.T) {
	req := llm.CompletionRequest{Tools: []llm.ToolSchema{
		{Name: "fine", InputSchema: map[string]any{"type": "object"}},
		{Name: "mcp__x__bad", InputSchema: map[string]any{"properties": map[string]any{"a": map[string]any{"pattern": " "}}}},
	}}
	err := clarifyError(errors.New("status 400: Invalid grammar specification: 'triggers'"), req)
	var pe *ProviderError
	if !errors.As(err, &pe) || !strings.Contains(err.Error(), "suspects: mcp__x__bad") || !IsToolGrammarError(err) {
		t.Fatalf("clarified = %v", err)
	}
	if clarifyError(err, req) != err {
		t.Fatal("clarifying twice must not wrap again")
	}
	plain := errors.New("status 500: boom")
	if clarifyError(plain, req) != plain {
		t.Fatal("unrelated errors must pass through unchanged")
	}
}

func TestValidateCACert(t *testing.T) {
	srv := httptest.NewTLSServer(http.NotFoundHandler())
	defer srv.Close()
	certPEM := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw}))
	if got, err := ValidateCACert("  " + certPEM + "  "); err != nil || !strings.HasPrefix(got, "-----BEGIN CERTIFICATE-----") {
		t.Fatalf("valid cert rejected: %v", err)
	}
	if got, err := ValidateCACert(""); err != nil || got != "" {
		t.Fatal("empty must mean system pool")
	}
	for _, bad := range []string{"hello", "-----BEGIN CERTIFICATE-----\nAAAA\n-----END CERTIFICATE-----"} {
		if _, err := ValidateCACert(bad); err == nil {
			t.Errorf("accepted %q", bad)
		}
	}
	// A client built with the extra CA trusts the server; one without it does not.
	withCA, err := quotaAwareHTTPClient("", "", "", certPEM)
	if err != nil {
		t.Fatal(err)
	}
	if resp, err := withCA.Get(srv.URL); err != nil {
		t.Fatalf("request with the CA failed: %v", err)
	} else {
		resp.Body.Close()
	}
	without, _ := quotaAwareHTTPClient("", "", "", "")
	if _, err := without.Get(srv.URL); err == nil {
		t.Fatal("a request without the CA must fail certificate verification")
	}
	_ = tls.VersionTLS12
}

// fakeGateway imitates cf-llm-ctl (clamps max_tokens, rejects long prompts by
// estimate, hides max_model_len) or vLLM (exposes max_model_len) .
func fakeGateway(t *testing.T, kind string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			if kind == "cf-llm-ctl" {
				_, _ = io.WriteString(w, `{"ok":true}`)
				return
			}
			http.NotFound(w, r)
		case "/health":
			http.NotFound(w, r)
		case "/v1/models":
			if kind == "vllm" {
				_, _ = io.WriteString(w, `{"data":[{"id":"qwen","max_model_len":32768}]}`)
				return
			}
			_, _ = io.WriteString(w, `{"data":[{"id":"qwen"},{"id":"embed"}]}`)
		case "/v1/chat/completions":
			var body struct {
				Messages []struct{ Content string } `json:"messages"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if len(body.Messages) > 0 && len(body.Messages[0].Content) > 500000 {
				w.WriteHeader(400)
				_, _ = io.WriteString(w, `{"error":{"message":"el prompt (~600000 tokens estimados) supera el contexto máximo de 150000 tokens; reduzca o compacte la conversación","code":"context_length_exceeded"}}`)
				return
			}
			_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestInspectGatewayCfLLMCtl(t *testing.T) {
	srv := fakeGateway(t, "cf-llm-ctl")
	defer srv.Close()
	rep := InspectGateway(context.Background(), ConfigFrom("openai", "qwen", srv.URL+"/v1", "k", ""))
	if rep.Healthy == nil || !*rep.Healthy || !strings.HasSuffix(rep.HealthURL, "/healthz") {
		t.Fatalf("health = %+v", rep)
	}
	if rep.ContextLimit != 150000 || rep.ContextSource != "probe" || rep.SuggestedK != 127 {
		t.Fatalf("context = %+v", rep)
	}
	if strings.Join(rep.Models, ",") != "qwen,embed" {
		t.Fatalf("models = %v", rep.Models)
	}
}

func TestInspectGatewayVLLM(t *testing.T) {
	srv := fakeGateway(t, "vllm")
	defer srv.Close()
	rep := InspectGateway(context.Background(), ConfigFrom("openai", "qwen", srv.URL+"/v1", "k", ""))
	if rep.Healthy != nil {
		t.Fatalf("no health route must report nil, got %+v", rep)
	}
	if rep.ContextLimit != 32768 || rep.ContextSource != "models" {
		t.Fatalf("context = %+v", rep)
	}
}
