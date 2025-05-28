package cmd

import (
	"github.com/spf13/cobra"
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

// Execute 执行根命令
func Execute() error {
	return rootCmd.Execute()
}

func init() {
	cobra.OnInitialize(initConfig)
}

func initConfig() {
	// 配置初始化逻辑可以在这里添加
} 