package command

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nopsai/pkg/buildinfo"
	"nopsai/pkg/compatibility"

	"github.com/spf13/cobra"
)

func ensureMutationCompatibility(command *cobra.Command, session session, cli buildinfo.Info) error {
	if cli.IsDevelopment() {
		return nil
	}
	request, err := session.Client.NewRequest(http.MethodGet, "/version", nil)
	if err != nil {
		return err
	}
	request = request.WithContext(command.Context())
	response, err := session.Client.DoUnauthenticated(request)
	if err != nil {
		return fmt.Errorf("check platform compatibility: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
		return fmt.Errorf("check platform compatibility: GET /version returned HTTP %d", response.StatusCode)
	}
	var platform compatibility.PlatformInfo
	decoder := json.NewDecoder(io.LimitReader(response.Body, 1<<20))
	if err := decoder.Decode(&platform); err != nil {
		return fmt.Errorf("check platform compatibility: decode /version: %w", err)
	}
	if err := compatibility.ValidatePlatformForCLI(platform, cli); err != nil {
		return fmt.Errorf("platform compatibility check failed: %w", err)
	}
	return nil
}

func mutatingMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
