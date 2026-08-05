//go:build integration

package action

import (
	"fmt"
	"os"
	"testing"

	"github.com/semx/helmtide/pkg/helper"
	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/chart/v2/loader"
	chartutil "helm.sh/helm/v4/pkg/chart/v2/util"
)

// ociPlainEnv names a plain-HTTP OCI registry (host:port, no scheme) the test
// may push to and pull from. The CI job runs a registry:2 service and sets it;
// locally, point it at any plain-HTTP registry.
const ociPlainEnv = "HELMTIDE_TEST_OCI_PLAIN"

// TestOCIPlainHTTPIntegration is the regression guard for the fix that made a
// chart's plain_http flag reach the registry client. It packages the cm chart,
// pushes it to a plain-HTTP registry through helmtide's own registry client
// (the push side of the same NewRegistryClient code path), then runs `up`
// against an oci:// reference with plain_http: true (the pull side). Before the
// fix the pull tried HTTPS against the plain-HTTP registry and failed.
func TestOCIPlainHTTPIntegration(t *testing.T) {
	tests.RequireCluster(t)

	host := os.Getenv(ociPlainEnv)
	if host == "" {
		t.Skipf("needs a plain-HTTP OCI registry: set %s to host:port (CI runs registry:2)", ociPlainEnv)
	}

	ctx := tests.GetContext(t)
	ns := namespace(t)

	// Package and push the chart with plain HTTP enabled.
	ch, err := loader.LoadDir(chartPath(t, "cm"))
	require.NoError(t, err, "load cm chart")

	tgz, err := chartutil.Save(ch, t.TempDir())
	require.NoError(t, err, "package cm chart")

	data, err := os.ReadFile(tgz)
	require.NoError(t, err)

	rc, err := helper.NewRegistryClient(true, false)
	require.NoError(t, err, "plain-http registry client")

	ref := fmt.Sprintf("%s/helmtide/cm:0.1.0", host)
	_, err = rc.Push(data, ref)
	require.NoError(t, err, "push chart to plain-HTTP registry")

	// Pull and install it through helmtide with plain_http on the chart.
	plan := fmt.Sprintf(`project: oci
registries:
  - host: %[2]s
releases:
  - name: cm
    namespace: %[1]s
    create_namespace: true
    wait: true
    chart:
      name: oci://%[2]s/helmtide/cm
      version: 0.1.0
      plain_http: true
`, ns, host)

	b := planBuild(t, plan)
	t.Cleanup(func() { down(ctx, t, b) })

	up(ctx, t, b)

	require.True(t, tests.ConfigMapExists(ctx, t, ns, "cm"),
		"chart from the plain-HTTP OCI registry must have been installed")
}

// TestOCIAuthenticatedIntegration is a placeholder for the authenticated-OCI
// path. Standing up a registry with htpasswd auth and threading credentials is
// impractical in this harness, so it is skipped with a reason rather than
// silently omitted. The plain-HTTP test already covers the registry-client
// wiring the fix touched; auth would additionally cover login/credential flow.
func TestOCIAuthenticatedIntegration(t *testing.T) {
	tests.RequireCluster(t)

	t.Skip("needs an auth-enabled OCI registry (htpasswd + credentials); not provisioned by this harness")
}
