package saveimage

import (
	"embed"
	"encoding/json"
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

// ImageSaver 镜像保存器
type ImageSaver struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// ImageSetConfig ImageSet 配置结构
type ImageSetConfig struct {
	OCPChannel       string
	OCPVerMajor      string
	OCPVer           string
	IncludeOperators bool
	OperatorPackages []string
	AdditionalImages []string
	HelmCharts       bool
	HelmRepositories []HelmRepository
	WorkspacePath    string
}

// HelmRepository Helm 仓库配置
type HelmRepository struct {
	Name   string
	URL    string
	Charts []HelmChart
}

// HelmChart Helm Chart 配置
type HelmChart struct {
	Name    string
	Version string
}

// NewImageSaver 创建新的镜像保存器
func NewImageSaver(clusterName, projectRoot string) (*ImageSaver, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %v", err)
	}

	return &ImageSaver{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: filepath.Join(clusterDir, cfg.Download.LocalPath),
	}, nil
}

// SaveImages 使用 oc-mirror 保存镜像到磁盘
func (s *ImageSaver) SaveImages() error {
	fmt.Println("📦 开始保存镜像到磁盘...")

	imagesDir := filepath.Join(s.ClusterDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("创建镜像目录失败: %v", err)
	}

	// 检查是否已经存在镜像文件（重复操作检测）
	if s.checkExistingMirrorFiles(imagesDir) {
		fmt.Println("✅ 镜像文件已存在，跳过下载")
		return nil
	}

	// 检查和处理 pull-secret
	if err := s.HandlePullSecret(); err != nil {
		return fmt.Errorf("处理 pull-secret 失败: %v", err)
	}

	imagesetConfigPath := filepath.Join(s.ClusterDir, "imageset-config-save.yaml")
	if err := s.generateImageSetConfig(imagesetConfigPath, false); err != nil {
		return fmt.Errorf("生成 ImageSet 配置文件失败: %v", err)
	}

	if err := s.runOcMirrorSave(imagesetConfigPath, imagesDir); err != nil {
		return fmt.Errorf("oc-mirror 保存镜像失败: %v", err)
	}

	fmt.Printf("✅ 镜像已保存到: %s\n", imagesDir)
	return nil
}

// checkExistingMirrorFiles 检查是否已经存在镜像文件
func (s *ImageSaver) checkExistingMirrorFiles(imagesDir string) bool {
	// 读取 images 目录下的文件
	files, err := os.ReadDir(imagesDir)
	if err != nil {
		return false
	}

	// 检查是否存在 mirror 开头的 tar 文件
	for _, file := range files {
		if !file.IsDir() && strings.HasPrefix(file.Name(), "mirror") && strings.HasSuffix(file.Name(), ".tar") {
			// 获取文件信息
			filePath := filepath.Join(imagesDir, file.Name())
			if fileInfo, err := os.Stat(filePath); err == nil {
				fmt.Printf("📦 发现镜像文件: %s (%.1f GB)\n", file.Name(), float64(fileInfo.Size())/(1024*1024*1024))
			}
			return true
		}
	}

	return false
}

// generateImageSetConfig 生成 ImageSet 配置文件
func (s *ImageSaver) generateImageSetConfig(configPath string, includeOperators bool) error {
	version := s.Config.ClusterInfo.OpenShiftVersion
	majorVersion := s.extractMajorVersion(version)

	imagesDir := filepath.Join(s.ClusterDir, "images")
	workspacePath := filepath.Join(imagesDir, "oc-mirror-workspace")

	if err := os.MkdirAll(workspacePath, 0755); err != nil {
		return fmt.Errorf("创建 oc-mirror workspace 目录失败: %v", err)
	}

	imagesetConfig := ImageSetConfig{
		OCPChannel:       "stable",
		OCPVerMajor:      majorVersion,
		OCPVer:           version,
		IncludeOperators: includeOperators,
		WorkspacePath:    workspacePath,
	}

	if includeOperators {
		imagesetConfig.OperatorPackages = []string{
			"advanced-cluster-management",
			"local-storage-operator",
			"ocs-operator",
			"odf-operator",
		}
		imagesetConfig.AdditionalImages = []string{
			"registry.redhat.io/ubi8/ubi:latest",
			"registry.redhat.io/ubi9/ubi:latest",
		}
		imagesetConfig.HelmCharts = true
		imagesetConfig.HelmRepositories = []HelmRepository{
			{
				Name: "bitnami",
				URL:  "https://charts.bitnami.com/bitnami",
				Charts: []HelmChart{
					{Name: "nginx", Version: "15.0.0"},
					{Name: "postgresql", Version: "12.0.0"},
				},
			},
		}
	}

	tmplContent, err := templates.ReadFile("templates/imageset-config.yaml")
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %v", err)
	}

	tmpl, err := template.New("imageset").Parse(string(tmplContent))
	if err != nil {
		return fmt.Errorf("解析模板失败: %v", err)
	}

	file, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("创建配置文件失败: %v", err)
	}
	defer file.Close()

	if err := tmpl.Execute(file, imagesetConfig); err != nil {
		return fmt.Errorf("生成配置文件失败: %v", err)
	}

	fmt.Printf("ImageSet 配置文件已生成: %s\n", configPath)
	return nil
}

// extractMajorVersion 提取主版本号
func (s *ImageSaver) extractMajorVersion(version string) string {
	return utils.ExtractMajorVersion(version)
}

// runOcMirrorSave 运行 oc-mirror 保存命令
func (s *ImageSaver) runOcMirrorSave(configPath, imagesDir string) error {
	ocMirrorPath := filepath.Join(s.DownloadDir, "bin", "oc-mirror")
	if _, err := os.Stat(ocMirrorPath); os.IsNotExist(err) {
		return fmt.Errorf("oc-mirror 工具不存在: %s", ocMirrorPath)
	}

	args := []string{
		fmt.Sprintf("--config=%s", configPath),
		fmt.Sprintf("file://%s", imagesDir),
	}

	return s.runOcMirrorCommand(ocMirrorPath, args)
}

// runOcMirrorCommand oc-mirror 命令执行器
func (s *ImageSaver) runOcMirrorCommand(ocMirrorPath string, args []string) error {
	fmt.Printf("执行命令: %s %v\n", ocMirrorPath, args)

	cmd := exec.Command(ocMirrorPath, args...)
	cmd.Dir = s.ClusterDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		if strings.Contains(err.Error(), "exec format error") {
			fmt.Printf("⚠️  警告: oc-mirror 工具架构不兼容当前系统\n")
			s.printManualInstructions(args)
			return nil
		}
		return fmt.Errorf("oc-mirror 命令执行失败: %v", err)
	}

	return nil
}

// printManualInstructions 打印手动执行指令
func (s *ImageSaver) printManualInstructions(args []string) {
	fmt.Printf("   请在目标 Linux 系统上手动执行以下命令:\n")
	fmt.Printf("   cd %s\n", s.ClusterDir)
	fmt.Printf("   oc-mirror %s\n", strings.Join(args, " "))
}

// HandlePullSecret 处理 pull-secret 文件
func (s *ImageSaver) HandlePullSecret() error {
	pullSecretPath := filepath.Join(s.ClusterDir, "pull-secret.txt")

	if _, err := os.Stat(pullSecretPath); os.IsNotExist(err) {
		return fmt.Errorf(`pull-secret.txt 文件不存在

请按照以下步骤获取 pull-secret:
1. 访问 https://console.redhat.com/openshift/install/pull-secret
2. 登录您的 Red Hat 账户
3. 下载 pull-secret 文件
4. 将文件保存为: %s`, pullSecretPath)
	}

	formattedContent, err := s.validateAndFormatPullSecret(pullSecretPath)
	if err != nil {
		return fmt.Errorf("pull-secret 文件处理失败: %v", err)
	}

	// 保存格式化版本到必要位置
	savePaths := map[string]string{
		"docker": filepath.Join(os.Getenv("HOME"), ".docker", "config.json"),
	}

	for name, path := range savePaths {
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("创建%s目录失败: %v", name, err)
		}

		if err := os.WriteFile(path, formattedContent, 0600); err != nil {
			continue // 静默处理失败
		}
	}

	return nil
}

// validateAndFormatPullSecret 验证并格式化 pull-secret 文件
func (s *ImageSaver) validateAndFormatPullSecret(filePath string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取文件失败: %v", err)
	}

	content = []byte(strings.TrimSpace(string(content)))

	var pullSecret map[string]interface{}
	if err := json.Unmarshal(content, &pullSecret); err != nil {
		return nil, fmt.Errorf("pull-secret 不是有效的 JSON 格式: %v", err)
	}

	if _, exists := pullSecret["auths"]; !exists {
		return nil, fmt.Errorf("pull-secret 缺少 'auths' 字段")
	}

	auths, ok := pullSecret["auths"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("pull-secret 中的 'auths' 字段格式不正确")
	}

	// 验证必要的 registry
	requiredRegistries := []string{
		"cloud.openshift.com",
		"quay.io",
		"registry.redhat.io",
		"registry.connect.redhat.com",
	}

	missingRegistries := make([]string, 0)
	for _, required := range requiredRegistries {
		if _, exists := auths[required]; !exists {
			missingRegistries = append(missingRegistries, required)
		}
	}

	if len(missingRegistries) > 0 {
		fmt.Printf("⚠️  pull-secret 缺少部分 registry 认证信息\n")
	}

	formattedContent, err := json.MarshalIndent(pullSecret, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("格式化 JSON 失败: %v", err)
	}

	return formattedContent, nil
}
