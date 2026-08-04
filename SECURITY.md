# Security Policy

## Reporting a vulnerability

Report anything you find in helmtide to **securityreport@sannikov.dev** or to
[@sannikovdev](https://t.me/sannikovdev). Please do not open a public issue for
it first.

Include enough to reproduce: the version, the configuration that triggers it and
what you observed. Every report is looked at, and you will hear back whether or
not it turns out to be exploitable.

## What this covers

helmtide is a fork of [helmwave](https://github.com/helmwave/helmwave), and much
of the code is shared. If a vulnerability affects upstream too, the report is
passed on to them once a fix exists here, with credit to whoever found it.

Bugs in helmwave itself belong to
[its own security policy](https://github.com/helmwave/helmwave/blob/main/SECURITY.md),
not to this address.

## Supported versions

The latest release. There are no long-term support branches, so a fix goes into
the next release rather than being backported.

## Dependencies

Known vulnerabilities in dependencies are checked on every push with
`govulncheck`; the build fails on anything reachable that is not written down,
with a reason, in [`.github/govulncheck-baseline.txt`](.github/govulncheck-baseline.txt).
No separate report is needed for those — but if one of them turns out to be
exploitable through helmtide rather than merely reachable, that is worth an
email.
