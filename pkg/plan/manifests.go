package plan

import "github.com/semx/helmtide/pkg/release/uniqname"

func (p *Plan) Manifests() map[uniqname.UniqName]string {
	return p.manifests
}
