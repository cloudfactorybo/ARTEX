---
name: cloud-attack
description: "Invoke this skill for authorized offensive testing of AWS, Azure or GCP: enumerate from leaked or assumed credentials, find IAM privilege-escalation paths, abuse over-permissive roles and policies, access exposed storage, exploit the instance metadata service (SSRF-to-credentials), and identify public misconfigurations. Records each misconfiguration so the defensive report can recommend the exact policy fix."
---

# Cloud Attack (AWS / Azure / GCP, authorized)

Test a cloud tenant you are authorized to assess, from leaked/assumed credentials or an SSRF foothold. Tie every finding to the IAM/config mistake so defense gets an exact fix.

Attribution: adapted from the `10-cloud-security` module of Masriyan/Claude-Code-CyberSecurity-Skill (MIT) and the Cloud Security / Red Teaming skills of mukul975/Anthropic-Cybersecurity-Skills (Apache-2.0), reframed to ARTEX's authorization and evidence model.

## Boundaries
- Scope-only: the accounts/subscriptions/projects the engagement names.
- Read and prove, do not alter: no resource creation/deletion, no data exfiltration beyond one authorized object as proof.
- Do not pivot into another tenant or a managed service's backend.
- Billing-safe: no actions that spin up cost (crypto, large compute).

## Phase 1: identify and enumerate
- [ ] Whose creds / what identity (aws sts get-caller-identity, az account show, gcloud auth list).
- [ ] Enumerate effective permissions (IAM policies, role bindings) — what this identity can actually do.
- [ ] Inventory reachable resources: storage buckets, secrets, functions, DBs, compute.

## Phase 2: privilege escalation paths
- [ ] IAM privesc: iam:PassRole + a service that assumes it, policy-version abuse, lambda/function role abuse, CreateAccessKey on another user (AWS); role assignment / managed identity abuse (Azure); service-account impersonation / actAs (GCP).
- [ ] Map the path to the engagement objective; prove with the minimal action.

## Phase 3: common exposures
- [ ] Public/over-shared storage (S3/Blob/GCS): read one authorized object as proof.
- [ ] Metadata service via SSRF from an in-scope app: retrieve the role credentials (proof only).
- [ ] Secrets in env/metadata/user-data, exposed snapshots, public AMIs/images.

## Phase 4: record for the defensive report (REQUIRED)
Record via record_fact / the finding tools: the exact policy/role/binding at fault, the action it allowed, and the proof. The defensive agent turns this into "scope this policy to X / remove iam:PassRole / enable IMDSv2 / make the bucket private / rotate this key".
