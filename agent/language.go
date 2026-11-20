package agent

import (
	"strings"
	"sync/atomic"
)

// Output language of the human-facing text agents write: replies, plans, and
// free-text tool arguments such as fact and finding text. The editable prompt
// bodies stay as they are; a code-owned block is appended at the end, like the
// noise block, and read at each prompt build so a change shows in the next one.
const (
	AgentLanguageSpanish = "es"
	AgentLanguageEnglish = "en"
	AgentLanguageAuto    = "auto"
	DefaultAgentLanguage = AgentLanguageSpanish
)

const languageHeader = "[Output language: "

var agentLanguage atomic.Value // string, one of the AgentLanguage* codes

func init() { agentLanguage.Store(DefaultAgentLanguage) }

// ParseAgentLanguage normalizes a stored or requested language; ok is false for
// an unknown value.
func ParseAgentLanguage(v string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "es", "spanish", "español", "espanol":
		return AgentLanguageSpanish, true
	case "en", "english":
		return AgentLanguageEnglish, true
	case "auto":
		return AgentLanguageAuto, true
	}
	return "", false
}

// SetAgentLanguage sets the language for prompts built from now on; an unknown
// value is ignored.
func SetAgentLanguage(v string) {
	if code, ok := ParseAgentLanguage(v); ok {
		agentLanguage.Store(code)
	}
}

// AgentLanguage returns the current language code.
func AgentLanguage() string { return agentLanguage.Load().(string) }

func languageBlock() string { return renderLanguageBlock(AgentLanguage()) }

// keepAsIs lists what never gets translated: the tools, the target and the
// schemas need the exact text.
const keepAsIs = "Keep exactly as they are: commands, code, payloads, URLs, hostnames, HTTP requests and responses, quoted tool output, file paths, identifiers, CVE/CWE ids, and every field name or enumerated value a tool schema defines (for example confidence and source values).\n"

// renderLanguageBlock is pure so tests cover every language. English returns
// "" and leaves the prompts byte-for-byte unchanged.
func renderLanguageBlock(code string) string {
	switch code {
	case AgentLanguageSpanish:
		return "\n" + languageHeader + "Spanish. Operator setting, not a target fact.]\n" +
			"Write all human-facing text in neutral Latin American Spanish: replies to the operator, plans and todo items, and every free-text value you put in tool arguments (fact summaries and details, evidence notes, finding titles, descriptions, impact and remediation, reports, reasons and wrap-up summaries).\n" +
			keepAsIs +
			"The operator may write in Spanish or English; understand both and answer in Spanish.\n"
	case AgentLanguageAuto:
		return "\n" + languageHeader + "the operator's. Operator setting, not a target fact.]\n" +
			"Write all human-facing text, including free-text tool arguments, in the language of the operator's latest message, or of the task description and goal when there is no message; use Spanish when it is unclear.\n" +
			keepAsIs
	}
	return ""
}
