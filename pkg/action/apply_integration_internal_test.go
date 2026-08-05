//go:build integration

package action

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/semx/helmtide/pkg/kubedog"
	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
)

// helmManager is the field-manager name helm records for the objects it applies.
// Helm derives it from the running binary's base name when nothing overrides it,
// which for `go test` is the compiled test binary. Computing it here keeps the
// assertion exact without hardcoding a name that changes per package.
func helmManager() string {
	return filepath.Base(os.Args[0])
}

func applyPlan(ns, chart, mode string, forceConflicts bool, values string) string {
	return fmt.Sprintf(`project: apply
releases:
  - name: cm
    namespace: %s
    create_namespace: true
    wait: true
    server_side_apply: %q
    force_conflicts: %t
    chart:
      name: %s
    values:
      - %s
`, ns, mode, forceConflicts, chart, values)
}

// valuesWith writes a cm-chart values file setting value and returns its path.
func valuesWith(t *testing.T, value string) string {
	t.Helper()

	return writeFile(t, t.TempDir(), "values.yaml", fmt.Sprintf("value: %q\n", value))
}

// TestServerSideApplyIntegration checks that server_side_apply picks the write
// path helm uses, visible in the object's managedFields: "auto" applies
// server-side (operation Apply), "false" writes client-side (operation Update).
func TestServerSideApplyIntegration(t *testing.T) {
	tests.RequireCluster(t)

	chart := chartPath(t, "cm")

	t.Run("auto_is_server_side", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		b := planBuild(t, applyPlan(ns, chart, "auto", false, valuesWith(t, "helm")))
		t.Cleanup(func() { down(ctx, t, b) })

		up(ctx, t, b)

		manager, operation, ok := tests.FieldManagerFor(ctx, t, ns, "cm", helmManager())
		require.True(t, ok, "helm's field manager %q must own the object", helmManager())
		require.Equal(t, helmManager(), manager)
		require.Equal(t, "Apply", operation, "auto must apply server-side")
	})

	t.Run("false_is_client_side", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		b := planBuild(t, applyPlan(ns, chart, "false", false, valuesWith(t, "helm")))
		t.Cleanup(func() { down(ctx, t, b) })

		up(ctx, t, b)

		manager, operation, ok := tests.FieldManagerFor(ctx, t, ns, "cm", helmManager())
		require.True(t, ok, "helm's field manager %q must own the object", helmManager())
		require.Equal(t, helmManager(), manager)
		require.Equal(t, "Update", operation, "false must write client-side")
	})
}

// TestForceConflictsIntegration exercises force_conflicts under server-side
// apply. A foreign field manager steals ownership of data.value; a later helm
// upgrade that wants to change it then conflicts -- unless force_conflicts lets
// helm win and take the field back.
func TestForceConflictsIntegration(t *testing.T) {
	tests.RequireCluster(t)

	chart := chartPath(t, "cm")

	// steal has a foreign manager grab data.value with server-side apply, which
	// is what a later helm apply has to conflict with.
	steal := func(ctx context.Context, t *testing.T, ns string) {
		t.Helper()

		require.NoError(t, tests.ApplyConfigMap(ctx, t, ns, "cm", "intruder",
			map[string]string{"value": "intruder"}, true))
	}

	t.Run("conflict_without_force", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		// helm installs and owns the object.
		b := planBuild(t, applyPlan(ns, chart, "auto", false, valuesWith(t, "alpha")))
		t.Cleanup(func() { down(ctx, t, b) })
		up(ctx, t, b)

		steal(ctx, t, ns)

		// helm now wants a different value but does not force: this must fail.
		require.NoError(t, os.WriteFile(b.yml.file,
			[]byte(applyPlan(ns, chart, "auto", false, valuesWith(t, "gamma"))), 0o600))
		require.NoError(t, b.Run(ctx), "build plan")
		err := (&Up{build: b, dog: &kubedog.Config{}}).Run(ctx)
		require.Error(t, err, "server-side apply must conflict with the foreign manager")
	})

	t.Run("takeover_with_force", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		b := planBuild(t, applyPlan(ns, chart, "auto", false, valuesWith(t, "alpha")))
		t.Cleanup(func() { down(ctx, t, b) })
		up(ctx, t, b)

		steal(ctx, t, ns)

		// Same change, but force_conflicts lets helm win.
		require.NoError(t, os.WriteFile(b.yml.file,
			[]byte(applyPlan(ns, chart, "auto", true, valuesWith(t, "gamma"))), 0o600))
		up(ctx, t, b)

		require.Equal(t, "gamma", tests.ConfigMap(ctx, t, ns, "cm").Data["value"],
			"force_conflicts must let helm overwrite the stolen field")

		_, operation, ok := tests.FieldManagerFor(ctx, t, ns, "cm", helmManager())
		require.True(t, ok, "helm must have reclaimed ownership")
		require.Equal(t, "Apply", operation)
	})
}
