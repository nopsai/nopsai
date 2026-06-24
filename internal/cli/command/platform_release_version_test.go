package command

import (
	"testing"

	"nopsai/pkg/buildinfo"
)

func TestReleasedCLIUsesBuildVersionAsPlatformDefault(t *testing.T) {
	root := &rootOptions{dependencies: Dependencies{BuildInfo: buildinfo.Info{Version: "2.7.184"}}}
	command := newPlatformReleaseCommand(root)
	flag := command.Flags().Lookup("version")
	if flag == nil || flag.DefValue != "2.7.184" {
		t.Fatalf("platform version default = %#v", flag)
	}

	root.dependencies.BuildInfo.Version = "dev"
	developmentCommand := newPlatformReleaseCommand(root)
	developmentFlag := developmentCommand.Flags().Lookup("version")
	if developmentFlag == nil || developmentFlag.DefValue != "" {
		t.Fatalf("development platform version default = %#v", developmentFlag)
	}
}
