---
name: remediation-advisory
description: "Invoke this skill to turn a confirmed finding into client-ready remediation: the concrete fix, a severity/danger rating with its rationale, compensating controls, and a verification step. Used by the defensive agent to write the 'what to do to protect yourselves' section of the report."
---

# Remediation Advisory (finding -> fix the client can act on)

Translate a technical finding into steps a developer or ops team can execute, with a defensible severity.

Attribution: adapted from the GRC / blue-team / remediation skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0) and Masriyan/Claude-Code-CyberSecurity-Skill #15/#19 (MIT).

## Severity (danger level)
Rate each finding Crítico / Alto / Medio / Bajo / Informativo. Justify with: exploitability
(how hard, auth needed?), impact (confidentiality/integrity/availability, data exposed),
and exposure (internet-facing? pre-auth?). State the reasoning, not just the label.
If a CVSS vector is warranted, include it; the label is what the client reads first.

## For each finding produce
- [ ] **Root cause** in one sentence (not the symptom — the missing control).
- [ ] **Fix**: the specific change (code, config, patch version, WAF rule, IAM policy),
  with a concrete example where possible. "Validate input" is not a fix; "parameterize
  this query / use prepared statements in <file>:<endpoint>" is.
- [ ] **Compensating control**: what to do now if the real fix takes time.
- [ ] **Verification**: how to confirm it is fixed (re-run the finding's reproducible proof;
  this ties to the retester).
- [ ] **Priority**: severity + effort, so the client knows what to do first.

## Quality bar
- Every recommendation maps to a specific confirmed finding and its evidence.
- No generic checklist dumps; advice is specific to what was actually found on this target.
