//go:build integration

package action

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
)

const (
	prMarker = "helmtide.io/postrendered"
	prBatch  = "helmtide.io/postrender-batch"
)

// buildPostRenderer compiles tests/postrender to a temp binary and returns its
// path. The binary stamps each manifest it is handed with its own batch size,
// which is what makes the difference between the strategies observable.
func buildPostRenderer(t *testing.T) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "postrender")

	out, err := exec.Command("go", "build", "-o", bin, "github.com/semx/helmtide/tests/postrender").CombinedOutput()
	require.NoError(t, err, "build postrender helper: %s", out)

	return bin
}

func postRenderPlan(ns, chart, strategy, binary string) string {
	return fmt.Sprintf(`project: postrender
releases:
  - name: pr
    namespace: %s
    create_namespace: true
    wait: true
    post_render_strategy: %s
    post_renderer:
      - %s
    chart:
      name: %s
`, ns, strategy, binary, chart)
}

// TestPostRenderStrategyIntegration checks that post_render_strategy decides
// whether Helm hooks pass through the post-renderer, and in how many batches, by
// reading the marker annotations the post-renderer leaves on the live objects.
//
//	strategy  ordinary manifest        hook resource
//	nohooks   post-rendered, batch=1   NOT post-rendered
//	combined  post-rendered, batch=2   post-rendered, batch=2 (one stream)
//	separate  post-rendered, batch=1   post-rendered, batch=1 (its own stream)
func TestPostRenderStrategyIntegration(t *testing.T) {
	tests.RequireCluster(t)

	chart := chartPath(t, "hooked")
	binary := buildPostRenderer(t)

	run := func(t *testing.T, strategy string) (main, hook map[string]string) {
		t.Helper()

		ctx := tests.GetContext(t)
		ns := namespace(t)

		b := planBuild(t, postRenderPlan(ns, chart, strategy, binary))
		t.Cleanup(func() { down(ctx, t, b) })

		up(ctx, t, b)

		return annotations(ctx, t, ns, "pr-main"), annotations(ctx, t, ns, "pr-hook")
	}

	t.Run("nohooks", func(t *testing.T) {
		main, hook := run(t, "nohooks")

		require.Equal(t, "true", main[prMarker], "manifest must be post-rendered")
		require.Equal(t, "1", main[prBatch])
		require.NotContains(t, hook, prMarker, "nohooks must not send the hook through the post-renderer")
	})

	t.Run("combined", func(t *testing.T) {
		main, hook := run(t, "combined")

		require.Equal(t, "true", main[prMarker])
		require.Equal(t, "2", main[prBatch], "combined feeds hook and manifest as one stream")
		require.Equal(t, "true", hook[prMarker], "combined must post-render the hook too")
		require.Equal(t, "2", hook[prBatch])
	})

	t.Run("separate", func(t *testing.T) {
		main, hook := run(t, "separate")

		require.Equal(t, "true", main[prMarker])
		require.Equal(t, "1", main[prBatch], "separate post-renders each stream on its own")
		require.Equal(t, "true", hook[prMarker], "separate must still post-render the hook")
		require.Equal(t, "1", hook[prBatch])
	})
}

// annotations returns the annotations of a ConfigMap, or an empty map with the
// marker absent if the object is missing (so a NotFound reads as "not stamped").
func annotations(ctx context.Context, t *testing.T, ns, name string) map[string]string {
	t.Helper()

	if !tests.ConfigMapExists(ctx, t, ns, name) {
		return map[string]string{}
	}

	return tests.ConfigMap(ctx, t, ns, name).GetAnnotations()
}
