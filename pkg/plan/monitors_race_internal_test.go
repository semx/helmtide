package plan

import (
	"context"
	"errors"
	"maps"
	"slices"
	"sync"
	"testing"

	"github.com/semx/helmtide/pkg/monitor"
	"github.com/semx/helmtide/pkg/parallel"
	"github.com/semx/helmtide/pkg/release"
	log "github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

// failingMonitor is a minimal monitor.Config whose Run always fails
// immediately. It is used to drive the concurrent failure path of
// monitorsWorker without any external dependency.
type failingMonitor struct {
	name string
}

func (m *failingMonitor) Name() string                { return m.name }
func (m *failingMonitor) Validate() error             { return nil }
func (m *failingMonitor) Logger() *log.Entry          { return log.WithField("monitor", m.name) }
func (m *failingMonitor) Run(_ context.Context) error { return errors.New("boom: " + m.name) }

// satisfiedLock returns a *parallel.WaitGroup whose counter is already zero, so
// WaitWithContext returns immediately and lets the worker proceed to Run.
func satisfiedLock() *parallel.WaitGroup {
	return parallel.NewWaitGroup()
}

// TestMonitorsWorkerConcurrentFailuresDeterministic exercises two monitor
// workers failing concurrently and writing into the shared monitorsFails map.
//
//   - Under the pre-fix code (no mutex around fails[mon] = err) this races on
//     the shared map and `go test -race` reports a data race / the runtime
//     panics with "concurrent map writes".
//   - After the fix the writes are serialized by the shared mutex, so both
//     failures are recorded and the derived remediation action is stable across
//     runs regardless of map iteration order.
func TestMonitorsWorkerConcurrentFailuresDeterministic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	// m1 requests the destructive Uninstall, m2 the safe Rollback. The safer
	// action (Rollback) must always win, no matter which monitor is iterated
	// first out of the randomized fails map.
	refs := []release.MonitorReference{
		{Name: "m1", Action: release.MonitorActionUninstall},
		{Name: "m2", Action: release.MonitorActionRollback},
	}

	var chosen release.MonitorFailedAction

	const iterations = 200

	for iter := range iterations {
		p := &Plan{body: &planBody{}}

		m1 := &failingMonitor{name: "m1"}
		m2 := &failingMonitor{name: "m2"}

		lockMap := map[string]*parallel.WaitGroup{
			"m1": satisfiedLock(),
			"m2": satisfiedLock(),
		}

		fails := make(map[monitor.Config]error)
		mu := &sync.Mutex{}

		wg := parallel.NewWaitGroup()
		wg.Add(2)

		go p.monitorsWorker(ctx, wg, m1, mu, fails, lockMap)
		go p.monitorsWorker(ctx, wg, m2, mu, fails, lockMap)

		// Collects errors and blocks until both workers are done.
		require.Error(t, wg.WaitWithContext(ctx))

		// (a) No lost update / no panic: both concurrent failures recorded.
		require.Len(t, fails, 2)

		// (b) Deterministic remediation: iterate the randomized-order fails map
		// into a slice and assert the selected action never changes.
		failed := slices.Collect(maps.Keys(fails))
		action := release.SelectMonitorFailedAction(refs, failed...)

		require.Equal(t, release.MonitorActionRollback, action)

		if iter == 0 {
			chosen = action
		}
		require.Equal(t, chosen, action)
	}
}
