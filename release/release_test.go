package release

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"nopsai"
	"nopsai/pkg/buildinfo"
	"nopsai/pkg/compatibility"
)

// version.txt is the one place a version is written, so the thing worth
// asserting is that it is an exact version and that everything downstream is
// still derived from it rather than copied beside it.
func TestVersionIsExactAndDrivesTheSeries(t *testing.T) {
	version := nopsai.Version()
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(version) {
		t.Fatalf("version.txt = %q, want an exact major.minor.patch version", version)
	}
	if want := version[:strings.LastIndex(version, ".")]; nopsai.Series() != want {
		t.Fatalf("Series() = %q, want %q", nopsai.Series(), want)
	}
	// A patch release must not move what an artifact is compatible with.
	if !strings.HasPrefix(nopsai.CompatibilityRange(), ">="+nopsai.Series()+".0 ") {
		t.Fatalf("CompatibilityRange() = %q, want it anchored at the series baseline", nopsai.CompatibilityRange())
	}
}

// The compatibility ranges are derived from the release series, so there is no
// longer a second copy for a test to hold in step. What remains worth asserting
// is that the derivation itself produces the range the linker stamps in.
func TestCompatibilityRangeFollowsTheReleaseSeries(t *testing.T) {
	series := nopsai.Series()
	if series == "" {
		t.Fatal("version.txt must name a release series")
	}
	want := ">=" + series + ".0 <" + nextMajorVersion(series)
	if got := nopsai.CompatibilityRange(); got != want {
		t.Fatalf("CompatibilityRange() = %q, want %q", got, want)
	}
	for name, got := range map[string]string{
		"platform": buildinfo.DefaultPlatformCompatibility,
		"CLI":      buildinfo.DefaultCLICompatibility,
		"runner":   buildinfo.DefaultRunnerCompatibility,
	} {
		if got != want {
			t.Errorf("default %s compatibility = %q, want %q", name, got, want)
		}
	}
}

// A release ships every component at one version, so the contract file must not
// reintroduce a per-component range for someone to forget.
func TestCompatibilityContractCarriesNoDerivedVersion(t *testing.T) {
	contents, err := os.ReadFile("compatibility.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"cliVersion", "platformCompatibility", "runnerCompatibility"} {
		if strings.Contains(string(contents), forbidden) {
			t.Errorf("compatibility.yaml declares %q, which is derived from version.txt", forbidden)
		}
	}
	contract := readCompatibilityContract(t)
	if len(contract.APICompatibility) == 0 || contract.RunnerProtocolVersion < 1 || len(contract.Capabilities) == 0 {
		t.Fatalf("contract = %#v", contract)
	}
}

func readCompatibilityContract(t *testing.T) compatibility.CompatibilityFile {
	t.Helper()
	file, err := os.Open("compatibility.yaml")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	contract, err := compatibility.DecodeCompatibility(file)
	if err != nil {
		t.Fatal(err)
	}
	return contract
}

func nextMajorVersion(series string) string {
	major := strings.SplitN(series, ".", 2)[0]
	return strconv.Itoa(mustAtoi(major)+1) + ".0.0"
}

func mustAtoi(value string) int {
	number, err := strconv.Atoi(value)
	if err != nil {
		panic(err)
	}
	return number
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
	helpersBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/_helpers.tpl")
	if err != nil {
		t.Fatal(err)
	}
	installer := string(installerBytes)
	values := string(valuesBytes)
	helpers := string(helpersBytes)
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
	for _, required := range []string{
		`releaseVersion: ""`,
		`tag: ""`,
		`define "nopsai.releaseVersion"`,
		"default .Chart.AppVersion $configured",
		`define "nopsai.imageWithDefaultTag"`,
		"default .defaultTag $image.tag",
	} {
		if !strings.Contains(values+"\n"+helpers, required) {
			t.Errorf("Helm chart is missing release-version image defaulting contract %q", required)
		}
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

func TestHelmChartSeparatesAPIReadinessAndLiveness(t *testing.T) {
	apiTemplateBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	apiTemplate := string(apiTemplateBytes)
	for _, required := range []string{
		"readinessProbe: {httpGet: {path: /healthz, port: http}",
		"livenessProbe: {httpGet: {path: /livez, port: http}",
	} {
		if !strings.Contains(apiTemplate, required) {
			t.Errorf("api.yaml is missing probe contract %q", required)
		}
	}
	if strings.Contains(apiTemplate, "livenessProbe: {httpGet: {path: /healthz, port: http}") {
		t.Error("api.yaml must not use readiness healthz for liveness")
	}
}

func TestHelmChartEnablesAuthenticatedMetricsForProduction(t *testing.T) {
	valuesBytes, err := os.ReadFile("../deploy/helm/nopsai/values.yaml")
	if err != nil {
		t.Fatal(err)
	}
	apiTemplateBytes, err := os.ReadFile("../deploy/helm/nopsai/templates/api.yaml")
	if err != nil {
		t.Fatal(err)
	}
	readmeBytes, err := os.ReadFile("../deploy/helm/nopsai/README.md")
	if err != nil {
		t.Fatal(err)
	}
	values := string(valuesBytes)
	apiTemplate := string(apiTemplateBytes)
	readme := string(readmeBytes)
	for _, required := range []string{
		"metricsRequireAuth: true",
		"METRICS_REQUIRE_AUTH",
		".Values.api.metricsRequireAuth",
		"`api.metricsRequireAuth` defaults to `true`",
	} {
		if !strings.Contains(values+"\n"+apiTemplate+"\n"+readme, required) {
			t.Errorf("Helm chart is missing authenticated metrics contract %q", required)
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
		"nopsai install kubernetes",
		"--secret-file nopsai-secrets.yaml",
		"Review `./nopsai-prod/installation.md`",
		"kubectl apply -f nopsai-secrets.yaml",
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
	if strings.Contains(readme, "--from-literal") {
		t.Error("Helm README should not document manual plaintext Secret generation")
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
		"dockerHost: \"\"",
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
		"SYSTEM_LOGS_DOCKER_HOST",
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
		"nopsai/pkg/buildinfo.APIVersion=${API_VERSION}",
		`asset="nopsai-cli_${VERSION}_${goos}_${goarch}"`,
		"shopt -s nullglob",
		"cli_assets=(dist/cli/*)",
		"No CLI release artifacts were built into dist/cli",
		`helm push "dist/release/nopsai-$VERSION.tgz" "$chart_repository" 2>&1`,
		"cp scripts/install-release-tools.sh dist/release/install-release-tools.sh",
		". dist/release/install-release-tools.sh",
		`oras_chart_reference="${chart_reference#oci://}"`,
		`oras copy "$oras_chart_reference:$VERSION" "$oras_chart_reference:$release_tag"`,
		`grep -Eo 'sha256:[a-f0-9]{64}'`,
		`tail -1 || true`,
		"apk add --no-cache bash coreutils perl-utils tar gzip",
		"helm_chart_asset=\"nopsai-helm-chart-$VERSION.tgz\"",
		"changelog_asset=\"nopsai-changelog-$VERSION.md\"",
		`cp "${cli_assets[@]}" dist/assets/`,
		`cp "dist/release/nopsai-$VERSION.tgz" "dist/assets/$helm_chart_asset"`,
		"publish-cli-package",
		"RELEASE_PHASE=publish-cli-package",
		`cli_package_reference="${NOPSAI_RELEASE_CLI_PACKAGE:-$REGISTRY/nopsai-cli}"`,
		"mkdir -p dist/cli-package",
		"cp dist/assets/SHA256SUMS dist/cli-package/SHA256SUMS",
		"--artifact-type application/vnd.nopsai.cli.release.v1",
		`"SHA256SUMS:text/plain"`,
		"cd dist/cli-package",
		`oras push "$cli_package_reference:$VERSION" "${oras_args[@]}"`,
		`oras copy "$cli_package_reference:$VERSION" "$cli_package_reference:$release_tag"`,
		`published_cli_package_alias=$cli_package_reference:$release_tag`,
		"apk add --no-cache bash curl git tar gzip",
		"legacy_assets=(",
		`gh release delete-asset "v$VERSION" "$asset"`,
		`update_release "v$VERSION" --title "NopsAI $VERSION" --latest`,
		`create_release "v$VERSION" true --title "NopsAI $VERSION" --latest`,
		`gh release edit "$release_tag" --repo "$GITHUB_REPOSITORY" "${edit_args[@]}" --notes-file dist/release/CHANGELOG.md`,
		`gh release create "$release_tag" dist/assets/* --repo "$GITHUB_REPOSITORY" --target "$SOURCE_COMMIT" "$@" --notes-file dist/release/CHANGELOG.md`,
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
		"cat >dist/release/install-release-tools.sh",
	} {
		if strings.Contains(pipeline, forbidden) {
			t.Errorf("NopsAI platform release pipeline should not contain %q", forbidden)
		}
	}
}

func TestReleaseToolInstallerIsCheckedInAndVerified(t *testing.T) {
	pipeline := readNopsAIReleasePipeline(t)
	installerBytes, err := os.ReadFile("../scripts/install-release-tools.sh")
	if err != nil {
		t.Fatal(err)
	}
	installer := string(installerBytes)
	if !strings.Contains(pipeline, "cp scripts/install-release-tools.sh dist/release/install-release-tools.sh") {
		t.Fatal("release pipeline must copy the checked-in release tool installer")
	}
	for _, required := range []string{
		"verify_release_download helm",
		"verify_release_download oras",
		"verify_release_download gh",
		"extract_release_archive",
		"tar --no-same-owner",
		"sha256sum -c -",
		"NOPSAI_RELEASE_HELM_SHA256_AMD64",
		"NOPSAI_RELEASE_ORAS_SHA256_AMD64",
		"NOPSAI_RELEASE_GH_SHA256_AMD64",
	} {
		if !strings.Contains(installer, required) {
			t.Errorf("release tool installer is missing %q", required)
		}
	}
}

func TestReleasePipelineInjectsCompatibilityContract(t *testing.T) {
	// The image build args live in the checked-in build scripts the pipeline
	// calls, so the contract is asserted across the pipeline and those scripts.
	pipeline := readNopsAIReleasePath(t)
	for _, required := range []string{
		"release/compatibility.yaml",
		`emit "CLI_COMPATIBILITY", series_range`,
		`emit "PLATFORM_COMPATIBILITY", series_range`,
		`emit "RUNNER_COMPATIBILITY", series_range`,
		`series = File.read("version.txt").strip`,
		`printf '%s=%q\n' "$key" "$value"`,
		"--build-arg \"API_VERSION=$API_VERSION\"",
		"--build-arg \"RUNNER_PROTOCOL_VERSION=$RUNNER_PROTOCOL_VERSION\"",
		"--build-arg \"CLI_COMPATIBILITY=$CLI_COMPATIBILITY\"",
		"--build-arg \"RUNNER_COMPATIBILITY=$RUNNER_COMPATIBILITY\"",
		"--build-arg \"PLATFORM_COMPATIBILITY=$PLATFORM_COMPATIBILITY\"",
		"--build-arg \"CAPABILITIES=$CAPABILITIES\"",
		"nopsai/pkg/buildinfo.RunnerProtocolVersion=${RUNNER_PROTOCOL_VERSION}",
		"nopsai/pkg/buildinfo.CLICompatibility=${CLI_COMPATIBILITY}",
		"nopsai/pkg/buildinfo.RunnerCompatibility=${RUNNER_COMPATIBILITY}",
		"nopsai/pkg/buildinfo.PlatformCompatibility=${PLATFORM_COMPATIBILITY}",
		"nopsai/pkg/buildinfo.Capabilities=${CAPABILITIES}",
	} {
		if !strings.Contains(pipeline, required) {
			t.Errorf("NopsAI platform release pipeline is missing compatibility contract %q", required)
		}
	}
	for _, forbidden := range []string{
		"nopsai/pkg/buildinfo.CLICompatibility=>=",
		"nopsai/pkg/buildinfo.RunnerCompatibility=>=",
		"nopsai/pkg/buildinfo.PlatformCompatibility=>=",
		`capabilities="api.v1,cli.api-catalog.v1`,
	} {
		if strings.Contains(pipeline, forbidden) {
			t.Errorf("NopsAI platform release pipeline embeds stale compatibility contract %q", forbidden)
		}
	}
}

func TestReleasePipelinePublishesStableContainerTagAliases(t *testing.T) {
	pipeline := readNopsAIReleasePath(t)
	for _, required := range []string{
		`while IFS= read -r release_tag; do`,
		`release_tag_args+=(--tag "$REGISTRY/$image_name:$release_tag")`,
		`release_tag_args+=(--tag "$REGISTRY/nopsai-base:$release_tag")`,
		`done < <(scripts/release-tags.sh "$VERSION")`,
		`"${release_tag_args[@]}"`,
		`--annotation "index,manifest:org.opencontainers.image.source=$SOURCE_URL"`,
		`--annotation "index,manifest:org.opencontainers.image.title=$image_name"`,
		`--annotation "index,manifest:org.opencontainers.image.title=nopsai-base"`,
		`"${oci_annotation_args[@]}"`,
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
	contents, err := os.ReadFile("nopsai-platform-release.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return string(contents)
}

// readNopsAIReleasePath returns the pipeline plus the checked-in scripts it
// calls, because release logic is allowed to live in either place.
func readNopsAIReleasePath(t *testing.T) string {
	t.Helper()
	combined := readNopsAIReleasePipeline(t)
	for _, path := range []string{
		"../scripts/build-release-base-image.sh",
		"../scripts/publish-release-image.sh",
		"../scripts/install-release-tools.sh",
		"../scripts/release-tags.sh",
	} {
		contents, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("os.ReadFile(%q) error = %v", path, err)
		}
		combined += "\n" + string(contents)
	}
	return combined
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

// A credential inside a publicly pullable image is permanently public, so the
// release must not be able to publish a discoverable version without scanning
// both the source tree and the images it just pushed.
func TestReleasePipelineScansForCredentialsBeforePublication(t *testing.T) {
	pipeline := readNopsAIReleasePipeline(t)
	for _, required := range []string{
		"- name: scan-published-images",
		"scripts/secret-scan.sh --image \"$reference\"",
		"- scan-published-images",
	} {
		if !strings.Contains(pipeline, required) {
			t.Errorf("release pipeline is missing credential scan contract %q", required)
		}
	}

	gates, err := os.ReadFile("../scripts/enterprise-gates.sh")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gates), "run scripts/secret-scan.sh") {
		t.Error("enterprise gates must run the credential scan so it gates local and CI runs, not only releases")
	}

	scan, err := os.ReadFile("../scripts/secret-scan.sh")
	if err != nil {
		t.Fatal(err)
	}
	// The counter is only meaningful when the scan loop runs in the current
	// shell; a pipeline would discard every finding and exit clean.
	if strings.Contains(string(scan), "| scan_stream") {
		t.Error("secret scan must not pipe into scan_stream, because the finding count is lost to the subshell")
	}
	// The scan must grep the whole file list once per pattern. Grepping each
	// file for each pattern spawned a subshell per combination -- about
	// nineteen thousand on this repository -- and killed the shell with a bus
	// error partway through, which reported a clean scan for a dirty tree.
	if !strings.Contains(string(scan), "scan_file_list") {
		t.Error("secret scan must scan the file list per pattern rather than per file")
	}
	if strings.Contains(string(scan), `grep -nEI -- "$regex" "$base/$entry"`) {
		t.Error("secret scan must not grep once per file per pattern; that form exhausts the shell on a repository this size")
	}
	// -H keeps the filename prefix when xargs hands grep a single-file batch.
	// Without it a finding can be reported with no path at all.
	if !strings.Contains(string(scan), "grep -nEIH") {
		t.Error("secret scan must force filename prefixes so no finding is reported without its path")
	}
	for _, required := range []string{"findings=$((findings + 1))", "exit 1"} {
		if !strings.Contains(string(scan), required) {
			t.Errorf("secret scan is missing fail-closed contract %q", required)
		}
	}
}

// THIRD_PARTY_NOTICES.md is a notice index that requires a release-specific
// bundle before external distribution, so the release must not be able to
// publish an artifact that does not carry one.
func TestReleasePipelineGeneratesThirdPartyNoticeBundle(t *testing.T) {
	pipeline := readNopsAIReleasePipeline(t)
	for _, required := range []string{
		"- name: release-notices",
		"scripts/generate-notices.sh",
		`--output "dist/notices/THIRD-PARTY-NOTICES-$VERSION.md"`,
		"npm ci --omit=dev",
	} {
		if !strings.Contains(pipeline, required) {
			t.Errorf("release pipeline is missing notice bundle contract %q", required)
		}
	}

	// Every distributed artifact family must wait for the bundle.
	for _, consumer := range []string{"publish-base-image", "package-helm-chart", "build-cli-archives", "package-release-assets"} {
		index := strings.Index(pipeline, "- name: "+consumer)
		if index < 0 {
			t.Fatalf("release pipeline has no %q step", consumer)
		}
		window := pipeline[index:]
		if next := strings.Index(window[1:], "\n  - name: "); next >= 0 {
			window = window[:next]
		}
		if !strings.Contains(window, "- release-notices") {
			t.Errorf("step %q must depend on release-notices so the artifact carries the bundle", consumer)
		}
	}

	generator, err := os.ReadFile("../scripts/generate-notices.sh")
	if err != nil {
		t.Fatal(err)
	}
	// A component with no reproducible licence text cannot lawfully be shipped,
	// so the generator must fail rather than emit an incomplete bundle.
	for _, required := range []string{"missing=$((missing + 1))", "Notice bundle incomplete", "exit 1"} {
		if !strings.Contains(string(generator), required) {
			t.Errorf("notice generator is missing fail-closed contract %q", required)
		}
	}
}
