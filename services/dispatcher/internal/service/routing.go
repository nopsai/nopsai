package service

import (
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
)

func (d *dispatcherServer) allowedRunnerIDs(scope string) []string {
	if len(d.routing) == 0 {
		return nil
	}
	scope = normalizeDispatcherRoutingScope(scope)
	var ids []string
	if runners, ok := d.routing[scope]; ok {
		ids = appendUniqueRunnerIDs(ids, runners...)
	}
	ids = appendUniqueRunnerIDs(ids, d.liveRunnerIDsForScopeLocked(scope)...)
	if runners, ok := d.routing["*"]; ok {
		ids = appendUniqueRunnerIDs(ids, runners...)
	}
	ids = appendUniqueRunnerIDs(ids, d.liveRunnerIDsForScopeLocked("*")...)
	return ids
}

func normalizeDispatcherRouting(routing map[string][]string) map[string][]string {
	if len(routing) == 0 {
		return map[string][]string{}
	}
	clean := make(map[string][]string, len(routing))
	for scope, ids := range routing {
		scopeKey := strings.TrimSpace(scope)
		if scopeKey == "" {
			scopeKey = "*"
		}
		next := make([]string, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			next = append(next, id)
		}
		if len(next) > 0 {
			clean[scopeKey] = next
		}
	}
	return clean
}

func normalizeDispatcherRoutingScope(scope string) string {
	scopeKey := strings.TrimSpace(scope)
	if scopeKey == "" {
		return "*"
	}
	return scopeKey
}

func (d *dispatcherServer) liveRunnerIDsForScopeLocked(scope string) []string {
	scope = normalizeDispatcherRoutingScope(scope)
	var ids []string
	for _, runner := range d.runners {
		if runner == nil || strings.TrimSpace(runner.id) == "" {
			continue
		}
		if runnerRoutesToScope(runner, scope) {
			ids = append(ids, runner.id)
		}
	}
	sort.Strings(ids)
	return ids
}

func runnerRoutesToScope(runner *runnerConn, scope string) bool {
	if runner == nil {
		return false
	}
	if scope == "*" {
		return len(runner.scopes) == 0
	}
	if len(runner.scopes) == 0 {
		return false
	}
	_, ok := runner.scopes[strings.ToLower(scope)]
	return ok
}

func appendUniqueRunnerIDs(ids []string, candidates ...string) []string {
	seen := make(map[string]struct{}, len(ids)+len(candidates))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		seen[id] = struct{}{}
	}
	for _, id := range candidates {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		ids = append(ids, id)
		seen[id] = struct{}{}
	}
	return ids
}

func dispatcherRoutingEqual(a, b map[string][]string) bool {
	if len(a) != len(b) {
		return false
	}
	for scope, left := range a {
		right, ok := b[scope]
		if !ok || len(left) != len(right) {
			return false
		}
		for i := range left {
			if left[i] != right[i] {
				return false
			}
		}
	}
	return true
}

func (d *dispatcherServer) applyRouting(routing map[string][]string) bool {
	normalized := normalizeDispatcherRouting(routing)

	d.mu.Lock()
	defer d.mu.Unlock()

	if dispatcherRoutingEqual(d.routing, normalized) {
		return false
	}
	d.routing = normalized
	d.triggerAssignments = make(map[string]string)
	return true
}

func (d *dispatcherServer) updateRouting(routing map[string][]string) bool {
	if !d.applyRouting(routing) {
		return false
	}

	log.Info().
		Int("scopes", len(routing)).
		Msg("dispatcher routing updated")
	go d.pumpQueue()
	return true
}
