# Kind demo cluster

Spin up a local [kind](https://kind.sigs.k8s.io/) cluster preloaded with realistic workloads for Pyxis demos, screen recordings, and manual e2e checks.

## Prerequisites

- [Docker](https://docs.docker.com/get-docker/) (running)
- [kind](https://kind.sigs.k8s.io/docs/user/quick-start/#installation)
- [kubectl](https://kubernetes.io/docs/tasks/tools/)

Optional: a built Pyxis binary (`make build`) or `make run` / `make run-web`.

## Quick start

```bash
# Create cluster + install metrics-server + seed workloads
./scripts/kind-demo-up.sh

# Point kubectl at it (script also selects the context)
kubectl config use-context kind-pyxis-demo

# Launch Pyxis
make run          # TUI
make run-web      # web UI at http://localhost:8080 (--no-auth)
```

## Script options

| Command | Effect |
|---------|--------|
| `./scripts/kind-demo-up.sh` | Create cluster if missing, then seed |
| `./scripts/kind-demo-up.sh --reset` | Delete existing cluster, recreate, seed |
| `./scripts/kind-demo-up.sh --down` | Delete the demo cluster only |
| `./scripts/kind-demo-up.sh --help` | Short usage |

Cluster name defaults to `pyxis-demo` (kubeconfig context `kind-pyxis-demo`). Override with:

```bash
export PYXIS_KIND_CLUSTER=my-demo
./scripts/kind-demo-up.sh
```

## What gets installed

### Cluster shape

- 1 control-plane node
- 2 worker nodes
- Host ports **8088→80** and **8443→443** on the control-plane (for optional Ingress experiments)

### Metrics

[metrics-server](https://github.com/kubernetes-sigs/metrics-server) is installed and patched for kind:

- `--kubelet-insecure-tls`
- `--kubelet-preferred-address-types=InternalIP`

That enables Pyxis CPU/memory gauges and HPA-related views.

### Namespaces and sample resources

| Namespace | Contents |
|-----------|----------|
| `demo` | `web` (nginx ×3) + Service + Ingress + HPA, `api` (http-echo), `redis`, ConfigMap/Secret, PVC + `writer` pod, DaemonSet `node-agent` |
| `staging` | Healthy `frontend`, CrashLoop `flaky`, ImagePullBackOff `missing-image` |
| `jobs` | One-shot Job `hello-once`, CronJob `heartbeat` (every 2 minutes) |

Useful for showcasing:

- Pod fuzzy search and multi-namespace browsing
- Running / Failed / Pending-style states
- Logs, env (ConfigMap/Secret refs), metrics gauges
- Deployments, Services, Jobs, CronJobs, PVCs, HPAs, Ingresses, DaemonSets
- Cluster overview rings and Events

## Verify

```bash
kubectl get nodes
kubectl get pods -n demo
kubectl get pods -n staging
kubectl top nodes          # after metrics-server is ready
kubectl top pods -n demo
```

## Tear down

```bash
./scripts/kind-demo-up.sh --down
```

## Troubleshooting

**`docker is not running`**  
Start Docker Desktop (or your engine) and retry.

**metrics empty in Pyxis**  
Wait ~1–2 minutes after bootstrap for metrics-server scrapes, then refresh. Confirm with `kubectl top pods -n demo`.

**Cluster already exists**  
Use `--reset` to wipe and recreate, or `--down` then a normal up.

**Slow first run**  
kind pulls node images and the script pulls workload images (`nginx`, `busybox`, `redis`, etc.). Later runs are faster if images are cached.

## Related

- Script: [`scripts/kind-demo-up.sh`](../scripts/kind-demo-up.sh)
- Compose (Dex + web): [`docker-compose.yml`](../docker-compose.yml)
