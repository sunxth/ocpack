package deploy

import (
	"fmt"

	"ocpack/pkg/config"
)

// DeployRegistry 部署 Registry 节点
func DeployRegistry(cfg *config.ClusterConfig, configFilePath string) error {
	fmt.Println("开始部署 Registry 节点...")
	
	// 验证配置
	if err := config.ValidateRegistryConfig(cfg); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
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