package release

import (
	"fmt"

	"github.com/invopop/jsonschema"
	"gopkg.in/yaml.v3"
	"helm.sh/helm/v4/pkg/kube"
)

// WaitStrategy tells helm how to wait for the resources of a release.
//
// helm v3 took a boolean, helm v4 replaced it with a named strategy. `wait: true` and `wait: false`
// are still accepted and mapped the same way helm maps its own deprecated boolean --wait.
type WaitStrategy string

const (
	// WaitStrategyWatcher waits for every resource using kubernetes watches. This is what
	// `wait: true` means.
	WaitStrategyWatcher = WaitStrategy(kube.StatusWatcherStrategy)

	// WaitStrategyLegacy waits by polling, the way helm 3 did.
	WaitStrategyLegacy = WaitStrategy(kube.LegacyStrategy)

	// WaitStrategyHookOnly waits only for hooks, not for the chart's own resources. This is what
	// `wait: false` means, and the default when `wait` is not set at all.
	WaitStrategyHookOnly = WaitStrategy(kube.HookOnlyStrategy)
)

// Helm returns the helm wait strategy. helm has no usable zero value here — it refuses to build a
// waiter for an empty strategy — so an unset strategy becomes the same default helm's own CLI uses.
func (s WaitStrategy) Helm() kube.WaitStrategy {
	if s == "" {
		return kube.HookOnlyStrategy
	}

	return kube.WaitStrategy(s)
}

// Enabled reports whether the release waits for its own resources, and not just for its hooks.
func (s WaitStrategy) Enabled() bool {
	return s.Helm() != kube.HookOnlyStrategy
}

// Validate checks that the strategy is one helm knows.
func (s WaitStrategy) Validate() error {
	switch s {
	case "", WaitStrategyWatcher, WaitStrategyLegacy, WaitStrategyHookOnly:
		return nil
	default:
		return NewInvalidWaitStrategyError(string(s))
	}
}

// UnmarshalYAML is an unmarshaller for gopkg.in/yaml.v3 that accepts both the helm 3 boolean and
// the helm 4 strategy name.
func (s *WaitStrategy) UnmarshalYAML(node *yaml.Node) error {
	var b bool
	if err := node.Decode(&b); err == nil {
		if b {
			*s = WaitStrategyWatcher
		} else {
			*s = WaitStrategyHookOnly
		}

		return nil
	}

	var str string
	if err := node.Decode(&str); err != nil {
		return fmt.Errorf("failed to decode wait strategy %q from YAML at %d line: %w", node.Value, node.Line, err)
	}

	parsed := WaitStrategy(str)
	if err := parsed.Validate(); err != nil {
		return err
	}

	*s = parsed

	return nil
}

func (WaitStrategy) JSONSchema() *jsonschema.Schema {
	return &jsonschema.Schema{
		OneOf: []*jsonschema.Schema{
			{
				Type: "string",
				Enum: []any{
					WaitStrategyWatcher,
					WaitStrategyLegacy,
					WaitStrategyHookOnly,
					"",
				},
			},
			{Type: "boolean"},
		},
	}
}
