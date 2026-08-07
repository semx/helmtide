package plan_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/semx/helmtide/pkg/plan"
	"github.com/stretchr/testify/require"
)

// writeTempPlan writes body to a temp helmtide.yml and returns its path.
func writeTempPlan(t *testing.T, body string) string {
	t.Helper()

	dir := t.TempDir()
	f := filepath.Join(dir, "helmtide.yml")
	require.NoError(t, os.WriteFile(f, []byte(body), 0o600))

	return f
}

// TestNewBodyStrictUnknownField makes sure a misspelled/unknown top-level key
// is rejected with an actionable error naming the field, while a valid config
// still parses. This guards the KnownFields decoding enabled in NewBody.
func TestNewBodyStrictUnknownField(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("valid config parses", func(t *testing.T) {
		t.Parallel()

		f := writeTempPlan(t, "project: myproject\nversion: 0.1.0\n")

		body, err := plan.NewBody(ctx, f, false)
		require.NoError(t, err)
		require.NotNil(t, body)
	})

	t.Run("unknown top-level field errors and names it", func(t *testing.T) {
		t.Parallel()

		f := writeTempPlan(t, "project: myproject\nunknownfield: x\n")

		_, err := plan.NewBody(ctx, f, false)
		require.Error(t, err)
		require.ErrorContains(t, err, "unknownfield")
	})

	t.Run("misspelled real field errors", func(t *testing.T) {
		t.Parallel()

		// `releasesss` instead of `releases`.
		f := writeTempPlan(t, "project: myproject\nreleasesss: []\n")

		_, err := plan.NewBody(ctx, f, false)
		require.Error(t, err)
		require.ErrorContains(t, err, "releasesss")
	})

	t.Run("dot-prefixed anchor holder is ignored", func(t *testing.T) {
		t.Parallel()

		// helmwave anchor idiom: reusable block under a `.`-prefixed key,
		// merged into a release. It must NOT be treated as an unknown field.
		body := `project: myproject
.default: &default
  namespace: test
  wait: true
releases:
  - name: nginx
    chart:
      name: bitnami/nginx
    <<: *default
`
		f := writeTempPlan(t, body)

		_, err := plan.NewBody(ctx, f, false)
		require.NoError(t, err)
	})
}
