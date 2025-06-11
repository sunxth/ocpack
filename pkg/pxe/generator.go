package pxe

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"ocpack/pkg/config"
	"ocpack/pkg/utils"
)

//go:embed templates/*
var templates embed.FS

// PXEGenerator PXE 生成器
type PXEGenerator struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// GenerateOptions PXE 生成选项
type GenerateOptions struct {
	AssetServerURL string
	SkipVerify     bool
}

// AgentConfigDataPXE PXE 版本的 agent-config.yaml 模板数据
type AgentConfigDataPXE struct {
	ClusterName          string
	RendezvousIP         string
	Hosts                []HostConfig
	Port0                string
	PrefixLength         int
	NextHopAddress       string
	DNSServers           []string
	BootArtifactsBaseURL string
}

// HostConfig 主机配置
type HostConfig struct {
	Hostname   string
	Role       string
	MACAddress string
	IPAddress  string
	Interface  string
}

// NewPXEGenerator 创建新的 PXE 生成器
func NewPXEGenerator(clusterName, projectRoot string) (*PXEGenerator, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %v", err)
	}

	return &PXEGenerator{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// GeneratePXE 生成 PXE 文件
func (g *PXEGenerator) GeneratePXE(options *GenerateOptions) error {
	fmt.Printf("开始为集群 %s 生成 PXE 文件\n", g.ClusterName)

	// 1. 验证配置和依赖
	if err := g.ValidateConfig(); err != nil {
		return fmt.Errorf("配置验证失败: %v", err)
	}

	// 2. 创建 PXE 目录结构
	pxeDir := filepath.Join(g.ClusterDir, "pxe")
	if err := g.createPXEDirs(pxeDir); err != nil {
		return fmt.Errorf("创建 PXE 目录失败: %v", err)
	}

	// 3. 生成 install-config.yaml（复制或重新生成）
	if err := g.generateInstallConfig(pxeDir); err != nil {
		return fmt.Errorf("生成 install-config.yaml 失败: %v", err)
	}

	// 4. 生成 agent-config.yaml（包含 bootArtifactsBaseURL）
	if err := g.generateAgentConfigPXE(pxeDir, options.AssetServerURL); err != nil {
		return fmt.Errorf("生成 agent-config.yaml 失败: %v", err)
	}

	// 5. 生成 PXE 文件
	if err := g.generatePXEFiles(pxeDir, options); err != nil {
		return fmt.Errorf("生成 PXE 文件失败: %v", err)
	}

	// 6. 自动上传 PXE 文件到服务器
	if err := g.uploadPXEFiles(pxeDir); err != nil {
		fmt.Printf("⚠️  自动上传 PXE 文件失败: %v\n", err)
		fmt.Printf("\n📋 手动上传步骤:\n")
		fmt.Printf("1. 上传文件到服务器:\n")
		fmt.Printf("   ssh %s@%s 'sudo /usr/local/bin/upload-pxe-files.sh %s'\n",
			g.Config.Bastion.Username, g.Config.Bastion.IP, filepath.Join(pxeDir, "files"))
		fmt.Printf("2. 或者手动复制文件:\n")
		fmt.Printf("   scp %s/* %s@%s:/tmp/\n", filepath.Join(pxeDir, "files"), g.Config.Bastion.Username, g.Config.Bastion.IP)
		fmt.Printf("   ssh %s@%s 'sudo cp /tmp/agent.x86_64-* /var/www/html/pxe/%s/'\n",
			g.Config.Bastion.Username, g.Config.Bastion.IP, g.ClusterName)
		fmt.Printf("3. 验证文件是否可访问:\n")
		fmt.Printf("   curl http://%s:8080/pxe/%s/\n", g.Config.Bastion.IP, g.ClusterName)
	} else {
		fmt.Println("✅ PXE 文件已自动上传到服务器")
	}

	fmt.Printf("✅ PXE 文件生成完成！文件位置: %s\n", pxeDir)
	return nil
}

// ValidateConfig 验证配置
func (g *PXEGenerator) ValidateConfig() error {
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

// createPXEDirs 创建 PXE 目录结构
func (g *PXEGenerator) createPXEDirs(pxeDir string) error {
	dirs := []string{
		pxeDir,
		filepath.Join(pxeDir, "config"),
		filepath.Join(pxeDir, "files"),
	}

	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %v", dir, err)
		}
	}

	return nil
}

// generateInstallConfig 生成或复制 install-config.yaml
func (g *PXEGenerator) generateInstallConfig(pxeDir string) error {
	fmt.Println("生成 install-config.yaml...")

	configPath := filepath.Join(pxeDir, "config", "install-config.yaml")

	// 检查是否已经存在 installation/install-config.yaml
	existingConfigPath := filepath.Join(g.ClusterDir, "installation", "install-config.yaml")
	if _, err := os.Stat(existingConfigPath); err == nil {
		// 如果存在，直接复制
		fmt.Printf("📋 复制现有的 install-config.yaml: %s\n", existingConfigPath)
		return utils.CopyFile(existingConfigPath, configPath)
	}

	// 如果不存在，重新生成（复用 ISO 生成器的逻辑）
	fmt.Printf("📋 重新生成 install-config.yaml\n")
	return g.generateInstallConfigFromTemplate(configPath)
}

// generateInstallConfigFromTemplate 从模板生成 install-config.yaml
func (g *PXEGenerator) generateInstallConfigFromTemplate(configPath string) error {
	// 读取 pull-secret
	var pullSecretBytes []byte
	var err error

	mergedAuthPath := filepath.Join(g.ClusterDir, "registry", "merged-auth.json")
	if _, err := os.Stat(mergedAuthPath); err == nil {
		pullSecretBytes, err = os.ReadFile(mergedAuthPath)
		if err != nil {
			return fmt.Errorf("读取合并认证文件失败: %v", err)
		}
	} else {
		pullSecretPath := filepath.Join(g.ClusterDir, "pull-secret.txt")
		pullSecretBytes, err = os.ReadFile(pullSecretPath)
		if err != nil {
			return fmt.Errorf("读取 pull-secret 失败: %v", err)
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

	// 尝试多个可能的证书路径
	possibleCertPaths := []string{
		filepath.Join(g.ClusterDir, "registry", g.Config.Registry.IP, "rootCA.pem"),
		filepath.Join(g.ClusterDir, "registry", fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain), "rootCA.pem"),
		filepath.Join(g.ClusterDir, "registry", "rootCA.pem"),
	}

	for _, certPath := range possibleCertPaths {
		if caCertBytes, err := os.ReadFile(certPath); err == nil {
			additionalTrustBundle = string(caCertBytes)
			fmt.Printf("📋 找到证书文件: %s\n", certPath)
			break
		}
	}

	if additionalTrustBundle == "" {
		fmt.Printf("⚠️  未找到证书文件，尝试的路径:\n")
		for _, path := range possibleCertPaths {
			fmt.Printf("   - %s\n", path)
		}
	}

	// 查找并解析 ICSP 文件
	imageContentSources, err := g.findAndParseICSP()
	if err != nil {
		fmt.Printf("⚠️  查找 ICSP 文件失败: %v\n", err)
		imageContentSources = ""
	}

	// 构建模板数据
	data := struct {
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
	}{
		BaseDomain:            g.Config.ClusterInfo.Domain,
		ClusterName:           g.Config.ClusterInfo.Name,
		NumWorkers:            len(g.Config.Cluster.Worker),
		NumMasters:            len(g.Config.Cluster.ControlPlane),
		MachineNetwork:        utils.ExtractNetworkBase(g.Config.Cluster.Network.MachineNetwork),
		PrefixLength:          utils.ExtractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		HostPrefix:            23,
		PullSecret:            pullSecret,
		SSHKeyPub:             sshKeyPub,
		AdditionalTrustBundle: additionalTrustBundle,
		ImageContentSources:   imageContentSources,
		ArchShort:             "amd64",
		UseProxy:              false,
	}

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
	return nil
}

// generateAgentConfigPXE 生成包含 bootArtifactsBaseURL 的 agent-config.yaml
func (g *PXEGenerator) generateAgentConfigPXE(pxeDir, assetServerURL string) error {
	fmt.Println("生成 agent-config.yaml (PXE 版本)...")

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

	// 如果没有提供 assetServerURL，使用 Bastion 节点的 IP（端口8080避免与HAProxy冲突）
	if assetServerURL == "" {
		assetServerURL = fmt.Sprintf("http://%s:8080/pxe", g.Config.Bastion.IP)
	}

	// 构建模板数据
	data := AgentConfigDataPXE{
		ClusterName:          g.Config.ClusterInfo.Name,
		RendezvousIP:         g.Config.Cluster.ControlPlane[0].IP, // 使用第一个 master 节点的 IP
		Hosts:                hosts,
		Port0:                "ens3",
		PrefixLength:         utils.ExtractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		NextHopAddress:       utils.ExtractGateway(g.Config.Cluster.Network.MachineNetwork),
		DNSServers:           []string{g.Config.Bastion.IP},
		BootArtifactsBaseURL: assetServerURL,
	}

	// 读取模板
	tmplContent, err := templates.ReadFile("templates/agent-config-pxe.yaml")
	if err != nil {
		return fmt.Errorf("读取 agent-config-pxe 模板失败: %v", err)
	}

	// 解析和执行模板
	tmpl, err := template.New("agent-config-pxe").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("解析 agent-config-pxe 模板失败: %v", err)
	}

	configPath := filepath.Join(pxeDir, "config", "agent-config.yaml")
	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("创建 agent-config.yaml 失败: %v", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("生成 agent-config.yaml 失败: %v", err)
	}

	fmt.Printf("✅ agent-config.yaml (PXE 版本) 已生成: %s\n", configPath)
	fmt.Printf("📋 bootArtifactsBaseURL: %s\n", assetServerURL)
	return nil
}

// generatePXEFiles 生成 PXE 文件
func (g *PXEGenerator) generatePXEFiles(pxeDir string, options *GenerateOptions) error {
	fmt.Println("生成 PXE 文件...")

	// 1. 查找 openshift-install 工具
	openshiftInstallPath, err := g.findOpenshiftInstall()
	if err != nil {
		return fmt.Errorf("查找 openshift-install 失败: %v", err)
	}

	// 2. 复制配置文件到临时目录（openshift-install 会修改这些文件）
	tempDir := filepath.Join(pxeDir, "temp")
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return fmt.Errorf("创建临时目录失败: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// 复制配置文件
	if err := utils.CopyFile(
		filepath.Join(pxeDir, "config", "install-config.yaml"),
		filepath.Join(tempDir, "install-config.yaml"),
	); err != nil {
		return fmt.Errorf("复制 install-config.yaml 失败: %v", err)
	}

	if err := utils.CopyFile(
		filepath.Join(pxeDir, "config", "agent-config.yaml"),
		filepath.Join(tempDir, "agent-config.yaml"),
	); err != nil {
		return fmt.Errorf("复制 agent-config.yaml 失败: %v", err)
	}

	// 3. 生成 PXE 文件
	cmd := exec.Command(openshiftInstallPath, "agent", "create", "pxe-files", "--dir", tempDir)
	cmd.Dir = tempDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	fmt.Printf("执行命令: %s agent create pxe-files --dir %s\n", openshiftInstallPath, tempDir)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("生成 PXE 文件失败: %v", err)
	}

	// 4. 移动生成的 PXE 文件到目标目录
	filesDir := filepath.Join(pxeDir, "files")

	// 检查是否有 boot-artifacts 目录
	bootArtifactsDir := filepath.Join(tempDir, "boot-artifacts")
	if _, err := os.Stat(bootArtifactsDir); err == nil {
		// 如果存在 boot-artifacts 目录，从中复制文件
		fmt.Println("发现 boot-artifacts 目录，复制 PXE 文件...")
		bootFiles, err := os.ReadDir(bootArtifactsDir)
		if err != nil {
			return fmt.Errorf("读取 boot-artifacts 目录失败: %v", err)
		}

		for _, file := range bootFiles {
			if file.IsDir() {
				continue
			}

			srcPath := filepath.Join(bootArtifactsDir, file.Name())
			dstPath := filepath.Join(filesDir, file.Name())

			if err := utils.CopyFile(srcPath, dstPath); err != nil {
				fmt.Printf("⚠️  复制文件 %s 失败: %v\n", file.Name(), err)
			} else {
				fmt.Printf("✅ 已生成 PXE 文件: %s\n", file.Name())
			}
		}

		// 处理 iPXE 脚本，更新其中的 URL
		if err := g.updateIPXEScript(filesDir, options.AssetServerURL); err != nil {
			fmt.Printf("⚠️  更新 iPXE 脚本失败: %v\n", err)
		}
	} else {
		// 如果没有 boot-artifacts 目录，查找临时目录中的文件
		fmt.Println("未发现 boot-artifacts 目录，查找临时目录中的文件...")
		tempFiles, err := os.ReadDir(tempDir)
		if err != nil {
			return fmt.Errorf("读取临时目录失败: %v", err)
		}

		for _, file := range tempFiles {
			if file.IsDir() {
				continue
			}

			// 跳过配置文件
			if file.Name() == "install-config.yaml" || file.Name() == "agent-config.yaml" {
				continue
			}

			srcPath := filepath.Join(tempDir, file.Name())
			dstPath := filepath.Join(filesDir, file.Name())

			if err := utils.MoveFile(srcPath, dstPath); err != nil {
				fmt.Printf("⚠️  移动文件 %s 失败: %v\n", file.Name(), err)
			} else {
				fmt.Printf("✅ 已生成 PXE 文件: %s\n", file.Name())
			}
		}
	}

	fmt.Printf("✅ PXE 文件已生成到: %s\n", filesDir)
	return nil
}

// updateIPXEScript 更新 iPXE 脚本中的 URL
func (g *PXEGenerator) updateIPXEScript(filesDir, assetServerURL string) error {
	// 查找 iPXE 脚本文件
	ipxeFiles, err := filepath.Glob(filepath.Join(filesDir, "*.ipxe"))
	if err != nil {
		return fmt.Errorf("查找 iPXE 文件失败: %v", err)
	}

	if len(ipxeFiles) == 0 {
		fmt.Println("未找到 iPXE 脚本文件")
		return nil
	}

	for _, ipxeFile := range ipxeFiles {
		fmt.Printf("更新 iPXE 脚本: %s\n", filepath.Base(ipxeFile))

		// 读取原始内容
		content, err := os.ReadFile(ipxeFile)
		if err != nil {
			return fmt.Errorf("读取 iPXE 文件失败: %v", err)
		}

		// 如果没有指定 assetServerURL，使用默认值
		if assetServerURL == "" {
			assetServerURL = fmt.Sprintf("http://%s:8080/pxe/%s", g.Config.Bastion.IP, g.ClusterName)
		}

		// 更新内容中的 URL
		updatedContent := string(content)

		// 替换 iPXE 脚本中的文件路径为带集群名称的路径
		updatedContent = strings.ReplaceAll(updatedContent,
			fmt.Sprintf("http://%s:8080/pxe/agent.x86_64-initrd.img", g.Config.Bastion.IP),
			fmt.Sprintf("%s/agent.x86_64-initrd.img", assetServerURL))

		updatedContent = strings.ReplaceAll(updatedContent,
			fmt.Sprintf("http://%s:8080/pxe/agent.x86_64-vmlinuz", g.Config.Bastion.IP),
			fmt.Sprintf("%s/agent.x86_64-vmlinuz", assetServerURL))

		updatedContent = strings.ReplaceAll(updatedContent,
			fmt.Sprintf("http://%s:8080/pxe/agent.x86_64-rootfs.img", g.Config.Bastion.IP),
			fmt.Sprintf("%s/agent.x86_64-rootfs.img", assetServerURL))

		// 写回文件
		if err := os.WriteFile(ipxeFile, []byte(updatedContent), 0644); err != nil {
			return fmt.Errorf("写入 iPXE 文件失败: %v", err)
		}

		fmt.Printf("✅ iPXE 脚本已更新: %s\n", filepath.Base(ipxeFile))
	}

	return nil
}

// findOpenshiftInstall 查找 openshift-install 工具
func (g *PXEGenerator) findOpenshiftInstall() (string, error) {
	// 1. 首先检查是否有从 registry 提取的版本
	registryHost := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.Name, g.Config.ClusterInfo.Domain)
	extractedBinary := filepath.Join(g.ClusterDir, fmt.Sprintf("openshift-install-%s-%s",
		g.Config.ClusterInfo.OpenShiftVersion, registryHost))

	if _, err := os.Stat(extractedBinary); err == nil {
		fmt.Printf("✅ 使用从 registry 提取的 openshift-install: %s\n", extractedBinary)
		return extractedBinary, nil
	}

	// 2. 查找当前目录中以 openshift-install 开头的文件
	files, err := filepath.Glob(filepath.Join(g.ClusterDir, "openshift-install*"))
	if err == nil && len(files) > 0 {
		fmt.Printf("✅ 使用集群目录中的 openshift-install: %s\n", files[0])
		return files[0], nil
	}

	// 3. 使用下载的版本
	downloadedBinary := filepath.Join(g.DownloadDir, "bin", "openshift-install")
	if _, err := os.Stat(downloadedBinary); err == nil {
		fmt.Printf("✅ 使用下载的 openshift-install: %s\n", downloadedBinary)
		return downloadedBinary, nil
	}

	return "", fmt.Errorf("未找到 openshift-install 工具")
}

// findAndParseICSP 查找并解析 ICSP 文件（复用 ISO 生成器的逻辑）
func (g *PXEGenerator) findAndParseICSP() (string, error) {
	// 查找 oc-mirror workspace 目录
	workspaceDir := filepath.Join(g.ClusterDir, "oc-mirror-workspace")
	if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
		workspaceDir = filepath.Join(g.ClusterDir, "images", "oc-mirror-workspace")
		if _, err := os.Stat(workspaceDir); os.IsNotExist(err) {
			return "", fmt.Errorf("oc-mirror workspace 目录不存在")
		}
	}

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

	// 读取并解析 ICSP 文件
	icspContent, err := os.ReadFile(icspFile)
	if err != nil {
		return "", fmt.Errorf("读取 ICSP 文件失败: %v", err)
	}

	// 解析 ICSP 内容并转换为 install-config.yaml 格式
	return g.parseICSPToInstallConfig(string(icspContent))
}

// findLatestResultsDir 查找最新的 results 目录
func (g *PXEGenerator) findLatestResultsDir(workspaceDir string) (string, error) {
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
		if !g.isDirNonEmpty(dirPath) {
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

	return latestDir, nil
}

// isDirNonEmpty 检查目录是否非空
func (g *PXEGenerator) isDirNonEmpty(dirPath string) bool {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return false
	}
	return len(entries) > 0
}

// parseICSPToInstallConfig 将 ICSP 内容转换为 install-config.yaml 格式
func (g *PXEGenerator) parseICSPToInstallConfig(icspContent string) (string, error) {
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
func (g *PXEGenerator) extractRepositoryDigestMirrors(doc string) []string {
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
func (g *PXEGenerator) formatMirrorConfig(mirror, source string) string {
	return fmt.Sprintf("- mirrors:\n  - %s\n  source: %s", mirror, source)
}

// uploadPXEFiles 自动上传 PXE 文件到服务器
func (g *PXEGenerator) uploadPXEFiles(pxeDir string) error {
	fmt.Println("📤 自动上传 PXE 文件到服务器...")

	filesDir := filepath.Join(pxeDir, "files")

	// 检查文件目录是否存在
	if _, err := os.Stat(filesDir); os.IsNotExist(err) {
		return fmt.Errorf("PXE 文件目录不存在: %s", filesDir)
	}

	// 构建上传命令
	uploadCmd := fmt.Sprintf("sudo /usr/local/bin/upload-pxe-files.sh %s", filesDir)

	// 使用 SSH 执行上传脚本
	var sshCmd *exec.Cmd
	if g.Config.Bastion.SSHKeyPath != "" {
		// 使用 SSH 密钥
		sshCmd = exec.Command("ssh",
			"-i", g.Config.Bastion.SSHKeyPath,
			"-o", "StrictHostKeyChecking=no",
			fmt.Sprintf("%s@%s", g.Config.Bastion.Username, g.Config.Bastion.IP),
			uploadCmd)
	} else {
		// 使用 sshpass 和密码
		sshCmd = exec.Command("sshpass", "-p", g.Config.Bastion.Password, "ssh",
			"-o", "StrictHostKeyChecking=no",
			fmt.Sprintf("%s@%s", g.Config.Bastion.Username, g.Config.Bastion.IP),
			uploadCmd)
	}

	// 设置输出
	sshCmd.Stdout = os.Stdout
	sshCmd.Stderr = os.Stderr

	fmt.Printf("执行上传命令: %s\n", uploadCmd)

	if err := sshCmd.Run(); err != nil {
		return fmt.Errorf("执行上传脚本失败: %v", err)
	}

	fmt.Println("✅ PXE 文件已自动上传到服务器")
	return nil
}
