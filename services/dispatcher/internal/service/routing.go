package service

import (
	"strings"

	"github.com/rs/zerolog/log"
)

func (d *dispatcherServer) allowedRunnerIDs(scope string) []string {
	if len(d.routing) == 0 {
		return nil
	}
	scope = strings.TrimSpace(scope)
	var ids []string
	if runners, ok := d.routing[scope]; ok {
		ids = append(ids, runners...)
	}
	if runners, ok := d.routing["*"]; ok {
		ids = append(ids, runners...)
	}
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
