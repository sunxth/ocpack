package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"ocpack/pkg/config"
	"ocpack/pkg/download"

	"github.com/spf13/cobra"
)

// downloadCmd 表示 download 命令
var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "下载 OpenShift 安装所需的介质",
	Long: `下载 OpenShift 安装所需的介质，包括镜像、工具等。
这些文件将被下载到配置文件中指定的目录。

使用方式:
  ocpack download demo`,
	Args: cobra.ExactArgs(1), // 必须提供一个集群名参数
	Run: func(cmd *cobra.Command, args []string) {
		clusterName := args[0]

		// 获取当前工作目录作为项目根目录
		projectRoot, err := os.Getwd()
		if err != nil {
			fmt.Printf("获取当前目录失败: %v\n", err)
			return
		}

		// 检查集群目录是否存在
		clusterDir := filepath.Join(projectRoot, clusterName)
		if _, err := os.Stat(clusterDir); os.IsNotExist(err) {
			fmt.Printf("集群目录不存在: %s\n", clusterDir)
			return
		}

		configPath := filepath.Join(clusterDir, "config.toml")

		// 检查配置文件是否存在
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			fmt.Printf("配置文件不存在: %s\n", configPath)
			return
		}

		fmt.Printf("使用集群配置文件: %s\n", configPath)

		// 加载配置
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			fmt.Printf("加载配置失败: %v\n", err)
			return
		}

		// 验证下载所需的配置
		if err := config.ValidateDownloadConfig(cfg); err != nil {
			fmt.Printf("配置验证失败: %v\n", err)
			return
		}

		// 创建下载目录
		downloadDir := filepath.Join(filepath.Dir(configPath), cfg.Download.LocalPath)
		fmt.Printf("将下载文件保存到: %s\n", downloadDir)

		// 执行下载
		downloader := download.NewDownloader(cfg, downloadDir)
		if err := downloader.DownloadAll(); err != nil {
			fmt.Printf("下载失败: %v\n", err)
			return
		}

		fmt.Println("所有文件下载完成！")
	},
}

func init() {
	rootCmd.AddCommand(downloadCmd)
}
