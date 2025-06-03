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
	Use:   "download [cluster-name]",
	Short: "下载 OpenShift 安装所需的介质",
	Long: `下载 OpenShift 安装所需的介质，包括镜像、工具等。
这些文件将被下载到配置文件中指定的目录。

使用方式:
  # 使用配置文件路径
  ocpack download -c demo/config.toml
  
  # 使用集群名（约定配置文件位于 <集群名>/config.toml）
  ocpack download demo`,
	Args: cobra.MaximumNArgs(1), // 最多接受1个参数（集群名）
	Run: func(cmd *cobra.Command, args []string) {
		var configPath string
		var err error

		// 获取配置文件路径
		configFlag, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Printf("获取配置文件路径失败: %v\n", err)
			return
		}

		// 判断配置文件路径的来源
		if cmd.Flags().Changed("config") {
			// 如果用户明确指定了 -c 参数，使用指定的路径
			configPath = configFlag
		} else if len(args) > 0 {
			// 如果没有指定 -c 参数但提供了集群名，使用约定路径
			clusterName := args[0]
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

			configPath = filepath.Join(clusterDir, "config.toml")

			// 检查配置文件是否存在
			if _, err := os.Stat(configPath); os.IsNotExist(err) {
				fmt.Printf("配置文件不存在: %s\n", configPath)
				return
			}

			fmt.Printf("使用集群配置文件: %s\n", configPath)
		} else {
			// 既没有指定 -c 参数也没有提供集群名，使用默认值
			configPath = configFlag
		}

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
	downloadCmd.Flags().StringP("config", "c", "config.toml", "配置文件路径")
}
