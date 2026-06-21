package nopsai

import (
	"encoding/json"
	"net/http"

	"nopsai/pkg/buildinfo"
)

func handleVersion(w http.ResponseWriter, _ *http.Request) {
	setNoStoreHeaders(w)
	writeJSON(w, http.StatusOK, buildinfo.Current().Public())
}

func versionInfoMap() map[string]any {
	encoded, err := json.Marshal(buildinfo.Current().Public())
	if err != nil {
		return map[string]any{"error": "build information is unavailable"}
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		return map[string]any{"error": "build information is unavailable"}
	}
	return result
}
