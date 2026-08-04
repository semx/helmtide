#!/usr/bin/env bash
#
# Runs govulncheck and fails on any vulnerability that helmtide's own code can
# reach and that is not written down in .github/govulncheck-baseline.txt.
#
# The baseline is empty as of the move to helm v4, but the gate stays: when an
# advisory turns up with no fixed version to move to, `govulncheck ./...` exits
# non-zero on every run, and a job that is red every single run teaches people
# to ignore it. `|| true` would make it green by lying. So the check is
# "nothing new", not "nothing at all":
#
#   * a reachable advisory that is not in the baseline fails the job -- that is
#     the regression we care about;
#   * a baseline entry that no longer shows up does not fail anything, it is
#     reported so the line can be deleted;
#   * the baseline holds ids, not counts, so swapping one vulnerability for
#     another cannot slip through.
#
# Run it locally exactly as CI does:
#
#   .github/scripts/govulncheck.sh
#
set -euo pipefail

root=$(cd -- "$(dirname -- "$0")/../.." && pwd)
baseline_file=${GOVULNCHECK_BASELINE:-$root/.github/govulncheck-baseline.txt}
report=${GOVULNCHECK_REPORT:-$root/govulncheck.json}
summary=${GITHUB_STEP_SUMMARY:-/dev/null}

govulncheck=$(command -v govulncheck || true)
if [ -z "$govulncheck" ]; then
	govulncheck=$(go env GOPATH)/bin/govulncheck
fi

if [ ! -x "$govulncheck" ]; then
	echo "govulncheck is not installed: go install golang.org/x/vuln/cmd/govulncheck@latest" >&2
	exit 2
fi

if [ ! -f "$baseline_file" ]; then
	echo "no baseline at $baseline_file" >&2
	exit 2
fi

work=$(mktemp -d)
# shellcheck disable=SC2064 # $work is fixed at trap time on purpose
trap "rm -rf '$work'" EXIT

cd "$root"

echo "scanning with $("$govulncheck" -version | head -n 2 | tr '\n' ' ')"

# In -format json govulncheck exits 0 whatever it finds; the verdict below is
# ours to make.
"$govulncheck" -format json ./... >"$report"

# A finding whose first trace frame names a function is one govulncheck could
# trace from our code into the vulnerable symbol -- the same set it prints as
# "Your code is affected by N vulnerabilities". Findings that stop at the module
# or package level are dependencies we merely carry, and are not gated here.
called='select(has("finding")) | .finding | select(.trace[0].function != null)'

jq -r "$called | .osv" "$report" | sort -u >"$work/found"

sed -e 's/#.*//' "$baseline_file" |
	tr -d '[:blank:]' |
	grep -E '^GO-[0-9]{4}-[0-9]+$' |
	sort -u >"$work/baseline" || true

comm -23 "$work/found" "$work/baseline" >"$work/new"
comm -13 "$work/found" "$work/baseline" >"$work/stale"

jq -r "$called | [.osv, (.trace[0].module // \"-\"), (.trace[0].version // \"-\"), (.fixed_version // \"none\")] | @tsv" "$report" |
	sort -u | awk '!seen[$1]++' >"$work/details"

{
	echo "### govulncheck"
	echo
	echo "| advisory | module | version | fixed in | status |"
	echo "| --- | --- | --- | --- | --- |"
	while IFS=$(printf '\t') read -r osv module version fixed; do
		status=accepted
		if grep -qx "$osv" "$work/new"; then
			status="**NEW**"
		fi
		echo "| [$osv](https://pkg.go.dev/vuln/$osv) | $module | $version | $fixed | $status |"
	done <"$work/details"
	echo
} >>"$summary"

echo
printf 'reachable: %s, accepted by the baseline: %s\n' \
	"$(wc -l <"$work/found" | tr -d ' ')" "$(wc -l <"$work/baseline" | tr -d ' ')"
cat "$work/details"
echo

status=0

if [ -s "$work/new" ]; then
	while read -r osv; do
		echo "::error::$osv is reachable from helmtide code and is not in $(basename "$baseline_file"). Update the dependency, or add the id with the reason it has to stay: https://pkg.go.dev/vuln/$osv"
	done <"$work/new"
	echo "new reachable vulnerabilities: $(wc -l <"$work/new" | tr -d ' ')" >>"$summary"
	status=1
fi

if [ -s "$work/stale" ]; then
	while read -r osv; do
		echo "::notice::$osv is no longer reachable. Drop it from $(basename "$baseline_file")."
	done <"$work/stale"
	echo "no longer reachable, can be removed from the baseline: $(tr '\n' ' ' <"$work/stale")" >>"$summary"
fi

exit "$status"
