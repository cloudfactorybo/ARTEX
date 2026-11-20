---
name: auth-session
description: "Invoke this skill for authorized deep testing of a web/API authentication and session layer: JWT flaws (alg confusion, weak secret, kid injection), OAuth/OIDC misconfig (redirect_uri, state/PKCE, token leakage), SAML (signature wrapping, comment injection), session fixation/predictability, MFA bypass and password-reset logic. Complements api-security; records the exact flaw so the defensive report can prescribe the fix."
---

# Auth & Session Testing (authorized)

Attack the authentication and session machinery of a target you are authorized to test. Each flaw is recorded with the control pair so defense gets a precise fix.

Attribution: adapted from the API/auth modules of Masriyan/Claude-Code-CyberSecurity-Skill #09 (MIT) and the IAM / Web App Security skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0).

## Boundaries
- Scope-only; use your own test accounts. Never hijack a real user's live session as proof — a second account you control proves it.
- No credential stuffing / brute force on production beyond a handful of requests under the noise profile.
- Password-reset and MFA tests only against test accounts.

## Checklists
JWT:
- [ ] alg:none; RS256->HS256 key confusion; weak HMAC secret (dictionary); kid path traversal / SQLi; expired/tampered claims (exp, sub, role).
OAuth 2.0 / OIDC:
- [ ] redirect_uri manipulation (open redirect -> code theft); missing/!bound state (CSRF); missing PKCE on public clients; token in Referer/logs; id_token signature not verified.
SAML:
- [ ] signature wrapping (XSW); comment injection in NameID; unsigned assertion accepted; IdP-initiated replay.
Session:
- [ ] predictable/short session id; fixation (id survives login); missing HttpOnly/Secure/SameSite; token not invalidated on logout/reset; concurrent-session trust.
Account logic:
- [ ] password-reset token predictable/reusable/no-expiry; host-header poisoning in reset link; MFA bypass (skip step, backup-code brute, response tampering).

## Evidence & handoff
For each confirmed flaw record the request, the control request that behaves differently, and the two-account/role context where authz is involved. This feeds the defensive agent's fix (rotate secret, verify signature, bind state, enforce PKCE, invalidate on logout).
