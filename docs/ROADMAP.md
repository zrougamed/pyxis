# Pyxis roadmap

Pyxis already covers the day-to-day operator loop: TUI + web + CLI, logs, metrics, exec, port-forward, scale/restart, resource browsing, Helm awareness, and optional Dex auth.

The next work is **not** “more resource types.” CronJobs and HPAs don’t differentiate a tool. What does: explaining *why* something is broken, what depends on what, and what an action will hit—before the API call fails or the outage spreads.

This document is the working backlog. Order reflects impact vs. fit with the current codebase (`internal/k8s`, TUI, web), not marketing themes.

---

## Direction

Treat Pyxis as an **operator cockpit**, not another dashboard.

Shared analysis should live outside the UI packages so TUI, CLI, and web all call the same engine:

```text
internal/
  graph/       # object relationships, ownership, references
  explain/     # pending / crashloop / imagepull / scheduling / pvc / …
  timeline/    # ordered change + event narrative
  audit/       # actions taken through Pyxis
  diagnosis/   # cluster score, incident bundles
```

Deterministic reasoning first. Any later assistant layer should read that graph—not invent answers from raw YAML.

---

## Now (next releases)

### 1. Explain mode

On a selected object (start with Pods), a dedicated key / panel that answers **why** this state exists.

Built from real owners, Events, conditions, and related objects—not free-form text.

Examples: Pending (scheduling / affinity / taints / PVC / CSI), CrashLoopBackOff, ImagePullBackOff, not-ready endpoints.

**TUI:** `E` (or similar) opens the explain view.  
**CLI:** `pyxis explain pod/<name>`.  
**Web:** side panel on the resource.

### 2. Relationship graph (depends-on / depended-by)

For the selected resource, show:

- What this object depends on
- What depends on this object

Ingress → Service → Deployment → ReplicaSet → Pods → PVC / Secret / ConfigMap / NetworkPolicy, etc.

Ship a usable list/tree first; a visual graph can follow once the data model is stable.

### 3. Blast radius before destructive ops

Before delete / restart / scale (and similar), show an impact summary:

- Affected workloads and approximate pod count
- Cascading references (e.g. ConfigMap used by N Deployments)
- Short risk note when capacity or HPA makes scale unsafe

Confirm only after the operator has seen the summary.

### 4. Operation audit log

Record actions performed **through Pyxis** (TUI and web):

- Who (Dex identity when auth is on; local/anonymous otherwise)
- What (restart, scale, delete, apply, exec start/stop, port-forward)
- When, target GVK/name/namespace, outcome, duration

Queryable later (“who restarted Redis?”). Persist locally by default; optional remote sink later.

### 5. Secret handling guardrails

Env / Secret views already show *sources*. Add:

- Values redacted by default
- Explicit reveal with confirmation
- Optional policy (never reveal in web; reveal only in TUI; require auth)

---

## Next

### 6. Cluster timeline & “what changed?”

Turn Events + watched object diffs into a readable timeline instead of a flat event dump.

CLI:

```bash
pyxis changes --since 1h
```

Surfaces image bumps, scale changes, cordons, PVC resizes, Ingress/cert churn, etc.

### 7. Smart apply / diff

When comparing or applying YAML, highlight field-level changes (image, memory, replicas, …) and attach a simple risk hint (e.g. replica jump vs. allocatable capacity)—operator-facing, not a raw `kubectl diff` dump.

### 8. Incident mode

One mode that packs the usual incident screens:

Failed / restarting pods, recent changes, Events, logs shortcut, metrics, obvious network symptoms.

Goal: stop hopping menus during a fire.

### 9. Cluster health score

Overview / homepage summary: health %, problem count, warnings, short recommendation list—each item deep-links into explain / timeline.

### 10. Network policy view

Read `NetworkPolicy` (and later CiliumNetworkPolicy when present). Render allowed paths between workloads in a simplified way—no eBPF/Hubble dependency for v1. Helps DevSecOps users who already live in network policy land.

### 11. Exec session recording

With Dex (or local identity), record interactive exec sessions (commands + timestamps, not necessarily full PTY replay in v1). Tie sessions to the audit log.

---

## Later

### 12. RBAC-aware UI

Use `SelfSubjectAccessReview` / discovery so actions the token cannot perform are disabled or hidden up front, instead of failing only at the API call.

### 13. GitOps status

Optional Argo CD / Flux sync and drift next to the workload (status only; Pyxis does not become a GitOps controller).

### 14. Multi-cluster fleet view

Context switching stays; add a simultaneous multi-cluster summary (health, problem counts) for operators who jump between clusters all day.

### 15. Right-sizing hints

From metrics-server (and later richer signals): requests vs. usage, obvious over/under-provision hints on pods/workloads.

### 16. Image provenance

When browsing pods/images: optional Sigstore / SBOM / signature status. Fits supply-chain workflows without blocking core navigation if data is missing.

### 17. Plugin / extension hooks

Allow cluster- or org-specific views without forking Pyxis (k9s/Lens-style). Only after the core graph/explain APIs are stable enough to hang plugins on.

### 18. Mobile-native client

Responsive web covers phones for light use. A native app is explicitly out of scope until the web shell and auth story are boringly solid.

---

## Explicitly deferred

| Idea | Why wait |
|------|----------|
| Chat / LLM as a primary interface | Commodity; unreliable without a structured diagnosis layer underneath |
| Feature parity resource sprawl (every GVK) | Diminishing returns vs. explain / impact / timeline |
| Full Hubble/eBPF pipeline | Heavy ops burden; start with API-visible policies |

---

## Suggested package ownership

| Area | Package (proposed) | Consumed by |
|------|--------------------|-------------|
| Ownership & references | `internal/graph` | explain, blast radius, search |
| State → cause rules | `internal/explain` | TUI `E`, `pyxis explain`, web panel |
| Change narrative | `internal/timeline` | `pyxis changes`, incident mode |
| Pyxis-originated actions | `internal/audit` | TUI/web ops, exec, apply |
| Scores / incident bundles | `internal/diagnosis` | overview, incident mode |

UI packages stay thin: fetch analysis results, don’t embed diagnosis rules.

---

## Success criteria (practical)

We know this roadmap is working when an operator can:

1. Select a broken Pod and get a **credible why** in under half a minute  
2. See **blast radius** before confirming a delete/restart  
3. Answer **who changed what** in the last hour from Pyxis itself  
4. Keep using Pyxis as a single binary—no mandatory sidecars for the core explain path  

---

## Related docs

- [Kind demo cluster](./kind-demo.md) — local cluster for demos and manual checks  
- [Contributing](../CONTRIBUTING.md)  
- [Security](../SECURITY.md)
