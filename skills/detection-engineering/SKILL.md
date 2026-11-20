---
name: detection-engineering
description: "Invoke this skill when turning a confirmed offensive finding into defensive detection: write a Sigma rule (or SIEM query) that would have caught the attack, map it to MITRE ATT&CK, and state the log source and tuning needed. Used by the defensive agent to add 'how to detect this next time' to the report."
---

# Detection Engineering (finding -> detection)

For a confirmed finding, answer: what log would have shown this, and what rule catches it with low false positives. Output goes into the defensive report, not into an attack.

Attribution: methodology adapted from the threat-hunting / detection skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0) and Masriyan/Claude-Code-CyberSecurity-Skill #06/#12/#15 (MIT).

## For each finding produce
- [ ] **Log source**: which telemetry carries the evidence (web access log, WAF log, auth log, DB audit, cloud trail, EDR). If the attack would be invisible, say so — that gap is itself a recommendation.
- [ ] **Detection logic**: a Sigma rule (portable) with a precise `detection:` and `condition:`. Prefer a behavioral signal (the differing request pattern from the finding's control pair) over a brittle string match.
- [ ] **ATT&CK mapping**: tactic + technique id (e.g. T1190 Exploit Public-Facing Application).
- [ ] **False-positive notes**: what legitimate traffic looks similar, and how to tune.
- [ ] **Severity of the detection gap**: how long this would have gone unseen with the current logging.

## Quality bar
- The rule must key on something in THIS finding's captured evidence, not a generic template.
- No rule without a named log source — an undetectable finding is reported as a visibility gap.
