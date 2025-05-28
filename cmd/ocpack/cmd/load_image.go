package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/spf13/cobra"
	"ocpack/pkg/loadimage"
)

var loadImageCmd = &cobra.Command{
	Use:     "load-image",
	Short:   "将镜像加载到 mirror registry",
	Long: `load-image 命令用于将下载的 OpenShift 镜像加载到 mirror registry 中。

此命令将执行以下操作：
1. 读取集群配置文件
2. 连接到 mirror registry
3. 将镜像推送到 registry
4. 生成镜像清单文件

使用方式:
  ocpack load-image demo`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		clusterName := args[0]
		
		// 获取当前工作目录作为项目根目录
		projectRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %v", err)
		}
		
		// 检查集群目录是否存在
		clusterDir := filepath.Join(projectRoot, clusterName)
		if _, err := os.Stat(clusterDir); os.IsNotExist(err) {
			return fmt.Errorf("集群目录不存在: %s", clusterDir)
		}
		
		fmt.Printf("开始加载镜像到集群: %s\n", clusterName)
		
		// 创建镜像加载器
		loader, err := loadimage.NewImageLoader(clusterName, projectRoot)
		if err != nil {
			return fmt.Errorf("创建镜像加载器失败: %v", err)
		}
		
		// 执行镜像加载
		if err := loader.LoadImages(); err != nil {
			return fmt.Errorf("镜像加载失败: %v", err)
		}
		
		fmt.Println("镜像加载完成!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loadImageCmd)
	
	// 添加命令行参数
	loadImageCmd.Flags().StringP("registry-url", "r", "", "指定 registry URL (可选，默认从配置文件读取)")
	loadImageCmd.Flags().StringP("username", "u", "", "registry 用户名 (可选，默认从配置文件读取)")
	loadImageCmd.Flags().StringP("password", "p", "", "registry 密码 (可选，默认从配置文件读取)")
	loadImageCmd.Flags().BoolP("skip-tls-verify", "k", false, "跳过 TLS 证书验证")
	loadImageCmd.Flags().StringP("image-set", "i", "", "指定要加载的镜像集 (可选，默认加载所有)")
} 