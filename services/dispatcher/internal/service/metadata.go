package service

import "strings"

func toSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, v := range values {
		trimmed := strings.ToLower(strings.TrimSpace(v))
		if trimmed == "" {
			continue
		}
		result[trimmed] = struct{}{}
	}
	return result
}

func keys(m map[string]struct{}) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func cloneSet(values map[string]struct{}) map[string]struct{} {
	if len(values) == 0 {
		return map[string]struct{}{}
	}
	out := make(map[string]struct{}, len(values))
	for key := range values {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = struct{}{}
	}
	return out
}

func mergeMetadata(meta map[string]string, connectionID string) map[string]string {
	if len(meta) == 0 && connectionID == "" {
		return nil
	}
	out := cloneMetadata(meta)
	if connectionID != "" {
		out["connection_id"] = connectionID
	}
	return out
}

func cloneMetadata(meta map[string]string) map[string]string {
	if len(meta) == 0 {
		return make(map[string]string)
	}
	out := make(map[string]string, len(meta))
	for k, v := range meta {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		out[key] = v
	}
	return out
}
