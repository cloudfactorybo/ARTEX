package agent

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"iter"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Autumn-27/norma/compaction"
	"github.com/Autumn-27/norma/llm"
	actool "github.com/Autumn-27/norma/tool"
)

// Self-hosted gateways in front of vLLM (GPUStack, cf-llm-ctl) compile every
// tool's JSON Schema into a decoding grammar. One schema the grammar compiler
// rejects fails the whole request with a 400, whether or not the model would
// call that tool, and the error says nothing about which tool it was. This file
// keeps such schemas from going out, names the culprit when one does, and
// reads the context limit these gateways report.

// ---- tool schema grammar safety ----

// GrammarIssue is one schema construct a grammar-constrained decoder may fail
// to compile, at a JSON path inside the tool's input schema.
type GrammarIssue struct {
	Path    string `json:"path"`
	Problem string `json:"problem"`
}

// unicodeEscape matches regex escapes that denote code points or Unicode
// classes ( , \x{2028}, \p{L}); grammar compilers handle them poorly.
var unicodeEscape = regexp.MustCompile(`\\(?:u[0-9a-fA-F]{4}|u\{[0-9a-fA-F]+\}|x\{[0-9a-fA-F]+\}|[pP]\{)`)

// patternProblem reports why a regex pattern is unsafe for grammar compilation,
// or "" when it only uses printable ASCII.
func patternProblem(p string) string {
	for _, r := range p {
		if r < 0x20 || r > 0x7e {
			return fmt.Sprintf("pattern contains the non-ASCII character %U", r)
		}
	}
	if m := unicodeEscape.FindString(p); m != "" {
		return fmt.Sprintf("pattern contains the Unicode escape %s", m)
	}
	return ""
}

// SchemaGrammarIssues lists the patterns in schema that a grammar-constrained
// decoder (vLLM) may fail to compile, sorted by path.
func SchemaGrammarIssues(schema map[string]any) []GrammarIssue {
	var out []GrammarIssue
	walkPatterns(schema, "$", func(path string, node map[string]any) {
		if p, ok := node["pattern"].(string); ok {
			if problem := patternProblem(p); problem != "" {
				out = append(out, GrammarIssue{Path: path + ".pattern", Problem: problem})
			}
		}
		if pp, ok := node["patternProperties"].(map[string]any); ok {
			for key := range pp {
				if problem := patternProblem(key); problem != "" {
					out = append(out, GrammarIssue{Path: path + ".patternProperties", Problem: problem})
				}
			}
		}
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out
}

func walkPatterns(v any, path string, visit func(string, map[string]any)) {
	switch node := v.(type) {
	case map[string]any:
		visit(path, node)
		keys := make([]string, 0, len(node))
		for k := range node {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			walkPatterns(node[k], path+"."+k, visit)
		}
	case []any:
		for i, child := range node {
			walkPatterns(child, path+"["+strconv.Itoa(i)+"]", visit)
		}
	}
}

// SanitizeToolSchema returns a copy of schema without the unsafe patterns and
// whether it removed any. A pattern is only a hint to the model; the tool
// handler validates its input either way, so dropping one loosens nothing.
func SanitizeToolSchema(schema map[string]any) (map[string]any, bool) {
	if len(SchemaGrammarIssues(schema)) == 0 {
		return schema, false
	}
	out, _ := sanitizeNode(schema).(map[string]any)
	return out, true
}

func sanitizeNode(v any) any {
	switch node := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(node))
		for k, child := range node {
			if k == "pattern" {
				if p, ok := child.(string); ok && patternProblem(p) != "" {
					continue
				}
			}
			if k == "patternProperties" {
				if pp, ok := child.(map[string]any); ok {
					kept := map[string]any{}
					for key, sub := range pp {
						if patternProblem(key) == "" {
							kept[key] = sanitizeNode(sub)
						}
					}
					if len(kept) > 0 {
						out[k] = kept
					}
					continue
				}
			}
			out[k] = sanitizeNode(child)
		}
		return out
	case []any:
		out := make([]any, len(node))
		for i, child := range node {
			out[i] = sanitizeNode(child)
		}
		return out
	}
	return v
}

var sanitizedWarned sync.Map // tool name -> struct{}, so each tool is logged once

// sanitizeTools swaps in a sanitized schema for every tool whose schema a
// grammar compiler may reject. Applied to every agent's final tool list.
func sanitizeTools(tools []actool.CoreTool) []actool.CoreTool {
	for i, t := range tools {
		schema, changed := SanitizeToolSchema(t.InputSchema())
		if !changed {
			continue
		}
		if _, seen := sanitizedWarned.LoadOrStore(t.Name(), struct{}{}); !seen {
			log.Printf("[tools] %s: removed schema patterns a model server may fail to compile: %v", t.Name(), SchemaGrammarIssues(t.InputSchema()))
		}
		tools[i] = DecorateTool(t, t.Description(), schema)
	}
	return tools
}

// ---- provider error classification ----

var grammarErrRE = regexp.MustCompile(`(?i)invalid grammar specification|failed to (?:compile|build) (?:the )?grammar|grammar compil`)

// IsToolGrammarError reports whether err is a model server rejecting a tool
// schema it cannot compile into a grammar. Resending never helps.
func IsToolGrammarError(err error) bool {
	return err != nil && grammarErrRE.MatchString(err.Error())
}

// IsContextOverflowError reports whether err says the request is longer than
// the model's context window, in the phrasing of vLLM, OpenAI-style APIs or the
// cf-llm-ctl gateway (Spanish).
func IsContextOverflowError(err error) bool {
	if err == nil {
		return false
	}
	if compaction.IsOverflow(err) {
		return true
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "contexto máximo") || strings.Contains(s, "contexto maximo")
}

var contextLimitPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)maximum context length is (\d+)`),
	regexp.MustCompile(`(?i)contexto m[aá]ximo de (\d+)`),
	regexp.MustCompile(`(?i)max_model_len[^0-9]{0,12}(\d{4,})`),
	regexp.MustCompile(`(?i)context (?:window|length) (?:of|is) (\d+)`),
	regexp.MustCompile(`(?i)limit(?:ed)? to (\d{4,}) tokens`),
}

// ParseContextLimit extracts the model's context window, in tokens, from an
// error message; 0 when the message names none.
func ParseContextLimit(msg string) int {
	for _, re := range contextLimitPatterns {
		if m := re.FindStringSubmatch(msg); m != nil {
			if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
				return n
			}
		}
	}
	return 0
}

// SuggestedContextK turns a gateway's context limit into the profile's
// context window setting (K tokens). It keeps a 15% margin because gateways
// such as cf-llm-ctl enforce the limit on a conservative, character-based
// estimate that runs ahead of the real token count.
func SuggestedContextK(limitTokens int) int {
	if limitTokens <= 0 {
		return 0
	}
	return max(limitTokens*85/100/1000, 8)
}

var deterministicStatus = regexp.MustCompile(`\bstatus (?:400|401|403|404|422)\b`)

// IsDeterministicRequestError reports whether err will recur on every resend
// of the same request: a tool schema the model server cannot compile, or a
// 400/401/403/404/422 rejection. A context overflow is excluded (compaction or
// a fresh run can fit), and so is quota exhaustion (handled by failover).
func IsDeterministicRequestError(err error) bool {
	if err == nil || IsContextOverflowError(err) || IsQuotaExhaustedMessage(err.Error()) {
		return false
	}
	return IsToolGrammarError(err) || deterministicStatus.MatchString(err.Error())
}

// ProviderError carries a readable explanation in front of a provider error.
type ProviderError struct {
	Summary string
	Err     error
}

func (e *ProviderError) Error() string { return e.Summary + ": " + e.Err.Error() }
func (e *ProviderError) Unwrap() error { return e.Err }

// clarifyError puts a readable explanation in front of the gateway errors that
// otherwise surface as a nested JSON blob. Other errors pass through unchanged.
func clarifyError(err error, req llm.CompletionRequest) error {
	if err == nil {
		return nil
	}
	var already *ProviderError
	if errors.As(err, &already) {
		return err
	}
	switch {
	case IsToolGrammarError(err):
		summary := "the model server could not compile a tool's JSON Schema into a grammar (Invalid grammar specification); resending cannot help, fix or disable the tool"
		if names := grammarSuspects(req.Tools); len(names) > 0 {
			summary += "; suspects: " + strings.Join(names, ", ")
		}
		return &ProviderError{Summary: summary, Err: err}
	case IsContextOverflowError(err):
		summary := "the request exceeds the model's context window"
		if n := ParseContextLimit(err.Error()); n > 0 {
			summary = fmt.Sprintf("the request exceeds the model's context window (%d tokens; a context window of %dK leaves room for the gateway's estimate)", n, SuggestedContextK(n))
		}
		return &ProviderError{Summary: summary, Err: err}
	}
	return err
}

func grammarSuspects(tools []llm.ToolSchema) []string {
	var names []string
	for _, t := range tools {
		if len(SchemaGrammarIssues(t.InputSchema)) > 0 {
			names = append(names, t.Name)
		}
	}
	return names
}

// clarifyingProvider wraps a provider so its errors carry clarifyError's
// explanation wherever they surface (worker logs, chat, main agent).
type clarifyingProvider struct{ llm.Provider }

func (p clarifyingProvider) Stream(ctx context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		for ev, err := range p.Provider.Stream(ctx, req) {
			if !yield(ev, clarifyError(err, req)) {
				return
			}
		}
	}
}

func (p clarifyingProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	msg, stop, usage, err := p.Provider.Complete(ctx, req)
	return msg, stop, usage, clarifyError(err, req)
}

// ---- extra CA per profile ----

// ValidateCACert checks that pemData holds at least one parseable CERTIFICATE
// block and returns it trimmed. Empty input is valid and means "system pool".
func ValidateCACert(pemData string) (string, error) {
	pemData = strings.TrimSpace(pemData)
	if pemData == "" {
		return "", nil
	}
	if len(pemData) > 256<<10 {
		return "", fmt.Errorf("ca_cert is too large (over 256 KiB)")
	}
	rest, found := []byte(pemData), 0
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return "", fmt.Errorf("ca_cert has an invalid certificate: %w", err)
		}
		found++
	}
	if found == 0 {
		return "", fmt.Errorf("ca_cert is not a PEM certificate (expected -----BEGIN CERTIFICATE-----)")
	}
	return pemData + "\n", nil
}

// caTLSConfig returns a TLS config trusting the system pool plus pemData, or
// nil when pemData is empty.
func caTLSConfig(pemData string) (*tls.Config, error) {
	pemData, err := ValidateCACert(pemData)
	if err != nil || pemData == "" {
		return nil, err
	}
	pool, err := x509.SystemCertPool()
	if err != nil || pool == nil {
		pool = x509.NewCertPool()
	}
	pool.AppendCertsFromPEM([]byte(pemData))
	return &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}, nil
}

// HTTPTransport returns a transport with this profile's proxy and extra CA,
// for side requests such as listing models or probing the gateway.
func HTTPTransport(proxy, caPEM string) (*http.Transport, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	if p := strings.TrimSpace(proxy); p != "" {
		u, err := url.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("invalid proxy %q: %w", p, err)
		}
		transport.Proxy = http.ProxyURL(u)
	}
	tlsCfg, err := caTLSConfig(caPEM)
	if err != nil {
		return nil, err
	}
	if tlsCfg != nil {
		transport.TLSClientConfig = tlsCfg
	}
	return transport, nil
}
