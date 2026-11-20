---
name: cloud-hardening
description: "Invoke this skill when a confirmed finding involves AWS, Azure or GCP, to write the defensive fix for the client: the exact policy/config change (least-privilege IAM, bucket privacy, IMDSv2, key rotation, logging), a compensating control, and the CIS/benchmark reference. Used by the defensive agent to turn a cloud finding into actionable hardening."
---

# Cloud Hardening (finding -> fix the client can apply)

For a confirmed cloud finding, write the concrete remediation, keyed to the exact IAM/config mistake in the evidence.

Attribution: adapted from the `10-cloud-security` / `15-blue-team-defense` modules of Masriyan/Claude-Code-CyberSecurity-Skill (MIT) and the Cloud Security / hardening skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0).

## For each finding produce
- [ ] **Root cause**: the specific policy, role binding or config that allowed it (quote it from the evidence).
- [ ] **Fix**: the exact change — least-privilege policy (scope the action/resource), make the bucket private + block public access, enable IMDSv2, remove iam:PassRole / actAs, disable the over-broad role assignment. Give the policy snippet or CLI command where possible.
- [ ] **Compensating control**: SCP/Azure Policy/Org Policy guardrail, GuardDuty/Defender/SCC alert, deny-by-default boundary.
- [ ] **Detection/logging**: ensure CloudTrail/Activity Log/Audit Log captures it; name the event.
- [ ] **Benchmark reference**: the relevant CIS Benchmark / Well-Architected / provider best-practice id.
- [ ] **Verification**: re-run the finding's reproducible proof after the fix.

## Quality bar
- Least privilege, not "tighten permissions". Name the action and resource to scope.
- Every recommendation maps to the confirmed finding's evidence, not a generic checklist.
