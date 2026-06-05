package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/rs/zerolog/log"
)

func (a *App) handleListRepoBranches(w http.ResponseWriter, r *http.Request) {
	repoOwner := r.PathValue("repoOwner")
	repoName := r.PathValue("repoName")
	fullName := fmt.Sprintf("%s/%s", repoOwner, repoName)

	query := `
		SELECT DISTINCT git_ref
		FROM pipeline_runs
		WHERE git_repo_name = $1 AND git_ref IS NOT NULL
		ORDER BY git_ref ASC
	`

	rows, err := a.db.Query(context.Background(), query, fullName)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query branches from database")
		http.Error(w, "Failed to retrieve branches", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var branches []string
	for rows.Next() {
		var branch sql.NullString
		if err := rows.Scan(&branch); err != nil {
			log.Error().Err(err).Msg("Failed to scan branch name")
			http.Error(w, "Failed to process branches", http.StatusInternalServerError)
			return
		}
		if branch.Valid {
			branches = append(branches, strings.TrimPrefix(branch.String, "refs/heads/"))
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(branches)
}
