package release

import (
	"context"

	"github.com/invopop/jsonschema"
	"github.com/semx/helmtide/pkg/monitor"
)

// MonitorFailedAction is a type for enumerating actions for handling failed monitors.
type MonitorFailedAction string

const (
	MonitorActionNone      MonitorFailedAction = ""
	MonitorActionRollback  MonitorFailedAction = "rollback"
	MonitorActionUninstall MonitorFailedAction = "uninstall"
)

func (MonitorFailedAction) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		Type:    "string",
		Default: MonitorActionNone,
		Enum: []any{
			MonitorActionNone,
			MonitorActionRollback,
			MonitorActionUninstall,
		},
	}
}

type MonitorReference struct {
	Name   string              `yaml:"name" json:"name" jsonschema:"required"`
	Action MonitorFailedAction `yaml:"action" json:"action" jsonschema:"title=Action if monitor fails"`
}

// monitorActionPriority ranks remediation actions by preference so the choice
// is fully deterministic and never depends on map/slice iteration order.
//
// Precedence (higher number wins): Rollback > Uninstall > None.
//
// The SAFER (least destructive) requested action always wins:
//   - Rollback is reversible and keeps the release, so it is preferred whenever
//     any failed monitor asks for it.
//   - Uninstall permanently deletes the release; it is only chosen when no
//     failed monitor requested a Rollback.
//   - None is the no-op fallback used when no failed monitor requested an action.
//
// This guarantees a release is never randomly deleted instead of rolled back
// (or vice versa) just because failed monitors were iterated in a different
// order between runs.
func monitorActionPriority(action MonitorFailedAction) int {
	switch action {
	case MonitorActionRollback:
		return 2
	case MonitorActionUninstall:
		return 1
	case MonitorActionNone:
		return 0
	default:
		return 0
	}
}

// SelectMonitorFailedAction deterministically selects a single remediation
// action for a release from its monitor references, given the set of failed
// monitors. When the failed monitors map to different actions the safest one
// wins (see monitorActionPriority). The result does not depend on the order of
// refs or failed.
func SelectMonitorFailedAction(refs []MonitorReference, failed ...monitor.Config) MonitorFailedAction {
	action := MonitorActionNone

	for _, mon := range failed {
		for i := range refs {
			monRef := refs[i]
			if mon.Name() != monRef.Name {
				continue
			}

			if monitorActionPriority(monRef.Action) > monitorActionPriority(action) {
				action = monRef.Action
			}
		}
	}

	return action
}

func (rel *config) NotifyMonitorsFailed(ctx context.Context, mons ...monitor.Config) {
	action := SelectMonitorFailedAction(rel.Monitors(), mons...)

	if action == MonitorActionNone {
		rel.Logger().Info("no actions will be performed for failed monitors")
	} else {
		rel.Logger().WithField("action", action).Info("chose action to perform for failed monitors")
		rel.performMonitorAction(ctx, action)
	}
}

func (rel *config) performMonitorAction(ctx context.Context, action MonitorFailedAction) {
	switch action {
	case MonitorActionRollback:
		err := rel.Rollback(ctx, 0)
		if err != nil {
			rel.Logger().WithError(err).Error("caught error while handling failed monitors")
		}
	case MonitorActionUninstall:
		_, err := rel.Uninstall(ctx)
		if err != nil {
			rel.Logger().WithError(err).Error("caught error while handling failed monitors")
		}
	default:
		rel.Logger().WithField("action", action).Error("unknown action to perform, skipping")
	}
}
