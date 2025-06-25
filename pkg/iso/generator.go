package iso

import (
	"bytes"
	"embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"

	"ocpack/pkg/config"
	"ocpack/pkg/utils"

	"gopkg.in/yaml.v3"
)

//go:embed templates/*
var templates embed.FS

// --- Constants ---
const (
	installDirName        = "installation"
	ignitionDirName       = "ignition"
	isoDirName            = "iso"
	tempDirName           = "temp"
	registryDirName       = "registry"
	ocMirrorWorkspaceDir  = "working-dir"
	clusterResourcesDir   = "cluster-resources"
	imagesDirName         = "images"
	installConfigFilename = "install-config.yaml"
	agentConfigFilename   = "agent-config.yaml"
	icspFilename          = "imageContentSourcePolicy.yaml"
	idmsFilename          = "idms-oc-mirror.yaml"
	pullSecretFilename    = "pull-secret.txt"
	mergedAuthFilename    = "merged-auth.json"
	tempIcspFilename      = ".icsp.yaml"
	rootCACertFilename    = "rootCA.pem"
	openshiftInstallCmd   = "openshift-install"
	ocCmd                 = "oc"
	defaultInterface      = "ens3"
	defaultHostPrefix     = 23
)

// --- Struct Definitions ---

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
	Force       bool // 新增: 用于接收 --force 标志
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

// ICSP a minimal struct for parsing ImageContentSourcePolicy
type ICSP struct {
	Spec struct {
		RepositoryDigestMirrors []struct {
			Source  string   `yaml:"source"`
			Mirrors []string `yaml:"mirrors"`
		} `yaml:"repositoryDigestMirrors"`
	} `yaml:"spec"`
}

// IDMS represents the structure for parsing ImageDigestMirrorSet YAML files
type IDMS struct {
	TypeMeta struct {
		APIVersion string `yaml:"apiVersion"`
		Kind       string `yaml:"kind"`
	} `yaml:",inline"`
	ObjectMeta struct {
		Name string `yaml:"name"`
	} `yaml:"metadata"`
	Spec struct {
		ImageDigestMirrors []struct {
			Source  string   `yaml:"source"`
			Mirrors []string `yaml:"mirrors"`
		} `yaml:"imageDigestMirrors"`
	} `yaml:"spec"`
}

// --- Main Logic ---

// NewISOGenerator 创建新的 ISO 生成器
func NewISOGenerator(clusterName, projectRoot string) (*ISOGenerator, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %w", err)
	}

	return &ISOGenerator{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// GenerateISO 作为"编排器"来协调整个 ISO 生成流程
func (g *ISOGenerator) GenerateISO(options *GenerateOptions) error {
	fmt.Printf("▶️  Starting ISO image generation for cluster %s\n", g.ClusterName)

	// --- 新增逻辑: 检查 ISO 是否已存在 ---
	installDir := filepath.Join(g.ClusterDir, installDirName)
	targetISOPath := filepath.Join(installDir, isoDirName, fmt.Sprintf("%s-agent.x86_64.iso", g.ClusterName))

	if !options.Force {
		if _, err := os.Stat(targetISOPath); err == nil {
			fmt.Printf("\n🟡 ISO 文件已存在: %s\n", targetISOPath)
			fmt.Println("   跳过生成。使用 --force 标志可强制重新生成。")
			return nil
		}
	}
	// --- 新增逻辑结束 ---

	steps := 5
	// 1. 验证配置和依赖
	fmt.Printf("➡️  Step 1/%d: Validating configuration and dependencies...\n", steps)
	if err := g.ValidateConfig(); err != nil {
		return fmt.Errorf("配置验证失败: %w", err)
	}
	fmt.Println("✅ 配置验证通过")

	// 2. 创建安装目录结构
	fmt.Printf("➡️  Step 2/%d: Creating installation directory structure...\n", steps)
	if err := g.createInstallationDirs(installDir); err != nil {
		return fmt.Errorf("创建安装目录失败: %w", err)
	}
	fmt.Println("✅ 目录结构已创建")

	// 3. 生成 install-config.yaml
	fmt.Printf("➡️  Step 3/%d: Generating install-config.yaml...\n", steps)
	if err := g.generateInstallConfig(installDir); err != nil {
		return fmt.Errorf("生成 install-config.yaml 失败: %w", err)
	}
	fmt.Println("✅ install-config.yaml 已生成")

	// 4. 生成 agent-config.yaml
	fmt.Printf("➡️  Step 4/%d: Generating agent-config.yaml...\n", steps)
	if err := g.generateAgentConfig(installDir); err != nil {
		return fmt.Errorf("生成 agent-config.yaml 失败: %w", err)
	}
	fmt.Println("✅ agent-config.yaml 已生成")

	// 5. 生成 ISO 文件
	fmt.Printf("➡️  Step 5/%d: Generating ISO file...\n", steps)
	generatedPath, err := g.generateISOFiles(installDir, targetISOPath)
	if err != nil {
		return fmt.Errorf("生成 ISO 文件失败: %w", err)
	}

	fmt.Printf("\n🎉 ISO 生成完成！\n   文件位置: %s\n", generatedPath)
	return nil
}

// --- Step Implementations ---

// ValidateConfig 验证所有前提条件
func (g *ISOGenerator) ValidateConfig() error {
	if err := config.ValidateConfig(g.Config); err != nil {
		return err
	}
	toolPath := filepath.Join(g.DownloadDir, "bin", openshiftInstallCmd)
	if _, err := os.Stat(toolPath); os.IsNotExist(err) {
		return fmt.Errorf("缺少必需的工具: %s，请先运行 'ocpack download' 命令", openshiftInstallCmd)
	}
	pullSecretPath := filepath.Join(g.ClusterDir, pullSecretFilename)
	if _, err := os.Stat(pullSecretPath); os.IsNotExist(err) {
		return fmt.Errorf("缺少 %s 文件，请先获取 Red Hat pull-secret", pullSecretFilename)
	}
	return nil
}

// createInstallationDirs 创建所需的工作目录
func (g *ISOGenerator) createInstallationDirs(installDir string) error {
	dirs := []string{
		installDir,
		filepath.Join(installDir, ignitionDirName),
		filepath.Join(installDir, isoDirName),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("创建目录 %s 失败: %w", dir, err)
		}
	}
	return nil
}

// generateInstallConfig 协调 install-config.yaml 的生成
func (g *ISOGenerator) generateInstallConfig(installDir string) error {
	pullSecret, err := g.getPullSecret()
	if err != nil {
		return err
	}

	sshKey, _ := g.getSSHKey() // SSH key is optional

	trustBundle, err := g.getAdditionalTrustBundle()
	if err != nil {
		fmt.Printf("ℹ️  未找到 CA 证书，将跳过: %v\n", err)
	}

	// 优先使用 IDMS，回退到 ICSP
	imageContentSources, err := g.findAndParseIDMS()
	if err != nil {
		fmt.Printf("ℹ️  未找到 IDMS 文件，尝试查找 ICSP: %v\n", err)
		imageContentSources, err = g.findAndParseICSP()
		if err != nil {
			fmt.Printf("ℹ️  未找到镜像源配置文件，将跳过: %v\n", err)
		}
	}

	data := InstallConfigData{
		BaseDomain:            g.Config.ClusterInfo.Domain,
		ClusterName:           g.Config.ClusterInfo.ClusterID,
		NumWorkers:            len(g.Config.Cluster.Worker),
		NumMasters:            len(g.Config.Cluster.ControlPlane),
		MachineNetwork:        utils.ExtractNetworkBase(g.Config.Cluster.Network.MachineNetwork),
		PrefixLength:          utils.ExtractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		HostPrefix:            defaultHostPrefix,
		PullSecret:            pullSecret,
		SSHKeyPub:             sshKey,
		AdditionalTrustBundle: trustBundle,
		ImageContentSources:   imageContentSources,
		ArchShort:             "amd64",
	}

	funcMap := template.FuncMap{
		"indent": func(spaces int, text string) string {
			if text == "" {
				return ""
			}
			indentStr := strings.Repeat(" ", spaces)
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				if line != "" {
					lines[i] = indentStr + line
				}
			}
			return strings.Join(lines, "\n")
		},
	}

	configPath := filepath.Join(installDir, installConfigFilename)
	return g.executeTemplate("templates/install-config.yaml", configPath, data, funcMap)
}

// generateAgentConfig 协调 agent-config.yaml 的生成
func (g *ISOGenerator) generateAgentConfig(installDir string) error {
	var hosts []HostConfig
	for _, cp := range g.Config.Cluster.ControlPlane {
		hosts = append(hosts, HostConfig{Hostname: cp.Name, Role: "master", MACAddress: cp.MAC, IPAddress: cp.IP, Interface: defaultInterface})
	}
	for _, worker := range g.Config.Cluster.Worker {
		hosts = append(hosts, HostConfig{Hostname: worker.Name, Role: "worker", MACAddress: worker.MAC, IPAddress: worker.IP, Interface: defaultInterface})
	}

	data := AgentConfigData{
		ClusterName:    g.Config.ClusterInfo.ClusterID,
		RendezvousIP:   g.Config.Cluster.ControlPlane[0].IP,
		Hosts:          hosts,
		Port0:          defaultInterface,
		PrefixLength:   utils.ExtractPrefixLength(g.Config.Cluster.Network.MachineNetwork),
		NextHopAddress: utils.ExtractGateway(g.Config.Cluster.Network.MachineNetwork),
		DNSServers:     []string{g.Config.Bastion.IP},
	}

	configPath := filepath.Join(installDir, agentConfigFilename)
	return g.executeTemplate("templates/agent-config.yaml", configPath, data, nil)
}

// generateISOFiles 协调 ISO 文件的实际生成过程
func (g *ISOGenerator) generateISOFiles(installDir, targetISOPath string) (string, error) {
	openshiftInstallPath, err := g.findOpenshiftInstall()
	if err != nil {
		return "", fmt.Errorf("查找 openshift-install 失败: %w", err)
	}

	tempDir := filepath.Join(installDir, tempDirName)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("创建临时目录失败: %w", err)
	}
	defer os.RemoveAll(tempDir)

	for _, filename := range []string{installConfigFilename, agentConfigFilename} {
		src := filepath.Join(installDir, filename)
		dst := filepath.Join(tempDir, filename)
		if err := utils.CopyFile(src, dst); err != nil {
			return "", fmt.Errorf("复制 %s 失败: %w", filename, err)
		}
	}

	fmt.Printf("ℹ️  执行命令: %s agent create image --dir %s\n", openshiftInstallPath, tempDir)
	cmd := exec.Command(openshiftInstallPath, "agent", "create", "image", "--dir", tempDir)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("生成 agent ISO 失败: %w", err)
	}

	agentISOPath := filepath.Join(tempDir, "agent.x86_64.iso")
	if err := utils.MoveFile(agentISOPath, targetISOPath); err != nil {
		return "", fmt.Errorf("移动 ISO 文件失败: %w", err)
	}

	ignitionDir := filepath.Join(installDir, ignitionDirName)
	filesToCopy := []string{"auth", ".openshift_install.log", ".openshift_install_state.json"}
	for _, file := range filesToCopy {
		srcPath := filepath.Join(tempDir, file)
		if _, err := os.Stat(srcPath); err == nil {
			dstPath := filepath.Join(ignitionDir, file)
			if err := utils.CopyFileOrDir(srcPath, dstPath); err != nil {
				fmt.Printf("⚠️  复制 %s 失败: %v\n", file, err)
			}
		}
	}

	return targetISOPath, nil
}

// --- Helper Functions ---

// executeTemplate 通用的模板执行函数
func (g *ISOGenerator) executeTemplate(templatePath, outputPath string, data interface{}, funcMap template.FuncMap) error {
	tmplContent, err := templates.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("读取模板 %s 失败: %w", templatePath, err)
	}

	tmpl := template.New(filepath.Base(templatePath))
	if funcMap != nil {
		tmpl = tmpl.Funcs(funcMap)
	}

	tmpl, err = tmpl.Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("解析模板 %s 失败: %w", templatePath, err)
	}

	file, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("创建文件 %s 失败: %w", outputPath, err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, data); err != nil {
		return fmt.Errorf("执行模板生成 %s 失败: %w", outputPath, err)
	}
	return nil
}

// getPullSecret 负责获取最终的 pull-secret 字符串
func (g *ISOGenerator) getPullSecret() (string, error) {
	mergedAuthPath := filepath.Join(g.ClusterDir, registryDirName, mergedAuthFilename)
	if _, err := os.Stat(mergedAuthPath); err == nil {
		fmt.Println("ℹ️  Using merged authentication file " + mergedAuthFilename)
		secretBytes, err := os.ReadFile(mergedAuthPath)
		if err != nil {
			return "", fmt.Errorf("failed to read merged auth file: %w", err)
		}
		return strings.TrimSpace(string(secretBytes)), nil
	}

	fmt.Println("ℹ️  Merged authentication file not found, will create and use it...")
	if err := g.createMergedAuthConfig(); err != nil {
		fmt.Printf("⚠️  Failed to create merged authentication file: %v. Will fall back to original pull-secret.\n", err)
		pullSecretPath := filepath.Join(g.ClusterDir, pullSecretFilename)
		secretBytes, err := os.ReadFile(pullSecretPath)
		if err != nil {
			return "", fmt.Errorf("failed to read original pull-secret: %w", err)
		}
		return strings.TrimSpace(string(secretBytes)), nil
	}
	return g.getPullSecret()
}

// getSSHKey 获取用户的公钥
func (g *ISOGenerator) getSSHKey() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("unable to get user home directory: %w", err)
	}
	sshKeyPath := filepath.Join(home, ".ssh", "id_rsa.pub")
	sshKeyBytes, err := os.ReadFile(sshKeyPath)
	if err != nil {
		return "", fmt.Errorf("failed to read SSH public key (%s): %w", sshKeyPath, err)
	}
	return strings.TrimSpace(string(sshKeyBytes)), nil
}

// getAdditionalTrustBundle 查找并读取自定义 CA 证书
func (g *ISOGenerator) getAdditionalTrustBundle() (string, error) {
	possibleCertPaths := []string{
		filepath.Join(g.ClusterDir, registryDirName, g.Config.Registry.IP, rootCACertFilename),
		filepath.Join(g.ClusterDir, registryDirName, rootCACertFilename),
	}
	for _, certPath := range possibleCertPaths {
		if caCertBytes, err := os.ReadFile(certPath); err == nil {
			return string(caCertBytes), nil
		}
	}
	return "", errors.New("在任何预期位置都未找到 " + rootCACertFilename)
}

// findAndParseICSP 使用健壮的 YAML 解析器
func (g *ISOGenerator) findAndParseICSP() (string, error) {
	workspaceDir, err := g.findOcMirrorWorkspace()
	if err != nil {
		return "", err
	}
	latestResultsDir, err := g.findLatestResultsDir(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("failed to find latest results directory: %w", err)
	}

	icspFile := filepath.Join(latestResultsDir, icspFilename)
	icspContent, err := os.ReadFile(icspFile)
	if err != nil {
		return "", fmt.Errorf("读取 ICSP 文件 %s 失败: %w", icspFile, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(icspContent))
	var resultBuilder strings.Builder
	for {
		var icspDoc ICSP
		if err := decoder.Decode(&icspDoc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("解析 ICSP YAML 文档失败: %w", err)
		}

		for _, rdm := range icspDoc.Spec.RepositoryDigestMirrors {
			mirrorBlock := fmt.Sprintf("- mirrors:\n  - %s\n  source: %s", strings.Join(rdm.Mirrors, "\n  - "), rdm.Source)
			resultBuilder.WriteString(mirrorBlock)
			resultBuilder.WriteString("\n")
		}
	}

	if resultBuilder.Len() == 0 {
		return "", errors.New("ICSP 文件中未找到有效的镜像源配置")
	}
	return strings.TrimSpace(resultBuilder.String()), nil
}

// findOcMirrorWorkspace 查找 oc-mirror 的工作空间
func (g *ISOGenerator) findOcMirrorWorkspace() (string, error) {
	dirsToCheck := []string{
		filepath.Join(g.ClusterDir, ocMirrorWorkspaceDir),
		filepath.Join(g.ClusterDir, imagesDirName, ocMirrorWorkspaceDir),
	}
	for _, dir := range dirsToCheck {
		if _, err := os.Stat(dir); err == nil {
			return dir, nil
		}
	}
	return "", errors.New("oc-mirror workspace 目录不存在")
}

// findLatestResultsDir 查找最新的 results-* 目录
func (g *ISOGenerator) findLatestResultsDir(workspaceDir string) (string, error) {
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
		if entries, _ := os.ReadDir(dirPath); len(entries) == 0 {
			continue // Skip empty dirs
		}

		if timeValue, err := utils.ParseTimestamp(strings.TrimPrefix(entry.Name(), "results-")); err == nil {
			if timeValue > latestTime {
				latestTime = timeValue
				latestDir = dirPath
			}
		}
	}

	if latestDir == "" {
		return "", errors.New("未找到有效的 results 目录")
	}
	return latestDir, nil
}

// findOpenshiftInstall 查找可用的 openshift-install 二进制文件
func (g *ISOGenerator) findOpenshiftInstall() (string, error) {
	// 1. 首先尝试提取的二进制文件
	registryHost := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.ClusterID, g.Config.ClusterInfo.Domain)
	extractedBinary := filepath.Join(g.ClusterDir, fmt.Sprintf("%s-%s-%s", openshiftInstallCmd, g.Config.ClusterInfo.OpenShiftVersion, registryHost))
	if _, err := os.Stat(extractedBinary); err == nil {
		fmt.Printf("ℹ️  Using openshift-install extracted from Registry: %s\n", extractedBinary)
		return extractedBinary, nil
	}

	// 2. 尝试从 registry 提取 openshift-install
	fmt.Printf("ℹ️  Attempting to extract openshift-install tool from private registry...\n")
	if err := g.extractOpenshiftInstall(); err != nil {
		fmt.Printf("⚠️  Registry extraction failed: %v\n", err)
	} else {
		// 再次检查提取的二进制文件
		if _, err := os.Stat(extractedBinary); err == nil {
			fmt.Printf("✅ Successfully extracted openshift-install from Registry: %s\n", extractedBinary)
			return extractedBinary, nil
		}
	}

	// 3. 回退到下载的二进制文件
	downloadedBinary := filepath.Join(g.DownloadDir, "bin", openshiftInstallCmd)
	if _, err := os.Stat(downloadedBinary); err == nil {
		fmt.Printf("ℹ️  Using downloaded openshift-install: %s\n", downloadedBinary)
		return downloadedBinary, nil
	}

	return "", fmt.Errorf("%s tool not found in either %s or %s", openshiftInstallCmd, extractedBinary, downloadedBinary)
}

// extractOpenshiftInstall 从私有 registry 提取 openshift-install 工具
func (g *ISOGenerator) extractOpenshiftInstall() error {
	registryHost := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.ClusterID, g.Config.ClusterInfo.Domain)

	// 构建认证文件路径
	pullSecretPath := filepath.Join(g.ClusterDir, registryDirName, mergedAuthFilename)
	if _, err := os.Stat(pullSecretPath); os.IsNotExist(err) {
		pullSecretPath = filepath.Join(g.ClusterDir, pullSecretFilename)
	}

	outputPath := filepath.Join(g.ClusterDir, fmt.Sprintf("%s-%s-%s", openshiftInstallCmd, g.Config.ClusterInfo.OpenShiftVersion, registryHost))

	// 尝试多种镜像标签格式
	imageVariants := []string{
		fmt.Sprintf("%s:8443/openshift/release-images:%s-x86_64", registryHost, g.Config.ClusterInfo.OpenShiftVersion),
		fmt.Sprintf("%s:8443/openshift/release-images:%s", registryHost, g.Config.ClusterInfo.OpenShiftVersion),
	}

	for _, imageRef := range imageVariants {
		fmt.Printf("ℹ️  Trying image reference: %s\n", imageRef)

		// 第一步：使用 skopeo 检查并获取镜像摘要
		fmt.Printf("ℹ️  Using skopeo to get image digest...\n")
		digest, err := g.getImageDigestWithSkopeo(imageRef, pullSecretPath)
		if err != nil {
			fmt.Printf("⚠️  Failed to get digest: %v\n", err)
			continue
		}

		// 第二步：使用摘要进行提取
		releaseImageWithDigest := fmt.Sprintf("%s@%s", strings.Split(imageRef, ":")[0], digest)
		fmt.Printf("ℹ️  Using digest for extraction: %s\n", releaseImageWithDigest)

		if err := g.extractWithDigest(releaseImageWithDigest, outputPath, pullSecretPath); err != nil {
			fmt.Printf("⚠️  Digest extraction failed: %v\n", err)
			// 作为备选，尝试使用标签直接提取
			if err := g.extractWithTag(imageRef, outputPath, pullSecretPath); err != nil {
				fmt.Printf("⚠️  Tag extraction also failed: %v\n", err)
				continue
			}
		}

		// 验证提取的文件
		if err := g.finalizeExtraction(outputPath); err != nil {
			fmt.Printf("⚠️  File finalization failed: %v\n", err)
			continue
		}

		return nil
	}

	return fmt.Errorf("failed to extract openshift-install from any image variant")
}

// getImageDigestWithSkopeo 使用 skopeo 获取镜像摘要
func (g *ISOGenerator) getImageDigestWithSkopeo(imageRef, authFile string) (string, error) {
	// 使用 skopeo inspect 检查镜像并获取摘要
	cmd := exec.Command("skopeo", "inspect",
		"--authfile", authFile,
		"--tls-verify=false",
		fmt.Sprintf("docker://%s", imageRef))

	fmt.Printf("ℹ️  执行命令: %s\n", cmd.String())
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("skopeo inspect 失败: %w, 输出: %s", err, string(output))
	}

	// 解析 skopeo inspect 输出
	var inspectResult struct {
		Digest string `json:"Digest"`
	}

	if err := json.Unmarshal(output, &inspectResult); err != nil {
		return "", fmt.Errorf("解析 skopeo inspect 输出失败: %w", err)
	}

	if inspectResult.Digest == "" {
		return "", fmt.Errorf("镜像摘要为空")
	}

	fmt.Printf("ℹ️  获取到镜像摘要: %s\n", inspectResult.Digest)
	return inspectResult.Digest, nil
}

// extractWithTag 使用标签提取 openshift-install（回退方法）
func (g *ISOGenerator) extractWithTag(releaseImage, outputPath, pullSecretPath string) error {
	cmd := exec.Command("oc", "adm", "release", "extract",
		"--command=openshift-install",
		"--to="+filepath.Dir(outputPath),
		"--registry-config="+pullSecretPath,
		"--insecure",
		releaseImage)

	fmt.Printf("ℹ️  执行命令: %s\n", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("提取 openshift-install 失败: %w, 输出: %s", err, string(output))
	}

	return g.finalizeExtraction(outputPath)
}

// extractWithDigest 使用摘要提取 openshift-install
func (g *ISOGenerator) extractWithDigest(releaseImage, outputPath, pullSecretPath string) error {
	cmd := exec.Command("oc", "adm", "release", "extract",
		"--command=openshift-install",
		"--to="+filepath.Dir(outputPath),
		"--registry-config="+pullSecretPath,
		"--insecure",
		releaseImage)

	fmt.Printf("ℹ️  执行命令: %s\n", cmd.String())

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("提取 openshift-install 失败: %w, 输出: %s", err, string(output))
	}

	return g.finalizeExtraction(outputPath)
}

// finalizeExtraction 完成提取过程的文件重命名和权限设置
func (g *ISOGenerator) finalizeExtraction(outputPath string) error {
	// 重命名提取的文件
	extractedFile := filepath.Join(filepath.Dir(outputPath), openshiftInstallCmd)
	if err := os.Rename(extractedFile, outputPath); err != nil {
		return fmt.Errorf("重命名提取的 openshift-install 失败: %w", err)
	}

	// 设置可执行权限
	if err := os.Chmod(outputPath, 0755); err != nil {
		return fmt.Errorf("设置 openshift-install 权限失败: %w", err)
	}

	return nil
}

// createMergedAuthConfig 创建包含私有仓库认证的 pull-secret 文件
func (g *ISOGenerator) createMergedAuthConfig() error {
	fmt.Println("🔐  Creating merged authentication configuration file...")

	pullSecretPath := filepath.Join(g.ClusterDir, pullSecretFilename)
	pullSecretContent, err := os.ReadFile(pullSecretPath)
	if err != nil {
		return fmt.Errorf("读取 %s 失败: %w", pullSecretFilename, err)
	}

	var pullSecretData map[string]interface{}
	if err := json.Unmarshal(pullSecretContent, &pullSecretData); err != nil {
		return fmt.Errorf("解析 %s JSON 失败: %w", pullSecretFilename, err)
	}

	auths, ok := pullSecretData["auths"].(map[string]interface{})
	if !ok {
		return errors.New("pull-secret.txt 格式无效: 缺少 'auths' 字段")
	}

	registryHostname := fmt.Sprintf("registry.%s.%s", g.Config.ClusterInfo.ClusterID, g.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("%s:8443", registryHostname)

	authString := fmt.Sprintf("%s:ztesoft123", g.Config.Registry.RegistryUser)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(authString))

	auths[registryURL] = map[string]interface{}{
		"auth":  authBase64,
		"email": "user@example.com",
	}

	mergedAuthContent, err := json.MarshalIndent(pullSecretData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化合并后的认证配置失败: %w", err)
	}

	registryDir := filepath.Join(g.ClusterDir, registryDirName)
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return fmt.Errorf("创建 registry 目录失败: %w", err)
	}

	mergedAuthPath := filepath.Join(registryDir, mergedAuthFilename)
	if err := os.WriteFile(mergedAuthPath, mergedAuthContent, 0600); err != nil {
		return fmt.Errorf("保存合并后的认证配置失败: %w", err)
	}

	fmt.Printf("✅  Authentication configuration saved to: %s\n", mergedAuthPath)
	return nil
}

// findAndParseIDMS 查找并解析 IDMS (ImageDigestMirrorSet) 文件
func (g *ISOGenerator) findAndParseIDMS() (string, error) {
	// 1. 首先在集群资源目录中查找
	clusterResourcesDir := filepath.Join(g.ClusterDir, imagesDirName, ocMirrorWorkspaceDir, clusterResourcesDir)
	idmsFile := filepath.Join(clusterResourcesDir, idmsFilename)
	if _, err := os.Stat(idmsFile); err == nil {
		fmt.Printf("ℹ️  Using IDMS file from cluster resources directory: %s\n", idmsFile)
		return g.parseIDMSFile(idmsFile)
	}

	// 2. 在 results 目录中查找
	workspaceDir, err := g.findOcMirrorWorkspace()
	if err != nil {
		return "", err
	}
	latestResultsDir, err := g.findLatestResultsDir(workspaceDir)
	if err != nil {
		return "", fmt.Errorf("failed to find latest results directory: %w", err)
	}

	idmsFile = filepath.Join(latestResultsDir, idmsFilename)
	if _, err := os.Stat(idmsFile); err == nil {
		fmt.Printf("ℹ️  Using IDMS file from results directory: %s\n", idmsFile)
		return g.parseIDMSFile(idmsFile)
	}

	return "", fmt.Errorf("no IDMS file found in cluster resources or results directories")
}

// parseIDMSFile 解析 IDMS 文件内容
func (g *ISOGenerator) parseIDMSFile(filePath string) (string, error) {
	idmsContent, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("读取 IDMS 文件 %s 失败: %w", filePath, err)
	}

	decoder := yaml.NewDecoder(bytes.NewReader(idmsContent))
	var resultBuilder strings.Builder

	for {
		var idmsDoc IDMS
		if err := decoder.Decode(&idmsDoc); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return "", fmt.Errorf("解析 IDMS YAML 文档失败: %w", err)
		}

		// 仅处理 ImageDigestMirrorSet 类型的文档
		if idmsDoc.TypeMeta.Kind != "ImageDigestMirrorSet" {
			continue
		}

		for _, idm := range idmsDoc.Spec.ImageDigestMirrors {
			mirrorBlock := fmt.Sprintf("- mirrors:\n  - %s\n  source: %s", strings.Join(idm.Mirrors, "\n  - "), idm.Source)
			resultBuilder.WriteString(mirrorBlock)
			resultBuilder.WriteString("\n")
		}
	}

	if resultBuilder.Len() == 0 {
		return "", errors.New("IDMS 文件中未找到有效的镜像源配置")
	}
	return strings.TrimSpace(resultBuilder.String()), nil
}
