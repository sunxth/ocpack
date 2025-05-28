package config

import (
	"fmt"
	"os"

	"github.com/pelletier/go-toml/v2"
)

// ClusterConfig 表示集群配置
type ClusterConfig struct {
	// 集群基本信息
	ClusterInfo struct {
		Name        string `toml:"name" comment:"集群名称"`
		Domain      string `toml:"domain" comment:"集群域名 (例如: example.com)"`
		ClusterID   string `toml:"cluster_id" comment:"集群ID (例如: mycluster)"`
		OpenShiftVersion string `toml:"openshift_version" comment:"OpenShift 版本 (例如: 4.14.0, 需要 4.14.0+ 才支持 oc-mirror)"`
	} `toml:"cluster_info" comment:"集群基本信息"`

	// Bastion 节点配置
	Bastion struct {
		IP        string `toml:"ip" comment:"Bastion 节点 IP 地址"`
		Username  string `toml:"username" comment:"SSH 用户名"`
		SSHKeyPath string `toml:"ssh_key_path" comment:"SSH 私钥路径 (可选)"`
		Password  string `toml:"password" comment:"SSH 密码 (如未提供 SSH 私钥则必填)"`
	} `toml:"bastion" comment:"Bastion 节点配置"`

	// Registry 节点配置
	Registry struct {
		IP        string `toml:"ip" comment:"Registry 节点 IP 地址"`
		Username  string `toml:"username" comment:"SSH 用户名"`
		SSHKeyPath string `toml:"ssh_key_path" comment:"SSH 私钥路径 (可选)"`
		Password  string `toml:"password" comment:"SSH 密码 (如未提供 SSH 私钥则必填)"`
		StoragePath string `toml:"storage_path" comment:"镜像存储路径 (例如: /var/lib/registry)"`
		RegistryUser string `toml:"registry_user" comment:"Registry 用户名 (默认: ocp4)"`
	} `toml:"registry" comment:"Registry 节点配置"`

	// 集群节点配置 (用于生成 ignition 文件和配置 DNS/HAProxy)
	Cluster struct {
		// Control Plane 节点 (Master 节点)
		ControlPlane []struct {
			Name string `toml:"name" comment:"节点名称 (例如: master-0)"`
			IP   string `toml:"ip" comment:"节点 IP 地址"`
			MAC  string `toml:"mac" comment:"节点 MAC 地址 (用于生成 ignition 文件)"`
		} `toml:"control_plane" comment:"Control Plane 节点配置"`

		// Worker 节点
		Worker []struct {
			Name string `toml:"name" comment:"节点名称 (例如: worker-0)"`
			IP   string `toml:"ip" comment:"节点 IP 地址"`
			MAC  string `toml:"mac" comment:"节点 MAC 地址 (用于生成 ignition 文件)"`
		} `toml:"worker" comment:"Worker 节点配置"`

		// 网络配置
		Network struct {
			ClusterNetwork string `toml:"cluster_network" comment:"集群网络 CIDR (例如: 10.128.0.0/14)"`
			ServiceNetwork string `toml:"service_network" comment:"服务网络 CIDR (例如: 172.30.0.0/16)"`
			MachineNetwork string `toml:"machine_network" comment:"机器网络 CIDR (例如: 192.168.1.0/24)"`
		} `toml:"network" comment:"集群网络配置"`
	} `toml:"cluster" comment:"集群节点配置"`

	// 下载配置
	Download struct {
		LocalPath string `toml:"local_path" comment:"下载文件本地存储路径"`
	} `toml:"download" comment:"下载配置"`
}

// NewDefaultConfig 创建默认配置
func NewDefaultConfig(clusterName string) *ClusterConfig {
	config := &ClusterConfig{}
	
	// 设置默认值
	config.ClusterInfo.Name = clusterName
	config.ClusterInfo.Domain = "example.com"
	config.ClusterInfo.ClusterID = clusterName
	config.ClusterInfo.OpenShiftVersion = "4.14.0"

	config.Bastion.Username = "root"
	
	config.Registry.Username = "root"
	config.Registry.StoragePath = "/var/lib/registry"
	config.Registry.RegistryUser = "ocp4"

	// 设置集群节点默认值
	config.Cluster.ControlPlane = []struct {
		Name string `toml:"name" comment:"节点名称 (例如: master-0)"`
		IP   string `toml:"ip" comment:"节点 IP 地址"`
		MAC  string `toml:"mac" comment:"节点 MAC 地址 (用于生成 ignition 文件)"`
	}{
		{Name: "master-0", IP: "", MAC: ""},
		{Name: "master-1", IP: "", MAC: ""},
		{Name: "master-2", IP: "", MAC: ""},
	}

	config.Cluster.Worker = []struct {
		Name string `toml:"name" comment:"节点名称 (例如: worker-0)"`
		IP   string `toml:"ip" comment:"节点 IP 地址"`
		MAC  string `toml:"mac" comment:"节点 MAC 地址 (用于生成 ignition 文件)"`
	}{
		{Name: "worker-0", IP: "", MAC: ""},
		{Name: "worker-1", IP: "", MAC: ""},
	}

	// 设置网络默认值
	config.Cluster.Network.ClusterNetwork = "10.128.0.0/14"
	config.Cluster.Network.ServiceNetwork = "172.30.0.0/16"
	config.Cluster.Network.MachineNetwork = "192.168.1.0/24"
	
	config.Download.LocalPath = "downloads"

	return config
}

// GenerateDefaultConfig 生成默认配置文件
func GenerateDefaultConfig(filePath string, clusterName string) error {
	config := NewDefaultConfig(clusterName)
	
	// 转换为TOML格式
	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("生成TOML配置失败: %w", err)
	}
	
	// 添加注释
	configWithComments := `# OpenShift 集群配置文件
# 请填写以下配置项，标记为必填的项必须提供有效值
# 配置完成后，请使用 ocpack 相关命令继续部署过程

`
	configWithComments += string(data)
	
	// 写入文件
	if err := os.WriteFile(filePath, []byte(configWithComments), 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	
	return nil
}

// LoadConfig 从文件加载配置
func LoadConfig(filePath string) (*ClusterConfig, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}
	
	config := &ClusterConfig{}
	if err := toml.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}
	
	return config, nil
}

// SaveConfig 保存配置到文件
func SaveConfig(config *ClusterConfig, filePath string) error {
	data, err := toml.Marshal(config)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}
	
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}
	
	return nil
}

// ValidateConfig 验证配置是否有效
func ValidateConfig(config *ClusterConfig) error {
	// 验证集群基本信息
	if config.ClusterInfo.Name == "" {
		return fmt.Errorf("集群名称不能为空")
	}
	if config.ClusterInfo.Domain == "" {
		return fmt.Errorf("集群域名不能为空")
	}
	if config.ClusterInfo.ClusterID == "" {
		return fmt.Errorf("集群ID不能为空")
	}
	if config.ClusterInfo.OpenShiftVersion == "" {
		return fmt.Errorf("OpenShift版本不能为空")
	}
	
	// 验证Bastion节点配置
	if config.Bastion.IP == "" {
		return fmt.Errorf("Bastion节点IP不能为空")
	}
	if config.Bastion.Username == "" {
		return fmt.Errorf("Bastion节点用户名不能为空")
	}
	if config.Bastion.SSHKeyPath == "" && config.Bastion.Password == "" {
		return fmt.Errorf("Bastion节点必须提供SSH密钥或密码")
	}
	
	// 验证Registry节点配置
	if config.Registry.IP == "" {
		return fmt.Errorf("Registry节点IP不能为空")
	}
	if config.Registry.Username == "" {
		return fmt.Errorf("Registry节点用户名不能为空")
	}
	if config.Registry.SSHKeyPath == "" && config.Registry.Password == "" {
		return fmt.Errorf("Registry节点必须提供SSH密钥或密码")
	}
	if config.Registry.StoragePath == "" {
		return fmt.Errorf("Registry节点存储路径不能为空")
	}

	// 验证集群节点配置
	if len(config.Cluster.ControlPlane) == 0 {
		return fmt.Errorf("至少需要配置一个Control Plane节点")
	}

	for i, cp := range config.Cluster.ControlPlane {
		if cp.Name == "" {
			return fmt.Errorf("Control Plane节点[%d]名称不能为空", i)
		}
		if cp.IP == "" {
			return fmt.Errorf("Control Plane节点[%d] %s 的IP不能为空", i, cp.Name)
		}
		if cp.MAC == "" {
			return fmt.Errorf("Control Plane节点[%d] %s 的MAC地址不能为空", i, cp.Name)
		}
	}

	for i, worker := range config.Cluster.Worker {
		if worker.Name == "" {
			return fmt.Errorf("Worker节点[%d]名称不能为空", i)
		}
		if worker.IP == "" {
			return fmt.Errorf("Worker节点[%d] %s 的IP不能为空", i, worker.Name)
		}
		if worker.MAC == "" {
			return fmt.Errorf("Worker节点[%d] %s 的MAC地址不能为空", i, worker.Name)
		}
	}

	// 验证网络配置
	if config.Cluster.Network.ClusterNetwork == "" {
		return fmt.Errorf("集群网络CIDR不能为空")
	}
	if config.Cluster.Network.ServiceNetwork == "" {
		return fmt.Errorf("服务网络CIDR不能为空")
	}
	if config.Cluster.Network.MachineNetwork == "" {
		return fmt.Errorf("机器网络CIDR不能为空")
	}
	
	return nil
}

// ValidateBastionConfig 验证 Bastion 部署所需的配置
func ValidateBastionConfig(config *ClusterConfig) error {
	// 验证集群基本信息
	if config.ClusterInfo.Name == "" {
		return fmt.Errorf("集群名称不能为空")
	}
	if config.ClusterInfo.Domain == "" {
		return fmt.Errorf("集群域名不能为空")
	}
	if config.ClusterInfo.ClusterID == "" {
		return fmt.Errorf("集群ID不能为空")
	}
	if config.ClusterInfo.OpenShiftVersion == "" {
		return fmt.Errorf("OpenShift版本不能为空")
	}
	
	// 验证Bastion节点配置
	if config.Bastion.IP == "" {
		return fmt.Errorf("Bastion节点IP不能为空")
	}
	if config.Bastion.Username == "" {
		return fmt.Errorf("Bastion节点用户名不能为空")
	}
	if config.Bastion.SSHKeyPath == "" && config.Bastion.Password == "" {
		return fmt.Errorf("Bastion节点必须提供SSH密钥或密码")
	}

	// 验证Registry节点IP（Bastion需要配置Registry的DNS解析）
	if config.Registry.IP == "" {
		return fmt.Errorf("Registry节点IP不能为空（Bastion需要配置Registry的DNS解析）")
	}

	// 验证集群节点配置（Bastion 需要这些信息来配置 DNS 和 HAProxy）
	if len(config.Cluster.ControlPlane) == 0 {
		return fmt.Errorf("至少需要配置一个Control Plane节点")
	}

	for i, cp := range config.Cluster.ControlPlane {
		if cp.Name == "" {
			return fmt.Errorf("Control Plane节点[%d]名称不能为空", i)
		}
		if cp.IP == "" {
			return fmt.Errorf("Control Plane节点[%d] %s 的IP不能为空", i, cp.Name)
		}
		// MAC 地址对于 Bastion 部署不是必需的
	}

	for i, worker := range config.Cluster.Worker {
		if worker.Name == "" {
			return fmt.Errorf("Worker节点[%d]名称不能为空", i)
		}
		if worker.IP == "" {
			return fmt.Errorf("Worker节点[%d] %s 的IP不能为空", i, worker.Name)
		}
		// MAC 地址对于 Bastion 部署不是必需的
	}

	// 验证网络配置
	if config.Cluster.Network.ClusterNetwork == "" {
		return fmt.Errorf("集群网络CIDR不能为空")
	}
	if config.Cluster.Network.ServiceNetwork == "" {
		return fmt.Errorf("服务网络CIDR不能为空")
	}
	if config.Cluster.Network.MachineNetwork == "" {
		return fmt.Errorf("机器网络CIDR不能为空")
	}
	
	return nil
}

// ValidateRegistryConfig 验证 Registry 部署所需的配置
func ValidateRegistryConfig(config *ClusterConfig) error {
	// 验证集群基本信息
	if config.ClusterInfo.Name == "" {
		return fmt.Errorf("集群名称不能为空")
	}
	if config.ClusterInfo.OpenShiftVersion == "" {
		return fmt.Errorf("OpenShift版本不能为空")
	}
	
	// 验证Registry节点配置
	if config.Registry.IP == "" {
		return fmt.Errorf("Registry节点IP不能为空")
	}
	if config.Registry.Username == "" {
		return fmt.Errorf("Registry节点用户名不能为空")
	}
	if config.Registry.SSHKeyPath == "" && config.Registry.Password == "" {
		return fmt.Errorf("Registry节点必须提供SSH密钥或密码")
	}
	if config.Registry.StoragePath == "" {
		return fmt.Errorf("Registry节点存储路径不能为空")
	}
	
	return nil
}

// ValidateRegistryConfigWithDownloads 验证 Registry 部署所需的配置和下载文件
func ValidateRegistryConfigWithDownloads(config *ClusterConfig, downloadDir string) error {
	// 先验证基本配置
	if err := ValidateRegistryConfig(config); err != nil {
		return err
	}
	
	// 验证必需的下载文件是否存在
	requiredFiles := []struct {
		path        string
		description string
	}{
		{
			path:        downloadDir + "/mirror-registry-amd64.tar.gz",
			description: "Quay 镜像仓库安装包",
		},
		{
			path:        downloadDir + "/bin/oc",
			description: "OpenShift 客户端工具",
		},
		{
			path:        downloadDir + "/bin/kubectl",
			description: "Kubernetes 客户端工具",
		},
		{
			path:        downloadDir + "/bin/oc-mirror",
			description: "OpenShift 镜像同步工具",
		},
	}
	
	for _, file := range requiredFiles {
		if _, err := os.Stat(file.path); os.IsNotExist(err) {
			return fmt.Errorf("缺少必需的文件: %s (%s)\n请先运行 'ocpack download' 命令下载所需文件", file.path, file.description)
		}
	}
	
	return nil
}

// ValidateDownloadConfig 验证下载功能所需的配置
func ValidateDownloadConfig(config *ClusterConfig) error {
	// 验证集群基本信息
	if config.ClusterInfo.OpenShiftVersion == "" {
		return fmt.Errorf("OpenShift版本不能为空")
	}
	
	return nil
} 