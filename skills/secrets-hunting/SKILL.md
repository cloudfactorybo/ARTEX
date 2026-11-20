---
name: secrets-hunting
description: "Invoke this skill to hunt exposed secrets on an authorized target: API keys, tokens and credentials in JS bundles, source maps, .git/.env/.svn exposure, backup and config files, cloud metadata, response headers and error pages. Verify each secret is live before reporting, and record where it was found so the defensive report can recommend rotation and the exposure fix."
---

# Secrets Hunting (authorized)

Find credentials and keys the target leaks, confirm they are live, and hand defense the exact exposure to close.

Attribution: adapted from the recon / supply-chain / web modules of Masriyan/Claude-Code-CyberSecurity-Skill #01/#20 (MIT) and the relevant mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0).

## Boundaries
- Scope-only. A found key is proven live with a single minimal, non-destructive call (e.g. an identity/whoami call), never by using it to reach data or another system.
- Do not store real secrets in the report verbatim beyond the minimum needed to identify them (mask the middle).

## Where to look
- [ ] JS bundles and source maps (`.map`): hardcoded keys, endpoints, tokens.
- [ ] Exposed VCS/config: `/.git/`, `/.svn/`, `/.env`, `/config.*`, `/backup.*`, `.DS_Store`, swagger with embedded tokens.
- [ ] Response headers, verbose errors, stack traces, debug endpoints.
- [ ] Cloud metadata / user-data (via an in-scope SSRF), public buckets/snapshots.
- [ ] Commit history if the repo is in scope (git log / dangling blobs).

## Verify before reporting
- [ ] Classify the secret (provider, scope) and confirm it is live with one minimal call.
- [ ] A dead/example/placeholder key is reported as an exposure of lower severity, not a live-credential finding.

## Record for the defensive report (REQUIRED)
Record: where it was exposed (exact URL/file/line), the secret type (masked), and the live-check proof. The defensive agent turns this into "rotate this credential now, remove the file / block the path, scrub it from history, add secret scanning to CI".
