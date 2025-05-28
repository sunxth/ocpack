package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"ocpack/pkg/config"
	"github.com/spf13/cobra"
)

// newCmd 表示 new 命令
var newCmd = &cobra.Command{
	Use:   "new cluster [集群名称]",
	Short: "创建一个新的集群配置",
	Long: `创建一个新的 OpenShift 集群配置目录和初始配置文件。

例如:
  ocpack new cluster my-cluster`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		if args[0] != "cluster" {
			fmt.Println("目前只支持 'ocpack new cluster [集群名称]' 命令")
			return
		}
		clusterName := args[1]
		createNewCluster(clusterName)
	},
}

func init() {
	rootCmd.AddCommand(newCmd)
}

// createNewCluster 创建新的集群目录和配置文件
func createNewCluster(clusterName string) {
	// 1. 创建集群目录
	clusterDir := clusterName
	if err := os.MkdirAll(clusterDir, 0755); err != nil {
		fmt.Printf("创建集群目录失败: %v\n", err)
		return
	}

	// 2. 生成并保存默认配置文件
	configPath := filepath.Join(clusterDir, "config.toml")
	if err := config.GenerateDefaultConfig(configPath, clusterName); err != nil {
		fmt.Printf("生成配置文件失败: %v\n", err)
		return
	}

	// 3. 创建子目录结构
	dirsToCreate := []string{
		filepath.Join(clusterDir, "downloads"),
		filepath.Join(clusterDir, "bastion"),
		filepath.Join(clusterDir, "registry"),
	}

	for _, dir := range dirsToCreate {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("创建目录 %s 失败: %v\n", dir, err)
			return
		}
	}

	fmt.Printf("集群 '%s' 初始化成功！\n", clusterName)
	fmt.Printf("请编辑配置文件: %s\n", configPath)
} 