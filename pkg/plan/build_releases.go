package plan

import (
	"slices"

	"github.com/semx/helmtide/pkg/helper"
	"github.com/semx/helmtide/pkg/release"
	"github.com/semx/helmtide/pkg/release/uniqname"
	log "github.com/sirupsen/logrus"
)

func (p *Plan) buildReleases(o BuildOptions) ([]release.Config, error) {
	log.Info("🔨 Building releases...")

	plan := make([]release.Config, 0)

	planAdderFunction := addToPlanWithDependencies
	if !o.EnableDependencies {
		planAdderFunction = addToPlanWithoutDependencies
	}

	for _, r := range p.body.Releases {
		if !checkTagInclusion(o.Tags, r.Tags(), o.MatchAll) {
			continue
		}

		var err error
		plan, err = planAdderFunction(plan, r, p.body.Releases)
		if err != nil {
			log.WithError(err).Error("failed to build releases plan")

			return nil, err
		}
	}

	return plan, nil
}

func addToPlan(
	plan release.Configs,
	rel release.Config,
) (_ release.Configs, err error) {
	if r, contains := plan.Contains(rel); contains {
		if r != rel {
			return nil, release.NewDuplicateError(rel.Uniq())
		} else {
			return plan, nil
		}
	}

	return append(plan, rel), nil
}

func addToPlanWithoutDependencies(
	plan release.Configs,
	rel release.Config,
	_ release.Configs,
) (release.Configs, error) {
	plan, err := addToPlan(plan, rel)
	if err != nil {
		return nil, err
	}

	rel.SetDependsOn([]*release.DependsOnReference{})

	return plan, nil
}

func addToPlanWithDependencies(
	plan release.Configs,
	rel release.Config,
	releases release.Configs,
) (release.Configs, error) {
	// visited guards against dependency cycles (e.g. A -> B -> A or A -> A):
	// without it the recursion below would descend forever and stack-overflow
	// before the graph cycle detector in Graph.Build ever runs.
	return addToPlanWithDependenciesVisited(plan, rel, releases, make(map[uniqname.UniqName]struct{}))
}

func addToPlanWithDependenciesVisited(
	plan release.Configs,
	rel release.Config,
	releases release.Configs,
	visited map[uniqname.UniqName]struct{},
) (release.Configs, error) {
	// Already expanded (or currently being expanded) in this dependency chain.
	// Stop descending so a cycle is tolerated here — each node is added once and
	// the resulting graph is left for Graph.Build to report as a loop.
	// NOTE: this pre-add check is keyed by Uniq, so it also short-circuits a
	// second, distinct config sharing an already-seen Uniq. That only happens for
	// already-invalid configs (duplicate uniqnames), which addToPlan would reject
	// anyway on the first, unguarded top-level pass — so no valid plan is masked.
	if _, seen := visited[rel.Uniq()]; seen {
		return plan, nil
	}

	visited[rel.Uniq()] = struct{}{}

	newPlan, err := addToPlan(plan, rel)
	if err != nil {
		return nil, err
	}

	deps := rel.DependsOn()
	newDeps := make([]*release.DependsOnReference, 0, len(deps))

	for _, dep := range deps {
		l := rel.Logger().WithField("dependency", dep.Uniq())
		l.Trace("searching for dependency")

		r, found := releases.ContainsUniq(dep.Uniq())
		if found {
			var err error
			newPlan, err = addToPlanWithDependenciesVisited(newPlan, r, releases, visited)
			if err != nil {
				return nil, err
			}

			newDeps = append(newDeps, dep)

			continue
		}

		if dep.Optional {
			l.Warn("can't find dependency in available releases, skipping")
		} else {
			l.Error("can't find required dependency")

			return nil, release.ErrDepFailed
		}
	}

	rel.SetDependsOn(newDeps)

	return newPlan, nil
}

func releaseNames(a []release.Config) []string {
	return helper.SlicesMap(a, func(r release.Config) string {
		return r.Uniq().String()
	})
}

// checkTagInclusion checks where any of release tags are included in target tags.
func checkTagInclusion(targetTags, releaseTags []string, matchAll bool) bool {
	if len(targetTags) == 0 {
		return true
	}

	if matchAll {
		return checkAllTagsInclusion(targetTags, releaseTags)
	}

	return checkAnyTagInclusion(targetTags, releaseTags)
}

func checkAllTagsInclusion(targetTags, releaseTags []string) bool {
	for _, t := range targetTags {
		contains := slices.Contains(releaseTags, t)
		if !contains {
			return false
		}
	}

	return true
}

func checkAnyTagInclusion(targetTags, releaseTags []string) bool {
	for _, t := range targetTags {
		contains := slices.Contains(releaseTags, t)
		if contains {
			return true
		}
	}

	return false
}
