---
name: client-side-attacks
description: "Invoke this skill for authorized testing of a web app's client-side surface: CSRF, CORS misconfiguration, clickjacking, postMessage/origin abuse, DOM-based XSS, prototype pollution, open redirect and insecure client-side storage. Confirms exploitability in a real browser context and records the exact sink so the defensive report can prescribe the fix."
---

# Client-Side Attacks (authorized)

Test what the browser executes, not just what the server returns. Confirm each flaw with a real, benign proof and record the source->sink.

Attribution: adapted from the `09-web-security` module of Masriyan/Claude-Code-CyberSecurity-Skill (MIT) and the Web Application Security skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0).

## Boundaries
- Scope-only; benign, reversible markers (an `alert`/console marker, a self-targeted request). Never a payload that persists against other users.
- CSRF/clickjacking proofs are self-contained PoC pages hosted locally, not against real users.

## Checklists
CSRF:
- [ ] state-changing request with no anti-CSRF token / token not validated / token not bound to session.
- [ ] SameSite cookie absent or Lax where cross-site POST still works; build a minimal PoC form.
CORS:
- [ ] `Access-Control-Allow-Origin` reflects arbitrary Origin + `Allow-Credentials: true`; prove cross-origin read with a second origin.
- [ ] null-origin / pre-flight bypass / subdomain trust.
Clickjacking:
- [ ] missing `X-Frame-Options` / `frame-ancestors`; framed PoC that overlays a sensitive action.
postMessage / DOM XSS / prototype pollution:
- [ ] DOM XSS: trace source (location/hash/postMessage) to sink (innerHTML/eval/document.write) in the JS bundle; benign proof (ties to codeaudit).
- [ ] postMessage handler with no origin check; prototype pollution via `__proto__` in a merge/clone sink.
Other:
- [ ] open redirect; sensitive data in localStorage/sessionStorage; tokens in URL.

## Evidence & handoff
Record the source, the sink, the response/headers, and the benign PoC. The defensive agent turns this into "add CSRF tokens + SameSite=Strict, lock CORS to an allowlist, set frame-ancestors, sanitize the DOM sink, validate postMessage origin".
