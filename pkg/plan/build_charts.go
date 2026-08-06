package plan

import (
	"github.com/semx/helmtide/pkg/release"
)

// buildReleaseChart downloads the chart of a single release into the plan
// tmpdir.
//
// It is called from buildReleaseManifest rather than as a plan-wide step,
// because it has to run after that release's pre_build hook: a hook is allowed
// to produce the chart itself, e.g. by cloning a repository that only ships a
// chart directory and is not a chart repository.
//
//nolint:wrapcheck // the release logs enough context of its own
func (p *Plan) buildReleaseChart(rel release.Config) error {
	rel.Logger().Info("building chart")

	return rel.DownloadChart(p.tmpDir)
}
