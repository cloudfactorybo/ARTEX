---
name: waf-evasion
description: "Invoke this skill during authorized offensive testing when a WAF, CDN or network firewall is filtering payloads or blocking the scanner: fingerprint the control, find the real origin, and apply request-level evasion (encoding, casing, parameter pollution, header tricks, chunking, timing) so a genuine vulnerability is not hidden by the filter. Record exactly which evasion worked, so the defensive report can recommend how to fix the control."
---

# WAF / Firewall Evasion (authorized testing)

A WAF that blocks a payload does not mean the app is safe — it means the control hid the bug. This skill gets an honest signal through the filter AND documents the bypass so the defensive agent can turn it into a concrete hardening recommendation. Every evasion that works is itself a finding about the control.

Attribution: techniques distilled from public references (OWASP, the red-team technique lists in A-poc/RedTeam-Tools and Masriyan/Claude-Code-CyberSecurity-Skill #09/#14), reframed to ARTEX's authorization and evidence model.

## Boundaries
- Scope-only. Evasion never justifies reaching a host outside the engagement.
- No DoS. Rate/timing evasion respects the task noise profile; never flood to exhaust a WAF.
- Record the bypass, do not weaponize it beyond proving the underlying bug once.

## Phase 1: fingerprint the control
- [ ] Identify the WAF/CDN from headers, cookies, block-page body, status codes (`Server`, `cf-ray`, `x-akamai`, `x-sucuri-id`, `incap_ses`, 403/406/501 patterns). wafw00f as a starting probe.
- [ ] Confirm it is a filter and not the app: a clean request succeeds, the payload-bearing one is blocked with the same parameters.
- [ ] Note the trigger: which token/pattern is blocked (narrow it down with a binary search of the payload).

## Phase 2: find the real origin (bypass the edge entirely)
- [ ] Historical DNS / certificate transparency for the pre-CDN IP (crt.sh, SecurityTrails-style data).
- [ ] SPF/MX records, subdomains that resolve to the origin, `Host`-header routing to the origin IP.
- [ ] If the origin is reachable and in scope, test it directly — the cleanest "bypass".

## Phase 3: request-level evasion (one technique per request, keep the control pair)
- [ ] Encoding: URL double-encoding, mixed/Unicode casing, HTML-entity, `\u`/`\x`, overlong UTF-8.
- [ ] Payload obfuscation: comment insertion (`/**/`), case toggling, string concatenation, whitespace alternatives (`\t\n\r\f\v`).
- [ ] HTTP parameter pollution: the same param twice; different back-end vs WAF parsing.
- [ ] Header tricks: `X-Forwarded-For`, `X-Originating-IP`, `Content-Type` swap (e.g. `application/json` vs form), `Transfer-Encoding`/chunked, oversized bodies the WAF skips.
- [ ] Verb/transport: method override (`X-HTTP-Method-Override`), HTTP/1.0 vs /2, request smuggling only if the engagement authorizes it.
- [ ] Timing: slow the probe under the noise profile so a rate-based rule does not trip.

## Phase 4: record for the defensive report (REQUIRED)
For every working bypass, record via record_fact / the finding tools:
- the control (WAF/CDN name + fingerprint evidence),
- the blocked request (the control pair),
- the exact evasion that passed (the one differing element),
- the underlying vulnerability it then exposed.
This is what lets the defensive agent say "the WAF was bypassed via X; fix the rule / normalize input / patch the app" instead of a vague "a WAF is present".
