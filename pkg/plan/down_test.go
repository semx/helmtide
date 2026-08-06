package plan_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/semx/helmtide/pkg/plan"
	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/suite"
	helmReleaseIface "helm.sh/helm/v4/pkg/release"
)

type DestroyTestSuite struct {
	suite.Suite

	ctx context.Context
}

func TestDestroyTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(DestroyTestSuite))
}

func (ts *DestroyTestSuite) SetupTest() {
	ts.ctx = tests.GetContext(ts.T())
}

func (ts *DestroyTestSuite) TestDestroy() {
	tmpDir := ts.T().TempDir()
	p := plan.New(filepath.Join(tmpDir, plan.Dir))

	mockedRelease := plan.NewMockReleaseConfig(ts.T())
	mockedRelease.On("Name").Return("redis")
	mockedRelease.On("Namespace").Return("defaultblabla")
	mockedRelease.On("KubeContext").Return("")
	mockedRelease.On("Uniq").Return()
	mockedRelease.On("Uninstall").Return(&helmReleaseIface.UninstallReleaseResponse{}, nil)
	mockedRelease.On("DependsOn").Return([]*release.DependsOnReference{})

	p.SetReleases(mockedRelease)

	err := p.Down(ts.ctx)
	ts.Require().NoError(err)

	mockedRelease.AssertExpectations(ts.T())
}

func (ts *DestroyTestSuite) TestDestroyFailedRelease() {
	tmpDir := ts.T().TempDir()
	p := plan.New(filepath.Join(tmpDir, plan.Dir))

	mockedRelease := plan.NewMockReleaseConfig(ts.T())
	mockedRelease.On("Name").Return("redis")
	mockedRelease.On("Namespace").Return("defaultblabla")
	mockedRelease.On("KubeContext").Return("")
	mockedRelease.On("Uniq").Return()
	e := errors.New(ts.T().Name())
	mockedRelease.On("Uninstall").Return(&helmReleaseIface.UninstallReleaseResponse{}, e)
	mockedRelease.On("DependsOn").Return([]*release.DependsOnReference{})

	p.SetReleases(mockedRelease)

	err := p.Down(ts.ctx)
	ts.Require().ErrorIs(err, e)

	mockedRelease.AssertExpectations(ts.T())
}

// TestDestroyFailedDependencyStrandsDependant reproduces the down deadlock:
// "app" depends on "redis", so on the reversed (uninstall) graph "app" is torn
// down first. When "app" fails to uninstall, "redis" is pruned from the graph
// and never emitted to a worker, so it never calls Done(). With the old
// accounting (Add(len(releases)) + context-insensitive Wait) the WaitGroup
// could never reach zero and Down hung forever. This test runs Down under a
// short timeout and asserts it RETURNS and surfaces the failure.
func (ts *DestroyTestSuite) TestDestroyFailedDependencyStrandsDependant() {
	tmpDir := ts.T().TempDir()
	p := plan.New(filepath.Join(tmpDir, plan.Dir))

	// redis is the dependency: it must be stranded (never uninstalled) once app
	// fails, so no Uninstall expectation is set — an unexpected call would panic.
	redis := plan.NewMockReleaseConfig(ts.T())
	redis.On("Name").Return("redis")
	redis.On("Namespace").Return("ns")
	redis.On("KubeContext").Return("")
	redis.On("Uniq").Return()
	redis.On("DependsOn").Return([]*release.DependsOnReference{})

	// app depends on redis and fails to uninstall.
	// Sentinel so the assertion can prove Down returned because of the real
	// uninstall failure, not because the watchdog/context deadline fired.
	errUninstall := errors.New("boom: app uninstall failed")

	app := plan.NewMockReleaseConfig(ts.T())
	app.On("Name").Return("app")
	app.On("Namespace").Return("ns")
	app.On("KubeContext").Return("")
	app.On("Uniq").Return()
	app.On("Uninstall").Return(&helmReleaseIface.UninstallReleaseResponse{}, errUninstall)
	app.On("DependsOn").Return([]*release.DependsOnReference{{Name: "redis@ns"}})

	p.SetReleases(redis, app)

	ctx, cancel := context.WithTimeout(ts.ctx, 5*time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() { done <- p.Down(ctx) }()

	select {
	case err := <-done:
		// Primary assertion: Down returned because of the real uninstall
		// failure, not a timeout. This fails if the error is the context
		// deadline (i.e. the WaitGroup never terminated).
		ts.Require().ErrorIs(err, errUninstall, "Down must surface the real uninstall failure")
	case <-time.After(10 * time.Second):
		// Watchdog hang-guard: on the buggy accounting Down never returns.
		ts.FailNow("Down hung: WaitGroup never terminated for stranded dependant")
	}
}

func (ts *DestroyTestSuite) TestDestroyNoReleases() {
	tmpDir := ts.T().TempDir()
	p := plan.New(filepath.Join(tmpDir, plan.Dir))
	p.NewBody()

	err := p.Down(ts.ctx)
	ts.Require().NoError(err)
}
