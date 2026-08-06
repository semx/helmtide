package registry

import (
	"sync"

	"github.com/semx/helmtide/pkg/helper"
	"helm.sh/helm/v4/pkg/registry"
)

// loginMu serializes helm registry logins. Registries are installed concurrently
// (see pkg/plan/up_registries.go), but Login mutates shared state on the
// package-global HelmRegistryClient (username/password/authorizer credential) and
// writes the shared on-disk credentials store. Without serialization concurrent
// logins race and can leak one registry's credentials into another or corrupt the
// credentials file. Single-registry behavior is unchanged (one lock/unlock).
var loginMu sync.Mutex

func (c *config) Install() error {
	// Allow public OCI registry #410.
	if c.Username == "" {
		c.Logger().Debugln("Public OCI chart. Skipping helm login.")

		return nil
	}

	loginMu.Lock()
	err := helper.HelmRegistryClient.Login(
		c.Host(),
		registry.LoginOptBasicAuth(c.Username, c.Password),
		registry.LoginOptInsecure(c.Insecure),
	)
	loginMu.Unlock()
	if err != nil {
		return NewLoginError(err)
	}

	return nil
}
