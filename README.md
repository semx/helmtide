# helmtide

**A maintained fork of [helmwave](https://github.com/helmwave/helmwave).**

helmwave is a helm3-native tool for deploying a whole set of Helm releases from
one file: a planfile you can review before applying, a dependency graph instead
of a wall of `helm upgrade` calls, and live resource tracking through
[kubedog](https://github.com/werf/kubedog). The design is good. It stopped
receiving releases in December 2025, with its own release pull request left
open and community contributions unreviewed since.

helmtide picks it up from there. This is a fork, not a rewrite, and not a
replacement blessed by the original authors — the credit for the design and for
almost all of the code belongs to [Dmitriy Zhilyaev](https://github.com/zhilyaev)
and the helmwave contributors. See [ATTRIBUTION.md](ATTRIBUTION.md).

## What is different

**Dependencies are current.** `govulncheck` reported 33 vulnerabilities the
code actually reaches, including helm 3.18.4 (panic on malformed YAML, memory
exhaustion through a crafted JSON schema), go-getter 1.7.8 (symlink attacks,
reached from `downloadRemoteSrc`) and go-git 5.13.0 (credentials forwarded
across a redirect to another host). That is down to 5, and each of those has no
upstream fix yet or needs a newer Go toolchain.

**The test suite cannot reach your cluster.** Running it used to send helm
dry-runs at whatever `~/.kube/config` pointed to — on the machine this fork was
started on, a production EKS endpoint, and it only failed because the
credentials had expired. Importing the test helpers now pins `KUBECONFIG` to a
kubeconfig that goes nowhere, and the twelve tests that genuinely need an API
server skip with a reason instead of failing with "cluster unreachable".

`go test ./...` is green on a laptop with no cluster and no credentials: 18
packages, 0 failures. To run the cluster-dependent ones:

```console
HELMTIDE_TEST_CLUSTER=~/.kube/config-of-a-throwaway-cluster go test ./...
```

## Drop-in

An existing helmwave setup runs unchanged:

| you have | helmtide |
| --- | --- |
| `helmwave.yml` | used as-is when there is no `helmtide.yml` |
| `helmwave.yml.tpl` | same |
| `HELMWAVE_*` variables | still read; `HELMTIDE_*` wins when both are set |

So `helmtide build` works in a repository written for helmwave, and you can
migrate one file at a time.

## Install

```console
go install github.com/semx/helmtide/cmd/helmtide@latest
```

## Usage

```console
helmtide build      # render the plan
helmtide up         # apply it
```

The [helmwave documentation](https://docs.helmwave.app) still describes the
configuration format accurately; where helmtide diverges it is written down here.

## Contributing

Issues and pull requests are welcome, including the ones that have been waiting
upstream. If you are the author of a pull request that was left open there and
you would rather it went to helmwave when maintenance resumes, say so and it
stays yours — nothing is merged here under someone else's name without credit.

## License

MIT, inherited from helmwave. The original copyright notice is kept in
[LICENSE](LICENSE) alongside ours, as the license requires.
