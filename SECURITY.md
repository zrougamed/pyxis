# Security Policy

## Supported versions

Security fixes are applied to the latest release on `master` and the most recent tagged release.

## Reporting a vulnerability

Please report security issues privately to the maintainer:

- Email: **zrouga.mohammed@gmail.com**

Include:

1. Description and impact
2. Affected commit / tag / binary version
3. Reproduction steps or proof-of-concept (non-destructive preferred)
4. Any known workarounds

You should receive an acknowledgement within a few days. Please do not disclose the issue publicly until a fix or mitigation is available.

## Hardening notes for operators

- Prefer Dex (or equivalent OIDC) for the web UI; use `--no-auth` only on trusted local networks.
- Restrict RBAC for the kubeconfig used by Pyxis to least privilege.
- Treat YAML apply / delete / exec / port-forward as privileged operations.
- Keep Go and dependency advisories current (`govulncheck`, Dependabot / renovate PRs).
