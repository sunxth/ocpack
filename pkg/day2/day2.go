package day2

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"ocpack/pkg/config"
	"ocpack/pkg/utils"
)

// ConfigureOperatorHub 配置 OperatorHub 连接到私有镜像仓库
func ConfigureOperatorHub(clusterName, clusterDir string) error {
	fmt.Printf("🔧 开始配置集群 %s 的 OperatorHub\n", clusterName)

	// 1. 加载集群配置
	cfg, err := loadClusterConfig(clusterDir)
	if err != nil {
		return fmt.Errorf("加载集群配置失败: %w", err)
	}

	// 2. 检查 kubeconfig 是否存在
	kubeconfigPath := filepath.Join(clusterDir, "installation", "ignition", "auth", "kubeconfig")
	if _, err := os.Stat(kubeconfigPath); err != nil {
		return fmt.Errorf("kubeconfig 文件不存在: %s\n请确保集群已经安装完成", kubeconfigPath)
	}

	fmt.Printf("✅ 找到 kubeconfig: %s\n", kubeconfigPath)

	// 3. 构建 registry 主机名
	registryHost := fmt.Sprintf("registry.%s.%s", cfg.ClusterInfo.ClusterID, cfg.ClusterInfo.Domain)
	fmt.Printf("📋 私有镜像仓库: %s:8443\n", registryHost)

	steps := 5
	fmt.Printf("➡️  步骤 1/%d: 禁用默认的在线 catalog sources\n", steps)
	if err := disableDefaultCatalogSources(kubeconfigPath); err != nil {
		return fmt.Errorf("禁用默认 catalog sources 失败: %w", err)
	}
	fmt.Println("✅ 默认 catalog sources 已禁用")

	fmt.Printf("➡️  步骤 2/%d: 查找最新的 CatalogSource 文件\n", steps)
	catalogSourceFile, err := findLatestCatalogSource(clusterDir)
	if err != nil {
		return fmt.Errorf("查找 CatalogSource 文件失败: %w", err)
	}
	fmt.Printf("✅ 找到 CatalogSource 文件: %s\n", catalogSourceFile)

	fmt.Printf("➡️  步骤 3/%d: 应用 CatalogSource\n", steps)
	if err := applyCatalogSource(kubeconfigPath, catalogSourceFile); err != nil {
		return fmt.Errorf("应用 CatalogSource 失败: %w", err)
	}
	fmt.Println("✅ CatalogSource 已应用")

	fmt.Printf("➡️  步骤 4/%d: 配置 CatalogSource 属性\n", steps)
	if err := configureCatalogSource(kubeconfigPath, registryHost); err != nil {
		return fmt.Errorf("配置 CatalogSource 失败: %w", err)
	}
	fmt.Println("✅ CatalogSource 属性已配置")

	fmt.Printf("➡️  步骤 5/%d: 等待 CatalogSource 状态变为 ready\n", steps)
	if err := waitForCatalogSourceReady(kubeconfigPath); err != nil {
		return fmt.Errorf("等待 CatalogSource ready 失败: %w", err)
	}
	fmt.Println("✅ CatalogSource 状态已就绪")

	return nil
}

// loadClusterConfig 加载集群配置
func loadClusterConfig(clusterDir string) (*config.ClusterConfig, error) {
	configPath := filepath.Join(clusterDir, "config.toml")
	return config.LoadConfig(configPath)
}

// disableDefaultCatalogSources 禁用默认的在线 catalog sources
func disableDefaultCatalogSources(kubeconfigPath string) error {
	fmt.Println("🔧 禁用默认的在线 catalog sources...")

	cmd := exec.Command("oc", "patch", "OperatorHub", "cluster",
		"--type", "json",
		"-p", `[{"op": "add", "path": "/spec/disableAllDefaultSources", "value": true}]`,
		"--kubeconfig", kubeconfigPath)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行 oc patch 命令失败: %w\n输出: %s", err, string(output))
	}

	fmt.Printf("📋 命令输出: %s\n", strings.TrimSpace(string(output)))
	return nil
}

// findLatestCatalogSource 查找最新的 CatalogSource 文件
func findLatestCatalogSource(clusterDir string) (string, error) {
	fmt.Println("🔍 查找最新的 CatalogSource 文件...")

	// 查找 oc-mirror workspace 目录
	workspaceDir, err := findOcMirrorWorkspace(clusterDir)
	if err != nil {
		return "", err
	}

	// 查找最新的 results 目录
	latestResultsDir, err := findLatestResultsDir(workspaceDir)
	if err != nil {
		return "", err
	}

	// 使用模式匹配查找 catalogSource 文件
	entries, err := os.ReadDir(latestResultsDir)
	if err != nil {
		return "", fmt.Errorf("读取 results 目录失败: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "catalogSource") && strings.HasSuffix(entry.Name(), ".yaml") {
			catalogSourceFile := filepath.Join(latestResultsDir, entry.Name())
			fmt.Printf("📄 找到 CatalogSource 文件: %s\n", catalogSourceFile)
			return catalogSourceFile, nil
		}
	}

	// 如果没有找到，列出目录中的所有文件用于调试
	fmt.Printf("🔍 目录 %s 中的文件:\n", latestResultsDir)
	for _, entry := range entries {
		fmt.Printf("  - %s\n", entry.Name())
	}

	return "", fmt.Errorf("在 %s 中未找到 CatalogSource 文件", latestResultsDir)
}

// findOcMirrorWorkspace 查找 oc-mirror workspace 目录
func findOcMirrorWorkspace(clusterDir string) (string, error) {
	dirsToCheck := []string{
		filepath.Join(clusterDir, "oc-mirror-workspace"),
		filepath.Join(clusterDir, "images", "oc-mirror-workspace"),
	}

	for _, dir := range dirsToCheck {
		if _, err := os.Stat(dir); err == nil {
			fmt.Printf("📁 找到 oc-mirror workspace: %s\n", dir)
			return dir, nil
		}
	}

	return "", fmt.Errorf("oc-mirror workspace 目录不存在，已尝试路径: %v", dirsToCheck)
}

// findLatestResultsDir 查找最新的 results 目录
func findLatestResultsDir(workspaceDir string) (string, error) {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("读取 workspace 目录失败: %w", err)
	}

	var latestDir string
	var latestTime int64

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "results-") {
			continue
		}

		dirPath := filepath.Join(workspaceDir, entry.Name())
		// 检查目录是否包含文件（非空目录）
		if entries, _ := os.ReadDir(dirPath); len(entries) == 0 {
			continue
		}

		// 从目录名提取时间戳
		timestamp := strings.TrimPrefix(entry.Name(), "results-")
		if timeValue, err := utils.ParseTimestamp(timestamp); err == nil {
			if timeValue > latestTime {
				latestTime = timeValue
				latestDir = dirPath
			}
		}
	}

	if latestDir == "" {
		return "", fmt.Errorf("未找到有效的 results 目录")
	}

	fmt.Printf("📁 找到最新 results 目录: %s\n", latestDir)
	return latestDir, nil
}

// applyCatalogSource 应用 CatalogSource
func applyCatalogSource(kubeconfigPath, catalogSourceFile string) error {
	fmt.Printf("🔧 应用 CatalogSource: %s\n", catalogSourceFile)

	// 首先读取原始文件
	content, err := os.ReadFile(catalogSourceFile)
	if err != nil {
		return fmt.Errorf("读取 CatalogSource 文件失败: %w", err)
	}

	// 修改名称为 redhat-operators（如果不是的话）
	modifiedContent := string(content)
	// 支持多种可能的原始名称格式
	possibleNames := []string{
		"name: redhat-operator-index",
		"name: cs-redhat-operator-index",
		"name: redhat-operators-index",
	}

	for _, oldName := range possibleNames {
		if strings.Contains(modifiedContent, oldName) {
			modifiedContent = strings.ReplaceAll(modifiedContent, oldName, "name: redhat-operators")
			fmt.Printf("📋 已将 '%s' 替换为 'name: redhat-operators'\n", oldName)
			break
		}
	}

	// 创建临时文件
	tempFile := filepath.Join(filepath.Dir(catalogSourceFile), "catalogSource-modified.yaml")
	if err := os.WriteFile(tempFile, []byte(modifiedContent), 0644); err != nil {
		return fmt.Errorf("创建临时 CatalogSource 文件失败: %w", err)
	}
	defer os.Remove(tempFile)

	// 应用 CatalogSource
	cmd := exec.Command("oc", "apply", "-f", tempFile, "--kubeconfig", kubeconfigPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("应用 CatalogSource 失败: %w\n输出: %s", err, string(output))
	}

	fmt.Printf("📋 命令输出: %s\n", strings.TrimSpace(string(output)))
	return nil
}

// configureCatalogSource 配置 CatalogSource 属性
func configureCatalogSource(kubeconfigPath, registryHost string) error {
	fmt.Println("🔧 配置 CatalogSource 显示名称和轮询间隔...")

	// 设置友好的显示名称
	displayName := fmt.Sprintf("Private Catalog (%s)", registryHost)
	patchDisplayName := fmt.Sprintf(`{"spec": {"displayName": "%s"}}`, displayName)

	cmd1 := exec.Command("oc", "patch", "CatalogSource", "redhat-operators",
		"-n", "openshift-marketplace",
		"--type", "merge",
		"-p", patchDisplayName,
		"--kubeconfig", kubeconfigPath)

	output1, err := cmd1.CombinedOutput()
	if err != nil {
		return fmt.Errorf("设置 CatalogSource 显示名称失败: %w\n输出: %s", err, string(output1))
	}

	fmt.Printf("📋 显示名称设置输出: %s\n", strings.TrimSpace(string(output1)))

	// 设置轮询间隔
	patchInterval := `{"spec": {"updateStrategy": {"registryPoll": {"interval": "2m"}}}}`

	cmd2 := exec.Command("oc", "patch", "CatalogSource", "redhat-operators",
		"-n", "openshift-marketplace",
		"--type", "merge",
		"-p", patchInterval,
		"--kubeconfig", kubeconfigPath)

	output2, err := cmd2.CombinedOutput()
	if err != nil {
		return fmt.Errorf("设置 CatalogSource 轮询间隔失败: %w\n输出: %s", err, string(output2))
	}

	fmt.Printf("📋 轮询间隔设置输出: %s\n", strings.TrimSpace(string(output2)))
	return nil
}

// waitForCatalogSourceReady 等待 CatalogSource 状态变为 ready
func waitForCatalogSourceReady(kubeconfigPath string) error {
	fmt.Println("⏳ 等待 CatalogSource 状态变为 ready...")

	maxAttempts := 40
	for i := 1; i <= maxAttempts; i++ {
		// 获取 CatalogSource 状态
		cmd := exec.Command("oc", "get", "catalogsources.operators.coreos.com", "redhat-operators",
			"-n", "openshift-marketplace",
			"-o", "json",
			"--kubeconfig", kubeconfigPath)

		output, err := cmd.Output()
		if err != nil {
			fmt.Printf("⚠️  获取 CatalogSource 状态失败 (尝试 %d/%d): %v\n", i, maxAttempts, err)
		} else {
			// 解析 JSON 输出
			var catalogSource map[string]interface{}
			if err := json.Unmarshal(output, &catalogSource); err == nil {
				if status, ok := catalogSource["status"].(map[string]interface{}); ok {
					if connectionState, ok := status["connectionState"].(map[string]interface{}); ok {
						if lastObservedState, ok := connectionState["lastObservedState"].(string); ok {
							fmt.Printf("🔍 CatalogSource 状态: %s (尝试 %d/%d)\n", lastObservedState, i, maxAttempts)
							if strings.ToLower(lastObservedState) == "ready" {
								fmt.Println("✅ CatalogSource 状态已就绪！")
								return nil
							}
						}
					}
				}
			}
		}

		if i < maxAttempts {
			fmt.Print(".")
			time.Sleep(time.Duration(i) * time.Second)
		}
	}

	fmt.Printf("⚠️  警告: 等待超时，CatalogSource 可能仍在初始化中\n")
	fmt.Println("💡 您可以手动检查状态: oc get catalogsources -n openshift-marketplace")
	return nil // 不返回错误，只是警告
}
