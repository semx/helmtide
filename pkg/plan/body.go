package plan

import "os"

// DefaultBody returns the config file to use when none was given.
//
// helmtide reads helmtide.yml, but it was forked from helmwave and an existing
// repository has a helmwave.yml instead. Rather than making everyone rename a
// file to try this out, the old name is accepted when the new one is absent.
// A directory holding both is answered with the new name, so a migration can be
// done by adding helmtide.yml and deleting helmwave.yml afterwards.
func DefaultBody() string {
	return firstExisting(Body, LegacyBody)
}

// DefaultTpl does the same for the template of the main config.
func DefaultTpl() string {
	return firstExisting(Tpl, LegacyTpl)
}

// firstExisting returns the first name that is on disk, or ours when neither is.
func firstExisting(ours, legacy string) string {
	if _, err := os.Stat(ours); err == nil {
		return ours
	}

	if _, err := os.Stat(legacy); err == nil {
		return legacy
	}

	return ours
}
