package main

import (
	"fmt"
	"os"

	"ocpack/cmd/ocpack/cmd"
)

// 版本信息变量，在构建时通过 ldflags 注入
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	// 设置版本信息到 root 命令
	cmd.SetVersionInfo(Version, Commit, BuildTime)
	
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
} 