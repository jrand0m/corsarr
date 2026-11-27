# Security policy

## Reporting a vulnerability

Please report vulnerabilities privately through GitHub:
**Security › Report a vulnerability** on https://github.com/jrand0m/corsarr
(private vulnerability reporting). Do not open a public issue for a security problem.

Include what you found, the version (`corsarr version --json`), how to reproduce it and the
impact you expect. You will get an acknowledgement within 7 days and a fix or a plan within
30 days for confirmed issues. Fixes ship as a new release with a GitHub security advisory
that credits you, unless you prefer otherwise.

## Supported versions

corsarr is before 1.0. Security fixes go into the latest release only; upgrade to it.
Pre-releases (`-rc.N`) are not supported once the release they lead to is out.

| Version | Supported |
| --- | --- |
| latest release | yes |
| older releases | no |

## What you can verify yourself

Releases are reproducible and signed. [`VERIFYING.md`](VERIFYING.md) shows how
to check checksums, build-provenance and SBOM attestations, the image signature, and how
to rebuild a tag and compare it byte for byte with what was published.

## How releases are protected

- Releases are built only by the tag workflow, from tags on `master`, twice on independent
  runners; the release fails if the builds differ.
- Build jobs run without secrets and with read-only repository access. Publishing uses
  short-lived OpenID Connect credentials; nothing is signed with a long-lived key.
- Third-party actions are pinned by commit, base images and the builder by digest, and
  every dependency by lockfile; automated updates propose changes to these pins.
- Pull requests are checked for known-vulnerable Go code paths, npm advisories in shipped
  UI dependencies, and vulnerable new dependencies.
- The container runs as a non-root user on `scratch`, with no shell or package manager.
