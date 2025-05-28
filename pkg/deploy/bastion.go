package deploy

import (
	"fmt"

	"ocpack/pkg/config"
)

// BastionDeployer 用于部署 Bastion 节点
type BastionDeployer struct {
	config      *config.ClusterConfig
	downloadDir string
}

// NewBastionDeployer 创建一个新的 Bastion 部署器
func NewBastionDeployer(cfg *config.ClusterConfig, downloadDir string) *BastionDeployer {
	return &BastionDeployer{
		config:      cfg,
		downloadDir: downloadDir,
	}
}

// Deploy 执行 Bastion 节点部署
func (d *BastionDeployer) Deploy(configFilePath string) error {
	fmt.Printf("开始部署 Bastion 节点 (%s)...\n", d.config.Bastion.IP)
	
	// 使用 Ansible 执行器进行部署
	executor, err := NewAnsibleExecutor(d.config, configFilePath)
	if err != nil {
		return fmt.Errorf("创建 Ansible 执行器失败: %w", err)
	}
	defer executor.Cleanup()

	// 执行 Bastion playbook
	if err := executor.RunBastionPlaybook(); err != nil {
		return fmt.Errorf("Bastion 节点部署失败: %w", err)
	}

	fmt.Println("Bastion 节点部署完成！")
	fmt.Printf("DNS 服务器: %s:53\n", d.config.Bastion.IP)
	fmt.Printf("HAProxy 统计页面: http://%s:9000/stats\n", d.config.Bastion.IP)
	fmt.Printf("API 服务器: https://%s:6443\n", d.config.Bastion.IP)
	fmt.Printf("应用入口: http://%s 和 https://%s\n", d.config.Bastion.IP, d.config.Bastion.IP)

	return nil
} 