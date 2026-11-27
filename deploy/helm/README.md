# Helm

The `corsarr` chart runs one pod under the `restricted` Pod Security Standard: uid/gid
65532, read-only root filesystem, in-memory `/tmp`, all capabilities dropped,
`RuntimeDefault` seccomp, no service-account token. The chart version follows the app
version (chart `0.4.0` runs `v0.4.0`).

**Prerequisites**: Kubernetes 1.25+, Helm 3.8+, a default StorageClass (or an existing
claim for `/data`).

```sh
helm install corsarr oci://ghcr.io/jrand0m/charts/corsarr --version 0.4.0 \
  --namespace corsarr --create-namespace -f my-values.yaml
# from a checkout:
helm install corsarr deploy/helm/corsarr --namespace corsarr --create-namespace
```

`kubectl -n corsarr port-forward svc/corsarr 8090` and open http://localhost:8090.

## Values

| Key | Default | Purpose |
|---|---|---|
| `image.repository` / `tag` / `digest` | `ghcr.io/jrand0m/corsarr` / appVersion / — | image, optionally pinned by digest |
| `resources` | requests 64Mi, limit 256Mi | memory sizing; `goMemLimit` (192MiB) tracks the limit |
| `persistence.*` | 1Gi PVC | `/data`; `existingClaim` reuses a claim; the PVC is kept on uninstall |
| `media[]` | none | media volumes: `name`, `mountPath`, `readOnly`, any `volume` source |
| `config` | defaults | rendered to a ConfigMap at `/etc/corsarr/corsarr.yml` (generated from `corsarr config defaults --format=yaml`); `existingConfigMap` instead |
| `envFromSecrets[]`, `extraEnv[]` | none | Secrets exposed as env for `${env:NAME}` references in `config` |
| `service.*` | ClusterIP 8090 | |
| `ingress.*` | off | Ingress |
| `httpRoute.*` | off | Gateway API HTTPRoute |
| `podSecurityContext`, `securityContext` | restricted | |
| `probes.*` | `GET /api/v1/health` | startup, liveness, readiness |
| `serviceMonitor.*` | off | Prometheus Operator ServiceMonitor |

Secrets never go in values. Create them, then reference them:

```sh
kubectl -n corsarr create secret generic corsarr-secrets --from-literal=TMDB_API_KEY=…
```

```yaml
envFromSecrets: [corsarr-secrets]
config:
  metadata:
    tmdb: { type: tmdb, apiKey: "${env:TMDB_API_KEY}" }
```

Shadow mode: mount media with `readOnly: true` and set `config.mediaKinds.<kind>.mode: shadow`.

**Data**: the PVC `<release>-corsarr-data` (or your `existingClaim`).

**Upgrade**: `helm upgrade corsarr oci://ghcr.io/jrand0m/charts/corsarr --version X.Y.Z
--reuse-values`. The pod is replaced (strategy `Recreate`); migrations run on start.
`helm test corsarr` probes the health endpoint.

**Backup**: scale to zero, copy the volume, scale back:

```sh
kubectl -n corsarr scale deploy/corsarr --replicas=0
# snapshot the PVC (VolumeSnapshot) or copy it from a helper pod, then:
kubectl -n corsarr scale deploy/corsarr --replicas=1
```
