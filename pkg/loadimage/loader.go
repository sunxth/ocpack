package loadimage

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"ocpack/pkg/config"
)

// ImageLoader 镜像加载器 - 专门负责从磁盘加载镜像到 registry
type ImageLoader struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// NewImageLoader 创建新的镜像加载器
func NewImageLoader(clusterName, projectRoot string) (*ImageLoader, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %v", err)
	}

	return &ImageLoader{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// LoadToRegistry 从磁盘加载镜像到 Quay registry
func (l *ImageLoader) LoadToRegistry() error {
	fmt.Println("=== 开始从磁盘加载镜像到 Quay registry ===")

	// 验证镜像目录是否存在
	imagesDir := filepath.Join(l.ClusterDir, "images")
	if _, err := os.Stat(imagesDir); os.IsNotExist(err) {
		return fmt.Errorf("镜像目录不存在: %s\n请先运行 'ocpack save-image' 命令保存镜像", imagesDir)
	}

	// 1. 配置CA证书 (在验证仓库之前)
	fmt.Println("步骤1: 配置CA证书...")
	if err := l.setupCACertificates(); err != nil {
		fmt.Printf("⚠️  CA证书配置失败: %v\n", err)
		fmt.Println("💡 提示: 请确保 registry 已正确部署并且证书文件存在")
	}

	// 2. 验证 registry 连接
	fmt.Println("步骤2: 验证 registry 连接...")
	if err := l.ValidateRegistry(); err != nil {
		return fmt.Errorf("registry 连接验证失败: %v", err)
	}

	// 3. 配置认证信息
	fmt.Println("步骤3: 配置认证信息...")
	if err := l.setupRegistryAuth(); err != nil {
		return fmt.Errorf("配置 registry 认证失败: %v", err)
	}

	// 4. 执行镜像加载
	fmt.Println("步骤4: 执行镜像加载...")
	if err := l.runOcMirrorLoad(); err != nil {
		return fmt.Errorf("oc-mirror 加载镜像失败: %v", err)
	}

	fmt.Println("=== 镜像加载到 Quay registry 完成 ===")
	registryHostname := fmt.Sprintf("registry.%s.%s", l.Config.ClusterInfo.Name, l.Config.ClusterInfo.Domain)
	fmt.Printf("🎉 镜像已成功加载到: https://%s:8443\n", registryHostname)
	fmt.Printf("📋 用户名: %s\n", l.Config.Registry.RegistryUser)
	fmt.Printf("🔑 密码: ztesoft123\n")
	return nil
}

// ValidateRegistry 验证 registry 连接
func (l *ImageLoader) ValidateRegistry() error {
	// 使用域名而不是 IP 地址
	registryHostname := fmt.Sprintf("registry.%s.%s", l.Config.ClusterInfo.Name, l.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("%s:8443", registryHostname)
	fmt.Printf("验证 Quay registry 连接: %s\n", registryURL)

	containerTool := l.getContainerTool()
	loginCmd := exec.Command(containerTool, "login",
		"--username", l.Config.Registry.RegistryUser,
		"--password", "ztesoft123",
		registryURL)

	fmt.Printf("执行登录测试: %s login --username %s %s\n",
		containerTool, l.Config.Registry.RegistryUser, registryURL)

	output, err := loginCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("登录失败: %v, 输出: %s", err, string(output))
	}

	fmt.Printf("✅ Quay registry 连接验证成功: %s\n", registryURL)
	return nil
}

// getContainerTool 获取可用的容器工具
func (l *ImageLoader) getContainerTool() string {
	if _, err := exec.LookPath("podman"); err == nil {
		return "podman"
	}
	if _, err := exec.LookPath("docker"); err == nil {
		return "docker"
	}
	return "podman"
}

// setupRegistryAuth 配置 registry 认证信息
func (l *ImageLoader) setupRegistryAuth() error {
	fmt.Println("配置 registry 认证信息...")

	if err := l.mergeAuthConfigs(); err != nil {
		return fmt.Errorf("合并认证配置失败: %v", err)
	}

	fmt.Println("✅ registry 认证配置完成")
	return nil
}

// mergeAuthConfigs 合并 Red Hat pull-secret 和 Quay registry 认证
func (l *ImageLoader) mergeAuthConfigs() error {
	pullSecretPath := filepath.Join(l.ClusterDir, "pull-secret.txt")
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
	registryHostname := fmt.Sprintf("registry.%s.%s", l.Config.ClusterInfo.Name, l.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("%s:8443", registryHostname)
	authString := fmt.Sprintf("%s:ztesoft123", l.Config.Registry.RegistryUser)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(authString))

	auths[registryURL] = map[string]interface{}{
		"auth":  authBase64,
		"email": "",
	}

	mergedAuthContent, err := json.MarshalIndent(pullSecret, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化合并后的认证配置失败: %v", err)
	}

	// 保存到多个位置
	authPaths := []string{
		filepath.Join(l.ClusterDir, "registry", "merged-auth.json"),
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

	return nil
}

// runOcMirrorLoad 运行 oc-mirror 加载命令
func (l *ImageLoader) runOcMirrorLoad() error {
	ocMirrorPath := filepath.Join(l.DownloadDir, "bin", "oc-mirror")
	if _, err := os.Stat(ocMirrorPath); os.IsNotExist(err) {
		return fmt.Errorf("oc-mirror 工具不存在: %s", ocMirrorPath)
	}

	// 使用域名而不是 IP 地址
	registryHostname := fmt.Sprintf("registry.%s.%s", l.Config.ClusterInfo.Name, l.Config.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("docker://%s:8443", registryHostname)
	imagesDir := filepath.Join(l.ClusterDir, "images")

	args := []string{
		fmt.Sprintf("--from=%s", imagesDir),
		registryURL,
	}

	return l.runOcMirrorCommand(ocMirrorPath, args)
}

// runOcMirrorCommand oc-mirror 命令执行器
func (l *ImageLoader) runOcMirrorCommand(ocMirrorPath string, args []string) error {
	fmt.Printf("执行命令: %s %v\n", ocMirrorPath, args)

	cmd := exec.Command(ocMirrorPath, args...)
	cmd.Dir = l.ClusterDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(),
		"REGISTRY_AUTH_FILE="+filepath.Join(l.ClusterDir, "registry", "merged-auth.json"),
	)

	if err := cmd.Run(); err != nil {
		if strings.Contains(err.Error(), "exec format error") {
			fmt.Printf("⚠️  警告: oc-mirror 工具架构不兼容当前系统\n")
			l.printManualInstructions(args)
			return nil
		}
		return fmt.Errorf("oc-mirror 命令执行失败: %v", err)
	}

	return nil
}

// printManualInstructions 打印手动执行指令
func (l *ImageLoader) printManualInstructions(args []string) {
	fmt.Printf("   请在目标 Linux 系统上手动执行以下命令:\n")
	fmt.Printf("   cd %s\n", l.ClusterDir)
	fmt.Printf("   export REGISTRY_AUTH_FILE=%s\n",
		filepath.Join(l.ClusterDir, "registry", "merged-auth.json"))
	fmt.Printf("   oc-mirror %s\n", strings.Join(args, " "))
}

// setupCACertificates 配置镜像仓库的CA证书信任
func (l *ImageLoader) setupCACertificates() error {
	fmt.Println("🔐 配置镜像仓库CA证书信任...")

	caCertPath := filepath.Join(l.ClusterDir, "registry", l.Config.Registry.IP, "rootCA.pem")

	if _, err := os.Stat(caCertPath); os.IsNotExist(err) {
		return fmt.Errorf("CA证书文件不存在: %s", caCertPath)
	}

	fmt.Printf("📄 找到CA证书: %s\n", caCertPath)

	switch runtime.GOOS {
	case "linux":
		if err := l.configureLinuxCertificateTrust(caCertPath); err != nil {
			fmt.Printf("⚠️  配置系统证书信任失败: %v\n", err)
		}
	case "darwin":
		fmt.Printf("💡 macOS用户请手动执行: sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s\n", caCertPath)
	case "windows":
		fmt.Printf("💡 Windows用户请手动将证书添加到受信任的根证书颁发机构: %s\n", caCertPath)
	default:
		fmt.Printf("⚠️  不支持的操作系统: %s\n", runtime.GOOS)
	}

	fmt.Println("✅ CA证书配置完成")
	return nil
}

// configureLinuxCertificateTrust 配置Linux系统证书信任
func (l *ImageLoader) configureLinuxCertificateTrust(caCertPath string) error {
	certDirs := []string{
		"/etc/pki/ca-trust/source/anchors",
		"/usr/local/share/ca-certificates",
	}

	var targetDir string
	for _, dir := range certDirs {
		if _, err := os.Stat(dir); err == nil {
			targetDir = dir
			break
		}
	}

	if targetDir == "" {
		fmt.Println("⚠️  未找到系统证书目录")
		return nil
	}

	certName := fmt.Sprintf("quay-registry-%s.crt",
		strings.ReplaceAll(l.Config.Registry.IP, ".", "-"))
	targetPath := filepath.Join(targetDir, certName)

	if err := l.copyFile(caCertPath, targetPath); err != nil {
		return fmt.Errorf("复制证书失败: %v", err)
	}

	updateCommands := [][]string{
		{"update-ca-trust", "extract"},
		{"update-ca-certificates"},
	}

	for _, cmd := range updateCommands {
		if _, err := exec.LookPath(cmd[0]); err != nil {
			continue
		}

		exec.Command(cmd[0], cmd[1:]...).Run()
		fmt.Printf("✅ 系统证书信任已配置: %s\n", targetPath)
		return nil
	}

	fmt.Println("⚠️  无法更新证书存储")
	return nil
}

// copyFile 复制文件
func (l *ImageLoader) copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)
	return err
}
