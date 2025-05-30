package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"ocpack/pkg/iso"

	"github.com/spf13/cobra"
)

var generateISOCmd = &cobra.Command{
	Use:   "generate-iso",
	Short: "生成 OpenShift 安装 ISO 镜像",
	Long: `generate-iso 命令用于生成 OpenShift 集群的安装 ISO 镜像。

此命令将执行以下操作：
1. 验证集群配置和依赖工具
2. 创建安装目录结构
3. 生成 install-config.yaml 配置文件
4. 生成 agent-config.yaml 配置文件
5. 使用 openshift-install 生成 agent ISO 镜像

生成的文件结构：
  installation/
  ├── install-config.yaml
  ├── agent-config.yaml
  ├── ignition/
  │   └── [ignition files]
  └── iso/
      └── [generated ISO files]

注意: 在运行此命令之前，请确保：
- 已运行 'ocpack download' 命令下载必要工具
- pull-secret.txt 文件存在
- 集群配置文件已正确填写

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

		fmt.Printf("开始为集群 %s 生成 ISO 镜像\n", clusterName)

		// 创建 ISO 生成器
		generator, err := iso.NewISOGenerator(clusterName, projectRoot)
		if err != nil {
			return fmt.Errorf("创建 ISO 生成器失败: %v", err)
		}

		// 获取命令行选项
		outputPath, _ := cmd.Flags().GetString("output")
		baseISOPath, _ := cmd.Flags().GetString("base-iso")
		skipVerify, _ := cmd.Flags().GetBool("skip-verify")

		// 构建生成选项
		options := &iso.GenerateOptions{
			OutputPath:    outputPath,
			BaseISOPath:   baseISOPath,
			SkipVerify:    skipVerify,
		}

		// 执行 ISO 生成
		if err := generator.GenerateISO(options); err != nil {
			return fmt.Errorf("ISO 生成失败: %v", err)
		}

		fmt.Println("ISO 生成完成!")
		fmt.Printf("📁 安装文件位置: %s/installation/\n", clusterDir)
		fmt.Printf("💿 ISO 文件位置: %s/installation/iso/\n", clusterDir)
		fmt.Printf("🔧 Ignition 文件位置: %s/installation/ignition/\n", clusterDir)
		
		return nil
	},
}

func init() {
	rootCmd.AddCommand(generateISOCmd)

	// 添加命令行参数
	generateISOCmd.Flags().StringP("output", "o", "", "指定输出目录 (可选)")
	generateISOCmd.Flags().StringP("base-iso", "b", "", "指定基础 ISO 路径 (可选)")
	generateISOCmd.Flags().BoolP("skip-verify", "", false, "跳过镜像验证步骤")
} 