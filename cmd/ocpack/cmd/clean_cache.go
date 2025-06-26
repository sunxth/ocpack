package cmd

import (
	"encoding/json"
	"fmt"
	"os"

	"ocpack/pkg/config"
	"ocpack/pkg/mirror/wrapper"

	"github.com/spf13/cobra"
)

var cleanCacheCmd = &cobra.Command{
	Use:   "clean-cache [cluster-name]",
	Short: "Clean oc-mirror cache to free up disk space",
	Long: `Clean oc-mirror cache directories to free up disk space.

This command helps manage the disk space used by oc-mirror operations by:
- Cleaning cache directories in the cluster directory
- Showing cache size and location information
- Preventing the accumulation of cache files in $HOME/.oc-mirror

Examples:
  # Clean cache for a specific cluster
  ocpack clean-cache my-cluster

  # Show cache information without cleaning
  ocpack clean-cache my-cluster --info

  # Clean cache for the cluster specified in config.toml
  ocpack clean-cache --config config.toml`,
	RunE: runCleanCache,
}

var (
	cleanCacheConfigPath string
	showCacheInfo        bool
)

func init() {
	rootCmd.AddCommand(cleanCacheCmd)

	cleanCacheCmd.Flags().StringVarP(&cleanCacheConfigPath, "config", "c", "config.toml", "Path to configuration file")
	cleanCacheCmd.Flags().BoolVar(&showCacheInfo, "info", false, "Show cache information without cleaning")
}

func runCleanCache(cmd *cobra.Command, args []string) error {
	var clusterName string
	var cfg *config.ClusterConfig
	var err error

	// 确定集群名称和配置
	if len(args) > 0 {
		clusterName = args[0]
		// 如果提供了集群名称，尝试加载配置（可选）
		if _, err := os.Stat(cleanCacheConfigPath); err == nil {
			cfg, err = config.LoadConfig(cleanCacheConfigPath)
			if err != nil {
				fmt.Printf("⚠️  Warning: Failed to load config file, using cluster name only: %v\n", err)
				// 创建最小配置
				cfg = &config.ClusterConfig{}
				cfg.ClusterInfo.ClusterID = clusterName
			}
		} else {
			// 创建最小配置
			cfg = &config.ClusterConfig{}
			cfg.ClusterInfo.ClusterID = clusterName
		}
	} else {
		// 必须从配置文件加载
		cfg, err = config.LoadConfig(cleanCacheConfigPath)
		if err != nil {
			return fmt.Errorf("failed to load configuration: %v", err)
		}
		clusterName = cfg.ClusterInfo.ClusterID
		if clusterName == "" {
			return fmt.Errorf("cluster ID not found in configuration")
		}
	}

	// 创建镜像包装器
	wrapper, err := wrapper.NewMirrorWrapper("info")
	if err != nil {
		return fmt.Errorf("failed to create mirror wrapper: %v", err)
	}

	if showCacheInfo {
		// 显示缓存信息
		info, err := wrapper.GetCacheInfo(cfg, clusterName)
		if err != nil {
			return fmt.Errorf("failed to get cache info: %v", err)
		}

		fmt.Printf("📊 Cache Information for cluster: %s\n", clusterName)
		fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")

		if info["cache_exists"].(bool) {
			fmt.Printf("📁 Cache Directory: %s\n", info["cache_dir"])
			fmt.Printf("💾 Cache Size: %s\n", info["cache_size_human"])
			fmt.Printf("📅 Last Modified: %v\n", info["cache_modified"])
		} else {
			fmt.Printf("📁 Cache Directory: %s (not exists)\n", info["cache_dir"])
		}

		if info["workspace_exists"].(bool) {
			fmt.Printf("🗂️  Workspace Directory: %s\n", info["workspace_dir"])
			fmt.Printf("💾 Workspace Size: %s\n", info["workspace_size_human"])
			fmt.Printf("📅 Last Modified: %v\n", info["workspace_modified"])
		} else {
			fmt.Printf("🗂️  Workspace Directory: %s (not exists)\n", info["workspace_dir"])
		}

		// 输出 JSON 格式（便于脚本处理）
		if len(args) == 0 {
			jsonData, _ := json.MarshalIndent(info, "", "  ")
			fmt.Printf("\n📄 JSON Output:\n%s\n", jsonData)
		}
	} else {
		// 清理缓存
		fmt.Printf("🧹 Cleaning cache for cluster: %s\n", clusterName)

		err = wrapper.CleanCache(cfg, clusterName)
		if err != nil {
			return fmt.Errorf("failed to clean cache: %v", err)
		}

		fmt.Printf("\n💡 Tips:\n")
		fmt.Printf("   - Cache will be recreated automatically on next mirror operation\n")
		fmt.Printf("   - Regular cache cleaning helps maintain disk space\n")
		fmt.Printf("   - Use 'ocpack clean-cache --info' to check cache size\n")
	}

	return nil
}
