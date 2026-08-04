package plan

import (
	"context"
	"sync"
	"testing"

	"github.com/semx/helmtide/pkg/hooks"
	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/release/uniqname"
	"github.com/semx/helmtide/tests"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/suite"
	helmRelease "helm.sh/helm/v3/pkg/release"
)

// journal records what happened during a build, in order.
type journal struct {
	mu     sync.Mutex
	events []string
}

func (j *journal) record(event string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	j.events = append(j.events, event)
}

func (j *journal) count(event string) int {
	j.mu.Lock()
	defer j.mu.Unlock()

	n := 0

	for _, e := range j.events {
		if e == event {
			n++
		}
	}

	return n
}

func (j *journal) all() []string {
	j.mu.Lock()
	defer j.mu.Unlock()

	return append([]string(nil), j.events...)
}

// journalHook is a hooks.Hook that records every run.
type journalHook struct {
	journal *journal
	name    string
}

func (h *journalHook) Run(context.Context) error {
	h.journal.record(h.name)

	return nil
}

func (h *journalHook) Log() *log.Entry {
	return log.WithField("hook", h.name)
}

type BuildLifecycleTestSuite struct {
	suite.Suite

	ctx context.Context
}

func TestBuildLifecycleTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(BuildLifecycleTestSuite))
}

func (ts *BuildLifecycleTestSuite) SetupTest() {
	ts.ctx = tests.GetContext(ts.T())
}

// A release's pre_build hook must run exactly once per build, and it must run
// before that release's chart is built — a hook is allowed to be the thing that
// produces the chart.
//
// Both halves used to be wrong: buildReleases ran pre_build for every release
// and then SyncDryRun ran it a second time from buildReleaseManifest, with the
// charts downloaded in between the two.
func (ts *BuildLifecycleTestSuite) TestPreBuildRunsOncePerRelease() {
	j := &journal{}

	lifecycle := hooks.Lifecycle{
		PreBuild:  hooks.Hooks{&journalHook{name: "pre_build", journal: j}},
		PostBuild: hooks.Hooks{&journalHook{name: "post_build", journal: j}},
	}

	uniq, err := uniqname.NewFromString("redis@testns")
	ts.Require().NoError(err)

	rel := NewMockReleaseConfig(ts.T())
	rel.On("Tags").Return([]string{})
	rel.On("Uniq").Return(uniq)
	rel.On("SetDependsOn", []*release.DependsOnReference{}).Return()
	rel.On("Lifecycle").Return(lifecycle)
	rel.On("DownloadChart").Return(nil).Run(func(mock.Arguments) { j.record("chart") })
	rel.On("ChartDepsUpd").Return(nil)
	rel.On("DryRun").Return()
	rel.On("Sync").Return(&helmRelease.Release{}, nil)
	rel.On("HooksDisabled").Return(false)

	p := New(ts.T().TempDir())
	p.SetReleases(rel)

	releases, err := p.buildReleases(BuildOptions{})
	ts.Require().NoError(err)
	p.body.Releases = releases

	ts.Require().NoError(p.buildManifest(ts.ctx))

	ts.Equal(1, j.count("pre_build"), "pre_build must run exactly once per release per build")
	ts.Equal(1, j.count("post_build"), "post_build must run exactly once per release per build")
	ts.Equal([]string{"pre_build", "chart", "post_build"}, j.all(),
		"the chart must be built after pre_build, so a hook can produce it")
}
