---
name: web-access-control
description: "Invoke this skill for authorized testing of web application access control at the page/function level: IDOR, forced browsing to privileged pages, horizontal and vertical privilege escalation, multi-step workflow bypass, and missing function-level authorization. Proves the boundary with two accounts/roles and records the exact check that is missing."
---

# Web Access Control (authorized)

An access-control finding exists only once a second account or role has proven the boundary is real. Prove it with the least data and record what check is missing.

Attribution: adapted from the web/auth modules of Masriyan/Claude-Code-CyberSecurity-Skill #09 (MIT) and the Web Application Security / IAM skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0). Pairs with api-security for the API layer.

## Boundaries
- Use your own second account / role; one record you are authorized to touch is enough proof. Never exfiltrate real users' data.
- Destructive actions only on resources you created for the test.

## Checklists
IDOR / horizontal:
- [ ] enumerate ids (numeric, UUID harvested from other responses, usernames/emails) in path and body.
- [ ] two-session replay: account A requesting account B's objects; diff the responses.
Vertical / function-level:
- [ ] standard user calling admin endpoints / hidden actions; forced browsing to `/admin`, `/internal`, backup/debug routes.
- [ ] parameter/role tampering (role=admin, isAdmin=true, mass-assignment on write).
Workflow:
- [ ] multi-step process with a skippable/forgeable step (price, quantity, approval); replay out of order.
- [ ] response/status tampering to pass a client-side gate.

## Evidence & handoff
Record the two-account context, the request that should have been denied, and the control request that is correctly denied. The defensive agent turns this into "enforce server-side authorization on <endpoint> keyed to the session's identity/role".
