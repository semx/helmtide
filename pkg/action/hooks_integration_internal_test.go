//go:build integration

package action

import (
	"fmt"
	"os"
	"testing"

	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
)

// hookedPlan renders a plan for the hooked chart: one ordinary ConfigMap
// (<name>-main) and one post-install/post-upgrade hook ConfigMap (<name>-hook).
func hookedPlan(t *testing.T, ns, chart string, disableHooks bool, rev string) string {
	t.Helper()

	return fmt.Sprintf(`project: hooks
releases:
  - name: app
    namespace: %s
    create_namespace: true
    wait: true
    disable_hooks: %t
    chart:
      name: %s
    values:
      - %s
`, ns, disableHooks, chart, writeInlineValues(t, rev))
}

// writeInlineValues writes a one-key values file and returns its path.
func writeInlineValues(t *testing.T, rev string) string {
	t.Helper()

	return writeFile(t, t.TempDir(), "values.yaml", fmt.Sprintf("rev: %q\n", rev))
}

// TestHooksFireIntegration proves Helm chart hooks run on install and again on
// upgrade, and that disable_hooks suppresses them while leaving ordinary
// resources untouched.
func TestHooksFireIntegration(t *testing.T) {
	tests.RequireCluster(t)

	chart := chartPath(t, "hooked")

	t.Run("enabled", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		b := planBuild(t, hookedPlan(t, ns, chart, false, "1"))
		t.Cleanup(func() { down(ctx, t, b) })

		up(ctx, t, b)

		// The ordinary resource is there, and so is the hook resource, so the
		// post-install hook fired.
		require.True(t, tests.ConfigMapExists(ctx, t, ns, "app-main"), "ordinary resource must exist")
		require.True(t, tests.ConfigMapExists(ctx, t, ns, "app-hook"), "post-install hook must have fired")

		uidInstall := tests.ConfigMap(ctx, t, ns, "app-hook").GetUID()

		// Force a real upgrade. before-hook-creation deletes and recreates the
		// hook resource, so a changed UID proves the post-upgrade hook fired.
		require.NoError(t, os.WriteFile(b.yml.file, []byte(hookedPlan(t, ns, chart, false, "2")), 0o600))
		up(ctx, t, b)

		require.True(t, tests.ConfigMapExists(ctx, t, ns, "app-hook"), "post-upgrade hook must have fired")
		require.NotEqual(t, uidInstall, tests.ConfigMap(ctx, t, ns, "app-hook").GetUID(),
			"post-upgrade hook must have recreated the resource")
	})

	t.Run("disabled", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		b := planBuild(t, hookedPlan(t, ns, chart, true, "1"))
		t.Cleanup(func() { down(ctx, t, b) })

		up(ctx, t, b)

		require.True(t, tests.ConfigMapExists(ctx, t, ns, "app-main"), "ordinary resource must still be applied")
		require.False(t, tests.ConfigMapExists(ctx, t, ns, "app-hook"), "disable_hooks must suppress the hook")
	})
}
