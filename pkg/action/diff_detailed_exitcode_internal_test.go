package action

import (
	"errors"
	"testing"

	"github.com/databus23/helm-diff/v3/diff"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// TestDetailedExitcodeErr proves the exit-code plumbing that turns the
// "changes found" signal into a process exit code.
//
// Truth table:
//   - flag off, changes found      -> exit 0 (nil)      : default behavior unchanged
//   - flag on,  changes found      -> exit 2 (ExitCoder): differences detected
//   - flag on,  no changes         -> exit 0 (nil)
//   - flag off, no changes         -> exit 0 (nil)
func TestDetailedExitcodeErr(t *testing.T) {
	t.Parallel()

	t.Run("flag off with changes exits 0", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, detailedExitcodeErr(false, true))
	})

	t.Run("flag on with changes exits 2", func(t *testing.T) {
		t.Parallel()

		err := detailedExitcodeErr(true, true)
		require.Error(t, err)

		var ec cli.ExitCoder
		require.True(t, errors.As(err, &ec), "must be a cli.ExitCoder so main propagates the code")
		require.Equal(t, DiffDetailedExitcode, ec.ExitCode())
		require.Equal(t, 2, ec.ExitCode())
	})

	t.Run("flag on without changes exits 0", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, detailedExitcodeErr(true, false))
	})

	t.Run("flag off without changes exits 0", func(t *testing.T) {
		t.Parallel()

		require.NoError(t, detailedExitcodeErr(false, false))
	})
}

// TestDiffDetailedExitcodeFlagRegistered ensures the flag is wired into every
// place a user can pass it: the parent `diff` command and both subcommands.
func TestDiffDetailedExitcodeFlagRegistered(t *testing.T) {
	t.Parallel()

	d := &Diff{Options: &diff.Options{}}

	hasFlag := func(flags []cli.Flag) bool {
		for _, f := range flags {
			for _, n := range f.Names() {
				if n == "detailed-exitcode" {
					return true
				}
			}
		}

		return false
	}

	require.True(t, hasFlag(d.flags()), "parent diff command must expose --detailed-exitcode")

	live := &DiffLive{diff: d}
	require.True(t, hasFlag(live.flags()), "diff live must expose --detailed-exitcode")

	local := &DiffLocal{diff: d}
	require.True(t, hasFlag(local.flags()), "diff local must expose --detailed-exitcode")
}

// TestDiffLocalRunRealErrorIsNotExitcode2 proves a genuine diff failure (here a
// missing plandir) is NOT masked as the "changes found" exit code 2. It stays a
// plain error, which main turns into the usual exit code 1 via log.Fatal.
func TestDiffLocalRunRealErrorIsNotExitcode2(t *testing.T) {
	t.Parallel()

	d := &DiffLocal{
		diff:     &Diff{Options: &diff.Options{}, DetailedExitcode: true},
		plandir1: t.TempDir() + "/does-not-exist",
		plandir2: t.TempDir() + "/does-not-exist-either",
	}

	err := d.Run(t.Context())
	require.Error(t, err)

	var ec cli.ExitCoder
	if errors.As(err, &ec) {
		require.NotEqual(t, DiffDetailedExitcode, ec.ExitCode(),
			"a real failure must not be reported as the detailed-exitcode value")
	}
}
