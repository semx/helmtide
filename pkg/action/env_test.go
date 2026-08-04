package action_test

import (
	"testing"

	"github.com/semx/helmtide/pkg/action"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Every flag is offered under both prefixes so an existing helmwave setup keeps
// working; urfave/cli reads the first name that is set, so ours takes priority.
func TestEnvVarsOffersBothPrefixes(t *testing.T) {
	got := action.EnvVars("PARALLEL_LIMIT")
	require.Len(t, got, 2)
	assert.Equal(t, "HELMTIDE_PARALLEL_LIMIT", got[0], "our own prefix must come first")
	assert.Equal(t, "HELMWAVE_PARALLEL_LIMIT", got[1], "the legacy prefix must still be accepted")

	multiple := action.EnvVars("YAML", "YML")
	assert.Equal(t,
		[]string{"HELMTIDE_YAML", "HELMTIDE_YML", "HELMWAVE_YAML", "HELMWAVE_YML"},
		multiple,
		"all of our names come before any legacy one, so a stale HELMWAVE_ value never wins",
	)

	assert.Empty(t, action.EnvVars())
}
