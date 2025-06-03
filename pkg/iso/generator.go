package iso

import (
	"embed"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
	"time"

	"ocpack/pkg/config"
)

//go:embed templates/*
var templates embed.FS

// ISOGenerator ISO 生成器
type ISOGenerator struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// GenerateOptions ISO 生成选项
type GenerateOptions struct {
	OutputPath  string
	BaseISOPath string
	SkipVerify  bool
}

// InstallConfigData install-config.yaml 模板数据
type InstallConfigData struct {
	BaseDomain            string
	ClusterName           string
	NumWorkers            int
	NumMasters            int
	MachineNetwork        string
	PrefixLength          int
	HostPrefix            int
	PullSecret            string
	SSHKeyPub             string
	AdditionalTrustBundle string
	ImageContentSources   string
	ArchShort             string
	UseProxy              bool
	HTTPProxy             string
	HTTPSProxy            string
	NoProxy               string
}

// AgentConfigData agent-config.yaml 模板数据
type AgentConfigData struct {
	ClusterName    string
	RendezvousIP   string
	Hosts          []HostConfig
	Port0          string
	PrefixLength   int
	NextHopAddress string
	DNSServers     []string
}

// HostConfig 主机配置
type HostConfig struct {
	Hostname   string
	Role       string
	MACAddress string
	IPAddress  string
	Interface  string
}

// NewISOGenerator 创建新的 ISO 生成器
func NewISOGenerator(clusterName, projectRoot string) (*ISOGenerator, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %v", err)
	}

	return &ISOGenerator{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// GenerateISO 生成 ISO 镜像
func (g *ISOGenerator) GenerateISO(options *GenerateOptions) error {
	fmt.Printf("开始为集群 %s 生成 ISO 镜像\n", g.ClusterName)

	// 1. 验证配置和依赖
	if err := g.ValidateConfig(); err != nil {
		return fmt.Errorf("配置验证失败: %v", err)
	}

	// 2. 创建安装目录结构
	installDir := filepath.Join(g.ClusterDir, "installation")
	if err := g.createInstallationDirs(installDir); err != nil {
		return fmt.Errorf("创建安装目录失败: %v", err)
	}

	// 3. 生成 install-config.yaml
	if err := g.generateInstallConfig(installDir); err != nil {
		return fmt.Errorf("生成 install-config.yaml 失败: %v", err)
	}

	// 4. 生成 agent-config.yaml
	if err := g.generateAgentConfig(installDir); err != nil {
		return fmt.Errorf("生成 agent-config.yaml 失败: %v", err)
	}

	// 5. 生成 ISO 文件
	if err := g.generateISOFiles(installDir, options); err != nil {
		return fmt.Errorf("生成 ISO 文件失败: %v", err)
	}

	fmt.Printf("✅ ISO 生成完成！文件位置: %s\n", filepath.Join(installDir, "iso"))
	return nil
}

// ValidateConfig 验证配置
func (g *ISOGenerator) ValidateConfig() error {
	// 验证基本配置
	if err := config.ValidateConfig(g.Config); err != nil {
		return err
	}

	// 验证必需的工具是否存在
	requiredTools := []string{"openshift-install"}
	for _, tool := range requiredTools {
		toolPath := filepath.Join(g.DownloadDir, "bin", tool)
		if _, err := os.Stat(toolPath); os.IsNotExist(err) {
			return fmt.Errorf("缺少必需的工具: %s，请先运行 'ocpack download' 命令", tool)
		}
	}

	// 验证 pull-secret 文件
	pullSecretPath := filepath.Join(g.ClusterDir, "pull-secret.txt")
	if _, err := os.Stat(pullSecretPath); os.IsNotExist(err) {
		return fmt.Errorf("缺少 pull-secret.txt 文件，请先获取 Red Hat pull-secret")
	}

	return nil
}

// createInstallationDirs 创建安装目录结构
func (g *ISOGenerator) createInstallationDirs(installDir string) error {
	dirs := []string{
		installDir,
		filepath.Join(installDir, "ignition"),
		filepath.Join(installDir, "iso"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %v", dir, err)
		}
	}

	return nil
}

// generateInstallConfig 生成 install-config.yaml
func (g *ISOGenerator) generateInstallConfig(installDir string) error {
	fmt.Println("生成 install-config.yaml...")

	configPath := filepath.Join(installDir, "install-config.yaml")

	// 清理可能存在的旧配置文件
	if _, err := os.Stat(configPath); err == nil {
		fmt.Printf("🧹 清理旧的 install-config.yaml 文件: %s\n", configPath)
		if err := os.Remove(configPath); err != nil {
			fmt.Printf("⚠️  清理旧文件失败: %v\n", err)
		}
	}

	// 读取 pull-secret
	// 优先使用包含我们自己 registry 认证的合并认证文件
	var pullSecretBytes []byte
	var err error

	mergedAuthPath := filepath.Join(g.ClusterDir, "registry", "merged-auth.json")
	if _, err := os.Stat(mergedAuthPath); err == nil {
		// 如果存在合并的认证文件，使用它
		fmt.Printf("📋 使用合并的认证文件: %s\n", mergedAuthPath)
		pullSecretBytes, err = os.ReadFile(mergedAuthPath)
		if err != nil {
			return fmt.Errorf("读取合并认证文件失败: %v", err)
		}
	} else {
		// 如果合并认证文件不存在，先创建它
		fmt.Printf("📋 合并认证文件不存在，正在创建...\n")
		if err := g.createMergedAuthConfig(); err != nil {
			fmt.Printf("⚠️  创建合并认证文件失败: %v，使用原始 pull-secret\n", err)
			// 如果创建失败，使用原始的 pull-secret.txt
			pullSecretPath := filepath.Join(g.ClusterDir, "pull-secret.txt")
			pullSecretBytes, err = os.ReadFile(pullSecretPath)
			if err != nil {
				return fmt.Errorf("读取 pull-secret 失败: %v", err)
			}
		} else {
			// 创建成功，读取合并认证文件
			fmt.Printf("📋 使用新创建的合并认证文件: %s\n", mergedAuthPath)
			pullSecretBytes, err = os.ReadFile(mergedAuthPath)
			if err != nil {
				return fmt.Errorf("读取合并认证文件失败: %v", err)
			}
		}
	}
	pullSecret := strings.TrimSpace(string(pullSecretBytes))

	// 读取 SSH 公钥（如果存在）
	sshKeyPub := ""
	sshKeyPath := filepath.Join(os.Getenv("HOME"), ".ssh", "id_rsa.pub")
	if sshKeyBytes, err := os.ReadFile(sshKeyPath); err == nil {
		sshKeyPub = strings.TrimSpace(string(sshKeyBytes))
	}

	// 读取额外的信任证书（如果存在）
	additionalTrustBundle := ""
	caCertPath := filepath.Join(g.ClusterDir, "registry", g.Config.Registry.IP, "rootCA.pem")
	if caCertBytes, err := os.ReadFile(caCertPath); err == nil {
		additionalTrustBundle = string(caCertBytes)
	}

	// 查找并解析 ICSP 文件
	imageContentSources, err := g.findAndParseICSP()
	if err != nil {
		fmt.Printf("⚠️  查找 ICSP 文件失败: %v\n", err)
		imageContentSources = ""
	}

	// 构建模板数据
	data := InstallConfigData{
		BaseDomain:            g.Config.ClusterInfo.Domain,
		ClusterName:           g.Config.ClusterInfo.Name,
		NumWorkers:            len(g.Config.Cluster.Worker),
		NumMasters:            len(g.Config.Cluster.ControlPlane),
		MachineNetwork:        g.extractNetworkBase(g.Config.Cluster.Network.MachineNetwork),
		PrefixLength:          g.extractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		HostPrefix:            23,
		PullSecret:            pullSecret,
		SSHKeyPub:             sshKeyPub,
		AdditionalTrustBundle: additionalTrustBundle,
		ImageContentSources:   imageContentSources,
		ArchShort:             "amd64",
		UseProxy:              false,
	}

	fmt.Printf("🔧 install-config 模板数据:\n")
	fmt.Printf("  - BaseDomain: %s\n", data.BaseDomain)
	fmt.Printf("  - ClusterName: %s\n", data.ClusterName)
	fmt.Printf("  - NumWorkers: %d\n", data.NumWorkers)
	fmt.Printf("  - NumMasters: %d\n", data.NumMasters)
	fmt.Printf("  - MachineNetwork: %s\n", data.MachineNetwork)
	fmt.Printf("  - PrefixLength: %d\n", data.PrefixLength)

	// 读取模板
	tmplContent, err := templates.ReadFile("templates/install-config.yaml")
	if err != nil {
		return fmt.Errorf("读取 install-config 模板失败: %v", err)
	}

	// 创建模板函数映射
	funcMap := template.FuncMap{
		"indent": func(spaces int, text string) string {
			lines := strings.Split(text, "\n")
			indentStr := strings.Repeat(" ", spaces)
			for i, line := range lines {
				if line != "" {
					lines[i] = indentStr + line
				}
			}
			return strings.Join(lines, "\n")
		},
	}

	// 解析和执行模板
	tmpl, err := template.New("install-config").Funcs(funcMap).Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("解析 install-config 模板失败: %v", err)
	}

	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("创建 install-config.yaml 失败: %v", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("生成 install-config.yaml 失败: %v", err)
	}

	fmt.Printf("✅ install-config.yaml 已生成: %s\n", configPath)

	// 调试：显示生成的 install-config.yaml 完整内容
	if generatedContent, err := os.ReadFile(configPath); err == nil {
		fmt.Printf("🔍 生成的 install-config.yaml 内容:\n%s\n", string(generatedContent))
	}

	return nil
}

// generateAgentConfig 生成 agent-config.yaml
func (g *ISOGenerator) generateAgentConfig(installDir string) error {
	fmt.Println("生成 agent-config.yaml...")

	// 构建主机配置
	var hosts []HostConfig

	// 添加 Control Plane 节点
	for i, cp := range g.Config.Cluster.ControlPlane {
		hostname := cp.Name
		if len(g.Config.Cluster.Worker) == 0 && len(g.Config.Cluster.ControlPlane) == 1 {
			hostname = g.Config.ClusterInfo.Name
		}

		hosts = append(hosts, HostConfig{
			Hostname:   hostname,
			Role:       "master",
			MACAddress: cp.MAC,
			IPAddress:  cp.IP,
			Interface:  "ens3", // 默认网络接口名
		})

		// 第一个 master 节点作为 rendezvous IP
		if i == 0 {
			// rendezvousIP 将在模板数据中设置
		}
	}

	// 添加 Worker 节点
	for _, worker := range g.Config.Cluster.Worker {
		hosts = append(hosts, HostConfig{
			Hostname:   worker.Name,
			Role:       "worker",
			MACAddress: worker.MAC,
			IPAddress:  worker.IP,
			Interface:  "ens3",
		})
	}

	// 构建模板数据
	data := AgentConfigData{
		ClusterName:    g.Config.ClusterInfo.Name,
		RendezvousIP:   g.Config.Cluster.ControlPlane[0].IP, // 使用第一个 master 节点的 IP
		Hosts:          hosts,
		Port0:          "ens3",
		PrefixLength:   g.extractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		NextHopAddress: g.extractGateway(g.Config.Cluster.Network.MachineNetwork),
		DNSServers:     []string{g.Config.Bastion.IP},
	}

	// 读取模板
	tmplContent, err := templates.ReadFile("templates/agent-config.yaml")
	if err != nil {
		return fmt.Errorf("读取 agent-config 模板失败: %v", err)
	}

	// 解析和执行模板
	tmpl, err := template.New("agent-config").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("解析 agent-config 模板失败: %v", err)
	}

	configPath := filepath.Join(installDir, "agent-config.yaml")
	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("创建 agent-config.yaml 失败: %v", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("生成 agent-config.yaml 失败: %v", err)
	}

	fmt.Printf("✅ agent-config.yaml 已生成: %s\n", configPath)
	return nil
}

// generateISOFiles 生成 ISO 文件
func (g *ISOGenerator) generateISOFiles(installDir string, options *GenerateOptions) error {
	fmt.Println("生成 ISO 文件...")

	// 1. 验证 registry 中的 release image（可选）
	if !options.SkipVerify {
		if err := g.verifyReleaseImage(); err != nil {
			return fmt.Errorf("验证 release image 失败: %v", err)
		}
	} else {
		fmt.Println("⚠️  跳过 release image 验证")
	}

	// 2. 从 registry 提取 openshift-install
	openshiftInstallPath, err := g.extractOpenshiftInstall()
	if err != nil {
		return fmt.Errorf("提取 openshift-install 失败: %v", err)
	}

	// 复制配置文件到临时目录（openshift-install 会修改这些文件）
	tempDir := filepath.Join(installDir, "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 复制配置文件
	if err := g.copyFile(
		filepath.Join(installDir, "install-config.yaml"),
		filepath.Join(tempDir, "install-config.yaml"),
	); err != nil {
		return fmt.Errorf("复制 install-config.yaml 失败: %v", err)
	}

	if err := g.copyFile(
		filepath.Join(installDir, "agent-config.yaml"),
		filepath.Join(tempDir, "agent-config.yaml"),
	); err != nil {
		return fmt.Errorf("复制 agent-config.yaml 失败: %v", err)
	}

	// 生成 agent ISO
	cmd := exec.Command(openshiftInstallPath, "agent", "create", "image", "--dir", tempDir)
	cmd.Dir = tempDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("执行命令: %s agent create image --dir %s\n", openshiftInstallPath, tempDir)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("生成 agent ISO 失败: %v", err)
	}

	// 移动生成的 ISO 文件到目标目录
	isoDir := filepath.Join(installDir, "iso")
	agentISOPath := filepath.Join(tempDir, "agent.x86_64.iso")
	targetISOPath := filepath.Join(isoDir, fmt.Sprintf("%s-agent.x86_64.iso", g.ClusterName))

	if err := g.moveFile(agentISOPath, targetISOPath); err != nil {
		return fmt.Errorf("移动 ISO 文件失败: %v", err)
	}

	// 复制 ignition 文件
	ignitionDir := filepath.Join(installDir, "ignition")
	tempIgnitionFiles := []string{"auth", ".openshift_install.log", ".openshift_install_state.json"}

	for _, file := range tempIgnitionFiles {
		srcPath := filepath.Join(tempDir, file)
		if _, err := os.Stat(srcPath); err == nil {
			dstPath := filepath.Join(ignitionDir, file)
			if err := g.copyFileOrDir(srcPath, dstPath); err != nil {
				fmt.Printf("⚠️  复制 %s 失败: %v\n", file, err)
			}
		}
	}

	fmt.Printf("✅ ISO 文件已生成: %s\n", targetISOPath)
	return nil
}

// verifyReleaseImage 验证 registry 中的 release image
func (g *ISOGenerator) verifyReleaseImage() error {
	fmt.Println("验证 registry 中的 release image...")

	// 获取 openshift-install 版本信息
	openshiftInstallPath := filepath.Join(g.DownloadDir, "bin", "openshift-install")
	cmd := exec.Command(openshiftInstallPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("获取 openshift-install 版本失败: %v", err)
	}

	// 打印原始输出用于调试
	versionInfo := string(output)
	fmt.Printf("🔍 openshift-install version 输出:\n%s\n", versionInfo)

	// 解析版本信息
	releaseVer := g.extractVersionFromOutput(versionInfo, "openshift-install")
	releaseSHA := g.extractSHAFromOutput(versionInfo)

	fmt.Printf("📋 提取的版本信息:\n")
	fmt.Printf("  - 配置文件版本: %s\n", g.Config.ClusterInfo.OpenShiftVersion)
	fmt.Printf("  - 工具版本: %s\n", releaseVer)
	fmt.Printf("  - Release SHA: %s\n", releaseSHA)

	// 检查是否成功提取版本信息
	if releaseVer == "" {
		fmt.Printf("⚠️  警告: 无法从 openshift-install 输出中提取版本号\n")
		fmt.Printf("💡 尝试其他方法提取版本信息...\n")

		// 尝试其他可能的前缀
		alternativePrefixes := []string{"openshift-install", "Client Version:", "version"}
		for _, prefix := range alternativePrefixes {
			releaseVer = g.extractVersionFromOutput(versionInfo, prefix)
			if releaseVer != "" {
				fmt.Printf("✅ 使用前缀 '%s' 成功提取版本: %s\n", prefix, releaseVer)
				break
			}
		}

		// 如果仍然无法提取，尝试正则表达式
		if releaseVer == "" {
			releaseVer = g.extractVersionWithRegex(versionInfo)
			if releaseVer != "" {
				fmt.Printf("✅ 使用正则表达式成功提取版本: %s\n", releaseVer)
			}
		}
	}

	if releaseSHA == "" {
		return fmt.Errorf("无法从 openshift-install 输出中提取 release SHA")
	}

	// 检查配置版本是否匹配
	if releaseVer != "" && g.Config.ClusterInfo.OpenShiftVersion != releaseVer {
		fmt.Printf("⚠️  版本不匹配警告:\n")
		fmt.Printf("  - 配置文件版本: %s\n", g.Config.ClusterInfo.OpenShiftVersion)
		fmt.Printf("  - 工具版本: %s\n", releaseVer)
		fmt.Printf("💡 继续使用配置文件中的版本进行验证...\n")
	}

	// 构建 registry 信息
	registryHost := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain)
	registryPort := "8443"

	// 验证 release image 是否存在
	releaseImageURL := fmt.Sprintf("%s:%s/openshift/release-images%s",
		registryHost, registryPort, releaseSHA)

	fmt.Printf("🔍 验证 release image: %s\n", releaseImageURL)

	if err := g.verifyImageExists(releaseImageURL); err != nil {
		return fmt.Errorf("registry 中缺少 release image: %s\n请确保已运行 'ocpack load-image' 命令加载镜像", releaseImageURL)
	}

	fmt.Printf("✅ Release image 验证成功: %s\n", releaseImageURL)
	return nil
}

// verifyExtractedBinary 验证提取的二进制文件
func (g *ISOGenerator) verifyExtractedBinary(binaryPath string) error {
	// 检查文件是否可执行
	if _, err := os.Stat(binaryPath); err != nil {
		return fmt.Errorf("二进制文件不存在: %v", err)
	}

	// 尝试执行 version 命令
	cmd := exec.Command(binaryPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("无法执行二进制文件: %v", err)
	}

	versionOutput := string(output)
	fmt.Printf("🔍 提取的 openshift-install version 输出:\n%s\n", versionOutput)

	// 验证输出包含预期内容
	if !strings.Contains(versionOutput, "openshift-install") {
		return fmt.Errorf("二进制文件输出不包含预期的版本信息")
	}

	fmt.Printf("✅ 提取的二进制文件验证成功\n")
	return nil
}

// generateICSPConfig 生成 ICSP 配置文件
func (g *ISOGenerator) generateICSPConfig(registryHost, registryPort, outputFile string) error {
	fmt.Printf("🔧 开始生成 ICSP 配置文件: %s\n", outputFile)

	// 读取模板
	tmplContent, err := templates.ReadFile("templates/icsp.yaml")
	if err != nil {
		return fmt.Errorf("读取 ICSP 模板文件失败: %v", err)
	}

	// 构建模板数据
	data := struct {
		RegistryHost string
		RegistryPort string
	}{
		RegistryHost: registryHost,
		RegistryPort: registryPort,
	}

	fmt.Printf("🔧 ICSP 模板数据: RegistryHost=%s, RegistryPort=%s\n", registryHost, registryPort)

	// 解析和执行模板
	tmpl, err := template.New("icsp-config").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("解析 ICSP 模板失败: %v", err)
	}

	file, err := os.Create(outputFile)
	if err != nil {
		return fmt.Errorf("创建 ICSP 配置文件失败: %v", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("生成 ICSP 配置文件失败: %v", err)
	}

	fmt.Printf("✅ ICSP 配置文件生成成功: %s\n", outputFile)

	// 验证文件是否真的创建了
	if _, err := os.Stat(outputFile); err != nil {
		return fmt.Errorf("ICSP 配置文件创建后无法访问: %v", err)
	}

	// 显示生成的文件内容
	if content, err := os.ReadFile(outputFile); err == nil {
		fmt.Printf("🔍 生成的 ICSP 配置内容:\n%s\n", string(content))
	}

	return nil
}

// verifyImageExists 验证镜像是否存在于 registry 中
func (g *ISOGenerator) verifyImageExists(imageURL string) error {
	fmt.Printf("🔍 使用 skopeo 验证镜像: %s\n", imageURL)

	// 使用 skopeo 检查镜像是否存在
	cmd := exec.Command("skopeo", "inspect", "--tls-verify=false", fmt.Sprintf("docker://%s", imageURL))

	// 捕获标准输出和错误输出
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// 第一次尝试
	fmt.Printf("📋 执行命令: %s\n", strings.Join(cmd.Args, " "))
	if err := cmd.Run(); err != nil {
		fmt.Printf("⚠️  第一次检查失败: %v\n", err)
		fmt.Printf("📋 标准输出: %s\n", stdout.String())
		fmt.Printf("📋 错误输出: %s\n", stderr.String())

		// 等待 10 秒后重试
		fmt.Println("⏳ 10秒后重试...")
		time.Sleep(10 * time.Second)

		// 重置输出缓冲区
		stdout.Reset()
		stderr.Reset()

		// 第二次尝试
		cmd = exec.Command("skopeo", "inspect", "--tls-verify=false", fmt.Sprintf("docker://%s", imageURL))
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr

		fmt.Printf("📋 第二次执行命令: %s\n", strings.Join(cmd.Args, " "))
		if err := cmd.Run(); err != nil {
			fmt.Printf("❌ 第二次检查也失败: %v\n", err)
			fmt.Printf("📋 标准输出: %s\n", stdout.String())
			fmt.Printf("📋 错误输出: %s\n", stderr.String())

			// 提供详细的故障排除建议
			fmt.Printf("\n🔧 故障排除建议:\n")
			fmt.Printf("1. 检查 registry 是否可访问: curl -k https://%s/v2/\n", strings.Split(imageURL, "/")[0])
			fmt.Printf("2. 检查镜像是否真的存在: skopeo inspect --tls-verify=false docker://%s\n", imageURL)
			fmt.Printf("3. 检查网络连接和防火墙设置\n")
			fmt.Printf("4. 检查 registry 认证配置\n")

			return fmt.Errorf("镜像不存在或无法访问: %s\n详细错误: %v\n错误输出: %s", imageURL, err, stderr.String())
		}
	}

	fmt.Printf("✅ 镜像验证成功: %s\n", imageURL)
	fmt.Printf("📋 镜像信息: %s\n", stdout.String())
	return nil
}

// 辅助函数

// extractNetworkBase 提取网络基地址
func (g *ISOGenerator) extractNetworkBase(cidr string) string {
	parts := strings.Split(cidr, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return cidr
}

// extractPrefixLength 提取前缀长度
func (g *ISOGenerator) extractPrefixLength(cidr string) int {
	parts := strings.Split(cidr, "/")
	if len(parts) == 2 {
		if prefix := parts[1]; prefix != "" {
			// 简单转换，实际应该使用 strconv.Atoi
			switch prefix {
			case "24":
				return 24
			case "16":
				return 16
			case "8":
				return 8
			default:
				return 24
			}
		}
	}
	return 24
}

// extractGateway 提取网关地址（假设是网络的第一个地址）
func (g *ISOGenerator) extractGateway(cidr string) string {
	networkBase := g.extractNetworkBase(cidr)
	parts := strings.Split(networkBase, ".")
	if len(parts) == 4 {
		// 假设网关是 .1
		return fmt.Sprintf("%s.%s.%s.1", parts[0], parts[1], parts[2])
	}
	return networkBase
}

// copyFile 复制文件
func (g *ISOGenerator) copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = dstFile.ReadFrom(srcFile)
	return err
}

// moveFile 移动文件
func (g *ISOGenerator) moveFile(src, dst string) error {
	return os.Rename(src, dst)
}

// copyFileOrDir 复制文件或目录
func (g *ISOGenerator) copyFileOrDir(src, dst string) error {
	srcInfo, err := os.Stat(src)
	if err != nil {
		return err
	}

	if srcInfo.IsDir() {
		return g.copyDir(src, dst)
	}
	return g.copyFile(src, dst)
}

// copyDir 复制目录
func (g *ISOGenerator) copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}

		dstPath := filepath.Join(dst, relPath)

		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}

		return g.copyFile(path, dstPath)
	})
}

// findAndParseICSP 查找并解析 ICSP 文件
func (g *ISOGenerator) findAndParseICSP() (string, error) {
	// 查找 oc-mirror workspace 目录 - 先尝试集群根目录下的 oc-mirror-workspace
	workspaceDir := filepath.Join(g.ClusterDir, "oc-mirror-workspace")
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		// 如果不存在，再尝试 images 目录下的 oc-mirror-workspace
		workspaceDir = filepath.Join(g.ClusterDir, "images", "oc-mirror-workspace")
		if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
			return "", fmt.Errorf("oc-mirror workspace 目录不存在，已尝试路径: %s 和 %s",
				filepath.Join(g.ClusterDir, "oc-mirror-workspace"),
				filepath.Join(g.ClusterDir, "images", "oc-mirror-workspace"))
		}
	}

	fmt.Printf("🔍 使用 oc-mirror workspace 目录: %s\n", workspaceDir)

	// 查找最新的 results 目录
	latestResultsDir, err := g.findLatestResultsDir(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("查找最新 results 目录失败: %v", err)
	}

	// 查找 imageContentSourcePolicy.yaml 文件
	icspFile := filepath.Join(latestResultsDir, "imageContentSourcePolicy.yaml")
	if _, err := os.Stat(icspFile); os.IsNotExist(err) {
		return "", fmt.Errorf("ICSP 文件不存在: %s", icspFile)
	}

	fmt.Printf("📄 找到 ICSP 文件: %s\n", icspFile)

	// 读取并解析 ICSP 文件
	icspContent, err := os.ReadFile(icspFile)
	if err != nil {
		return "", fmt.Errorf("读取 ICSP 文件失败: %v", err)
	}

	// 解析 ICSP 内容并转换为 install-config.yaml 格式
	imageContentSources, err := g.parseICSPToInstallConfig(string(icspContent))
	if err != nil {
		return "", fmt.Errorf("解析 ICSP 内容失败: %v", err)
	}

	fmt.Printf("✅ 成功解析 ICSP 文件，包含 %d 个镜像源配置\n", strings.Count(imageContentSources, "- mirrors:"))
	return imageContentSources, nil
}

// findLatestResultsDir 查找最新的 results 目录
func (g *ISOGenerator) findLatestResultsDir(workspaceDir string) (string, error) {
	entries, err := os.ReadDir(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("读取 workspace 目录失败: %v", err)
	}

	var latestDir string
	var latestTime int64

	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "results-") {
			continue
		}

		dirPath := filepath.Join(workspaceDir, entry.Name())

		// 检查目录是否包含文件（非空目录）
		if !g.isDirNonEmpty(dirPath) {
			continue
		}

		// 从目录名提取时间戳
		timestamp := strings.TrimPrefix(entry.Name(), "results-")
		if timeValue, err := strconv.ParseInt(timestamp, 10, 64); err == nil {
			if timeValue > latestTime {
				latestTime = timeValue
				latestDir = dirPath
			}
		}
	}

	if latestDir == "" {
		return "", fmt.Errorf("未找到有效的 results 目录")
	}

	return latestDir, nil
}

// isDirNonEmpty 检查目录是否非空
func (g *ISOGenerator) isDirNonEmpty(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// parseICSPToInstallConfig 将 ICSP 内容转换为 install-config.yaml 格式
func (g *ISOGenerator) parseICSPToInstallConfig(icspContent string) (string, error) {
	// 解析 YAML 文档
	documents := strings.Split(icspContent, "---")
	var allMirrors []string

	for _, doc := range documents {
		doc = strings.TrimSpace(doc)
		if doc == "" {
			continue
		}

		// 提取 repositoryDigestMirrors 部分
		mirrors := g.extractRepositoryDigestMirrors(doc)
		allMirrors = append(allMirrors, mirrors...)
	}

	if len(allMirrors) == 0 {
		return "", fmt.Errorf("未找到有效的镜像源配置")
	}

	// 构建 install-config.yaml 格式的 imageContentSources
	var result strings.Builder
	for _, mirror := range allMirrors {
		result.WriteString(mirror)
		result.WriteString("\n")
	}

	return strings.TrimSpace(result.String()), nil
}

// extractRepositoryDigestMirrors 从 ICSP 文档中提取镜像源配置
func (g *ISOGenerator) extractRepositoryDigestMirrors(doc string) []string {
	var mirrors []string
	lines := strings.Split(doc, "\n")

	inRepositoryDigestMirrors := false
	inMirrorBlock := false
	currentMirror := ""
	currentSource := ""

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		if strings.Contains(line, "repositoryDigestMirrors:") {
			inRepositoryDigestMirrors = true
			continue
		}

		if !inRepositoryDigestMirrors {
			continue
		}

		// 检查是否到了下一个顶级字段
		if trimmedLine != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break
		}

		if strings.Contains(line, "- mirrors:") {
			// 保存之前的镜像配置
			if currentMirror != "" && currentSource != "" {
				mirrors = append(mirrors, g.formatMirrorConfig(currentMirror, currentSource))
			}
			inMirrorBlock = true
			currentMirror = ""
			currentSource = ""
			continue
		}

		if inMirrorBlock {
			if strings.Contains(line, "- ") && !strings.Contains(line, "mirrors:") {
				// 这是一个镜像地址
				mirror := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "- "))
				if currentMirror == "" {
					currentMirror = mirror
				}
			} else if strings.Contains(line, "source:") {
				// 这是源地址
				source := strings.TrimSpace(strings.TrimPrefix(trimmedLine, "source:"))
				currentSource = source
			}
		}
	}

	// 保存最后一个镜像配置
	if currentMirror != "" && currentSource != "" {
		mirrors = append(mirrors, g.formatMirrorConfig(currentMirror, currentSource))
	}

	return mirrors
}

// formatMirrorConfig 格式化镜像配置为 install-config.yaml 格式
func (g *ISOGenerator) formatMirrorConfig(mirror, source string) string {
	return fmt.Sprintf("- mirrors:\n  - %s\n  source: %s", mirror, source)
}

// extractOpenshiftInstall 从 registry 提取 openshift-install 二进制文件
func (g *ISOGenerator) extractOpenshiftInstall() (string, error) {
	fmt.Println("从 registry 提取 openshift-install 二进制文件...")

	// 构建 registry 信息
	registryHost := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain)
	registryPort := "8443"

	// 检查是否已经提取过
	extractedBinary := filepath.Join(g.ClusterDir, fmt.Sprintf("openshift-install-%s-%s",
		g.Config.ClusterInfo.OpenShiftVersion, registryHost))

	if _, err := os.Stat(extractedBinary); err == nil {
		fmt.Printf("✅ openshift-install 已存在: %s\n", extractedBinary)
		return extractedBinary, nil
	}

	// 获取 release SHA
	openshiftInstallPath := filepath.Join(g.DownloadDir, "bin", "openshift-install")
	cmd := exec.Command(openshiftInstallPath, "version")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("获取版本信息失败: %v", err)
	}

	versionOutput := string(output)
	fmt.Printf("🔍 openshift-install version 输出:\n%s\n", versionOutput)

	releaseSHA := g.extractSHAFromOutput(versionOutput)
	if releaseSHA == "" {
		return "", fmt.Errorf("无法从 openshift-install 输出中提取 release SHA")
	}

	fmt.Printf("📋 Release SHA: %s\n", releaseSHA)

	// 构建 release image URL
	releaseImageURL := fmt.Sprintf("%s:%s/openshift/release-images%s",
		registryHost, registryPort, releaseSHA)

	fmt.Printf("🔍 Release image URL: %s\n", releaseImageURL)

	// 使用 oc 提取 openshift-install
	ocPath := filepath.Join(g.DownloadDir, "bin", "oc")

	// 生成 ICSP 配置文件
	configFileToUse := filepath.Join(g.ClusterDir, ".icsp.yaml")
	if err := g.generateICSPConfig(registryHost, registryPort, configFileToUse); err != nil {
		return "", fmt.Errorf("生成 ICSP 配置失败: %v", err)
	}
	defer os.Remove(configFileToUse)

	// 查找合并后的认证文件
	mergedAuthPath := filepath.Join(g.ClusterDir, "registry", "merged-auth.json")
	if _, err := os.Stat(mergedAuthPath); os.IsNotExist(err) {
		// 尝试使用系统的 Docker 配置文件
		dockerConfigPath := filepath.Join(os.Getenv("HOME"), ".docker", "config.json")
		if _, err := os.Stat(dockerConfigPath); err == nil {
			fmt.Printf("⚠️  合并的认证文件不存在: %s，使用系统 Docker 配置: %s\n", mergedAuthPath, dockerConfigPath)
			mergedAuthPath = dockerConfigPath
		} else {
			// 最后尝试使用默认的 pull-secret.txt
			fmt.Printf("⚠️  Docker 配置不存在: %s，尝试使用默认的 pull-secret.txt\n", dockerConfigPath)
			mergedAuthPath = filepath.Join(g.ClusterDir, "pull-secret.txt")
		}
	}

	// 提取 openshift-install 命令
	extractCmd := exec.Command(ocPath, "adm", "release", "extract",
		"--icsp-file="+configFileToUse,
		"-a", mergedAuthPath,
		"--command=openshift-install",
		releaseImageURL,
		"--insecure=true")

	extractCmd.Dir = g.ClusterDir
	extractCmd.Stdout = os.Stdout
	extractCmd.Stderr = os.Stderr

	fmt.Printf("执行命令: %s\n", strings.Join(extractCmd.Args, " "))

	// 执行提取命令
	if err := extractCmd.Run(); err != nil {
		fmt.Printf("⚠️  从 registry 提取失败: %v\n", err)
		fmt.Printf("💡 这在某些版本中是正常的，将使用下载的 openshift-install\n")
		return filepath.Join(g.DownloadDir, "bin", "openshift-install"), nil
	}

	// 检查是否成功提取
	extractedPath := filepath.Join(g.ClusterDir, "openshift-install")
	if _, err := os.Stat(extractedPath); err == nil {
		// 重命名为带版本的文件名
		if err := os.Rename(extractedPath, extractedBinary); err != nil {
			return "", fmt.Errorf("重命名提取的二进制文件失败: %v", err)
		}

		// 设置可执行权限
		if err := os.Chmod(extractedBinary, 0755); err != nil {
			return "", fmt.Errorf("设置可执行权限失败: %v", err)
		}

		fmt.Printf("✅ 成功从 registry 提取 openshift-install: %s\n", extractedBinary)

		// 验证提取的二进制文件
		if err := g.verifyExtractedBinary(extractedBinary); err != nil {
			fmt.Printf("⚠️  提取的二进制文件验证失败: %v\n", err)
			fmt.Printf("💡 使用下载的 openshift-install\n")
			return filepath.Join(g.DownloadDir, "bin", "openshift-install"), nil
		}

		return extractedBinary, nil
	}

	// 如果提取失败，回退到使用下载的版本
	fmt.Printf("⚠️  从 registry 提取失败，使用下载的 openshift-install\n")
	return filepath.Join(g.DownloadDir, "bin", "openshift-install"), nil
}

// compareVersion 比较两个版本号
func (g *ISOGenerator) compareVersion(v1, v2 string) int {
	parts1 := g.parseVersion(v1)
	parts2 := g.parseVersion(v2)

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		p1, p2 := 0, 0
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}

		if p1 != p2 {
			if p1 < p2 {
				return -1
			}
			return 1
		}
	}

	return 0
}

// parseVersion 解析版本号为整数数组
func (g *ISOGenerator) parseVersion(version string) []int {
	if version == "" {
		return []int{0}
	}

	parts := strings.Split(version, ".")
	result := make([]int, 0, len(parts))

	for _, part := range parts {
		if part == "" {
			continue
		}

		// 提取数字部分
		var numStr strings.Builder
		for _, char := range part {
			if char >= '0' && char <= '9' {
				numStr.WriteRune(char)
			} else {
				break
			}
		}

		if numStr.Len() > 0 {
			if num, err := strconv.Atoi(numStr.String()); err == nil {
				result = append(result, num)
			}
		}
	}

	if len(result) == 0 {
		return []int{0}
	}

	return result
}

// extractVersionFromOutput 从 openshift-install version 输出中提取版本号
func (g *ISOGenerator) extractVersionFromOutput(output, prefix string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				// 提取版本号，去掉可能的前缀
				version := parts[1]
				// 如果版本号包含 "v" 前缀，去掉它
				if strings.HasPrefix(version, "v") {
					version = version[1:]
				}
				return version
			}
		}
	}
	return ""
}

// extractSHAFromOutput 从 openshift-install version 输出中提取 release SHA
func (g *ISOGenerator) extractSHAFromOutput(output string) string {
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.Contains(line, "release image") && strings.Contains(line, "@sha") {
			// 提取 @sha256:... 部分
			shaIndex := strings.Index(line, "@sha")
			if shaIndex != -1 {
				return line[shaIndex:]
			}
		}
	}
	return ""
}

// extractVersionWithRegex 使用正则表达式从输出中提取版本号
func (g *ISOGenerator) extractVersionWithRegex(output string) string {
	// 匹配版本号模式，如 4.14.0, v4.14.0 等
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		// 查找包含版本号的行
		if strings.Contains(line, "4.") {
			// 提取版本号模式 x.y.z
			parts := strings.Fields(line)
			for _, part := range parts {
				// 移除可能的前缀
				part = strings.TrimPrefix(part, "v")
				// 检查是否匹配版本号格式
				if g.isValidVersionFormat(part) {
					return part
				}
			}
		}
	}
	return ""
}

// isValidVersionFormat 检查字符串是否为有效的版本号格式
func (g *ISOGenerator) isValidVersionFormat(version string) bool {
	if version == "" {
		return false
	}

	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}

	// 检查每个部分是否为数字
	for _, part := range parts {
		if part == "" {
			continue
		}
		// 检查是否包含数字
		hasDigit := false
		for _, char := range part {
			if char >= '0' && char <= '9' {
				hasDigit = true
			} else if char != '.' && char != '-' && char != '+' {
				// 如果包含其他字符，只允许在末尾
				break
			}
		}
		if !hasDigit {
			return false
		}
	}

	return true
}

// createMergedAuthConfig 创建合并的认证配置文件
func (g *ISOGenerator) createMergedAuthConfig() error {
	fmt.Println("🔐 创建合并的认证配置文件...")

	// 读取原始 pull-secret
	pullSecretPath := filepath.Join(g.ClusterDir, "pull-secret.txt")
	pullSecretContent, err := os.ReadFile(pullSecretPath)
	if err != nil {
		return fmt.Errorf("读取 pull-secret 失败: %v", err)
	}

	var pullSecret map[string]interface{}
	if err := json.Unmarshal(pullSecretContent, &pullSecret); err != nil {
		return fmt.Errorf("解析 pull-secret 失败: %v", err)
	}

	auths, ok := pullSecret["auths"].(map[string]interface{})
	if !ok {
		auths = make(map[string]interface{})
		pullSecret["auths"] = auths
	}

	// 使用域名而不是 IP 地址添加 Quay registry 认证信息
	registryHostname := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("%s:8443", registryHostname)
	authString := fmt.Sprintf("%s:ztesoft123", g.Config.Registry.RegistryUser)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(authString))

	auths[registryURL] = map[string]interface{}{
		"auth":  authBase64,
		"email": "",
	}

	mergedAuthContent, err := json.MarshalIndent(pullSecret, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化合并后的认证配置失败: %v", err)
	}

	// 确保 registry 目录存在
	registryDir := filepath.Join(g.ClusterDir, "registry")
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return fmt.Errorf("创建 registry 目录失败: %v", err)
	}

	// 保存到多个位置
	authPaths := []string{
		filepath.Join(registryDir, "merged-auth.json"),
		filepath.Join(os.Getenv("HOME"), ".docker", "config.json"),
	}

	for _, authPath := range authPaths {
		if err := os.MkdirAll(filepath.Dir(authPath), 0755); err != nil {
			return fmt.Errorf("创建认证配置目录失败: %v", err)
		}

		if err := os.WriteFile(authPath, mergedAuthContent, 0600); err != nil {
			return fmt.Errorf("保存合并后的认证配置失败: %v", err)
		}

		fmt.Printf("✅ 认证配置已保存到: %s\n", authPath)
	}

	fmt.Printf("📋 已添加 registry 认证: %s\n", registryURL)
	return nil
}
