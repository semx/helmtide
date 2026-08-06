package registry_test

import (
	"fmt"
	"net"
	"sync"
	"testing"

	"github.com/semx/helmtide/pkg/registry"
	"github.com/stretchr/testify/require"
)

// closedLoopbackHost binds a loopback port, then closes it so that connections to
// the returned host are refused immediately. This keeps the concurrent login test
// hermetic and fast: no real registry is required and Ping fails without hanging.
func closedLoopbackHost(t *testing.T) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)

	addr := l.Addr().String()
	require.NoError(t, l.Close())

	return addr
}

// TestConcurrentLoginNoRace logs in to many registries concurrently, exactly like
// pkg/plan.syncRegistries does. Each login mutates shared state on the package-global
// helm registry client (username/password/authorizer credential) and the shared
// credentials store. Before the fix these unsynchronized mutations race; with the
// login serialized this test is clean under `go test -race`.
//
// Every login is pointed at a closed loopback port, so Ping fails fast and each
// Install returns a *LoginError -- no real registry is needed.
func TestConcurrentLoginNoRace(t *testing.T) {
	t.Parallel()

	const n = 32

	host := closedLoopbackHost(t)

	var wg sync.WaitGroup
	wg.Add(n)

	errs := make([]error, n)

	for i := range n {
		go func(i int) {
			defer wg.Done()

			reg := registry.NewConfig()
			reg.HostF = host
			// Distinct per-registry credentials: if logins interleave, the race
			// detector observes concurrent writes to the shared client's fields.
			reg.Username = fmt.Sprintf("user-%d", i)
			reg.Password = fmt.Sprintf("pass-%d", i)

			errs[i] = reg.Install()
		}(i)
	}

	wg.Wait()

	// Sanity: every login attempt reached the shared mutable state (was not skipped
	// as a public registry) and failed at Ping against the closed port.
	for i, err := range errs {
		var e *registry.LoginError
		require.ErrorAsf(t, err, &e, "login %d should fail with LoginError", i)
	}
}
