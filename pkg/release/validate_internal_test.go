package release

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateServerSideApply(t *testing.T) {
	for _, v := range []string{"", "true", "false", "auto"} {
		rel := &config{NameF: "x", NamespaceF: "default", ServerSideApply: v}
		require.NoError(t, rel.Validate(), "value %q must be accepted", v)
	}
	rel := &config{NameF: "x", NamespaceF: "default", ServerSideApply: "flase"}
	require.Error(t, rel.Validate(), "a typo must be rejected, not silently enable SSA")
}
