package runnerinstall

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"strings"
)

const (
	RunnerNameEnv = "RUNNER_NAME"
)

type runnerIdentity struct {
	ID   string
	Name string
	UID  string
}

func runnerIdentityForInstall(name string, query url.Values) (runnerIdentity, error) {
	baseName := normalizeRunnerName(name)
	uid := runnerUIDFromQuery(query)
	if uid == "" {
		generated, err := randomRunnerUID()
		if err != nil {
			return runnerIdentity{}, err
		}
		uid = generated
	}
	runnerID := kubernetesManifestName(baseName, uid)
	return runnerIdentity{ID: runnerID, Name: baseName, UID: uid}, nil
}

func normalizeRunnerName(value string) string {
	name := strings.TrimSpace(value)
	if name == "" {
		return "runner"
	}
	return name
}

func runnerUIDFromQuery(query url.Values) string {
	for _, key := range []string{"runner_uid", "runner_instance_id"} {
		uid := kubernetesManifestName(strings.TrimSpace(query.Get(key)))
		if uid != "" && uid != "nopsai" {
			return uid
		}
	}
	return ""
}

func randomRunnerUID() (string, error) {
	var data [5]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", fmt.Errorf("generate runner uid: %w", err)
	}
	return hex.EncodeToString(data[:]), nil
}

func EnsureRunnerUID(r *http.Request) error {
	if r == nil || r.URL == nil {
		return nil
	}
	query := r.URL.Query()
	if strings.TrimSpace(query.Get("runner_uid")) != "" || strings.TrimSpace(query.Get("runner_instance_id")) != "" {
		return nil
	}
	uid, err := randomRunnerUID()
	if err != nil {
		return err
	}
	query.Set("runner_uid", uid)
	r.URL.RawQuery = query.Encode()
	return nil
}
