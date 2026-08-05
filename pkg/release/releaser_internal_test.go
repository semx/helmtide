package release

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	release "helm.sh/helm/v4/pkg/release/v1"
)

// helm hands back an interface, and callers here dereference the result the moment the error is
// nil -- pkg/plan.List reads r.Info.Status straight away. So "no release" has to arrive as an
// error, never as a nil value with a nil error.
func TestAsReleaseNeverReturnsNilWithoutError(t *testing.T) {
	t.Parallel()

	rel, err := asRelease(nil)
	assert.Nil(t, rel)
	require.ErrorIs(t, err, ErrNilRelease, "a nil release must be an error, or callers panic on it")

	want := &release.Release{Name: "x"}
	got, err := asRelease(want)
	require.NoError(t, err)
	assert.Same(t, want, got)

	byValue := release.Release{Name: "y"}
	got, err = asRelease(byValue)
	require.NoError(t, err)
	assert.Equal(t, "y", got.Name)

	_, err = asRelease("not a release")
	require.Error(t, err)
	var unexpected *UnexpectedReleaseTypeError
	assert.True(t, errors.As(err, &unexpected), "an unknown type must say so, not look like a nil")
}

// A typed nil pointer must be treated as "no release", not returned as a nil
// value with a nil error, or Plan.List dereferences it.
func TestAsReleaseRejectsTypedNil(t *testing.T) {
	t.Parallel()

	var typed *release.Release
	r, err := asRelease(typed)
	assert.Nil(t, r)
	require.ErrorIs(t, err, ErrNilRelease)
}

// helm hands back a partially built release alongside its error when a sync fails. unwrapRelease
// must forward the action's error verbatim and never let a conversion error (e.g. ErrNilRelease
// from a nil release) mask it, or callers report the wrong cause of failure.
func TestUnwrapReleaseActionErrorWins(t *testing.T) {
	t.Parallel()

	actionErr := errors.New("upgrade failed")

	// Partial release + action error: forward both, action error wins.
	partial := &release.Release{Name: "x"}
	rel, err := unwrapRelease(partial, actionErr)
	assert.Same(t, partial, rel)
	require.ErrorIs(t, err, actionErr)

	// No release + action error: the action error wins over ErrNilRelease.
	rel, err = unwrapRelease(nil, actionErr)
	assert.Nil(t, rel)
	require.ErrorIs(t, err, actionErr)
	require.NotErrorIs(t, err, ErrNilRelease, "conversion error must not mask the action error")

	// No release, no action error: the conversion error surfaces so callers don't deref nil.
	rel, err = unwrapRelease(nil, nil)
	assert.Nil(t, rel)
	require.ErrorIs(t, err, ErrNilRelease)
}
