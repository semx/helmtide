package tests_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Importing the test helpers must make it impossible to reach a real cluster by
// accident. If this ever fails, a suite somewhere is one working credential away
// from running dry-runs against whatever the developer is logged into.
func TestKubeconfigIsIsolated(t *testing.T) {
	t.Parallel()

	if _, ok := os.LookupEnv(tests.ClusterEnv); ok {
		t.Skipf("%s is set, so pointing at a real cluster is the intent here", tests.ClusterEnv)
	}

	kubeconfig, ok := os.LookupEnv("KUBECONFIG")
	require.True(t, ok, "KUBECONFIG must be set by the helpers, not inherited")

	assert.True(t, filepath.IsAbs(kubeconfig), "must be absolute, tests change directories")

	body, err := os.ReadFile(kubeconfig)
	require.NoError(t, err)

	assert.Contains(t, string(body), "localhost",
		"the test kubeconfig must point at a dead end")
	assert.NotContains(t, strings.ToLower(string(body)), "eks.amazonaws.com",
		"a real cluster leaked into the test kubeconfig")
}
