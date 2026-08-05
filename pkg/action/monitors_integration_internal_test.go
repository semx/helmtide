//go:build integration

package action

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/semx/helmtide/pkg/kubedog"
	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
)

// monitorPlan wires a single named monitor onto a cm-chart release. The monitor
// block itself is passed in verbatim so each test supplies its own type.
func monitorPlan(t *testing.T, ns, chart, monitorBlock string) string {
	t.Helper()

	return fmt.Sprintf(`project: monitors
monitors:
%s
releases:
  - name: cm
    namespace: %s
    create_namespace: true
    wait: true
    chart:
      name: %s
    monitors:
      - name: probe
    values:
      - %s
`, monitorBlock, ns, chart, writeFile(t, t.TempDir(), "values.yaml", "value: monitored\n"))
}

// TestHTTPMonitorIntegration drives the http monitor end to end: the release is
// deployed to the cluster, then the monitor polls an in-process server. A green
// server makes up succeed and the server records the polls; a server that never
// returns the expected code makes up fail.
func TestHTTPMonitorIntegration(t *testing.T) {
	tests.RequireCluster(t)

	chart := chartPath(t, "cm")

	t.Run("healthy_passes", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		var hits atomic.Int64
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			hits.Add(1)
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)

		block := fmt.Sprintf(`  - name: probe
    type: http
    total_timeout: 30s
    iteration_timeout: 5s
    interval: 200ms
    success_threshold: 2
    failure_threshold: 3
    http:
      url: %s
      method: GET
      expected_codes:
        - 200
`, srv.URL)

		b := planBuild(t, monitorPlan(t, ns, chart, block))
		t.Cleanup(func() { down(ctx, t, b) })

		up(ctx, t, b)

		require.GreaterOrEqual(t, hits.Load(), int64(2),
			"the http monitor must actually have polled the endpoint")
	})

	t.Run("unhealthy_fails", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}))
		t.Cleanup(srv.Close)

		block := fmt.Sprintf(`  - name: probe
    type: http
    total_timeout: 30s
    iteration_timeout: 5s
    interval: 200ms
    success_threshold: 3
    failure_threshold: 2
    http:
      url: %s
      method: GET
      expected_codes:
        - 200
`, srv.URL)

		b := planBuild(t, monitorPlan(t, ns, chart, block))
		t.Cleanup(func() { down(ctx, t, b) })

		require.NoError(t, b.Run(ctx), "build plan")
		err := (&Up{build: b, dog: &kubedog.Config{}}).Run(ctx)
		require.Error(t, err, "a failing http monitor must fail up")
	})
}

// TestPrometheusMonitorIntegration drives the prometheus monitor against an
// in-process stub that speaks the Prometheus query API. This keeps the test
// hermetic -- no Prometheus deployment, no scrape targets -- while still
// exercising helmtide's real prometheus client, expression, and success mode.
func TestPrometheusMonitorIntegration(t *testing.T) {
	tests.RequireCluster(t)

	chart := chartPath(t, "cm")

	// vectorResult replies to /api/v1/query with a one-sample instant vector,
	// so success_mode if_vector treats it as up.
	vectorHandler := func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"success","data":{"resultType":"vector",`+
			`"result":[{"metric":{"__name__":"up"},"value":[1700000000,"1"]}]}}`)
	}

	t.Run("vector_passes", func(t *testing.T) {
		ctx := tests.GetContext(t)
		ns := namespace(t)

		srv := httptest.NewServer(http.HandlerFunc(vectorHandler))
		t.Cleanup(srv.Close)

		block := fmt.Sprintf(`  - name: probe
    type: prometheus
    total_timeout: 30s
    iteration_timeout: 5s
    interval: 200ms
    success_threshold: 2
    failure_threshold: 3
    prometheus:
      url: %s
      success_mode: if_vector
      expr: up == 1
`, srv.URL)

		b := planBuild(t, monitorPlan(t, ns, chart, block))
		t.Cleanup(func() { down(ctx, t, b) })

		up(ctx, t, b)
	})
}
