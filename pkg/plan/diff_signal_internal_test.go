package plan

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/databus23/helm-diff/v3/diff"
	"github.com/semx/helmtide/pkg/release"
	"github.com/stretchr/testify/suite"
	"helm.sh/helm/v4/pkg/action"
	chart "helm.sh/helm/v4/pkg/chart/v2"
	kubeFake "helm.sh/helm/v4/pkg/kube/fake"
	helmRelease "helm.sh/helm/v4/pkg/release/v1"
)

// DiffSignalTestSuite exercises the "changes found" boolean and the error
// classification of the diff paths fully hermetically: no cluster, no helm
// repositories, just crafted manifests and mocked release lookups.
type DiffSignalTestSuite struct {
	suite.Suite

	ctx context.Context
}

//nolint:paralleltest // helm-diff writes to the global standard logger output
func TestDiffSignalTestSuite(t *testing.T) {
	suite.Run(t, new(DiffSignalTestSuite))
}

func (ts *DiffSignalTestSuite) SetupTest() {
	ts.ctx = context.Background()
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
// signal the command turns into exit code 2.
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

// TestRemovedReleaseReportsChanges: a release present only in the OLD plan is a
// removal. It must count as a change (issue #2) so a CI gate catches it.
func (ts *DiffSignalTestSuite) TestRemovedReleaseReportsChanges() {
	rel := ts.newRelease()

	// New plan has no releases; old plan still has "redis" with a manifest.
	p := New(ts.T().TempDir())
	b := New(ts.T().TempDir())
	p.body = &planBody{}
	b.body = &planBody{Releases: release.Configs{rel}}

	b.manifests[rel.Uniq()] = configMapManifest("value1")

	ts.Require().True(p.DiffPlan(b, diffOpts()))
}

// TestAddedReleaseReportsChanges: a release present only in the NEW plan is an
// addition and must count as a change.
func (ts *DiffSignalTestSuite) TestAddedReleaseReportsChanges() {
	rel := ts.newRelease()

	p := New(ts.T().TempDir())
	b := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}
	b.body = &planBody{}

	p.manifests[rel.Uniq()] = configMapManifest("value1")

	ts.Require().True(p.DiffPlan(b, diffOpts()))
}

// TestGetLiveNotFoundIsNotError: a release absent from the cluster is reported
// via notFound, not as an error.
func (ts *DiffSignalTestSuite) TestGetLiveNotFoundIsNotError() {
	rel := ts.newRelease()
	rel.On("Get", 0).Return(&helmRelease.Release{}, release.ErrNotFound)

	p := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}

	found, notFound, err := p.GetLive(ts.ctx)
	ts.Require().NoError(err)
	ts.Require().Empty(found)
	ts.Require().Len(notFound, 1)
}

// TestGetLiveRealErrorPropagates: an RBAC/network style failure must surface as
// an error, never be swallowed into notFound (issue #3).
func (ts *DiffSignalTestSuite) TestGetLiveRealErrorPropagates() {
	rel := ts.newRelease()
	rel.On("Get", 0).Return(&helmRelease.Release{}, errors.New("forbidden: RBAC deny"))

	p := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}

	_, _, err := p.GetLive(ts.ctx)
	ts.Require().Error(err)
}

// TestDiffLiveRealErrorReturnsError: on the live path a genuine lookup failure
// must return an error (exit 1), not be counted as a change (exit 2).
func (ts *DiffSignalTestSuite) TestDiffLiveRealErrorReturnsError() {
	rel := ts.newRelease()
	rel.On("Get", 0).Return(&helmRelease.Release{}, errors.New("connection refused"))

	p := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}

	changed, err := p.DiffLive(ts.ctx, diffOpts(), false, false)
	ts.Require().Error(err)
	ts.Require().False(changed)
}

// TestDiffLiveNotDeployedIsChange: a release that does not exist on the cluster
// would be installed, which is a change (exit 2), with no error.
func (ts *DiffSignalTestSuite) TestDiffLiveNotDeployedIsChange() {
	rel := ts.newRelease()
	rel.On("Get", 0).Return(&helmRelease.Release{}, release.ErrNotFound)

	p := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}

	changed, err := p.DiffLive(ts.ctx, diffOpts(), false, false)
	ts.Require().NoError(err)
	ts.Require().True(changed)
}

// failingKube returns a fake KubeClient whose reachability check fails, standing
// in for an unreachable cluster / RBAC-forbidden live lookup.
func failingKube(reason string) *kubeFake.FailingKubeClient {
	return &kubeFake.FailingKubeClient{
		PrintingKubeClient: kubeFake.PrintingKubeClient{Out: io.Discard, LogOutput: io.Discard},
		ConnectionError:    errors.New(reason),
	}
}

// TestGet3WayMergeUnreachableReturnsError: a genuine cluster failure during the
// 3-way merge is now reported as an error (not swallowed), and the untouched
// input manifest is returned so a lenient caller can still fall back.
func (ts *DiffSignalTestSuite) TestGet3WayMergeUnreachableReturnsError() {
	rel := ts.newRelease()
	rel.On("Cfg").Return(&action.Configuration{KubeClient: failingKube("dial tcp: connection refused")})

	merged, err := get3WayMergeManifests(rel, "some-manifest")
	ts.Require().Error(err)
	ts.Require().Equal("some-manifest", merged)
}

// TestDiffLiveThreeWayMergeStrictReturnsError: on the --detailed-exitcode path a
// real RBAC/network failure in the 3-way merge must surface as an error (exit 1),
// never be counted as a change (exit 2). This is the swallow-path codex flagged.
func (ts *DiffSignalTestSuite) TestDiffLiveThreeWayMergeStrictReturnsError() {
	active := &helmRelease.Release{Manifest: configMapManifest("value1")}

	rel := ts.newRelease()
	rel.On("Get", 0).Return(active, nil)
	rel.On("Cfg").Return(&action.Configuration{KubeClient: failingKube("forbidden: cannot get deployments")})

	p := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}

	changed, err := p.DiffLive(ts.ctx, diffOpts(), true /* threeWayMerge */, true /* strictErrors */)
	ts.Require().Error(err)
	ts.Require().False(changed)
}

// TestDiffLiveThreeWayMergeLenientSwallowsError: without --detailed-exitcode the
// historical lenient warn+fallback is preserved, so the same 3-way failure does
// not turn into an error (behavior unchanged for existing users).
func (ts *DiffSignalTestSuite) TestDiffLiveThreeWayMergeLenientSwallowsError() {
	ch := &chart.Chart{Metadata: &chart.Metadata{Name: "redis"}}
	active := &helmRelease.Release{Manifest: "", Chart: ch}

	rel := ts.newRelease()
	rel.On("Get", 0).Return(active, nil)
	rel.On("Cfg").Return(&action.Configuration{KubeClient: failingKube("forbidden")})
	rel.On("DryRun").Return()
	rel.On("Sync").Return(&helmRelease.Release{Chart: ch}, nil)

	p := New(ts.T().TempDir())
	p.body = &planBody{Releases: release.Configs{rel}}

	changed, err := p.DiffLive(ts.ctx, diffOpts(), true /* threeWayMerge */, false /* strictErrors */)
	ts.Require().NoError(err)
	ts.Require().False(changed)
}
