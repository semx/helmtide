package release

import (
	"testing"

	"github.com/stretchr/testify/suite"
	"helm.sh/helm/v3/pkg/action"
)

type HelmActionsInternalTestSuite struct {
	suite.Suite
}

func TestHelmActionsInternalTestSuite(t *testing.T) {
	t.Parallel()
	suite.Run(t, new(HelmActionsInternalTestSuite))
}

// isDryRun and interactWithRemote mirror helm's own predicates in
// action.Install (helm.sh/helm/v3 pkg/action/install.go). They are copied here
// because they are unexported, and because the options newInstall sets only
// matter through them: when interactWithRemote is true helm renders the chart
// through a live REST client, so it needs a reachable cluster and `lookup`
// queries it.
func isDryRun(c *action.Install) bool {
	return c.DryRun || c.DryRunOption == "client" || c.DryRunOption == "server" || c.DryRunOption == "true"
}

func interactWithRemote(c *action.Install) bool {
	return !isDryRun(c) || c.DryRunOption == "server" || c.DryRunOption == "none" || c.DryRunOption == "false"
}

// A real install must not be turned into a dry run by a stray DryRunOption.
func (ts *HelmActionsInternalTestSuite) TestNewInstallWithoutDryRun() {
	rel := NewConfig()

	client := rel.newInstall()

	ts.False(client.DryRun)
	ts.Empty(client.DryRunOption, "setting DryRunOption alone puts helm into dry-run mode")
	ts.False(isDryRun(client))
	ts.False(client.ClientOnly)
}

func (ts *HelmActionsInternalTestSuite) TestNewInstallDryRun() {
	rel := NewConfig()
	rel.DryRun(true)

	client := rel.newInstall()

	ts.True(client.Replace)
	ts.Equal("server", client.DryRunOption)
	ts.False(client.ClientOnly)
	ts.True(interactWithRemote(client), "a normal build renders against the cluster so `lookup` works")
}

// offline_kube_version is the way to build without a cluster, so the render
// must not reach for one.
func (ts *HelmActionsInternalTestSuite) TestNewInstallDryRunOfflineKubeVersion() {
	rel := NewConfig()
	rel.DryRun(true)
	rel.OfflineKubeVersionF = "1.29.0"

	client := rel.newInstall()

	ts.True(client.ClientOnly)
	ts.Require().NotNil(client.KubeVersion)
	ts.Equal("v1.29.0", client.KubeVersion.Version)

	ts.Equal("client", client.DryRunOption)
	ts.False(interactWithRemote(client), "offline build must not render against a cluster")
}
