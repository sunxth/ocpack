package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"ocpack/pkg/config"
	"ocpack/pkg/mirror/wrapper"

	"github.com/spf13/cobra"
)

var loadImageCmd = &cobra.Command{
	Use:   "load-image",
	Short: "从本地磁盘加载镜像到 mirror registry",
	Long: `load-image 命令将已保存到本地磁盘的 OpenShift 镜像加载到 mirror registry 中。

此命令将执行以下操作：
1. 读取集群配置文件
2. 验证本地镜像目录是否存在
3. 将镜像推送到 registry

注意: 在运行此命令之前，请确保：
- 已运行 'ocpack save-image' 命令保存镜像
- Registry 已正确部署并运行

使用方式:
  ocpack load-image demo`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		clusterName := args[0]

		// 获取命令行参数
		logLevel, _ := cmd.Flags().GetString("log-level")
		dryRun, _ := cmd.Flags().GetBool("dry-run")
		enableRetry, _ := cmd.Flags().GetBool("enable-retry")
		maxRetries, _ := cmd.Flags().GetInt("max-retries")
		retryInterval, _ := cmd.Flags().GetInt("retry-interval")

		// 获取当前工作目录作为项目根目录
		projectRoot, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("获取当前目录失败: %v", err)
		}

		// 构建配置文件路径
		configPath := filepath.Join(projectRoot, clusterName, "config.toml")

		// 检查配置文件是否存在
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			return fmt.Errorf("配置文件不存在: %s", configPath)
		}

		// 检查镜像数据是否存在
		imagesPath := filepath.Join(projectRoot, clusterName, "images")
		if _, err := os.Stat(imagesPath); os.IsNotExist(err) {
			return fmt.Errorf("镜像数据不存在: %s\n请先运行 'ocpack save-image %s'", imagesPath, clusterName)
		}

		fmt.Printf("🔄 开始从本地磁盘加载镜像到 registry: %s\n", clusterName)
		fmt.Printf("⚙️  配置文件: %s\n", configPath)
		if dryRun {
			fmt.Printf("🔍 干运行模式: 只显示操作而不实际执行\n")
		}

		// 读取配置
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			return fmt.Errorf("读取配置文件失败: %v", err)
		}

		// 创建镜像包装器
		mirrorWrapper, err := wrapper.NewMirrorWrapper(logLevel)
		if err != nil {
			return fmt.Errorf("创建镜像服务失败: %v", err)
		}

		// 设置选项
		opts := &wrapper.MirrorOptions{
			ClusterName:   clusterName,
			ConfigPath:    configPath,
			Port:          55000, // 使用默认端口
			DryRun:        dryRun,
			Force:         false,
			EnableRetry:   enableRetry,
			MaxRetries:    maxRetries,
			RetryInterval: retryInterval,
		}

		// 构建目标仓库地址
		registryHost := fmt.Sprintf("registry.%s.%s:8443", cfg.ClusterInfo.ClusterID, cfg.ClusterInfo.Domain)
		destination := fmt.Sprintf("docker://%s", registryHost)
		source := fmt.Sprintf("file://%s", imagesPath)

		// 执行磁盘到仓库操作
		err = mirrorWrapper.DiskToMirror(cfg, source, destination, opts)
		if err != nil {
			return fmt.Errorf("镜像加载失败: %v", err)
		}

		if dryRun {
			fmt.Printf("✅ 干运行完成！实际操作请移除 --dry-run 参数\n")
		} else {
			fmt.Printf("✅ 镜像加载完成！目标仓库: %s\n", registryHost)
			fmt.Printf("📋 集群资源配置文件已生成在: %s/images/working-dir/cluster-resources/\n", clusterName)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(loadImageCmd)

	// 保留基本和有用的参数
	loadImageCmd.Flags().String("log-level", "info", "日志级别 (info, debug, error)")
	loadImageCmd.Flags().Bool("dry-run", false, "只显示操作而不实际执行")
	loadImageCmd.Flags().Bool("enable-retry", false, "启用重试机制")
	loadImageCmd.Flags().Int("max-retries", 3, "最大重试次数")
	loadImageCmd.Flags().Int("retry-interval", 5, "重试间隔时间（秒）")
}
