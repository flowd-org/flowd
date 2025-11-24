package handlers

import (
	"strings"

	"github.com/flowd-org/flowd/internal/indexer"
)

type aliasLookup struct {
	byName     map[string]indexer.AliasInfo
	collisions map[string][]indexer.AliasInfo
	invalid    map[string]indexer.AliasValidation
}

func newAliasLookup() *aliasLookup {
	return &aliasLookup{
		byName:     make(map[string]indexer.AliasInfo),
		collisions: make(map[string][]indexer.AliasInfo),
		invalid:    make(map[string]indexer.AliasValidation),
	}
}

func (l *aliasLookup) merge(result indexer.Result) {
	for _, alias := range result.Aliases {
		l.byName[strings.ToLower(alias.Name)] = alias
	}
	for key, list := range result.AliasCollisions {
		if len(list) == 0 {
			continue
		}
		lower := strings.ToLower(key)
		copied := make([]indexer.AliasInfo, len(list))
		copy(copied, list)
		l.collisions[lower] = copied
	}
	for key, val := range result.AliasInvalid {
		l.invalid[strings.ToLower(key)] = val
	}
}

func (l *aliasLookup) resolve(name string) (alias indexer.AliasInfo, hasAlias bool, colliders []indexer.AliasInfo, hasCollision bool, validation indexer.AliasValidation, hasInvalid bool) {
	key := strings.ToLower(name)
	alias, hasAlias = l.byName[key]
	colliders, hasCollision = l.collisions[key]
	validation, hasInvalid = l.invalid[key]
	return
}

func mergeJobInfo(dest map[string]indexer.JobInfo, res indexer.Result) {
	for _, job := range res.Jobs {
		dest[strings.ToLower(job.ID)] = job
	}
}
