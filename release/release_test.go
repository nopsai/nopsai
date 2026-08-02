package release

import (
	"os"
	"strings"
	"testing"

	"nopsai/pkg/compatibility"
)

func TestCompatibilityContract(t *testing.T) {
	file, err := os.Open("compatibility.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contract, err := compatibility.DecodeCompatibility(file)
	if err != nil {
		t.Fatal(err)
	}
	if contract.CLIVersion != "0.22.0" || contract.RunnerProtocolVersion != 1 || !compatibility.HasCapability(contract.Capabilities, compatibility.CapabilityPlatformHelm) || !compatibility.HasCapability(contract.Capabilities, compatibility.CapabilityPlatformCompose) {
		t.Fatalf("contract = %#v", contract)
	}
}

func TestCommitCountVersionSeriesMatchesCompatibilityBaseline(t *testing.T) {
	contents, err := os.ReadFile("version.txt")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(contents)); got != "0.22" {
		t.Fatalf("release version series = %q, want 0.22", got)
	}
}

func TestCLIInstallGeneratorOwnsEveryVersionedImage(t *testing.T) {
	installerBytes, err := os.ReadFile("../internal/cli/platform/install.go")
	if err != nil {
		t.Fatal(err)
	}
	valuesBytes, err := os.ReadFile("../deploy/helm/nopsai/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	installer := string(installerBytes)
	values := string(valuesBytes)
	installRepositories := []string{
		"ghcr.io/nopsai/nopsai-aaa",
		"ghcr.io/nopsai/nopsai-agent",
		"ghcr.io/nopsai/nopsai-api",
		"ghcr.io/nopsai/nopsai-dispatcher",
		"ghcr.io/nopsai/nopsai-docker-socket-proxy",
		"ghcr.io/nopsai/nopsai-git-bot",
		"ghcr.io/nopsai/nopsai-docker-runner",
		"ghcr.io/nopsai/nopsai-k8s-runner",
		"ghcr.io/nopsai/nopsai-ui",
	}
	for _, repository := range installRepositories {
		if !strings.Contains(installer, repository) {
			t.Errorf("CLI install generator is missing %s", repository)
		}
	}
	for _, repository := range installRepositories {
		if repository == "ghcr.io/nopsai/nopsai-docker-socket-proxy" {
			continue
		}
		if !strings.Contains(values, repository) {
			t.Errorf("Helm chart values are missing %s", repository)
		}
	}
	if strings.Contains(values, "ghcr.io/nopsai/nopsai-docker-socket-proxy") {
		t.Error("Helm chart values should not include the Docker-only socket proxy image")
	}
	if !strings.Contains(installer, "DefaultInstallChartReference") || !strings.Contains(installer, "oci://ghcr.io/nopsai/charts/nopsai") {
		t.Fatal("CLI install generator does not declare the default OCI chart reference")
	}
	if strings.Contains(installer, ":latest") || strings.Contains(values, ":latest") {
		t.Fatal("install generator or chart values contain a floating NopsAI image")
	}
}

func TestHelmChartOwnsControlPlaneAndRunnerResources(t *testing.T) {
	for _, chartFile := range []string{
		"../deploy/helm/nopsai/Chart.yaml",
		"../deploy/helm/nopsai/values.yaml",
		"../deploy/helm/nopsai/templates/api.yaml",
		"../deploy/helm/nopsai/templates/aaa.yaml",
		"../deploy/helm/nopsai/templates/dispatcher.yaml",
		"../deploy/helm/nopsai/templates/git-bot.yaml",
		"../deploy/helm/nopsai/templates/ui.yaml",
		"../deploy/helm/nopsai/templates/k8s-runner.yaml",
		"../deploy/helm/nopsai/templates/postgres.yaml",
		"../deploy/helm/nopsai/files/db/init.sql",
	} {
		if info, err := os.Stat(chartFile); err != nil || info.Size() == 0 {
			t.Errorf("required Helm chart file %s is missing or empty", chartFile)
		}
	}
	pipeline := readNopsAIReleasePipeline(t)
	for _, required := range []string{"helm push", "nopsai-$VERSION.tgz", "oci://ghcr.io/"} {
		if !strings.Contains(pipeline, required) {
			t.Errorf("NopsAI platform release pipeline is missing Helm publication contract %q", required)
		}
	}
}

func TestHelmChartConfiguresBundledPostgreSQL(t *testing.T) {
	valuesBytes, err := os.ReadFile("../deploy/helm/nopsai/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	templateBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/postgres.yaml")
	if err != nil {
		t.Fatal(err)
	}
	notesBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/NOTES.txt")
	if err != nil {
		t.Fatal(err)
	}
	values := string(valuesBytes)
	template := string(templateBytes)
	notes := string(notesBytes)
	for _, required := range []string{
		"postgres:",
		"enabled: true",
		"repository: postgres",
		"postgresPassword: postgres-password",
		"passwordKey: postgres-password",
		"storageClass: \"\"",
		"size: 20Gi",
	} {
		if !strings.Contains(values, required) {
			t.Errorf("values.yaml is missing bundled PostgreSQL contract %q", required)
		}
	}
	for _, required := range []string{
		"kind: ConfigMap",
		"kind: Service",
		"kind: StatefulSet",
		"POSTGRES_DB",
		"POSTGRES_PASSWORD",
		"files/db/init.sql",
		"volumeClaimTemplates",
		"app.kubernetes.io/component: postgres",
	} {
		if !strings.Contains(template, required) {
			t.Errorf("postgres.yaml is missing bundled PostgreSQL contract %q", required)
		}
	}
	if !strings.Contains(notes, "{{ .Values.secrets.keys.postgresPassword }}") {
		t.Error("NOTES.txt does not tell operators about the PostgreSQL password key")
	}
}

func TestHelmReadmeDocumentsSecretsAndServiceAccountPullSecrets(t *testing.T) {
	readmeBytes, err := os.ReadFile("../deploy/helm/nopsai/README.md")
	if err != nil {
		t.Fatal(err)
	}
	readme := string(readmeBytes)
	for _, required := range []string{
		"kubectl -n nopsai create secret generic nopsai-secrets",
		"--from-literal=postgres-password=\"$POSTGRES_PASSWORD\"",
		"global.imagePullSecrets",
		"nopsai-api",
		"nopsai-runner",
		"patch serviceaccount nopsai-api",
		"patch serviceaccount nopsai-runner",
	} {
		if !strings.Contains(readme, required) {
			t.Errorf("Helm README is missing secret/ServiceAccount guidance %q", required)
		}
	}
}

func TestHelmPostgreSQLInitSQLMatchesCanonicalDatabaseBootstrap(t *testing.T) {
	canonical, err := os.ReadFile("../db/init.sql")
	if err != nil {
		t.Fatal(err)
	}
	chartCopy, err := os.ReadFile("../deploy/helm/nopsai/files/db/init.sql")
	if err != nil {
		t.Fatal(err)
	}
	if string(chartCopy) != string(canonical) {
		t.Fatal("Helm PostgreSQL init SQL must match db/init.sql")
	}
}

func TestHelmChartConfiguresKubernetesSystemLogs(t *testing.T) {
	valuesBytes, err := os.ReadFile("../deploy/helm/nopsai/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	apiTemplateBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	helpersBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/_helpers.tpl")
	if err != nil {
		t.Fatal(err)
	}
	values := string(valuesBytes)
	apiTemplate := string(apiTemplateBytes)
	helpers := string(helpersBytes)
	for _, required := range []string{
		"systemLogs:",
		"provider: kubernetes",
		"serviceAccount:",
		"name: nopsai-api",
	} {
		if !strings.Contains(values, required) {
			t.Errorf("values.yaml is missing Kubernetes system logs contract %q", required)
		}
	}
	for _, required := range []string{
		"kind: ServiceAccount",
		"serviceAccountName: {{ include \"nopsai.apiServiceAccountName\" . }}",
		"resources: [pods/log]",
		"verbs: [get]",
		"SYSTEM_LOGS_PROVIDER",
		"SYSTEM_LOGS_KUBERNETES_LABEL_SELECTOR",
	} {
		if !strings.Contains(apiTemplate, required) {
			t.Errorf("api.yaml is missing Kubernetes system logs contract %q", required)
		}
	}
	for _, required := range []string{"nopsai.apiServiceAccountName", "nopsai.systemLogsKubernetesEnabled", "nopsai.systemLogsKubernetesLabelSelector"} {
		if !strings.Contains(helpers, required) {
			t.Errorf("_helpers.tpl is missing %q", required)
		}
	}
}

func TestHelmChartTopologyDispatcherAddressIsOverrideable(t *testing.T) {
	valuesBytes, err := os.ReadFile("../deploy/helm/nopsai/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	apiTemplateBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	runnerTemplateBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/k8s-runner.yaml")
	if err != nil {
		t.Fatal(err)
	}
	dispatcherTemplateBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/dispatcher.yaml")
	if err != nil {
		t.Fatal(err)
	}
	helpersBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/_helpers.tpl")
	if err != nil {
		t.Fatal(err)
	}
	values := string(valuesBytes)
	apiTemplate := string(apiTemplateBytes)
	runnerTemplate := string(runnerTemplateBytes)
	dispatcherTemplate := string(dispatcherTemplateBytes)
	helpers := string(helpersBytes)
	for _, required := range []string{
		"topology:",
		"dispatcherGRPCAddress: dispatcher:9090",
		"dispatcherTLSSecret: dispatcher-tls-secret",
	} {
		if !strings.Contains(values, required) {
			t.Errorf("values.yaml is missing topology dispatcher contract %q", required)
		}
	}
	for name, template := range map[string]string{"api.yaml": apiTemplate, "k8s-runner.yaml": runnerTemplate} {
		if !strings.Contains(template, `include "nopsai.topology.dispatcherGRPCAddress" . | quote`) {
			t.Errorf("%s does not use topology dispatcher helper", name)
		}
	}
	for name, template := range map[string]string{"api.yaml": apiTemplate, "dispatcher.yaml": dispatcherTemplate, "k8s-runner.yaml": runnerTemplate} {
		if !strings.Contains(template, "DISPATCHER_TLS_SECRET") || !strings.Contains(template, ".Values.secrets.keys.dispatcherTLSSecret") {
			t.Errorf("%s is missing dispatcher TLS secret wiring", name)
		}
	}
	for _, required := range []string{
		`define "nopsai.topology.dispatcherGRPCAddress"`,
		`index $topology "dispatcherGRPCAddress"`,
		`default "dispatcher:9090"`,
	} {
		if !strings.Contains(helpers, required) {
			t.Errorf("_helpers.tpl is missing topology dispatcher helper contract %q", required)
		}
	}
	if strings.Contains(helpers, `dig "topology"`) {
		t.Fatal("_helpers.tpl must not use dig for topology values because Helm passes .Values as chartutil.Values")
	}
}

func TestPlatformReleasePublishesCLIArtifactsAndParsesHelmDigest(t *testing.T) {
	pipeline := readNopsAIReleasePipeline(t)
	for _, required := range []string{
		"package-helm-chart",
		"scripts/release-tags.sh \"$version\"",
		`release_tags+=("$release_tag")`,
		"RELEASE_TAGS",
		"helm lint dist/release/chart",
		"helm package dist/release/chart --destination dist/release",
		"for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64",
		"./cmd/nopsai-cli",
		"rm -rf dist/cli",
		"nopsai/pkg/buildinfo.APIVersion=v1",
		`asset="nopsai-cli_${VERSION}_${goos}_${goarch}"`,
		"shopt -s nullglob",
		"cli_assets=(dist/cli/*)",
		"No CLI release artifacts were built into dist/cli",
		`helm push "dist/release/nopsai-$VERSION.tgz" "$chart_repository" 2>&1`,
		"install_oras()",
		`oras_chart_reference="${chart_reference#oci://}"`,
		`oras copy "$oras_chart_reference:$VERSION" "$oras_chart_reference:$release_tag"`,
		`grep -Eo 'sha256:[a-f0-9]{64}'`,
		`tail -1 || true`,
		"apk add --no-cache bash coreutils perl-utils tar gzip",
		"helm_chart_asset=\"nopsai-helm-chart-$VERSION.tgz\"",
		"changelog_asset=\"nopsai-changelog-$VERSION.md\"",
		`cp "${cli_assets[@]}" dist/assets/`,
		`cp "dist/release/nopsai-$VERSION.tgz" "dist/assets/$helm_chart_asset"`,
		"apk add --no-cache bash curl git tar gzip",
		"legacy_assets=(",
		`gh release delete-asset "v$VERSION" "$asset"`,
		`gh release edit "v$VERSION" --repo "$GITHUB_REPOSITORY" --title "NopsAI $VERSION" --notes-file dist/release/CHANGELOG.md --latest`,
		`gh release create "v$VERSION" dist/assets/* --repo "$GITHUB_REPOSITORY" --target "$SOURCE_COMMIT" --title "NopsAI $VERSION" --notes-file dist/release/CHANGELOG.md --latest`,
		"publish_alias_release()",
		`delete_release_assets "$release_tag"`,
		`publish_alias_release "$release_tag" "v$release_tag"`,
		`done < <(scripts/release-tags.sh "$VERSION")`,
	} {
		if !strings.Contains(pipeline, required) {
			t.Errorf("NopsAI platform release pipeline is missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"render-release-bundle",
		"validate-release-compose",
		"release-manifest.json is required before building CLI archives",
		"release_manifest_digest",
		"EmbeddedReleaseManifestBase64",
		"release-index.json",
		"compose_asset=",
		"deployment_bundle_asset=",
		"tar -C dist/release -czf",
	} {
		if strings.Contains(pipeline, forbidden) {
			t.Errorf("NopsAI platform release pipeline should not contain %q", forbidden)
		}
	}
}

func TestReleasePipelinePublishesStableContainerTagAliases(t *testing.T) {
	pipeline := readNopsAIReleasePipeline(t)
	for _, required := range []string{
		`while IFS= read -r release_tag; do`,
		`release_tag_args+=(--tag "$REGISTRY/$image_name:$release_tag")`,
		`release_tag_args+=(--tag "$REGISTRY/nopsai-base:$release_tag")`,
		`done < <(scripts/release-tags.sh "$VERSION")`,
		`"${release_tag_args[@]}"`,
		`org.opencontainers.image.source="${SOURCE_URL}"`,
	} {
		if !strings.Contains(pipeline, required) && !dockerfilesContain(t, required) {
			t.Errorf("release container tagging/source contract is missing %q", required)
		}
	}
	for _, legacy := range []string{`--tag "$REGISTRY/$image_name:$VERSION"`, `--tag "$REGISTRY/$image_name:latest"`, `--tag "$REGISTRY/nopsai-base:$VERSION"`, `--tag "$REGISTRY/nopsai-base:latest"`} {
		if strings.Contains(pipeline, legacy) {
			t.Errorf("release pipeline should use generated release tag aliases instead of hard-coded tag %q", legacy)
		}
	}
}

func TestInstallGeneratorDeclaresEveryCompatibilityImageKey(t *testing.T) {
	contents, err := os.ReadFile("../internal/cli/platform/install.go")
	if err != nil {
		t.Fatal(err)
	}
	installer := string(contents)
	for _, image := range compatibility.RequiredPlatformImages {
		if !strings.Contains(installer, `"`+image+`"`) {
			t.Errorf("install generator is missing compatibility image key %q", image)
		}
	}
	for _, forbidden := range []string{"@latest", `:latest"`, `:edge"`} {
		if strings.Contains(installer, forbidden) {
			t.Errorf("install generator contains floating reference %q", forbidden)
		}
	}
}

func readNopsAIReleasePipeline(t *testing.T) string {
	t.Helper()
	contents, err := os.ReadFile("../.nopsai/nopsai-platform-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

func dockerfilesContain(t *testing.T, required string) bool {
	t.Helper()
	for _, path := range []string{
		"../Dockerfile",
		"../container/Dockerfile.aaa",
		"../container/Dockerfile.agent",
		"../container/Dockerfile.dispatcher",
		"../container/Dockerfile.docker-runner",
		"../container/Dockerfile.git-bot",
		"../container/Dockerfile.k8s-runner",
		"../container/Dockerfile.nopsai",
		"../container/Dockerfile.pipeline",
		"../container/Dockerfile.socket-proxy",
		"../services/ui/Dockerfile",
	} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(contents), required) {
			return true
		}
	}
	return false
}
