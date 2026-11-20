---
name: active-directory
description: "Invoke this skill for authorized offensive testing of a Windows/Active Directory environment once you have a foothold or valid credentials: enumeration (BloodHound), Kerberos attacks (kerberoasting, AS-REP roasting, delegation), ACL abuse, credential access, and lateral movement. Records exactly which misconfiguration enabled each step so the defensive report can recommend the fix."
---

# Active Directory (authorized internal attack)

Attack a Windows/AD estate you are authorized to test, from a foothold or a set of valid credentials. Every step is tied to a concrete misconfiguration so the defensive agent can turn it into a hardening recommendation.

Attribution: methodology adapted from the `14-red-team-ops` module of Masriyan/Claude-Code-CyberSecurity-Skill (MIT) and the Red Teaming / AD skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0), reframed to ARTEX's authorization and evidence model.

## Boundaries
- Scope-only: the domain(s), hosts and accounts the engagement authorizes.
- No destructive actions (no account disable, no GPO changes, no DCSync dump of all hashes unless explicitly authorized and only as proof, one account).
- No persistence (golden/silver tickets, skeleton key) unless the engagement authorizes it in writing; if used, document and remove it.
- Respect the noise profile; AD enumeration is loud — pace it and prefer targeted queries over full sweeps on production.

## Phase 1: enumerate (map, don't touch)
- [ ] Domain, trusts, users, groups, computers, GPOs, ACLs — SharpHound/BloodHound (or ldapsearch/rpcclient when stealth matters).
- [ ] Find the shortest path to Domain Admin / the engagement's objective; note each edge (what right enables it).
- [ ] Identify kerberoastable (SPN) and AS-REP-roastable (no-preauth) accounts.

## Phase 2: credential access (prove, minimally)
- [ ] Kerberoasting: request the SPN ticket, crack offline; report the weak-password account, not a domain-wide dump.
- [ ] AS-REP roasting: same, for no-preauth accounts.
- [ ] Delegation abuse: unconstrained / constrained / RBCD — only to prove the path.
- [ ] From a host you own: LSASS/SAM only if authorized, one credential as proof.

## Phase 3: lateral movement / escalation (one hop, proven)
- [ ] ACL abuse (GenericAll, WriteDACL, AddMember…): show the right, perform the minimal action to prove it.
- [ ] Prove lateral access (WinRM/SMB/WMI) to one authorized host; do not spread.

## Phase 4: record for the defensive report (REQUIRED)
For each step, record via record_fact / the finding tools: the misconfiguration (the BloodHound edge / weak SPN / delegation / ACL), the action taken, and the proof. This is what the defensive agent turns into "remove this right / rotate this account / disable unconstrained delegation / enforce AES Kerberos".
