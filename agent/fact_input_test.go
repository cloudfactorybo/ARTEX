package agent

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	actool "github.com/Autumn-27/norma/tool"
)

func TestRecordFactSummaryContract(t *testing.T) {
	tools := NewToolSet(guardTaskStore(t), "worker")
	tool := tools.recordFact()
	for _, input := range []string{
		`{}`, `null`, `[]`,
		`{"summary":""}`, `{"summary":"   "}`,
		`{"summary":"must not substitute","facts":[]}`,
		`{"summary":"must not substitute","facts":null}`,
		`{"summary":"must not inherit","facts":[{"detail":"missing summary"}]}`,
		`{"facts":[{"summary":"must not save"},{"summary":42}]}`,
	} {
		out := callGuardTool(t, tool, input)
		if !out.IsError {
			t.Errorf("accepted %s: %s", input, out.Flatten())
		}
	}
	if tools.Writes().Facts != 0 {
		t.Fatal("invalid inputs wrote facts")
	}
	single := callGuardTool(t, tool, `{"summary":"valid single"}`)
	if single.IsError {
		t.Fatal(single.Flatten())
	}
	batch := callGuardTool(t, tool, `{"facts":[{"summary":"valid batch"},{"detail":"missing summary"}]}`)
	var result struct {
		IDs    []int64           `json:"ids"`
		Errors map[string]string `json:"errors"`
		Retry  string            `json:"retry_instruction"`
	}
	if err := json.Unmarshal([]byte(batch.Flatten()), &result); err != nil {
		t.Fatal(batch.Flatten(), err)
	}
	if !batch.IsError || len(result.IDs) != 2 || result.IDs[0] <= 0 || result.IDs[1] != 0 ||
		!strings.Contains(result.Errors["1"], "summary") || result.Retry == "" || tools.Writes().Facts != 2 {
		t.Fatalf("partial failure is not actionable: %s", batch.Flatten())
	}
}

func TestRecordFactStoredContractRepair(t *testing.T) {
	old := map[string]any{
		"type": "object", "required": []any{"summary"},
		"anyOf": []any{map[string]any{"required": []string{"summary"}}},
		"properties": map[string]any{
			"summary":      map[string]any{"type": "string", "description": "operator summary"},
			"confidence":   map[string]any{"type": "string", "default": "inferred"},
			"custom_field": map[string]any{"type": "string"},
			"facts":        map[string]any{"type": "array", "items": map[string]any{"type": "object"}},
		},
	}
	base := NewToolSet(nil, "").recordFact()
	repaired := DecorateTool(base, "operator tool", old)
	schema := repaired.InputSchema()
	if _, required := schema["required"]; required {
		t.Fatal("batch still requires a top-level summary")
	}
	if _, exists := schema["anyOf"]; exists {
		t.Fatal("root anyOf reaches the provider")
	}
	props := schema["properties"].(map[string]any)
	item := props["facts"].(map[string]any)["items"].(map[string]any)
	required := item["required"].([]string)
	if len(required) != 1 || required[0] != "summary" || props["facts"].(map[string]any)["minItems"] != 1 {
		t.Fatal("batch must require a nonempty list with a summary per item")
	}
	if props["summary"].(map[string]any)["minLength"] != 1 ||
		item["properties"].(map[string]any)["summary"].(map[string]any)["minLength"] != 1 {
		t.Fatal("empty summaries permitted by schema")
	}
	if repaired.Description() != "operator tool" ||
		props["summary"].(map[string]any)["description"] != "operator summary" ||
		props["confidence"].(map[string]any)["default"] != "inferred" || props["custom_field"] != nil {
		t.Fatal("operator annotations lost")
	}
	if _, ok := old["properties"].(map[string]any)["tested_asset_ids"]; ok {
		t.Fatal("shared old schema mutated")
	}
	first, _ := json.Marshal(schema)
	second, _ := json.Marshal(DecorateTool(base, "", schema).InputSchema())
	if string(first) != string(second) {
		t.Fatal("repair is not idempotent")
	}
	var roundTrip map[string]any
	if err := json.Unmarshal(first, &roundTrip); err != nil {
		t.Fatal(err)
	}
	for _, input := range []string{`{"summary":"single"}`, `{"facts":[{"summary":"batch"}]}`} {
		if err := actool.ValidateInput(roundTrip, json.RawMessage(input)); err != nil {
			t.Fatalf("SDK rejects valid %s after catalog round trip: %v", input, err)
		}
	}
}

func TestRecordFactStrictFields(t *testing.T) {
	tools := NewToolSet(guardTaskStore(t), "worker")
	for _, tc := range []struct{ input, field string }{
		{`{"summary":"valid","typo":1}`, "typo"},
		{`{"Summary":"case alias"}`, "Summary"},
		{`{"summary":"valid","Facts":[]}`, "Facts"},
		{`{"facts":[{"summary":"first"},{"summary":"second","typo":1}]}`, "typo"},
		{`{"facts":[{"summary":"first"},{"Summary":"case alias"}]}`, "Summary"},
		{`{"facts":[{"summary":"first"},{"summary":"second","ASSET_IDS":[]}]}`, "ASSET_IDS"},
		{`{"facts":[{"summary":"nested","facts":[]}]}`, "facts"},
		{`{"summary":"ignored","facts":[{"summary":"batch"}]}`, "summary"},
		{`{"detail":"ignored","facts":[{"summary":"batch"}]}`, "detail"},
		{`{"evidence":"ignored","facts":[{"summary":"batch"}]}`, "evidence"},
		{`{"source":"inference","facts":[{"summary":"batch"}]}`, "source"},
		{`{"confidence":"inferred","facts":[{"summary":"batch"}]}`, "confidence"},
		{`{"asset_ids":[],"facts":[{"summary":"batch"}]}`, "asset_ids"},
		{`{"tested_asset_ids":[],"facts":[{"summary":"batch"}]}`, "tested_asset_ids"},
	} {
		out := callGuardTool(t, tools.recordFact(), tc.input)
		if !out.IsError || !strings.Contains(out.Flatten(), tc.field) {
			t.Errorf("input %s: want named rejection for %s, got %s", tc.input, tc.field, out.Flatten())
		}
	}
	if tools.Writes().Facts != 0 {
		t.Fatal("malformed fields caused partial writes")
	}
	for _, input := range []string{
		`{"facts":[null]}`, `{"facts":[[]]}`, `{"summary":"valid"} {"summary":"extra object"}`,
	} {
		if _, _, err := decodeRecordFactInput(json.RawMessage(input)); err == nil {
			t.Errorf("accepted %s", input)
		}
	}
	if _, batch, err := decodeRecordFactInput(json.RawMessage(`{"intent_id":1,"facts":[{"summary":"valid","intent_id":2}]}`)); err != nil || !batch {
		t.Fatalf("valid batch: %v %v", batch, err)
	}
}

func TestRecordFactSchemaMatchesStrings(t *testing.T) {
	props := recordFactSchema()["properties"].(map[string]any)
	summary := props["summary"].(map[string]any)
	if _, ok := summary["pattern"]; ok || summary["minLength"] != 1 {
		t.Fatalf("summary schema = %v, want minLength 1 and no pattern (the handler rejects blanks)", summary)
	}
	id := regexp.MustCompile(factAssetIDSchema()["pattern"].(string))
	for _, value := range []string{"1", "001", "+1", " +001 ", "\t7\n", "", "0", "-1", "1.5", "a"} {
		raw, _ := json.Marshal(value)
		if got, want := id.MatchString(value), pid(raw) > 0; got != want {
			t.Errorf("asset ID %q: schema=%v runtime=%v", value, got, want)
		}
	}
	// The ASCII-only schema may be stricter than the runtime, never looser.
	for _, value := range []string{"\u200312\u3000", "\u00a05"} {
		raw, _ := json.Marshal(value)
		if id.MatchString(value) || pid(raw) <= 0 {
			t.Errorf("asset ID %q: schema=%v runtime=%v, want schema false runtime true", value, id.MatchString(value), pid(raw) > 0)
		}
	}
	for _, input := range []string{
		`{"summary":"single","asset_ids":["1",2]}`,
		`{"facts":[{"summary":"batch","tested_asset_ids":["1",2]}]}`,
	} {
		if err := actool.ValidateInput(recordFactSchema(), json.RawMessage(input)); err != nil {
			t.Errorf("numeric-string compatibility rejected: %v", err)
		}
	}
}

// legacyFactWhitespace is the Unicode class earlier releases published in the
// record_fact patterns; vLLM cannot compile it into a grammar.
const legacyFactWhitespace = "\t\n\v\f\r \u0085   -     　"

func schemaPatterns(v any, path string, visit func(path, pattern string)) {
	switch node := v.(type) {
	case map[string]any:
		for key, child := range node {
			if p, ok := child.(string); ok && key == "pattern" {
				visit(path, p)
				continue
			}
			schemaPatterns(child, path+"."+key, visit)
		}
	case []any:
		for _, child := range node {
			schemaPatterns(child, path+"[]", visit)
		}
	}
}

func asciiPattern(t *testing.T, schema map[string]any) {
	t.Helper()
	raw, _ := json.Marshal(schema)
	var generic map[string]any
	_ = json.Unmarshal(raw, &generic)
	schemaPatterns(generic, "$", func(path, pattern string) {
		for _, r := range pattern {
			if r > 0x7e {
				t.Errorf("%s.pattern %q has non-ASCII %U; grammar-constrained decoders reject it", path, pattern, r)
				return
			}
		}
	})
}

func TestRecordFactSchemaPatternsAreASCII(t *testing.T) {
	asciiPattern(t, recordFactSchema())
}

func TestRepairRecordFactSchemaDropsUnicodePatterns(t *testing.T) {
	legacy := recordFactSchema()
	legacyID := "^[" + legacyFactWhitespace + "]*\\+?0*[1-9][0-9]*[" + legacyFactWhitespace + "]*$"
	props := legacy["properties"].(map[string]any)
	itemProps := props["facts"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for _, p := range []map[string]any{props, itemProps} {
		p["summary"].(map[string]any)["pattern"] = "[^" + legacyFactWhitespace + "]"
		for _, field := range []string{"asset_ids", "tested_asset_ids"} {
			p[field].(map[string]any)["items"].(map[string]any)["pattern"] = legacyID
		}
	}
	repaired := RepairRecordFactSchema(legacy)
	asciiPattern(t, repaired)
	rp := repaired["properties"].(map[string]any)
	rItems := rp["facts"].(map[string]any)["items"].(map[string]any)["properties"].(map[string]any)
	for _, p := range []map[string]any{rp, rItems} {
		if _, ok := p["summary"].(map[string]any)["pattern"]; ok {
			t.Errorf("repaired summary still has a pattern: %v", p["summary"])
		}
		if got, want := p["asset_ids"].(map[string]any)["items"].(map[string]any)["pattern"], factAssetIDSchema()["pattern"]; got != want {
			t.Errorf("repaired asset_ids pattern = %v, want %v", got, want)
		}
	}
}
