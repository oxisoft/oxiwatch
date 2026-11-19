# Security policy

## Reporting a vulnerability

Please do not open a public issue for security problems.

Report them privately through GitHub's private vulnerability reporting:
https://github.com/oxisoft/oxiwatch/security/advisories/new

If you cannot use GitHub, email security@oxisoft.io (see also
https://oxiwatch.oxisoft.io/.well-known/security.txt).

We aim to answer within 3 working days. Fixes ship in a new release, and the
CHANGELOG credits the reporter if they want to be named.

## Supported versions

Only the latest release gets security fixes. Upgrade with:

```bash
sudo oxiwatch upgrade
sudo systemctl restart oxiwatch
```

## Scope

The `oxiwatch` binary, `scripts/install.sh`, the systemd unit it installs, and
the self-upgrade path (`oxiwatch upgrade`, which verifies the release checksum).
