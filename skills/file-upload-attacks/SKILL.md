---
name: file-upload-attacks
description: "Invoke this skill for authorized testing of file-upload and deserialization surfaces: upload filter bypass (extension, content-type, magic bytes) toward web-shell or stored XSS, path traversal on upload, SSRF via file parsers, XXE, and insecure deserialization. Proves impact with a benign marker and records the bypass so the defensive report can prescribe validation."
---

# File Upload & Deserialization (authorized)

Turn an upload or a parser into demonstrated impact, with a benign proof, and hand defense the exact bypass.

Attribution: adapted from the `09-web-security` / `03-exploit-development` modules of Masriyan/Claude-Code-CyberSecurity-Skill (MIT) and mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0).

## Boundaries
- Scope-only. Web-shell proof is a benign marker that echoes a fixed string, never a live interactive shell on production; remove any artifact you upload.
- Deserialization proof uses a non-destructive gadget (e.g. a DNS/HTTP callback to an operator-controlled host), not a destructive payload.

## Checklists
Upload:
- [ ] extension bypass (double ext, case, trailing dot/space, `.phtml`/`.phar`, null byte on legacy stacks).
- [ ] content-type / magic-byte spoof; polyglot (valid image + code).
- [ ] where it lands: is the path web-accessible and executed? (ties to php2shell for PHP).
- [ ] path traversal in the filename to write outside the upload dir.
- [ ] stored XSS via SVG/HTML upload rendered in-origin.
Parsers / deserialization:
- [ ] SSRF/XXE via document/image/XML parsers (external entity, SSRF in a URL field).
- [ ] insecure deserialization (Java/PHP/Python/.NET/Node): identify the format, prove with a callback gadget.

## Evidence & handoff
Record the exact bypass (what filter, what defeated it), where the file landed, and the benign proof of execution/callback. The defensive agent turns this into "validate by magic bytes + allowlist, store outside webroot and serve non-executable, patch the parser / stop deserializing untrusted input".
