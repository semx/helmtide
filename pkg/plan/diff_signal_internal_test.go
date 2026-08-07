package plan

import (
	"testing"

	"github.com/databus23/helm-diff/v3/diff"
	"github.com/semx/helmtide/pkg/release"
	"github.com/stretchr/testify/suite"
)

// DiffSignalTestSuite exercises the "changes found" boolean returned by DiffPlan
// (and, indirectly, showChangesReport) fully hermetically: no cluster, no helm
// repositories, just crafted manifests compared locally.
type DiffSignalTestSuite struct {
	suite.Suite
}

//nolint:paralleltest // helm-diff writes to the global standard logger output
func TestDiffSignalTestSuite(t *testing.T) {
	suite.Run(t, new(DiffSignalTestSuite))
}

func diffOpts() *diff.Options {
	return &diff.Options{OutputFormat: "diff"}
}

func configMapManifest(value string) string {
	return "apiVersion: v1\n" +
		"kind: ConfigMap\n" +
		"metadata:\n" +
		"  name: redis\n" +
		"  namespace: default\n" +
		"data:\n" +
		"  key: " + value + "\n"
}

func (ts *DiffSignalTestSuite) newRelease() *MockReleaseConfig {
	r := NewMockReleaseConfig(ts.T())
	r.On("Name").Return("redis")
	r.On("Namespace").Return("default")
	r.On("KubeContext").Return("")
	r.On("Uniq").Return()

	return r
}

// TestNoReleasesReportsNoChanges: empty plans have nothing to diff.
func (ts *DiffSignalTestSuite) TestNoReleasesReportsNoChanges() {
	p := New(ts.T().TempDir())
	b := New(ts.T().TempDir())
	p.body = &planBody{}
	b.body = &planBody{}

	ts.Require().False(p.DiffPlan(b, diffOpts()))
}

// TestIdenticalManifestsReportNoChanges: same manifest on both sides -> false.
func (ts *DiffSignalTestSuite) TestIdenticalManifestsReportNoChanges() {
	rel := ts.newRelease()

	p := New(ts.T().TempDir())
	b := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}
	b.body = &planBody{Releases: release.Configs{rel}}

	same := configMapManifest("value1")
	p.manifests[rel.Uniq()] = same
	b.manifests[rel.Uniq()] = same

	ts.Require().False(p.DiffPlan(b, diffOpts()))
}

// TestDifferingManifestsReportChanges: a real difference -> true. This is the
// signal that the command turns into exit code 2.
func (ts *DiffSignalTestSuite) TestDifferingManifestsReportChanges() {
	rel := ts.newRelease()

	p := New(ts.T().TempDir())
	b := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}
	b.body = &planBody{Releases: release.Configs{rel}}

	p.manifests[rel.Uniq()] = configMapManifest("new-value")
	b.manifests[rel.Uniq()] = configMapManifest("old-value")

	ts.Require().True(p.DiffPlan(b, diffOpts()))
}
