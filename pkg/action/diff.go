package action

import (
	"github.com/databus23/helm-diff/v3/diff"
	logSetup "github.com/semx/helmtide/pkg/log"
	"github.com/urfave/cli/v2"
)

const (
	// DiffModeLive is a subcommand name for diffing manifests in plan with actually running manifests in k8s.
	DiffModeLive = "live"

	// DiffModeLocal is a subcommand name for diffing manifests in two plans.
	DiffModeLocal = "local"

	// DiffModeNone is a subcommand name for skipping diffing.
	DiffModeNone = "none"

	// DiffDetailedExitcode is the exit code returned by the diff command when
	// --detailed-exitcode is enabled and differences were found. It mirrors the
	// convention used by `terraform plan -detailed-exitcode` and
	// `helm diff --detailed-exitcode`.
	DiffDetailedExitcode = 2
)

// Diff is a struct for running 'diff' commands.
type Diff struct {
	*diff.Options
	kindSuppressHelper cli.StringSlice
	findRenamesHelper  float64
	ThreeWayMerge      bool // maybe it should move to DiffLive?

	// DetailedExitcode makes the diff subcommands exit with DiffDetailedExitcode
	// when differences are found (and 0 when there are none), like
	// `helm diff --detailed-exitcode`. It is bound to the flag on the parent
	// `diff` command so it works before the subcommand (`diff --detailed-exitcode
	// live`).
	DetailedExitcode bool

	// detailedExitcodeSub is bound to the copy of the flag on each subcommand so
	// it also works after the subcommand (`diff live --detailed-exitcode`). It is
	// a separate destination on purpose: a shared one would let the subcommand's
	// default (false) overwrite a value already set on the parent. wantDetailedExitcode
	// ORs the two.
	detailedExitcodeSub bool
}

// wantDetailedExitcode reports whether --detailed-exitcode was requested, no
// matter which side of the subcommand it was placed on.
func (d *Diff) wantDetailedExitcode() bool {
	return d.DetailedExitcode || d.detailedExitcodeSub
}

// detailedExitcodeErr returns a cli.ExitCoder with DiffDetailedExitcode when the
// --detailed-exitcode flag is enabled and the diff found changes. It returns nil
// otherwise, keeping the default behavior (exit 0) intact. Real diff failures are
// handled separately by returning their own error, so they still exit with 1.
func detailedExitcodeErr(enabled, changed bool) error {
	if enabled && changed {
		return cli.Exit("", DiffDetailedExitcode)
	}

	return nil
}

// Cmd returns 'diff' *cli.Command.
func (d *Diff) Cmd() *cli.Command {
	plan := DiffLocal{diff: d}
	live := DiffLive{diff: d}

	return &cli.Command{
		Name:     "diff",
		Category: Step1,
		Usage:    "show differences",
		Aliases:  []string{"vs"},
		Flags:    d.flags(),
		Before: func(q *cli.Context) error {
			d.FixFields()

			return nil
		},
		Subcommands: []*cli.Command{
			plan.Cmd(),
			live.Cmd(),
		},
	}
}

// flags return flag set of CLI urfave.
func (d *Diff) flags() []cli.Flag {
	d.Options = &diff.Options{}

	self := []cli.Flag{
		flagDiffWide(&d.OutputContext),
		flagDiffShowSecret(&d.ShowSecrets),
		flagDiffThreeWayMerge(&d.ThreeWayMerge),
		flagDiffDetailedExitcode(&d.DetailedExitcode),
		&cli.BoolFlag{
			Name:        "strip-trailing-cr",
			Usage:       "strip trailing carriage return on input",
			Value:       false,
			Category:    "DIFF",
			EnvVars:     EnvVars("DIFF_STRIP_TRAILING_CR"),
			Destination: &d.StripTrailingCR,
		},
		&cli.Float64Flag{
			Name:        "find-renames",
			Usage:       "enable rename detection if set to any value greater than 0.",
			Value:       1,
			Category:    "DIFF",
			EnvVars:     EnvVars("DIFF_FIND_RENAMES"),
			Destination: &d.findRenamesHelper,
		},
		&cli.StringSliceFlag{
			Name:  "suppress",
			Usage: "allows suppression kinds of the values listed in the diff output (\"Secret\" for example)",
			// Value: cli.NewStringSlice("Secret"),
			Category:    "DIFF",
			EnvVars:     EnvVars("DIFF_SUPPRESS", "DIFF_SUPPRESS_KINDS"),
			Destination: &d.kindSuppressHelper,
		},
	}

	return self
}

// FixFields initializes struct for diff action.
func (d *Diff) FixFields() {
	d.OutputFormat = logSetup.Default.Format()
	d.SuppressedKinds = d.kindSuppressHelper.Value()
	d.FindRenames = float32(d.findRenamesHelper)
}
