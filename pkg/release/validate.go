package release

import "regexp"

var nsRegexp = regexp.MustCompile("[a-z0-9]([-a-z0-9]*[a-z0-9])?")

func (rel *config) Validate() error {
	if rel.Name() == "" {
		return ErrNameEmpty
	}

	if rel.Namespace() == "" {
		rel.Logger().Warnf("namespace is empty. I will use the namespace of your k8s context.")
	}

	if !validateNS(rel.Namespace()) {
		return NewInvalidNamespaceError(rel.Namespace())
	}

	if err := rel.Uniq().Validate(); err != nil {
		return err
	}

	// server_side_apply is a three-way string, and install treats anything but
	// exact "false" as on. So "flase" silently enables SSA on install and then
	// fails on upgrade -- reject an unknown value up front instead.
	switch rel.ServerSideApply {
	case "", "true", "false", "auto":
	default:
		return NewInvalidServerSideApplyError(rel.ServerSideApply)
	}

	return nil
}

func validateNS(ns string) bool {
	return nsRegexp.MatchString(ns)
}
