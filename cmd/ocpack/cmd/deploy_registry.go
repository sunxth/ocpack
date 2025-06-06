package cmd

import (
	"fmt"
	"ocpack/pkg/config"
	"ocpack/pkg/deploy"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

// deployRegistryCmd 表示 deploy registry 命令
var deployRegistryCmd = &cobra.Command{
	Use:   "deploy-registry",
	Short: "部署 Registry 节点",
	Long: `配置和部署 OpenShift 集群的 Registry 节点。
这将安装和配置 Registry 节点所需的所有软件和服务，包括：
- 基础软件包安装
- 容器运行时 (Podman)
- OpenShift 工具 (oc, kubectl, oc-mirror)
- Quay 镜像仓库

使用方式:
  ocpack deploy-registry demo`,
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

		// 获取下载目录
		downloadDir := filepath.Join(filepath.Dir(configPath), cfg.Download.LocalPath)

		// 验证 Registry 部署所需的配置和下载文件
		if err := config.ValidateRegistryConfigWithDownloads(cfg, downloadDir); err != nil {
			fmt.Printf("配置验证失败: %v\n", err)
			return
		}

		// 执行部署
		if err := deploy.DeployRegistry(cfg, configPath); err != nil {
			fmt.Printf("Registry 节点部署失败: %v\n", err)
			return
		}

		fmt.Println("Registry 节点部署成功！")
	},
}

func init() {
	rootCmd.AddCommand(deployRegistryCmd)
}
