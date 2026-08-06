package tests

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

var ErrTestTimeout = errors.New("tests timeout exceeded")

// Tests must not be able to reach a real cluster.
//
// Without this the helm client picks up whatever ~/.kube/config points at,
// which on a developer machine is usually a real cluster and often a production
// one. The suite then reports "Kubernetes cluster unreachable", which reads as a
// broken test rather than an attempt to talk to someone's infrastructure — and
// with working credentials present it would not fail at all, it would run
// dry-runs against that cluster.
//
// This runs on import, before any test, because KUBECONFIG is process-wide and
// t.Setenv cannot be used from a parallel test. Set HELMTIDE_TEST_CLUSTER to a
// kubeconfig to opt into running against a real one.
func init() {
	if cluster, ok := os.LookupEnv(ClusterEnv); ok {
		// The variable names a kubeconfig, so point the suite at that file.
		// Treating it as a mere flag would hand the tests back whatever
		// KUBECONFIG happened to be set to -- the accident this file exists to
		// prevent -- while looking like an opt-in to a chosen cluster.
		if cluster == "" {
			panic("tests: " + ClusterEnv + " is empty: unset it, or set it to a kubeconfig")
		}

		// Absolute, because tests change directories. Give it an absolute path:
		// a relative one is resolved against the directory of whichever package
		// is being tested, which is rarely what you meant.
		kubeconfig, err := filepath.Abs(cluster)
		if err != nil {
			panic("tests: cannot resolve " + ClusterEnv + ": " + err.Error())
		}

		if _, err := os.Stat(kubeconfig); err != nil {
			panic("tests: " + ClusterEnv + " is not a readable kubeconfig, use an absolute path: " + err.Error())
		}

		if err := os.Setenv("KUBECONFIG", kubeconfig); err != nil {
			panic("tests: cannot set KUBECONFIG: " + err.Error())
		}

		return
	}

	// Resolved from this file rather than from Root, which is relative and only
	// correct for a package two levels down.
	_, self, _, ok := runtime.Caller(0)
	if !ok {
		panic("tests: cannot locate the helpers package")
	}

	kubeconfig := filepath.Join(filepath.Dir(self), "kubeconfig.yaml")

	if _, err := os.Stat(kubeconfig); err != nil {
		panic("tests: the dead-end kubeconfig is missing: " + err.Error())
	}

	if err := os.Setenv("KUBECONFIG", kubeconfig); err != nil {
		panic("tests: cannot isolate KUBECONFIG: " + err.Error())
	}
}

// ClusterEnv names the kubeconfig, by absolute path, to run cluster-dependent
// tests against. Setting it both selects the cluster and turns the isolation
// off; nothing else does either.
const ClusterEnv = "HELMTIDE_TEST_CLUSTER"

// RequireCluster skips a test unless a cluster was explicitly provided, so a
// suite that needs one is opted into rather than silently borrowing whichever
// cluster the developer happens to be logged into.
func RequireCluster(t *testing.T) {
	t.Helper()

	if _, ok := os.LookupEnv(ClusterEnv); !ok {
		t.Skipf("needs a cluster: set %s to a kubeconfig to run this", ClusterEnv)
	}
}

// RemoteChartsEnv opts a test into pulling charts from the public internet
// (bitnami, prometheus-community, jetstack, raw gists, ...). Those repositories
// are outside our control: bitnami moved its public catalog to an archive in
// 2025, gists get deleted, indexes rate-limit. A release must not go red because
// someone else's CDN blinked, so the network-dependent suites inherited from
// upstream are opt-in and nothing but this variable turns them on.
const RemoteChartsEnv = "HELMTIDE_TEST_REMOTE_CHARTS"

// RequireRemoteCharts skips a test unless pulling charts from the public
// internet was explicitly requested. Symmetric to RequireCluster: the hermetic
// suite (local charts + an in-cluster registry) always runs, while the tests
// that reach out to third-party chart repos only run when asked to.
func RequireRemoteCharts(t *testing.T) {
	t.Helper()

	if _, ok := os.LookupEnv(RemoteChartsEnv); !ok {
		t.Skipf("needs public chart repos: set %s=1 to run this", RemoteChartsEnv)
	}
}

// RequireLocalPrometheus skips a test whose monitors query Prometheus at
// localhost:9090 unless something is actually listening there. Fixture 20 pins
// that address, which only answers when the test host can reach the deployed
// Prometheus over localhost -- true when the suite runs on the cluster node (a
// local kind cluster, or with a port-forward), false in a split runner/cluster
// topology where the cluster lives on another host. Rather than fail on the
// topology, skip with a reason; the hermetic prometheus monitor coverage in
// pkg/action already exercises the real client against an in-process stub.
func RequireLocalPrometheus(t *testing.T) {
	t.Helper()

	conn, err := net.DialTimeout("tcp", "127.0.0.1:9090", 500*time.Millisecond)
	if err != nil {
		t.Skipf("needs a Prometheus reachable at localhost:9090 (fixture 20 pins it): %v", err)
	}
	_ = conn.Close()
}

func GetContext(t *testing.T) context.Context {
	t.Helper()

	ctx := context.Background()

	deadline, ok := t.Deadline()
	if ok {
		ctx, cancel := context.WithDeadlineCause(ctx, deadline, ErrTestTimeout)
		t.Cleanup(cancel)

		return ctx
	}

	return ctx
}
