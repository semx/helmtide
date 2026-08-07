package hooks

import (
	"context"
	"fmt"
	"os"

	"github.com/semx/helmtide/pkg/helper"
)

const (
	EnvReleaseUniqname = "HELMTIDE_LIFECYCLE_RELEASE_UNIQNAME"
	EnvLifecycleType   = "HELMTIDE_LIFECYCLE_TYPE"

	// LegacyEnvReleaseUniqname and LegacyEnvLifecycleType are the names
	// inherited from helmwave, which helmtide was forked from. They are still
	// exported alongside the HELMTIDE_ names so existing hook scripts that read
	// HELMWAVE_LIFECYCLE_* keep working unchanged.
	LegacyEnvReleaseUniqname = "HELMWAVE_LIFECYCLE_RELEASE_UNIQNAME"
	LegacyEnvLifecycleType   = "HELMWAVE_LIFECYCLE_TYPE"
)

func (h *hook) getCommandEnviron(ctx context.Context) []string {
	env := os.Environ()

	if uniq, exists := helper.ContextGetReleaseUniq(ctx); exists {
		env = addToEnviron(env, EnvReleaseUniqname, uniq.String())
		env = addToEnviron(env, LegacyEnvReleaseUniqname, uniq.String())
	}

	if typ, exists := helper.ContextGetLifecycleType(ctx); exists {
		env = addToEnviron(env, EnvLifecycleType, typ)
		env = addToEnviron(env, LegacyEnvLifecycleType, typ)
	}

	return env
}

func addToEnviron(env []string, key, value string) []string {
	return append(env, fmt.Sprintf("%s=%s", key, value))
}
