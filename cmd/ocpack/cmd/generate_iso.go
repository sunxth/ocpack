package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	
	"github.com/spf13/cobra"
	"ocpack/pkg/iso"
)

var generateIsoCmd = &cobra.Command{
	Use:     "generate-iso",
	Short:   "生成 OpenShift 安装 ISO 镜像",
	Long: `generate-iso 命令用于生成用于 OpenShift 集群安装的 ISO 镜像文件。

此命令将执行以下操作：
1. 读取集群配置文件
2. 生成 ignition 配置文件
3. 创建自定义 ISO 镜像
4. 集成 registry 证书和配置

使用方式:
  ocpack generate-iso demo`,
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
		
		fmt.Printf("开始为集群生成 ISO 镜像: %s\n", clusterName)
		
		// 创建 ISO 生成器
		generator, err := iso.NewISOGenerator(clusterName, projectRoot)
		if err != nil {
			return fmt.Errorf("创建 ISO 生成器失败: %v", err)
		}
		
		// 获取命令行参数
		outputPath, _ := cmd.Flags().GetString("output")
		baseISOPath, _ := cmd.Flags().GetString("base-iso")
		nodeType, _ := cmd.Flags().GetString("node-type")
		bootstrapOnly, _ := cmd.Flags().GetBool("bootstrap-only")
		masterOnly, _ := cmd.Flags().GetBool("master-only")
		workerOnly, _ := cmd.Flags().GetBool("worker-only")
		
		// 构建生成选项
		options := &iso.GenerateOptions{
			OutputPath:    outputPath,
			BaseISOPath:   baseISOPath,
			NodeType:      iso.NodeType(nodeType),
			BootstrapOnly: bootstrapOnly,
			MasterOnly:    masterOnly,
			WorkerOnly:    workerOnly,
		}
		
		// 执行 ISO 生成
		if err := generator.GenerateISO(options); err != nil {
			return fmt.Errorf("ISO 生成失败: %v", err)
		}
		
		fmt.Println("ISO 生成完成!")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(generateIsoCmd)
	
	// 添加命令行参数
	generateIsoCmd.Flags().StringP("output", "o", "", "指定输出 ISO 文件路径 (可选，默认在集群目录下)")
	generateIsoCmd.Flags().StringP("base-iso", "b", "", "指定基础 ISO 文件路径 (可选，默认从 downloads 目录读取)")
	generateIsoCmd.Flags().BoolP("bootstrap-only", "", false, "仅生成 bootstrap 节点的 ISO")
	generateIsoCmd.Flags().BoolP("master-only", "", false, "仅生成 master 节点的 ISO")
	generateIsoCmd.Flags().BoolP("worker-only", "", false, "仅生成 worker 节点的 ISO")
	generateIsoCmd.Flags().StringP("node-type", "t", "all", "指定节点类型 (all, bootstrap, master, worker)")
} 