package loadimage

import (
	"embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
	
	"ocpack/pkg/config"
)

//go:embed templates/*
var templates embed.FS

// ImageLoader 镜像加载器
type ImageLoader struct {
	Config      *config.ClusterConfig
	ClusterName string
	ProjectRoot string
	ClusterDir  string
	DownloadDir string
}

// ImageSetConfig ImageSet 配置结构
type ImageSetConfig struct {
	OCPChannel        string
	OCPVerMajor       string
	OCPVer            string
	IncludeOperators  bool
	OperatorPackages  []string
	AdditionalImages  []string
	HelmCharts        bool
	HelmRepositories  []HelmRepository
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

// NewImageLoader 创建新的镜像加载器
func NewImageLoader(clusterName, projectRoot string) (*ImageLoader, error) {
	clusterDir := filepath.Join(projectRoot, clusterName)
	configPath := filepath.Join(clusterDir, "config.toml")
	
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载配置文件失败: %v", err)
	}

	downloadDir := filepath.Join(clusterDir, cfg.Download.LocalPath)

	return &ImageLoader{
		Config:      cfg,
		ClusterName: clusterName,
		ProjectRoot: projectRoot,
		ClusterDir:  clusterDir,
		DownloadDir: downloadDir,
	}, nil
}

// LoadImages 加载镜像到 registry (包含 save 和 load 两个步骤)
func (l *ImageLoader) LoadImages() error {
	fmt.Println("=== 开始镜像加载流程 ===")
	
	// 步骤1: Save - 使用 oc-mirror 保存镜像到磁盘
	fmt.Println("步骤1: 保存镜像到磁盘...")
	if err := l.SaveImages(); err != nil {
		return fmt.Errorf("保存镜像失败: %v", err)
	}
	
	// 步骤2: Load - 从磁盘加载镜像到 Quay registry
	fmt.Println("步骤2: 加载镜像到 Quay registry...")
	if err := l.LoadToRegistry(); err != nil {
		return fmt.Errorf("加载镜像到 registry 失败: %v", err)
	}
	
	fmt.Println("=== 镜像加载流程完成 ===")
	return nil
}

// SaveImages 使用 oc-mirror 保存镜像到磁盘
func (l *ImageLoader) SaveImages() error {
	// 1. 生成 ImageSet 配置文件
	imagesetConfigPath := filepath.Join(l.ClusterDir, "imageset-config-save.yaml")
	if err := l.generateImageSetConfig(imagesetConfigPath); err != nil {
		return fmt.Errorf("生成 ImageSet 配置文件失败: %v", err)
	}
	
	// 2. 创建镜像保存目录
	imagesDir := filepath.Join(l.ClusterDir, "images")
	if err := os.MkdirAll(imagesDir, 0755); err != nil {
		return fmt.Errorf("创建镜像目录失败: %v", err)
	}
	
	// 3. 使用 oc-mirror 保存镜像
	if err := l.runOcMirrorSave(imagesetConfigPath, imagesDir); err != nil {
		return fmt.Errorf("oc-mirror 保存镜像失败: %v", err)
	}
	
	fmt.Printf("镜像已保存到: %s\n", imagesDir)
	return nil
}

// generateImageSetConfig 生成 ImageSet 配置文件
func (l *ImageLoader) generateImageSetConfig(configPath string) error {
	// 提取版本信息
	version := l.Config.ClusterInfo.OpenShiftVersion
	majorVersion := l.extractMajorVersion(version)
	
	// 构建配置数据 - 目前只包含 OpenShift 平台镜像
	imagesetConfig := ImageSetConfig{
		OCPChannel:       "stable",
		OCPVerMajor:      majorVersion,
		OCPVer:           version,
		IncludeOperators: false, // 暂时不包含 operators
		OperatorPackages: []string{},
		AdditionalImages: []string{},
		HelmCharts:       false,
		HelmRepositories: []HelmRepository{},
	}
	
	// 从嵌入的文件系统读取模板
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

// generateImageSetConfigWithOperators 生成包含 operators 的 ImageSet 配置文件
func (l *ImageLoader) generateImageSetConfigWithOperators(configPath string) error {
	// 提取版本信息
	version := l.Config.ClusterInfo.OpenShiftVersion
	majorVersion := l.extractMajorVersion(version)
	
	// 构建配置数据 - 包含 operators 和其他可选组件
	imagesetConfig := ImageSetConfig{
		OCPChannel:       "stable",
		OCPVerMajor:      majorVersion,
		OCPVer:           version,
		IncludeOperators: true,
		OperatorPackages: []string{
			"advanced-cluster-management",
			"local-storage-operator",
			"ocs-operator",
			"odf-operator",
		},
		AdditionalImages: []string{
			"registry.redhat.io/ubi8/ubi:latest",
			"registry.redhat.io/ubi9/ubi:latest",
		},
		HelmCharts: true,
		HelmRepositories: []HelmRepository{
			{
				Name: "bitnami",
				URL:  "https://charts.bitnami.com/bitnami",
				Charts: []HelmChart{
					{Name: "nginx", Version: "15.0.0"},
					{Name: "postgresql", Version: "12.0.0"},
				},
			},
		},
	}
	
	// 从嵌入的文件系统读取模板
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
	
	fmt.Printf("ImageSet 配置文件已生成 (包含 operators): %s\n", configPath)
	return nil
}

// extractMajorVersion 提取主版本号
func (l *ImageLoader) extractMajorVersion(version string) string {
	// 从版本号中提取主版本（如 4.18.1 -> 4.18）
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	// 如果版本号格式不正确，返回默认版本
	return "4.18"
}

// runOcMirrorSave 运行 oc-mirror 保存命令
func (l *ImageLoader) runOcMirrorSave(configPath, imagesDir string) error {
	// 查找 oc-mirror 工具
	ocMirrorPath := filepath.Join(l.DownloadDir, "bin", "oc-mirror")
	if _, err := os.Stat(ocMirrorPath); os.IsNotExist(err) {
		return fmt.Errorf("oc-mirror 工具不存在: %s", ocMirrorPath)
	}
	
	// 构建 oc-mirror 命令
	// oc-mirror --v2 --config=imageset-config-save.yaml file://. --parallel-images 4 --retry-delay 10s --retry-times 3
	args := []string{
		"--v2",
		fmt.Sprintf("--config=%s", configPath),
		fmt.Sprintf("file://%s", imagesDir),
		"--parallel-images", "4",
		"--retry-delay", "10s",
		"--retry-times", "3",
	}
	
	fmt.Printf("执行命令: %s %v\n", ocMirrorPath, args)
	
	cmd := exec.Command(ocMirrorPath, args...)
	cmd.Dir = l.ClusterDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Run(); err != nil {
		// 检查是否是架构不兼容的问题
		if err.Error() == "fork/exec "+ocMirrorPath+": exec format error" {
			fmt.Printf("⚠️  警告: oc-mirror 工具架构不兼容当前系统\n")
			fmt.Printf("   ImageSet 配置文件已生成: %s\n", configPath)
			fmt.Printf("   请在目标 Linux 系统上手动执行以下命令:\n")
			fmt.Printf("   cd %s\n", l.ClusterDir)
			fmt.Printf("   oc-mirror --v2 --config=%s file://%s --parallel-images 4 --retry-delay 10s --retry-times 3\n", 
				filepath.Base(configPath), filepath.Base(imagesDir))
			return nil // 不返回错误，允许继续执行
		}
		return fmt.Errorf("oc-mirror 命令执行失败: %v", err)
	}
	
	return nil
}

// LoadToRegistry 从磁盘加载镜像到 Quay registry
func (l *ImageLoader) LoadToRegistry() error {
	// TODO: 实现从磁盘加载镜像到 Quay registry 的逻辑
	fmt.Println("TODO: 实现从磁盘加载镜像到 Quay registry")
	return nil
}

// getOperatorVersion 根据 OpenShift 版本获取对应的 operator catalog 版本
func getOperatorVersion(openshiftVersion string) string {
	// 简化处理，直接返回主版本号
	// 例如: 4.17.1 -> 4.17
	if len(openshiftVersion) >= 4 {
		return openshiftVersion[:4]
	}
	return openshiftVersion
}

// ValidateRegistry 验证 registry 连接
func (l *ImageLoader) ValidateRegistry() error {
	// TODO: 实现 registry 连接验证
	return nil
}

// PushImages 推送镜像到 registry
func (l *ImageLoader) PushImages(imagePath string) error {
	// TODO: 实现镜像推送逻辑
	return nil
}

// GenerateImageManifest 生成镜像清单文件
func (l *ImageLoader) GenerateImageManifest() error {
	// TODO: 实现镜像清单生成
	return nil
} 