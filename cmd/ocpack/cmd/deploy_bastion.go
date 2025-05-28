package cmd

import (
	"fmt"
	"path/filepath"

	"ocpack/pkg/config"
	"ocpack/pkg/deploy"
	"github.com/spf13/cobra"
)

// deployBastionCmd 表示 deploy bastion 命令
var deployBastionCmd = &cobra.Command{
	Use:     "deploy-bastion",
	Short:   "部署 Bastion 节点",
	Long: `配置和部署 OpenShift 集群的 Bastion 节点。

这将安装和配置 Bastion 节点所需的所有软件和服务。

使用方式:
  ocpack deploy-bastion -c demo/config.toml`,
	Run: func(cmd *cobra.Command, args []string) {
		// 获取配置文件路径
		configPath, err := cmd.Flags().GetString("config")
		if err != nil {
			fmt.Printf("获取配置文件路径失败: %v\n", err)
			return
		}

		// 加载配置
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			fmt.Printf("加载配置失败: %v\n", err)
			return
		}

		// 验证 Bastion 部署所需的配置
		if err := config.ValidateBastionConfig(cfg); err != nil {
			fmt.Printf("配置验证失败: %v\n", err)
			return
		}

		// 获取下载目录
		downloadDir := filepath.Join(filepath.Dir(configPath), cfg.Download.LocalPath)

		// 创建部署器
		deployer := deploy.NewBastionDeployer(cfg, downloadDir)

		// 执行部署
		fmt.Println("开始部署 Bastion 节点...")
		if err := deployer.Deploy(configPath); err != nil {
			fmt.Printf("Bastion 节点部署失败: %v\n", err)
			return
		}

		fmt.Println("Bastion 节点部署成功！")
	},
}

func init() {
	rootCmd.AddCommand(deployBastionCmd)
	deployBastionCmd.Flags().StringP("config", "c", "config.toml", "配置文件路径")
} 