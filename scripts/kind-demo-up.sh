#!/usr/bin/env bash
# Start a local kind cluster preloaded with demo Kubernetes resources for Pyxis showcase / e2e.
#
# Usage:
#   ./scripts/kind-demo-up.sh          # create + seed
#   ./scripts/kind-demo-up.sh --reset  # delete existing cluster first
#   ./scripts/kind-demo-up.sh --down   # delete cluster only
#
# Requires: docker, kind, kubectl

set -euo pipefail

CLUSTER_NAME="${PYXIS_KIND_CLUSTER:-pyxis-demo}"
CONTEXT="kind-${CLUSTER_NAME}"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

log()  { printf '==> %s\n' "$*"; }
warn() { printf '!!  %s\n' "$*" >&2; }
die()  { printf 'error: %s\n' "$*" >&2; exit 1; }

need() {
  command -v "$1" >/dev/null 2>&1 || die "missing dependency: $1"
}

cluster_exists() {
  kind get clusters 2>/dev/null | grep -qx "${CLUSTER_NAME}"
}

delete_cluster() {
  if cluster_exists; then
    log "Deleting kind cluster '${CLUSTER_NAME}'"
    kind delete cluster --name "${CLUSTER_NAME}"
  else
    log "Cluster '${CLUSTER_NAME}' does not exist"
  fi
}

create_cluster() {
  if cluster_exists; then
    log "Cluster '${CLUSTER_NAME}' already exists (use --reset to recreate)"
    return 0
  fi

  log "Creating kind cluster '${CLUSTER_NAME}' (1 control-plane + 2 workers)"
  kind create cluster --name "${CLUSTER_NAME}" --config - <<EOF
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
  - role: control-plane
    kubeadmConfigPatches:
      - |
        kind: InitConfiguration
        nodeRegistration:
          kubeletExtraArgs:
            node-labels: "ingress-ready=true"
    extraPortMappings:
      - containerPort: 80
        hostPort: 8088
        protocol: TCP
      - containerPort: 443
        hostPort: 8443
        protocol: TCP
  - role: worker
  - role: worker
EOF

  kubectl config use-context "${CONTEXT}" >/dev/null
  log "Waiting for nodes to be Ready"
  kubectl wait --for=condition=Ready nodes --all --timeout=180s
}

install_metrics_server() {
  log "Installing metrics-server (kind-friendly: --kubelet-insecure-tls)"
  kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

  # Kind kubelets use self-signed certs; metrics-server needs insecure TLS + InternalIP.
  kubectl -n kube-system patch deployment metrics-server --type='json' -p='[
    {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"},
    {"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-preferred-address-types=InternalIP"}
  ]' >/dev/null

  kubectl -n kube-system rollout status deployment/metrics-server --timeout=180s
}

seed_workloads() {
  log "Seeding demo namespaces and workloads"
  kubectl apply -f - <<'EOF'
---
apiVersion: v1
kind: Namespace
metadata:
  name: demo
  labels:
    purpose: pyxis-showcase
---
apiVersion: v1
kind: Namespace
metadata:
  name: staging
  labels:
    purpose: pyxis-showcase
---
apiVersion: v1
kind: Namespace
metadata:
  name: jobs
  labels:
    purpose: pyxis-showcase
---
# --- Config / secrets -------------------------------------------------------
apiVersion: v1
kind: ConfigMap
metadata:
  name: demo-config
  namespace: demo
data:
  APP_ENV: showcase
  LOG_LEVEL: info
  WELCOME: "Hello from Pyxis demo"
---
apiVersion: v1
kind: Secret
metadata:
  name: demo-secret
  namespace: demo
type: Opaque
stringData:
  API_TOKEN: "demo-token-not-real"
  DB_PASSWORD: "s3cr3t"
---
# --- Web frontend -----------------------------------------------------------
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web
  namespace: demo
  labels:
    app: web
spec:
  replicas: 3
  selector:
    matchLabels:
      app: web
  template:
    metadata:
      labels:
        app: web
    spec:
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          ports:
            - containerPort: 80
          envFrom:
            - configMapRef:
                name: demo-config
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
            limits:
              cpu: 200m
              memory: 128Mi
          readinessProbe:
            httpGet:
              path: /
              port: 80
            initialDelaySeconds: 2
            periodSeconds: 5
---
apiVersion: v1
kind: Service
metadata:
  name: web
  namespace: demo
spec:
  selector:
    app: web
  ports:
    - name: http
      port: 80
      targetPort: 80
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: web
  namespace: demo
  annotations:
    nginx.ingress.kubernetes.io/rewrite-target: /
spec:
  rules:
    - host: web.demo.local
      http:
        paths:
          - path: /
            pathType: Prefix
            backend:
              service:
                name: web
                port:
                  number: 80
---
apiVersion: autoscaling/v2
kind: HorizontalPodAutoscaler
metadata:
  name: web
  namespace: demo
spec:
  scaleTargetRef:
    apiVersion: apps/v1
    kind: Deployment
    name: web
  minReplicas: 2
  maxReplicas: 6
  metrics:
    - type: Resource
      resource:
        name: cpu
        target:
          type: Utilization
          averageUtilization: 70
---
# --- API + redis ------------------------------------------------------------
apiVersion: apps/v1
kind: Deployment
metadata:
  name: api
  namespace: demo
  labels:
    app: api
spec:
  replicas: 2
  selector:
    matchLabels:
      app: api
  template:
    metadata:
      labels:
        app: api
    spec:
      containers:
        - name: api
          image: hashicorp/http-echo:1.0
          args: ["-text=pyxis-api-ok", "-listen=:5678"]
          ports:
            - containerPort: 5678
          env:
            - name: TOKEN
              valueFrom:
                secretKeyRef:
                  name: demo-secret
                  key: API_TOKEN
          resources:
            requests:
              cpu: 25m
              memory: 32Mi
            limits:
              cpu: 100m
              memory: 64Mi
---
apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: demo
spec:
  selector:
    app: api
  ports:
    - name: http
      port: 5678
      targetPort: 5678
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: redis
  namespace: demo
  labels:
    app: redis
spec:
  replicas: 1
  selector:
    matchLabels:
      app: redis
  template:
    metadata:
      labels:
        app: redis
    spec:
      containers:
        - name: redis
          image: redis:7-alpine
          ports:
            - containerPort: 6379
          resources:
            requests:
              cpu: 25m
              memory: 64Mi
            limits:
              cpu: 100m
              memory: 128Mi
---
apiVersion: v1
kind: Service
metadata:
  name: redis
  namespace: demo
spec:
  selector:
    app: redis
  ports:
    - name: redis
      port: 6379
      targetPort: 6379
---
# --- Storage ----------------------------------------------------------------
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: demo-data
  namespace: demo
spec:
  accessModes: ["ReadWriteOnce"]
  resources:
    requests:
      storage: 1Gi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: writer
  namespace: demo
  labels:
    app: writer
spec:
  replicas: 1
  selector:
    matchLabels:
      app: writer
  template:
    metadata:
      labels:
        app: writer
    spec:
      containers:
        - name: writer
          image: busybox:1.36
          command: ["sh", "-c", "while true; do date >> /data/log.txt; sleep 30; done"]
          volumeMounts:
            - name: data
              mountPath: /data
          resources:
            requests:
              cpu: 10m
              memory: 16Mi
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: demo-data
---
# --- Staging: mixed health --------------------------------------------------
apiVersion: apps/v1
kind: Deployment
metadata:
  name: frontend
  namespace: staging
  labels:
    app: frontend
spec:
  replicas: 2
  selector:
    matchLabels:
      app: frontend
  template:
    metadata:
      labels:
        app: frontend
    spec:
      containers:
        - name: nginx
          image: nginx:1.27-alpine
          ports:
            - containerPort: 80
---
apiVersion: v1
kind: Service
metadata:
  name: frontend
  namespace: staging
spec:
  selector:
    app: frontend
  ports:
    - port: 80
      targetPort: 80
---
# Intentionally CrashLoopBackOff for the UI
apiVersion: apps/v1
kind: Deployment
metadata:
  name: flaky
  namespace: staging
  labels:
    app: flaky
spec:
  replicas: 1
  selector:
    matchLabels:
      app: flaky
  template:
    metadata:
      labels:
        app: flaky
    spec:
      containers:
        - name: boom
          image: busybox:1.36
          command: ["sh", "-c", "echo boom; exit 1"]
          resources:
            requests:
              cpu: 10m
              memory: 8Mi
---
# Image pull backlog / pending-looking workload
apiVersion: apps/v1
kind: Deployment
metadata:
  name: missing-image
  namespace: staging
  labels:
    app: missing-image
spec:
  replicas: 1
  selector:
    matchLabels:
      app: missing-image
  template:
    metadata:
      labels:
        app: missing-image
    spec:
      containers:
        - name: nope
          image: ghcr.io/pyxis-demo/does-not-exist:never
          imagePullPolicy: Always
---
# --- Jobs / CronJobs --------------------------------------------------------
apiVersion: batch/v1
kind: Job
metadata:
  name: hello-once
  namespace: jobs
spec:
  backoffLimit: 1
  template:
    spec:
      restartPolicy: Never
      containers:
        - name: hello
          image: busybox:1.36
          command: ["sh", "-c", "echo hello-from-job; sleep 2"]
---
apiVersion: batch/v1
kind: CronJob
metadata:
  name: heartbeat
  namespace: jobs
spec:
  schedule: "*/2 * * * *"
  successfulJobsHistoryLimit: 3
  failedJobsHistoryLimit: 1
  jobTemplate:
    spec:
      template:
        spec:
          restartPolicy: OnFailure
          containers:
            - name: tick
              image: busybox:1.36
              command: ["sh", "-c", "date; echo tick"]
---
# --- DaemonSet across nodes -------------------------------------------------
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-agent
  namespace: demo
  labels:
    app: node-agent
spec:
  selector:
    matchLabels:
      app: node-agent
  template:
    metadata:
      labels:
        app: node-agent
    spec:
      containers:
        - name: agent
          image: busybox:1.36
          command: ["sh", "-c", "while true; do sleep 3600; done"]
          resources:
            requests:
              cpu: 10m
              memory: 16Mi
EOF

  log "Waiting for core demo Deployments"
  kubectl -n demo rollout status deployment/web --timeout=180s
  kubectl -n demo rollout status deployment/api --timeout=180s
  kubectl -n demo rollout status deployment/redis --timeout=180s
  kubectl -n staging rollout status deployment/frontend --timeout=180s || true

  # Generate a few Warning events for the Events view
  kubectl -n staging create event demo-warning \
    --type=Warning \
    --reason=Showcase \
    --message="Synthetic warning for Pyxis Events panel" \
    --dry-run=client -o yaml 2>/dev/null | kubectl apply -f - 2>/dev/null || true
}

print_summary() {
  echo
  log "Cluster ready"
  echo
  echo "  Cluster : ${CLUSTER_NAME}"
  echo "  Context : ${CONTEXT}"
  echo "  Nodes   :"
  kubectl get nodes -o wide
  echo
  echo "  Namespaces / pods:"
  kubectl get ns demo staging jobs
  echo
  kubectl get pods,svc,deploy,job,cronjob,pvc,hpa,ingress -n demo
  echo
  kubectl get pods,deploy -n staging
  echo
  kubectl get job,cronjob -n jobs
  echo
  echo "  Tip: switch context and launch Pyxis:"
  echo "    kubectl config use-context ${CONTEXT}"
  echo "    cd ${ROOT_DIR} && make run          # TUI"
  echo "    cd ${ROOT_DIR} && make run-web      # web UI on :8080 (--no-auth)"
  echo
  echo "  Tear down:"
  echo "    ${ROOT_DIR}/scripts/kind-demo-up.sh --down"
  echo
}

main() {
  need docker
  need kind
  need kubectl

  docker info >/dev/null 2>&1 || die "docker is not running"

  case "${1:-}" in
    --down|down|delete)
      delete_cluster
      exit 0
      ;;
    --reset|reset)
      delete_cluster
      ;;
    ""|--up|up)
      ;;
    -h|--help|help)
      sed -n '1,12p' "$0"
      exit 0
      ;;
    *)
      die "unknown argument: $1 (try --reset, --down, or --help)"
      ;;
  esac

  create_cluster
  install_metrics_server
  seed_workloads
  print_summary
}

main "$@"
