package command

import (
	"strings"

	"nopsai/internal/cli/interactive"
)

func installPreviewScreenOptions(target string) interactive.ScreenOptions {
	return interactive.ScreenOptions{
		Breadcrumb: []string{"Home", "Install", target, "Preview"},
		Title:      target + " Command Preview",
		Header: []string{
			"Review the equivalent noninteractive command before execution.",
			"No files are written and no external process is started until you continue.",
		},
	}
}

func previewInteractiveDockerComposeInstall(prompter *interactive.Prompter, options *installDockerComposeOptions) error {
	effects := []string{
		"Generate docker-compose.yaml, .env, db/init.sql, and install lock metadata.",
		"Write files under " + installCommandOutputDir(options.outputDir) + ".",
	}
	if options.run {
		effects = append(effects, "Run Docker Compose after file generation.")
	}
	return showInstallCommandPreview(prompter, "Docker Compose", dockerComposeInstallPreviewArgs(options), effects)
}

func previewInteractiveKubernetesInstall(prompter *interactive.Prompter, options *installKubernetesOptions) error {
	effects := []string{
		"Generate Helm values and install lock metadata.",
		"Write files under " + installCommandOutputDir(options.outputDir) + ".",
	}
	if options.deploy {
		effects = append(effects, "Run Helm upgrade --install after values are generated.")
	}
	return showInstallCommandPreview(prompter, "Kubernetes", kubernetesInstallPreviewArgs(options), effects)
}

func showInstallCommandPreview(prompter *interactive.Prompter, target string, args []string, effects []string) error {
	return showInteractiveCommandPreview(prompter, target+" command preview", args, effects, installPreviewScreenOptions(target))
}

func dockerComposeInstallPreviewArgs(options *installDockerComposeOptions) []string {
	args := []string{
		"nopsai", "install", "docker-compose",
		"--version", strings.TrimSpace(options.version),
		"--output-dir", strings.TrimSpace(options.outputDir),
		"--project", strings.TrimSpace(options.projectName),
		"--api-port", strings.TrimSpace(options.apiPort),
		"--ui-port", strings.TrimSpace(options.uiPort),
		"--nopsai-api-url", strings.TrimSpace(options.nopsaiAPIURL),
		"--dispatcher-address", strings.TrimSpace(options.dispatcherAddr),
		"--aaa-api-url", strings.TrimSpace(options.aaaAPIURL),
		"--git-bot-api-url", strings.TrimSpace(options.gitBotAPIURL),
		"--gotenberg-url", strings.TrimSpace(options.gotenbergURL),
		"--docker-network", strings.TrimSpace(options.dockerNetwork),
		"--bootstrap-admin-email", strings.TrimSpace(options.bootstrapAdminEmail),
	}
	if strings.TrimSpace(options.bootstrapAdminPassword) != "" {
		args = append(args, "--bootstrap-admin-password", "<redacted>")
	}
	if options.force {
		args = append(args, "--force")
	}
	if options.run {
		args = append(args, "--run")
	}
	return args
}

func kubernetesInstallPreviewArgs(options *installKubernetesOptions) []string {
	args := []string{
		"nopsai", "install", "kubernetes",
		"--version", strings.TrimSpace(options.version),
		"--output-dir", strings.TrimSpace(options.outputDir),
		"--values-file", strings.TrimSpace(options.valuesFile),
		"--release", strings.TrimSpace(options.releaseName),
		"--namespace", strings.TrimSpace(options.namespace),
		"--existing-secret", strings.TrimSpace(options.existingSecret),
		"--bootstrap-admin-email", strings.TrimSpace(options.bootstrapAdminEmail),
		"--bootstrap-admin-password-secret-key", strings.TrimSpace(options.bootstrapAdminPasswordSecretKey),
		"--nopsai-api-url", strings.TrimSpace(options.nopsaiAPIURL),
		"--dispatcher-address", strings.TrimSpace(options.dispatcherAddr),
		"--aaa-api-url", strings.TrimSpace(options.aaaAPIURL),
		"--git-bot-api-url", strings.TrimSpace(options.gitBotAPIURL),
		"--gotenberg-url", strings.TrimSpace(options.gotenbergURL),
	}
	if strings.TrimSpace(options.ingressHost) != "" {
		args = append(args, "--ingress-host", strings.TrimSpace(options.ingressHost))
	}
	for _, values := range options.values {
		if strings.TrimSpace(values) != "" {
			args = append(args, "--values", strings.TrimSpace(values))
		}
	}
	if options.force {
		args = append(args, "--force")
	}
	if options.deploy {
		args = append(args, "--deploy")
	}
	if options.wait {
		args = append(args, "--wait")
	}
	if strings.TrimSpace(options.lockFile) != "" {
		args = append(args, "--lock-file", strings.TrimSpace(options.lockFile))
	}
	return args
}
