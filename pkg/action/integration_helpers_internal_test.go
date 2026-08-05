//go:build integration

package action

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/semx/helmtide/pkg/kubedog"
	"github.com/semx/helmtide/pkg/template"
	"github.com/semx/helmtide/tests"
	"github.com/stretchr/testify/require"
	"github.com/urfave/cli/v2"
)

// The tests in this package that end in _integration_test.go drive the real
// `helmtide up`/`down`/`rollback` code paths against a live cluster and then
// read the result back off that cluster. They compile only under -tags=integration
// and every one of them calls tests.RequireCluster first, so a plain
// `go test ./...` neither builds nor runs them, and `go test -tags=integration`
// without HELMTIDE_TEST_CLUSTER skips them with a reason instead of talking to
// whatever kubeconfig happens to be around.

// chartPath returns the absolute path of one of the local test charts under
// tests/charts, so a generated plan can name it regardless of the working
// directory the test runs in.
func chartPath(t *testing.T, name string) string {
	t.Helper()

	return filepath.Join(tests.Dir(t), "charts", name)
}

// namespace derives an RFC1123 namespace from the test name and registers its
// deletion, so each test is isolated and the run leaves the cluster clean.
func namespace(t *testing.T) string {
	t.Helper()

	ns := strings.ToLower(t.Name())
	ns = strings.NewReplacer("/", "-", "_", "-", ".", "-").Replace(ns)
	ns = strings.Trim(ns, "-")

	if len(ns) > 63 {
		ns = ns[len(ns)-63:]
		ns = strings.Trim(ns, "-")
	}

	t.Cleanup(func() { tests.DeleteNamespace(t, ns) })

	return ns
}

// writeFile writes content into a fresh file under dir and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()

	path := filepath.Join(dir, name)

	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	return path
}

// planBuild writes a helmtide.yml holding plan and returns a *Build wired to it,
// with its own plandir. autoYml is off: plan is already final YAML, not a template.
func planBuild(t *testing.T, plan string) *Build {
	t.Helper()

	dir := t.TempDir()
	ymlPath := writeFile(t, dir, "helmtide.yml", plan)

	return &Build{
		plandir: filepath.Join(dir, "plan"),
		tags:    cli.StringSlice{},
		autoYml: false,
		yml: &Yml{
			file:      ymlPath,
			templater: template.TemplaterSprig,
		},
	}
}

// up builds the plan and applies it, failing the test on any error.
func up(ctx context.Context, t *testing.T, b *Build) {
	t.Helper()

	require.NoError(t, b.Run(ctx), "build plan")
	require.NoError(t, (&Up{build: b, dog: &kubedog.Config{}}).Run(ctx), "up")
}

// down uninstalls the release behind an already-built plan. Best effort: used
// in cleanup, so it logs rather than fails.
func down(ctx context.Context, t *testing.T, b *Build) {
	t.Helper()

	if err := (&Down{build: b}).Run(ctx); err != nil {
		t.Logf("cleanup: down: %s", err)
	}
}

// rollback rolls every release in the built plan back to revision.
func rollback(ctx context.Context, t *testing.T, b *Build, revision int) error {
	t.Helper()

	return (&Rollback{build: b, dog: &kubedog.Config{}, revision: revision}).Run(ctx)
}
