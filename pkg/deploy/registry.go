package deploy

import (
	"crypto/tls"
	"fmt"
	"net/http"
	"time"

	"ocpack/pkg/config"
)

// checkRegistryDeployed 检查 Registry 是否已经部署
func checkRegistryDeployed(cfg *config.ClusterConfig) bool {
	// 构建 Registry URL
	registryURL := fmt.Sprintf("https://%s:8443/health/instance", cfg.Registry.IP)

	fmt.Printf("🔍 检查 Registry 是否已部署: %s\n", registryURL)

	// 创建 HTTP 客户端，跳过 SSL 验证
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// 尝试访问 Registry 健康检查端点
	resp, err := client.Get(registryURL)
	if err != nil {
		fmt.Printf("🔍 Registry 检查失败: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	fmt.Printf("🔍 Registry 检查响应状态: %d\n", resp.StatusCode)

	// 如果返回 200 状态码，说明 Registry 已经部署并运行
	return resp.StatusCode == 200
}

// DeployRegistry 部署 Registry 节点
func DeployRegistry(cfg *config.ClusterConfig, configFilePath string) error {
	fmt.Println("开始部署 Registry 节点...")

	// 验证配置
	if err := config.ValidateRegistryConfig(cfg); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}

	// 检查 Registry 是否已经部署
	if checkRegistryDeployed(cfg) {
		fmt.Printf("✅ Registry 节点已经部署并运行在 %s:8443\n", cfg.Registry.IP)
		fmt.Printf("🔄 跳过重复部署\n")
		fmt.Printf("Quay 镜像仓库: https://%s:8443\n", cfg.Registry.IP)
		fmt.Printf("用户名: %s\n", cfg.Registry.RegistryUser)
		fmt.Printf("密码: ztesoft123\n")
		return nil
	}

	fmt.Printf("开始部署 Registry 节点 (%s)...\n", cfg.Registry.IP)

	// 创建 Ansible 执行器
	executor, err := NewAnsibleExecutor(cfg, configFilePath)
	if err != nil {
		return fmt.Errorf("创建 Ansible 执行器失败: %w", err)
	}
	defer executor.Cleanup()

	// 执行 Registry playbook
	if err := executor.RunRegistryPlaybook(); err != nil {
		return fmt.Errorf("Registry 节点部署失败: %w", err)
	}

	fmt.Println("Registry 节点部署完成！")
	fmt.Printf("Quay 镜像仓库: https://%s:8443\n", cfg.Registry.IP)
	fmt.Printf("用户名: %s\n", cfg.Registry.RegistryUser)
	fmt.Printf("密码: ztesoft123\n")

	return nil
}
