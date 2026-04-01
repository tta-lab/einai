package main

import "github.com/tta-lab/einai/cmd"

var (
	version   = "dev"
	buildDate = "unknown"
	goVersion = "unknown"
)

func main() {
	cmd.SetBuildInfo(version, buildDate, goVersion)
	cmd.Execute()
}
