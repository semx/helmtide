//go:build integration

package action

import (
	"fmt"
	"os"
	"testing"

	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
)

// TestRollbackIntegration installs a release, upgrades it, then rolls it back to
// the first revision and checks both the recorded revision and the live object:
// a rollback must move helm's revision forward (to a new record) while restoring
// the earlier content, not merely edit in place.
func TestRollbackIntegration(t *testing.T) {
	tests.RequireCluster(t)

	ctx := tests.GetContext(t)
	ns := namespace(t)
	chart := chartPath(t, "cm")
	dir := t.TempDir()

	valuesAlpha := writeFile(t, dir, "alpha.yaml", "value: alpha\n")
	valuesBravo := writeFile(t, dir, "bravo.yaml", "value: bravo\n")

	planFor := func(values string) string {
		return fmt.Sprintf(`project: rollback
releases:
  - name: cm
    namespace: %s
    create_namespace: true
    wait: true
    chart:
      name: %s
    values:
      - %s
`, ns, chart, values)
	}

	// Revision 1: alpha.
	b := planBuild(t, planFor(valuesAlpha))
	t.Cleanup(func() { down(ctx, t, b) })

	up(ctx, t, b)
	require.Equal(t, 1, tests.DeployedRevision(ctx, t, ns, "cm"))
	require.Equal(t, "alpha", tests.ConfigMap(ctx, t, ns, "cm").Data["value"])

	// Revision 2: bravo. Same plan file, new values.
	require.NoError(t, os.WriteFile(b.yml.file, []byte(planFor(valuesBravo)), 0o600))
	up(ctx, t, b)
	require.Equal(t, 2, tests.DeployedRevision(ctx, t, ns, "cm"))
	require.Equal(t, "bravo", tests.ConfigMap(ctx, t, ns, "cm").Data["value"])

	// Roll back to revision 1. Helm writes this as a fresh revision (3) whose
	// content matches revision 1.
	require.NoError(t, rollback(ctx, t, b, 1))
	require.Equal(t, 3, tests.DeployedRevision(ctx, t, ns, "cm"),
		"rollback must advance the revision, not reuse the target")
	require.Equal(t, "alpha", tests.ConfigMap(ctx, t, ns, "cm").Data["value"],
		"rollback must restore the earlier live state")
}
