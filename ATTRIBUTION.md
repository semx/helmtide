# Attribution

helmtide is a fork of [helmwave](https://github.com/helmwave/helmwave), created
by [Dmitriy Zhilyaev](https://github.com/zhilyaev) in November 2020 and
developed with [r3nic1e](https://github.com/r3nic1e) and others. It is released
under the MIT license, which this fork keeps.

Nearly all of the code here was written by them. The planfile model, the
dependency graph, the kubedog integration, bundling helm as a library rather
than shelling out to it — all of that is their design, and it is the reason
this fork exists at all rather than something written from scratch.

The full history of their work is preserved in this repository: `git log` goes
back to the first helmwave commit, and [CHANGELOG.md](CHANGELOG.md) is theirs
up to v0.42.3.

## Why a fork

helmwave's last release was v0.42.3 on 17 December 2025 and its last commit to
`main` was on 23 December 2025. Its own release pull request (helmwave#1186,
"Release/0.43.0") has been open since 29 December 2025, and pull requests from
contributors have been waiting without review since.

This is not a criticism. Maintaining a tool for years is unpaid work and
stopping is allowed. But the dependencies kept moving, and a deployment tool
carrying known vulnerabilities is a problem for the people running it.

If upstream maintenance resumes, the changes here are offered back — they are
MIT and were built to be portable. A fork that becomes unnecessary is a good
outcome.

## Not affiliated

helmtide is not endorsed by, affiliated with, or supported by the helmwave
project or its authors. Please do not report helmtide problems to them.
