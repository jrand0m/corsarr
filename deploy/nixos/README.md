# NixOS (flake)

[`flake.nix`](flake.nix) packages the release binary for Linux and macOS (amd64, arm64),
pinned to the archive checksums from the release's `SHA256SUMS`, and provides the
`services.corsarr` NixOS module ([`module.nix`](module.nix)): a dedicated `corsarr` user,
the state directory `/var/lib/corsarr`, `/etc/corsarr/corsarr.yml` generated from Nix and
the same systemd sandbox as [`../systemd/corsarr.service`](../systemd/corsarr.service).

**Prerequisites**: NixOS 25.05 or later with flakes enabled.

```nix
# flake.nix of your system
{
  inputs.corsarr.url = "github:jrand0m/corsarr?dir=deploy/nixos";
  outputs = { nixpkgs, corsarr, ... }: {
    nixosConfigurations.myhost = nixpkgs.lib.nixosSystem {
      system = "x86_64-linux";
      modules = [
        corsarr.nixosModules.default
        {
          services.corsarr = {
            enable = true;
            openFirewall = true;
            mediaPaths = [ "/srv/media" ];      # downloads + library, one filesystem
            group = "media";                     # a group that may write it (create it yourself)
            credentials.tmdb_api_key = "/run/secrets/tmdb_api_key";
            settings.metadata.tmdb = {
              type = "tmdb";
              apiKey = "\${file:/run/credentials/corsarr.service/tmdb_api_key}";
            };
          };
        }
      ];
    };
  };
}
```

`nixos-rebuild switch`, then open http://localhost:8090. Without a NixOS system:
`nix run github:jrand0m/corsarr?dir=deploy/nixos -- --data.dir=$HOME/.local/share/corsarr`.

| Option | Default | |
|---|---|---|
| `port` | `8090` | sets `server.address` |
| `openFirewall` | `false` | |
| `settings` | `{}` | `corsarr.yml` as Nix; `corsarr config defaults --format=yaml` lists every key |
| `configFile` | `null` | a ready `corsarr.yml` instead of `settings` |
| `mediaPaths` / `readOnlyMediaPaths` | `[]` | writable paths / shadow mode |
| `credentials` | `{}` | `LoadCredential=` files, referenced as `${file:/run/credentials/corsarr.service/<name>}` |
| `environmentFile` | `null` | values for `${env:NAME}` references |
| `user` / `group` | `corsarr` | |

Secrets are only ever references: write `''${…}` inside Nix `''` strings and `"\${…}"` in
double-quoted ones so Nix does not interpolate them.

**Data**: `/var/lib/corsarr`.

**Upgrade**: `nix flake update corsarr`, then `nixos-rebuild switch`.

**Backup**: `systemctl stop corsarr`, archive `/var/lib/corsarr`, `systemctl start corsarr`.

**Check**: `nix flake check ./deploy/nixos` builds the package, renders the unit of a sample
system and runs `corsarr config validate` on its generated config.
