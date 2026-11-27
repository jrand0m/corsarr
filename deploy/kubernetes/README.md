# Kubernetes (plain manifests + Kustomize)

| Path | What it adds |
|---|---|
| `base/` | namespace with the `restricted` Pod Security Standard enforced, service account without token, config, data and media claims, deployment, service |
| `overlays/shadow/` | every media kind in shadow mode, media mounted read-only |
| `overlays/ingress/` | an Ingress for `corsarr.example.com` |

The pod runs as uid/gid 65532 with a read-only root filesystem, in-memory `/tmp`, no
capabilities, `RuntimeDefault` seccomp, probes on `/api/v1/health`, 64Mi request / 256Mi
limit and `GOMEMLIMIT`.

**Prerequisites**: Kubernetes 1.25+ with a default StorageClass; `kubectl` (Kustomize is
built in).

```sh
kubectl apply -k deploy/kubernetes/base
kubectl -n corsarr port-forward svc/corsarr 8090      # then open http://localhost:8090
```

Replace the `corsarr-media` claim in `base/pvc.yaml` with your media volume (NFS, a CSI
share, …); downloads and library belong on that one volume, mounted at the same path in the
download client. Copy `base/` or add your own overlay rather than editing in place.

**Config**: `base/corsarr.yml` becomes the `corsarr-config` ConfigMap (generated from
`corsarr config defaults --format=yaml`). Secrets: create a Secret, uncomment `envFrom`
in `deployment.yaml` and reference the keys as `${env:NAME}` in `corsarr.yml`.

**Data**: the PVC `corsarr-data` at `/data`.

**Upgrade**: change `newTag` in `base/kustomization.yaml` (or
`kustomize edit set image ghcr.io/jrand0m/corsarr:vX.Y.Z`) and re-apply.

**Backup**: `kubectl -n corsarr scale deploy/corsarr --replicas=0`, snapshot or copy the
`corsarr-data` volume, scale back to 1.
