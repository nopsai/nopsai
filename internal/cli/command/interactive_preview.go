package command

import (
	"strings"

	"nopsai/internal/cli/interactive"
)

func commandPreviewScreenOptions(breadcrumb []string, title string, header []string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: breadcrumb,
		Title:      title,
		Header:     header,
	}
}

func showInteractiveCommandPreview(prompter *interactive.Prompter, title string, args []string, effects []string, options interactive.ScreenOptions) error {
	lines := []string{
		"Command preview",
		"",
		commandShellJoin(args),
		"",
		"Enter continues. Esc backs out before anything is executed.",
	}
	if len(effects) > 0 {
		lines = append(lines, "", "What will happen")
		for _, effect := range effects {
			if strings.TrimSpace(effect) != "" {
				lines = append(lines, "  - "+strings.TrimSpace(effect))
			}
		}
	}
	if err := prompter.ShowTextScreen(title, lines, options); err != nil {
		return err
	}
	if prompter.CanUseLiveSelector() {
		return nil
	}
	confirmed, err := prompter.Confirm("Execute this command now", true)
	if err != nil {
		return err
	}
	if !confirmed {
		return interactive.ErrBack
	}
	return nil
}

func commandShellJoin(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, commandShellQuote(arg))
	}
	return strings.Join(quoted, " ")
}

func commandShellQuote(value string) string {
	if value == "" {
		return "''"
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("/._-:=@", r))
	}) == -1 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

func appendAPIRequestPreviewFlags(args []string, options apiRequestOptions) []string {
	if strings.TrimSpace(options.dataPath) != "" {
		args = append(args, "--data", strings.TrimSpace(options.dataPath))
	}
	if options.dataRaw != "" {
		args = append(args, "--data-raw", options.dataRaw)
	}
	for _, header := range options.headers {
		if strings.TrimSpace(header) != "" {
			args = append(args, "--header", strings.TrimSpace(header))
		}
	}
	if strings.TrimSpace(options.contentType) != "" {
		args = append(args, "--content-type", strings.TrimSpace(options.contentType))
	}
	if strings.TrimSpace(options.accept) != "" {
		args = append(args, "--accept", strings.TrimSpace(options.accept))
	}
	if strings.TrimSpace(options.outputFile) != "" {
		args = append(args, "--output-file", strings.TrimSpace(options.outputFile))
	}
	if options.noAuth {
		args = append(args, "--no-auth")
	}
	if options.showHeaders {
		args = append(args, "--show-headers")
	}
	return args
}

func apiRequestPreviewArgs(method, path string, options apiRequestOptions) []string {
	args := []string{"nopsai", "api", "request", strings.ToUpper(strings.TrimSpace(method)), strings.TrimSpace(path)}
	return appendAPIRequestPreviewFlags(args, options)
}

func apiCatalogCallPreviewArgs(routeMethod, routePath string, pathValues, queryValues []string, options apiRequestOptions) []string {
	args := []string{"nopsai", "api", "call", strings.ToUpper(strings.TrimSpace(routeMethod)), strings.TrimSpace(routePath)}
	for _, value := range pathValues {
		if strings.TrimSpace(value) != "" {
			args = append(args, "--path", strings.TrimSpace(value))
		}
	}
	for _, value := range queryValues {
		if strings.TrimSpace(value) != "" {
			args = append(args, "--query", strings.TrimSpace(value))
		}
	}
	return appendAPIRequestPreviewFlags(args, options)
}

func apiRoutesPreviewArgs(domain, method, audience, output string) []string {
	args := []string{"nopsai", "api", "routes"}
	if strings.TrimSpace(domain) != "" {
		args = append(args, "--domain", strings.TrimSpace(domain))
	}
	if strings.TrimSpace(method) != "" {
		args = append(args, "--method", strings.ToUpper(strings.TrimSpace(method)))
	}
	if audience = strings.ToLower(strings.TrimSpace(audience)); audience != "" && audience != "all" {
		args = append(args, "--audience", audience)
	}
	if output = strings.ToLower(strings.TrimSpace(output)); output != "" && output != "text" {
		args = append(args, "--output", output)
	}
	return args
}

func apiDescribePreviewArgs(method, path, output string) []string {
	args := []string{"nopsai", "api", "describe", strings.ToUpper(strings.TrimSpace(method)), strings.TrimSpace(path)}
	if output = strings.ToLower(strings.TrimSpace(output)); output != "" {
		args = append(args, "--output", output)
	}
	return args
}

func completionPreviewArgs(shell, outputDir string, stdout bool) []string {
	args := []string{"nopsai", "completion", strings.TrimSpace(shell)}
	if stdout {
		return append(args, "--stdout")
	}
	return append(args, "--output-dir", strings.TrimSpace(outputDir))
}

func platformReleasePreviewArgs(options *platformReleaseOptions) []string {
	args := []string{
		"nopsai", "platform", "release", "kubernetes",
		"--version", strings.TrimSpace(options.version),
	}
	if strings.TrimSpace(options.manifest) != "" {
		args = append(args, "--manifest", strings.TrimSpace(options.manifest))
	}
	if strings.TrimSpace(options.manifestDigest) != "" {
		args = append(args, "--manifest-digest", strings.TrimSpace(options.manifestDigest))
	}
	for _, values := range options.values {
		if strings.TrimSpace(values) != "" {
			args = append(args, "--values", strings.TrimSpace(values))
		}
	}
	args = append(args,
		"--release", strings.TrimSpace(options.releaseName),
		"--namespace", strings.TrimSpace(options.namespace),
		"--output", strings.TrimSpace(options.output),
		"--lock-file", releaseLockPath(*options),
	)
	if options.wait {
		args = append(args, "--wait")
	}
	if options.deploy {
		args = append(args, "--deploy")
	}
	return args
}
