package cmd

import (
	"fmt"
	"github.com/spf13/cobra"
)

// 版本信息变量
var (
	version   string
	commit    string
	buildTime string
)

var rootCmd = &cobra.Command{
	Use:   "ocpack",
	Short: "ocpack 是用于离线环境中部署 OpenShift 集群的工具",
	Long: `
 ╔═══════════════════════════════════════════════════════════════════════════════╗
 ║                                                                               ║
 ║      ██████╗  ██████╗██████╗  █████╗  ██████╗██╗  ██╗                         ║
 ║     ██╔═══██╗██╔════╝██╔══██╗██╔══██╗██╔════╝██║ ██╔╝                         ║
 ║     ██║   ██║██║     ██████╔╝███████║██║     █████╔╝                          ║
 ║     ██║   ██║██║     ██╔═══╝ ██╔══██║██║     ██╔═██╗                          ║
 ║     ╚██████╔╝╚██████╗██║     ██║  ██║╚██████╗██║  ██╗                         ║
 ║      ╚═════╝  ╚═════╝╚═╝     ╚═╝  ╚═╝ ╚═════╝╚═╝  ╚═╝                         ║
 ║                                                                               ║
 ╚═══════════════════════════════════════════════════════════════════════════════╝`,
	// 禁用自动生成的completion命令
	CompletionOptions: cobra.CompletionOptions{
		DisableDefaultCmd: true,
	},
}

// versionCmd 版本命令
var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "显示版本信息",
	Long:  "显示 ocpack 的版本信息，包括版本号、提交哈希和构建时间",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("ocpack 版本信息:\n")
		fmt.Printf("  版本: %s\n", version)
		fmt.Printf("  提交: %s\n", commit)
		fmt.Printf("  构建时间: %s\n", buildTime)
	},
}

// SetVersionInfo 设置版本信息
func SetVersionInfo(v, c, bt string) {
	version = v
	commit = c
	buildTime = bt
	
	// 设置 root 命令的版本
	rootCmd.Version = fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, buildTime)
}

// Execute 执行根命令
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
	
	// 添加版本命令
	rootCmd.AddCommand(versionCmd)
}

func initConfig() {
	// 配置初始化逻辑可以在这里添加
} 