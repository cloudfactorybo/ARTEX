---
name: compliance-mapping
description: "Invoke this skill to map a confirmed finding to the standards a client report needs: OWASP Top 10 / API Top 10, CWE, MITRE ATT&CK, and the control frameworks (NIST CSF/800-53, ISO 27001, PCI-DSS, CIS). Used by the defensive agent to give each finding its standards references and strengthen the deliverable."
---

# Compliance Mapping (finding -> standards for the report)

Attach the standards references a professional report carries, so the client can place each finding in their framework. Reference only; no attack.

Attribution: adapted from the `19-grc-compliance` module of Masriyan/Claude-Code-CyberSecurity-Skill (MIT) and the Compliance & Governance skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0).

## For each finding produce
- [ ] **CWE**: the specific weakness id (e.g. CWE-89 SQL Injection), not just a category.
- [ ] **OWASP**: Top 10 (web) or API Security Top 10 category, with the current year's id (e.g. A03:2021 Injection, API1:2023 BOLA).
- [ ] **MITRE ATT&CK**: tactic + technique id for the attacker behavior (e.g. T1190).
- [ ] **Control frameworks** (only those the client uses — ask/infer from scope): the specific control that fails — NIST 800-53 (e.g. AC-6), ISO 27001 Annex A, PCI-DSS requirement, CIS Control.
- [ ] **One-line rationale** per mapping: why this finding maps to that control.

## Quality bar
- Specific ids, not framework names. "Violates ISO 27001" is useless; "A.8.3 fails because…" is actionable.
- Do not invent a mapping to pad the report; only map what the finding actually demonstrates.
