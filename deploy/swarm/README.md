# Docker Swarm

[`stack.yml`](stack.yml): one replica, stop-first updates, non-root, read-only root
filesystem with tmpfs `/tmp`, no capabilities, health check, memory limit. The config is
a Swarm config; secrets are Swarm secrets read as `${file:/run/secrets/name}`.

**Prerequisites**: a Swarm manager; the node holding the media share labelled
`corsarr=true`, with `/srv/media` writable by uid 65532.

```sh
docker node update --label-add corsarr=true <node>
docker config create corsarr_yml ../compose/corsarr.yml      # edit the indexer URL first
printf %s 'qbittorrent-password' | docker secret create qbit_password -
TMDB_API_KEY=… INDEXER_API_KEY=… docker stack deploy -c stack.yml corsarr
```

**Config**: [`../compose/corsarr.yml`](../compose/corsarr.yml) (generated from
`corsarr config defaults --format=yaml`). Swarm configs are immutable: create
`corsarr_yml_v2` and point the stack at it to change it.

**Data**: the volume `corsarr_corsarr-data` on the labelled node.

**Upgrade**: change the image tag and `docker stack deploy -c stack.yml corsarr` again.

**Backup**: `docker service scale corsarr_corsarr=0`, archive the volume on the node,
scale back to 1.
