package command

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"nopsai/pkg/buildinfo"

	"github.com/spf13/cobra"
)

type apiRequestOptions struct {
	dataPath    string
	dataRaw     string
	headers     []string
	contentType string
	accept      string
	outputFile  string
	noAuth      bool
	showHeaders bool
}

func newAPIRequestCommand(options *rootOptions) *cobra.Command {
	requestOptions := &apiRequestOptions{}
	command := &cobra.Command{
		Use:   "request METHOD PATH",
		Short: "Send a request to any concrete REST path",
		Args:  cobra.ExactArgs(2),
		RunE: func(command *cobra.Command, args []string) error {
			session, err := options.resolveSessionWithToken(false, !requestOptions.noAuth)
			if err != nil {
				return err
			}
			return executeAPIRequest(command, session, strings.ToUpper(args[0]), args[1], *requestOptions, options.dependencies.BuildInfo)
		},
	}
	addAPIRequestFlags(command, requestOptions)
	return command
}

func addAPIRequestFlags(command *cobra.Command, options *apiRequestOptions) {
	command.Flags().StringVar(&options.dataPath, "data", "", "request body file, or - for stdin")
	command.Flags().StringVar(&options.dataRaw, "data-raw", "", "literal request body")
	command.Flags().StringArrayVarP(&options.headers, "header", "H", nil, "additional request header (repeatable)")
	command.Flags().StringVar(&options.contentType, "content-type", "", "request content type (default: application/json when a body is present)")
	command.Flags().StringVar(&options.accept, "accept", "", "accepted response content type")
	command.Flags().StringVarP(&options.outputFile, "output-file", "o", "", "write a successful response body atomically to a file")
	command.Flags().BoolVar(&options.noAuth, "no-auth", false, "do not attach the configured bearer token")
	command.Flags().BoolVarP(&options.showHeaders, "show-headers", "i", false, "write response status and headers to stderr")
	command.MarkFlagsMutuallyExclusive("data", "data-raw")
}

func executeAPIRequest(command *cobra.Command, session session, method, path string, options apiRequestOptions, cli buildinfo.Info) error {
	if mutatingMethod(method) {
		if err := ensureMutationCompatibility(command, session, cli); err != nil {
			return err
		}
	}
	body, closeBody, err := openRequestBody(command.InOrStdin(), options.dataPath, options.dataRaw)
	if err != nil {
		return err
	}
	if closeBody != nil {
		defer closeBody()
	}
	request, err := session.Client.NewRequest(method, path, body)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	for _, rawHeader := range options.headers {
		name, value, ok := strings.Cut(rawHeader, ":")
		if !ok || strings.TrimSpace(name) == "" {
			return fmt.Errorf("invalid header %q; expected NAME: VALUE", rawHeader)
		}
		if strings.ContainsAny(name+value, "\r\n") {
			return fmt.Errorf("invalid header %q: newlines are not allowed", name)
		}
		request.Header.Set(strings.TrimSpace(name), strings.TrimSpace(value))
	}
	if contentType := strings.TrimSpace(options.contentType); contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if accept := strings.TrimSpace(options.accept); accept != "" {
		request.Header.Set("Accept", accept)
	}
	var response *http.Response
	if options.noAuth {
		response, err = session.Client.DoUnauthenticated(request)
	} else {
		response, err = session.Client.Do(request)
	}
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if options.showHeaders {
		if err := writeResponseHeaders(command.ErrOrStderr(), response); err != nil {
			return err
		}
	}
	success := response.StatusCode >= http.StatusOK && response.StatusCode < http.StatusMultipleChoices
	if success && strings.TrimSpace(options.outputFile) != "" && options.outputFile != "-" {
		if err := writeResponseFile(options.outputFile, response.Body); err != nil {
			return err
		}
		return nil
	}
	if err := writeResponse(command.OutOrStdout(), response.Body); err != nil {
		return err
	}
	if !success {
		return fmt.Errorf("API returned HTTP %d", response.StatusCode)
	}
	return nil
}

func openRequestBody(stdin io.Reader, path, raw string) (io.Reader, func() error, error) {
	path = strings.TrimSpace(path)
	if path != "" && raw != "" {
		return nil, nil, errors.New("--data and --data-raw cannot be used together")
	}
	if raw != "" {
		return strings.NewReader(raw), nil, nil
	}
	if path == "" {
		return nil, nil, nil
	}
	if path == "-" {
		if stdin == nil {
			return nil, nil, errors.New("stdin is unavailable")
		}
		return stdin, nil, nil
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, nil, fmt.Errorf("open request data: %w", err)
	}
	return file, file.Close, nil
}

func writeResponse(writer io.Writer, body io.Reader) error {
	if _, err := io.Copy(writer, body); err != nil {
		return fmt.Errorf("read API response: %w", err)
	}
	return nil
}

func writeResponseHeaders(writer io.Writer, response *http.Response) error {
	if _, err := fmt.Fprintf(writer, "%s %s\n", response.Proto, response.Status); err != nil {
		return err
	}
	names := make([]string, 0, len(response.Header))
	for name := range response.Header {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		for _, value := range response.Header.Values(name) {
			if _, err := fmt.Fprintf(writer, "%s: %s\n", name, value); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(writer)
	return err
}

func writeResponseFile(path string, body io.Reader) error {
	dir := filepath.Dir(path)
	temp, err := os.CreateTemp(dir, ".nopsai-response-*.tmp")
	if err != nil {
		return fmt.Errorf("create response file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("secure response file: %w", err)
	}
	if _, err := io.Copy(temp, body); err != nil {
		temp.Close()
		return fmt.Errorf("write response file: %w", err)
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return fmt.Errorf("sync response file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close response file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace response file: %w", err)
	}
	return nil
}
