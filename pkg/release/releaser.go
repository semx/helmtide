package release

import (
	releaseiface "helm.sh/helm/v4/pkg/release"
	release "helm.sh/helm/v4/pkg/release/v1"
)

// asRelease unwraps the release interface helm v4 actions return.
//
// helm v3 actions returned *release.Release. helm v4 versioned the release schema and returns the
// release.Releaser interface (an `any`) instead. Every storage driver in helm 4 still decodes into
// the v1 struct, so unwrap it here rather than spreading the interface through helmtide, and fail
// loudly if helm ever starts handing out something else.
func asRelease(r releaseiface.Releaser) (*release.Release, error) {
	switch v := r.(type) {
	case nil:
		return nil, nil
	case *release.Release:
		return v, nil
	case release.Release:
		return &v, nil
	default:
		return nil, NewUnexpectedReleaseTypeError(r)
	}
}

// unwrapRelease adapts the (Releaser, error) pair a helm v4 action returns. helm hands back a
// partially built release alongside the error when a sync fails, and callers here forward both, so
// the action's own error always wins over a conversion failure.
func unwrapRelease(r releaseiface.Releaser, err error) (*release.Release, error) {
	rel, convErr := asRelease(r)
	if err != nil {
		return rel, err
	}

	return rel, convErr
}
