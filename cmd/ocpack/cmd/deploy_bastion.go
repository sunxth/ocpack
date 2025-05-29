package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"ocpack/pkg/config"
	"ocpack/pkg/deploy"

	"github.com/spf13/cobra"
)

// deployBastionCmd 表示 deploy bastion 命令
var deployBastionCmd = &cobra.Command{
	Use:   "deploy-bastion [cluster-name]",
	Short: "部署 Bastion 节点",
	Long: `配置和部署 OpenShift 集群的 Bastion 节点。

这将安装和配置 Bastion 节点所需的所有软件和服务。

使用方式:
  # 使用配置文件路径
  ocpack deploy-bastion -c demo/config.toml
  
  # 使用集群名（约定配置文件位于 <集群名>/config.toml）
  ocpack deploy-bastion demo`,
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
