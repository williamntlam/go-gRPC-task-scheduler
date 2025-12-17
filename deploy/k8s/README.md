## Kubernetes load balancing + horizontal scaling (DIY)

This repo already demonstrates **load balancing** in Docker Compose using Envoy (`deploy/envoy.yaml`) with multiple `api-*` containers.

This directory adds a **Kubernetes** setup so you can:

- Run **multiple API replicas** behind a single endpoint (load balanced)
- Run **multiple worker replicas** (horizontal scaling)
- Enable **autoscaling** via `HorizontalPodAutoscaler` (HPA)
- Stress-test with very high request volume and observe scaling

### What gets deployed

Manifests live in `deploy/k8s/base/`:

- **CockroachDB** (single-node, dev) + Service
- **Redis** + Service
- **scheduler-api** Deployment + Service
- **scheduler-worker** Deployment
- **grpc-gateway** (Envoy) Deployment + NodePort Service
- **HPA** for API + Worker (CPU-based)

The gRPC entrypoint is Envoy on a NodePort.

### Prereqs

- Docker
- `kubectl`
- `kind`
- `kustomize` (or `kubectl -k`)
- Metrics server (required for HPA)

### 1) Create a local Kubernetes cluster (kind)

```bash
kind create cluster --config deploy/k8s/kind/cluster.yaml
```

This maps the cluster NodePort `30080` to host port `8080`.

### 2) Build images and load them into kind

Build the images using the existing Dockerfiles:

```bash
docker build -t scheduler-api:local -f cmd/api/Dockerfile .
docker build -t scheduler-worker:local -f cmd/worker/Dockerfile .
```

Load them into the kind cluster:

```bash
kind load docker-image scheduler-api:local --name task-scheduler
kind load docker-image scheduler-worker:local --name task-scheduler
```

### 3) Install metrics-server (for HPA)

If you don’t have it installed, apply the upstream components:

```bash
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml
```

If metrics-server can’t scrape kubelets in kind, patch it (common in local clusters):

```bash
kubectl -n kube-system patch deployment metrics-server \
  --type='json' \
  -p='[{"op":"add","path":"/spec/template/spec/containers/0/args/-","value":"--kubelet-insecure-tls"}]'
```

Verify:

```bash
kubectl top nodes
```

### 4) Deploy everything

```bash
kubectl apply -k deploy/k8s/base
```

Wait for pods:

```bash
kubectl -n task-scheduler get pods -w
```

### 5) Connect to the gRPC endpoint

Because of the kind port mapping, Envoy is reachable at:

- `localhost:8080`

Try listing services using your existing scripts or `grpcurl`:

```bash
grpcurl -plaintext localhost:8080 list
```

### 6) Observe load balancing (API replicas)

Check API replicas:

```bash
kubectl -n task-scheduler get deploy scheduler-api
kubectl -n task-scheduler get pods -l app=scheduler-api -o wide
```

Kubernetes load balances at the Service level; Envoy forwards to the Service.

### 7) Manual horizontal scaling

Scale API:

```bash
kubectl -n task-scheduler scale deployment scheduler-api --replicas=10
```

Scale Worker:

```bash
kubectl -n task-scheduler scale deployment scheduler-worker --replicas=20
```

### 8) Autoscaling (HPA)

HPAs are included in `deploy/k8s/base/hpa.yaml`.

Watch autoscaling decisions:

```bash
kubectl -n task-scheduler get hpa -w
```

### 9) Run very large load tests (millions of requests)

You already have `scripts/load-test.sh`. For Kubernetes:

- Point your client at `localhost:8080` (Envoy)
- The script uses `GRPC_PORT`, so set `GRPC_PORT=8080`

Example:

```bash
GRPC_PORT=8080 ./scripts/load-test.sh 5000 600 default
```

While testing, watch:

```bash
kubectl -n task-scheduler get pods -w
kubectl -n task-scheduler top pods
kubectl -n task-scheduler get hpa -w
```

### Notes / limitations (important for your experiment)

- **CockroachDB is single-node here** (dev only). For realistic scaling tests, use a multi-node CockroachDB StatefulSet or a managed DB.
- **HPA is CPU-based** in this starter setup. For gRPC throughput tests, you’ll often get better results with **custom metrics** (RPS, latency, queue depth).
- **Worker scaling** tends to be constrained by Redis + DB + job handler cost; scaling workers is usually the bigger win than scaling API.

### Cleanup

```bash
kubectl delete -k deploy/k8s/base
kind delete cluster --name task-scheduler
```
