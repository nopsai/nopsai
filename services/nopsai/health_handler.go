package nopsai

import "net/http"

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	setNoStoreHeaders(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleLivez(w http.ResponseWriter, _ *http.Request) {
	setNoStoreHeaders(w)
	writeJSON(w, http.StatusOK, map[string]string{"status": "alive"})
}
