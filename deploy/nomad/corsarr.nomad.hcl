# corsarr on HashiCorp Nomad with the Docker driver: nomad job run corsarr.nomad.hcl
# Needs a host volume "corsarr-data" (writable by uid 65532) and "media" on the client:
#   client { host_volume "corsarr-data" { path = "/opt/corsarr" }  host_volume "media" { path = "/srv/media" } }
job "corsarr" {
  type = "service"

  group "corsarr" {
    count = 1

    network {
      port "http" {
        static = 8090
        to     = 8090
      }
    }

    volume "data" {
      type      = "host"
      source    = "corsarr-data"
      read_only = false
    }

    volume "media" {
      type      = "host"
      source    = "media"
      read_only = false # true for shadow mode
    }

    service {
      name     = "corsarr"
      port     = "http"
      provider = "nomad"

      check {
        type     = "http"
        path     = "/api/v1/health"
        interval = "30s"
        timeout  = "5s"
      }
    }

    restart {
      attempts = 3
      interval = "5m"
      delay    = "10s"
      mode     = "delay"
    }

    task "corsarr" {
      driver = "docker"
      user   = "65532:65532"

      config {
        image           = "ghcr.io/jrand0m/corsarr:v0.4.0"
        ports           = ["http"]
        args            = ["--configFile=/local/corsarr.yml"]
        readonly_rootfs = true
        cap_drop        = ["ALL"]
        security_opt    = ["no-new-privileges"]

        mount {
          type   = "tmpfs"
          target = "/tmp"
          tmpfs_options {
            size = 67108864
          }
        }
      }

      env {
        GOMEMLIMIT = "96MiB"
      }

      volume_mount {
        volume      = "data"
        destination = "/data"
      }

      volume_mount {
        volume      = "media"
        destination = "/media"
      }

      # Generated from `corsarr config defaults --format=yaml`. Secrets: ${env:NAME} with
      # NAME from a Nomad Variable rendered into env by a second template block.
      template {
        destination = "local/corsarr.yml"
        data        = <<-EOF
          version: 1
          server:
            address: ":8090"
          data:
            dir: /data
          log:
            level: info
          auth:
            mode: password
          mediaKinds:
            tv: { mode: active }
            movie: { mode: active }
            music: { mode: active }
            book: { mode: disabled }
        EOF
      }

      resources {
        cpu        = 200
        memory     = 64
        memory_max = 256
      }
    }
  }
}
