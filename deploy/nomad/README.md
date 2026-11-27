# HashiCorp Nomad

[`corsarr.nomad.hcl`](corsarr.nomad.hcl): Docker driver, one allocation, non-root,
read-only root filesystem with tmpfs `/tmp`, no capabilities, HTTP health check, config
rendered by a template.

**Prerequisites**: Nomad 1.7+ with the Docker driver; client host volumes `corsarr-data`
(writable by uid 65532) and `media`. `memory_max` needs memory oversubscription enabled
(otherwise `memory` is the hard limit: raise it to 256).

```sh
nomad job run corsarr.nomad.hcl
```

**Data**: the `corsarr-data` host volume path on the client.

**Upgrade**: change the image tag and `nomad job run` again.

**Backup**: `nomad job stop corsarr`, archive the host volume path, run the job again.
