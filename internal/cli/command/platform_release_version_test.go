package command

import (
	"testing"

	"nopsai/pkg/buildinfo"
)

func TestReleasedCLIUsesBuildVersionAsPlatformDefault(t *testing.T) {
	root := &rootOptions{dependencies: Dependencies{BuildInfo: buildinfo.Info{Version: commandTestVersion}}}
	command := newPlatformReleaseCommand(root)
	flag := command.Flags().Lookup("version")
	if flag == nil || flag.DefValue != commandTestVersion {
		t.Fatalf("platform version default = %#v", flag)
	}

	root.dependencies.BuildInfo.Version = "dev"
	developmentCommand := newPlatformReleaseCommand(root)
	developmentFlag := developmentCommand.Flags().Lookup("version")
	if developmentFlag == nil || developmentFlag.DefValue != "" {
		t.Fatalf("development platform version default = %#v", developmentFlag)
	}

	root.dependencies.Version = commandOtherCompatibleVersion
	fallbackCommand := newInstallDockerComposeCommand(root)
	fallbackFlag := fallbackCommand.Flags().Lookup("version")
	if fallbackFlag == nil || fallbackFlag.DefValue != commandOtherCompatibleVersion {
		t.Fatalf("install version fallback default = %#v", fallbackFlag)
	}
}
