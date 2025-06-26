package wrapper

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ocpack/pkg/config"
	"ocpack/pkg/mirror/api/v2alpha1"
	"ocpack/pkg/mirror/cli"
	clog "ocpack/pkg/mirror/log"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// MirrorWrapper oc-mirror 功能的内置包装器
type MirrorWrapper struct {
	log      clog.PluggableLoggerInterface
	executor *cli.ExecutorSchema
}

// MirrorOptions 镜像操作选项
type MirrorOptions struct {
	ClusterName string
	ConfigPath  string
	Port        uint16
	DryRun      bool
	Force       bool
	// 重试相关配置
	EnableRetry   bool // 是否启用重试
	MaxRetries    int  // 最大重试次数，默认为 2
	RetryInterval int  // 重试间隔(秒)，默认为 30
}

// NewMirrorWrapper 创建新的镜像包装器
func NewMirrorWrapper(logLevel string) (*MirrorWrapper, error) {
	log := clog.New(logLevel)

	return &MirrorWrapper{
		log: log,
	}, nil
}

// MirrorToDisk 执行镜像到磁盘操作
func (w *MirrorWrapper) MirrorToDisk(cfg *config.ClusterConfig, destination string, opts *MirrorOptions) error {
	w.log.Info("🔄 Mirroring to disk...")

	// 定义执行函数
	executeFunc := func() error {
		// 设置缓存目录，避免使用默认的 $HOME/.oc-mirror
		// 注意：MirrorToDisk 不需要自定义 workspace，oc-mirror 会在目标目录自动创建
		_, cacheDir, err := w.setupWorkspaceAndCache(cfg, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to setup cache directory: %v", err)
		}

		// 设置认证配置
		authFilePath, err := w.setupAuthentication(cfg, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to setup authentication: %v", err)
		}

		// 优先使用内置生成的配置（从 config.toml 读取）
		w.log.Info("📋 Loading config...")
		mirrorConfig, err := w.generateMirrorConfig(cfg)
		if err != nil {
			return fmt.Errorf("failed to generate mirror config: %v", err)
		}

		tempConfigPath, err := w.createTempMirrorConfig(mirrorConfig, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to create temporary config file: %v", err)
		}
		defer os.Remove(tempConfigPath)

		cmd := cli.NewMirrorCmd(w.log)

		// 设置Command arguments
		// 注意：对于 mirror-to-disk 操作（目标是 file://），不需要 --workspace 参数
		args := []string{
			"-c", tempConfigPath,
			"--v2",
			"-p", strconv.Itoa(int(opts.Port)),
			"--cache-dir", cacheDir, // 明确指定缓存目录
			"--src-tls-verify=false",
			"--dest-tls-verify=false",
		}

		if opts.DryRun {
			args = append(args, "--dry-run")
		}

		if opts.Force {
			args = append(args, "--force")
		}

		// 添加目标路径
		args = append(args, destination)

		// 添加认证文件参数（如果存在）
		if authFilePath != "" {
			args = append(args, "--authfile", authFilePath)
			w.log.Debug("Using authentication file: %s", authFilePath)
		}

		cmd.SetArgs(args)

		w.log.Debug("Command arguments: %v", args)
		w.log.Info("💾 Cache: %s", cacheDir)

		err = cmd.Execute()
		if err != nil {
			// 检查错误是否提到了部分失败但成功率较高的情况
			if strings.Contains(err.Error(), "some errors occurred during the mirroring") {
				// 这表示有部分镜像失败，但可能不是致命错误
				w.log.Warn("⚠️  Some issues encountered during mirroring process, but may not affect overall deployment")
				w.log.Warn("   Details: %v", err)
				w.log.Info("💡 Suggestion: You can choose to ignore individual image failures and continue with subsequent deployment")
				w.log.Info("   If deployment issues occur later, you can re-run this command to retry failed images")
			}
			return err
		}

		w.log.Info("✅ Mirror operation completed")
		return nil
	}

	// 使用重试机制执行
	return w.executeWithRetry(executeFunc, destination, opts)
}

// DiskToMirror 执行磁盘到仓库操作
func (w *MirrorWrapper) DiskToMirror(cfg *config.ClusterConfig, source, destination string, opts *MirrorOptions) error {
	w.log.Info("🔄 Disk to mirror...")

	// 定义执行函数
	executeFunc := func() error {
		// 设置工作空间和缓存目录，避免使用默认的 $HOME/.oc-mirror
		workspaceDir, cacheDir, err := w.setupWorkspaceAndCache(cfg, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to setup workspace: %v", err)
		}

		// 设置认证配置
		authFilePath, err := w.setupAuthentication(cfg, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to setup authentication: %v", err)
		}

		// 优先使用内置生成的配置（从 config.toml 读取）
		w.log.Info("📋 Loading config...")
		mirrorConfig, err := w.generateMirrorConfig(cfg)
		if err != nil {
			return fmt.Errorf("failed to generate mirror config: %v", err)
		}

		tempConfigPath, err := w.createTempMirrorConfig(mirrorConfig, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to create temporary config file: %v", err)
		}
		defer os.Remove(tempConfigPath)

		cmd := cli.NewMirrorCmd(w.log)

		// 设置Command arguments
		args := []string{
			"-c", tempConfigPath,
			"--v2",
			"-p", strconv.Itoa(int(opts.Port)),
			"--from", source,
			"--workspace", workspaceDir, // 明确指定工作空间
			"--cache-dir", cacheDir, // 明确指定缓存目录
			"--src-tls-verify=false",
			"--dest-tls-verify=false",
		}

		// 添加认证文件参数（如果存在）
		if authFilePath != "" {
			args = append(args, "--authfile", authFilePath)
			w.log.Debug("Using authentication file: %s", authFilePath)
		}

		if opts.DryRun {
			args = append(args, "--dry-run")
		}

		if opts.Force {
			args = append(args, "--force")
		}

		// 添加目标路径
		args = append(args, destination)

		cmd.SetArgs(args)

		w.log.Debug("Command arguments: %v", args)
		w.log.Info("💾 Using workspace: %s", workspaceDir)
		w.log.Info("💾 Using cache: %s", cacheDir)

		err = cmd.Execute()
		if err != nil {
			// 检查错误是否提到了部分失败但成功率较高的情况
			if strings.Contains(err.Error(), "some errors occurred during the mirroring") {
				// 这表示有部分镜像失败，但可能不是致命错误
				w.log.Warn("⚠️  Some issues encountered during mirroring process, but may not affect overall deployment")
				w.log.Warn("   Details: %v", err)
				w.log.Info("💡 Suggestion: You can choose to ignore individual image failures and continue with subsequent deployment")
				w.log.Info("   If deployment issues occur later, you can re-run this command to retry failed images")
			}
			return err
		}

		w.log.Info("✅ Mirror operation completed")
		return nil
	}

	// 使用重试机制执行
	return w.executeWithRetry(executeFunc, source, opts)
}

// MirrorDirect 执行直接镜像操作
func (w *MirrorWrapper) MirrorDirect(cfg *config.ClusterConfig, workspace, destination string, opts *MirrorOptions) error {
	w.log.Info("🔄 Mirror to mirror...")

	// 定义执行函数
	executeFunc := func() error {
		// 设置工作空间和缓存目录，避免使用默认的 $HOME/.oc-mirror
		workspaceDir, cacheDir, err := w.setupWorkspaceAndCache(cfg, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to setup workspace: %v", err)
		}

		// 生成 oc-mirror 配置
		mirrorConfig, err := w.generateMirrorConfig(cfg)
		if err != nil {
			return fmt.Errorf("failed to generate mirror config: %v", err)
		}

		// 创建临时配置文件
		tempConfigPath, err := w.createTempMirrorConfig(mirrorConfig, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to create temporary config file: %v", err)
		}
		defer os.Remove(tempConfigPath)

		cmd := cli.NewMirrorCmd(w.log)

		// 如果用户提供了workspace参数，使用用户的，否则使用我们计算的
		if workspace == "" {
			workspace = workspaceDir
		}

		// 设置Command arguments
		args := []string{
			"-c", tempConfigPath,
			"--v2",
			"-p", strconv.Itoa(int(opts.Port)),
			"--workspace", workspace,
			"--cache-dir", cacheDir, // 明确指定缓存目录
			"--src-tls-verify=false",
			"--dest-tls-verify=false",
		}

		// 添加认证文件参数（如果存在）
		authFilePath, err := w.setupAuthentication(cfg, opts.ClusterName)
		if err != nil {
			return fmt.Errorf("failed to setup authentication: %v", err)
		}
		if authFilePath != "" {
			args = append(args, "--authfile", authFilePath)
			w.log.Debug("Using authentication file: %s", authFilePath)
		}

		if opts.DryRun {
			args = append(args, "--dry-run")
		}

		if opts.Force {
			args = append(args, "--force")
		}

		// 添加目标路径
		args = append(args, destination)

		cmd.SetArgs(args)

		w.log.Debug("Command arguments: %v", args)
		w.log.Info("💾 Using workspace: %s", workspace)
		w.log.Info("💾 Using cache: %s", cacheDir)

		err = cmd.Execute()
		if err != nil {
			// 检查错误是否提到了部分失败但成功率较高的情况
			if strings.Contains(err.Error(), "some errors occurred during the mirroring") {
				// 这表示有部分镜像失败，但可能不是致命错误
				w.log.Warn("⚠️  Some issues encountered during mirroring process, but may not affect overall deployment")
				w.log.Warn("   Details: %v", err)
				w.log.Info("💡 Suggestion: You can choose to ignore individual image failures and continue with subsequent deployment")
				w.log.Info("   If deployment issues occur later, you can re-run this command to retry failed images")
			}
			return err
		}

		w.log.Info("✅ Mirror operation completed")
		return nil
	}

	// 使用重试机制执行
	return w.executeWithRetry(executeFunc, workspace, opts)
}

// generateMirrorConfig 根据 ocpack 配置生成 oc-mirror 配置
func (w *MirrorWrapper) generateMirrorConfig(cfg *config.ClusterConfig) (*v2alpha1.ImageSetConfiguration, error) {
	mirrorConfig := &v2alpha1.ImageSetConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "mirror.openshift.io/v2alpha1",
			Kind:       "ImageSetConfiguration",
		},
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			ArchiveSize: 10, // 默认 10GB
			Mirror: v2alpha1.Mirror{
				Platform: v2alpha1.Platform{
					Channels: []v2alpha1.ReleaseChannel{
						{
							Name:       "stable-" + extractMajorMinorVersion(cfg.ClusterInfo.OpenShiftVersion),
							MinVersion: cfg.ClusterInfo.OpenShiftVersion,
							MaxVersion: cfg.ClusterInfo.OpenShiftVersion,
						},
					},
				},
			},
		},
	}

	// 添加 Operators 配置（如果启用）
	if cfg.SaveImage.IncludeOperators && len(cfg.SaveImage.Ops) > 0 {
		w.log.Info("📦 Including Operator images: %d operators", len(cfg.SaveImage.Ops))

		// 构建 packages 列表
		var packages []v2alpha1.IncludePackage
		for _, opName := range cfg.SaveImage.Ops {
			packages = append(packages, v2alpha1.IncludePackage{
				Name: opName,
				// 可以根据需要添加更多配置，如 channels, minVersion, maxVersion
			})
		}

		operator := v2alpha1.Operator{
			Catalog: cfg.GetOperatorCatalog(),
			IncludeConfig: v2alpha1.IncludeConfig{
				Packages: packages,
			},
		}

		mirrorConfig.ImageSetConfigurationSpec.Mirror.Operators = []v2alpha1.Operator{operator}
	}

	// 添加额外镜像配置（如果有）
	if len(cfg.SaveImage.AdditionalImages) > 0 {
		w.log.Info("📦 Including additional images: %d images", len(cfg.SaveImage.AdditionalImages))

		var additionalImages []v2alpha1.Image
		for _, imgName := range cfg.SaveImage.AdditionalImages {
			additionalImages = append(additionalImages, v2alpha1.Image{
				Name: imgName,
			})
		}

		mirrorConfig.ImageSetConfigurationSpec.Mirror.AdditionalImages = additionalImages
	}

	return mirrorConfig, nil
}

// createTempMirrorConfig 创建临时的 oc-mirror 配置文件
func (w *MirrorWrapper) createTempMirrorConfig(config *v2alpha1.ImageSetConfiguration, clusterName string) (string, error) {
	// 创建临时目录
	tempDir := filepath.Join(os.TempDir(), "ocpack-mirror", clusterName)
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create temporary directory: %v", err)
	}

	// 创建配置文件路径
	configPath := filepath.Join(tempDir, "mirror-config.yaml")

	// 这里需要将配置序列化为 YAML
	// 暂时使用简化的配置生成
	configContent := w.generateConfigYAML(config)

	// 写入文件
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		return "", fmt.Errorf("写入配置文件失败: %v", err)
	}

	w.log.Debug("临时配置文件创建: %s", configPath)
	w.log.Debug("配置文件内容:\n%s", configContent)
	return configPath, nil
}

// generateConfigYAML 生成 YAML 配置内容
func (w *MirrorWrapper) generateConfigYAML(config *v2alpha1.ImageSetConfiguration) string {
	yaml := fmt.Sprintf(`apiVersion: %s
kind: %s
mirror:
  platform:
    channels:
`, config.APIVersion, config.Kind)

	// 添加平台通道
	for _, channel := range config.ImageSetConfigurationSpec.Mirror.Platform.Channels {
		yaml += fmt.Sprintf(`    - name: %s
      minVersion: %s
      maxVersion: %s
`, channel.Name, channel.MinVersion, channel.MaxVersion)
	}

	// 添加额外镜像
	if len(config.ImageSetConfigurationSpec.Mirror.AdditionalImages) > 0 {
		yaml += "  additionalImages:\n"
		for _, img := range config.ImageSetConfigurationSpec.Mirror.AdditionalImages {
			yaml += fmt.Sprintf("    - name: %s\n", img.Name)
		}
	}

	// 添加 Operator
	if len(config.ImageSetConfigurationSpec.Mirror.Operators) > 0 {
		yaml += "  operators:\n"
		for _, op := range config.ImageSetConfigurationSpec.Mirror.Operators {
			yaml += fmt.Sprintf("    - catalog: %s\n", op.Catalog)
			if len(op.Packages) > 0 {
				yaml += "      packages:\n"
				for _, pkg := range op.Packages {
					yaml += fmt.Sprintf("        - name: %s\n", pkg.Name)
				}
			}
		}
	}

	return yaml
}

// extractMajorMinorVersion 从版本字符串中提取主版本号和次版本号
func extractMajorMinorVersion(version string) string {
	// 简单的版本提取，例如 "4.16.0" -> "4.16"
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	return version
}

// parseErrorLogFile 解析错误日志文件，提取失败的镜像
func (w *MirrorWrapper) parseErrorLogFile(logFilePath string) ([]string, error) {
	if _, err := os.Stat(logFilePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("错误日志文件不存在: %s", logFilePath)
	}

	file, err := os.Open(logFilePath)
	if err != nil {
		return nil, fmt.Errorf("无法打开错误日志文件: %v", err)
	}
	defer file.Close()

	var failedImages []string
	// 正则表达式匹配错误日志中的镜像名称
	// 格式: error mirroring image docker://registry.redhat.io/...@sha256:...
	imageRegex := regexp.MustCompile(`error mirroring image (?:docker://)?([^@\s]+@sha256:[a-f0-9]+)`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		matches := imageRegex.FindStringSubmatch(line)
		if len(matches) > 1 {
			failedImages = append(failedImages, matches[1])
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取错误日志文件失败: %v", err)
	}

	// 去重
	uniqueImages := make(map[string]bool)
	var result []string
	for _, img := range failedImages {
		if !uniqueImages[img] {
			uniqueImages[img] = true
			result = append(result, img)
		}
	}

	return result, nil
}

// findLatestErrorLog 查找最新的错误日志文件
func (w *MirrorWrapper) findLatestErrorLog(workingDir string) (string, error) {
	logsDir := filepath.Join(workingDir, "logs")
	if _, err := os.Stat(logsDir); os.IsNotExist(err) {
		return "", fmt.Errorf("日志目录不存在: %s", logsDir)
	}

	files, err := os.ReadDir(logsDir)
	if err != nil {
		return "", fmt.Errorf("无法读取日志目录: %v", err)
	}

	var latestFile string
	var latestTime time.Time

	for _, file := range files {
		if strings.HasPrefix(file.Name(), "mirroring_errors_") && strings.HasSuffix(file.Name(), ".txt") {
			info, err := file.Info()
			if err != nil {
				continue
			}
			if info.ModTime().After(latestTime) {
				latestTime = info.ModTime()
				latestFile = filepath.Join(logsDir, file.Name())
			}
		}
	}

	if latestFile == "" {
		return "", fmt.Errorf("未找到错误日志文件")
	}

	return latestFile, nil
}

// createRetryConfig 为重试创建特殊的配置文件，只包含失败的镜像
func (w *MirrorWrapper) createRetryConfig(cfg *config.ClusterConfig, failedImages []string, clusterName string) (string, error) {
	// 创建只包含失败镜像的配置
	retryConfig := &v2alpha1.ImageSetConfiguration{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "mirror.openshift.io/v2alpha1",
			Kind:       "ImageSetConfiguration",
		},
		ImageSetConfigurationSpec: v2alpha1.ImageSetConfigurationSpec{
			ArchiveSize: 10,
			Mirror: v2alpha1.Mirror{
				AdditionalImages: []v2alpha1.Image{},
			},
		},
	}

	// 将失败的镜像添加为额外镜像
	for _, imgName := range failedImages {
		retryConfig.ImageSetConfigurationSpec.Mirror.AdditionalImages = append(
			retryConfig.ImageSetConfigurationSpec.Mirror.AdditionalImages,
			v2alpha1.Image{Name: imgName},
		)
	}

	return w.createTempMirrorConfig(retryConfig, clusterName+"-retry")
}

// executeWithRetry 执行带重试的镜像操作
func (w *MirrorWrapper) executeWithRetry(executeFunc func() error, workingDir string, opts *MirrorOptions) error {
	if !opts.EnableRetry {
		return executeFunc()
	}

	maxRetries := opts.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2 // 默认重试2次
	}

	retryInterval := opts.RetryInterval
	if retryInterval <= 0 {
		retryInterval = 30 // 默认间隔30秒
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt == 0 {
			w.log.Info("🔄 Starting mirror operation (attempt %d/%d)", attempt+1, maxRetries+1)
		} else {
			w.log.Info("🔄 Retrying mirror operation (attempt %d/%d)", attempt+1, maxRetries+1)
		}

		err := executeFunc()
		if err == nil {
			w.log.Info("✅ Mirror operation completed successfully")
			return nil
		}

		lastErr = err

		// 检查是否包含部分失败的提示 - 如果已经有高成功率，不需要重试
		if strings.Contains(err.Error(), "some errors occurred during the mirroring") {
			w.log.Info("✅ Mirror operation partially successful with high success rate, no retry needed")
			return nil
		}

		// 如果还有重试机会，尝试重试失败的镜像
		if attempt < maxRetries {
			w.log.Warn("❌ Mirror operation failed: %v", err)
			w.log.Info("⏰ Waiting %d seconds before retry...", retryInterval)
			time.Sleep(time.Duration(retryInterval) * time.Second)
		}
	}

	w.log.Error("❌ Mirror operation failed after %d retries", maxRetries)
	return fmt.Errorf("mirror operation failed after %d retries: %v", maxRetries, lastErr)
}

// setupAuthentication 设置认证配置
func (w *MirrorWrapper) setupAuthentication(cfg *config.ClusterConfig, clusterName string) (string, error) {
	// 获取当前工作目录
	workingDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get working directory: %v", err)
	}

	clusterDir := filepath.Join(workingDir, clusterName)
	pullSecretPath := filepath.Join(clusterDir, "pull-secret.txt")
	mergedAuthPath := filepath.Join(clusterDir, "registry", "merged-auth.json")

	// 检查 pull-secret.txt 是否存在
	if _, err := os.Stat(pullSecretPath); os.IsNotExist(err) {
		w.log.Warn("⚠️  pull-secret.txt 不存在，将使用默认认证配置")
		return "", nil
	}

	// 检查合并认证文件是否已存在
	if _, err := os.Stat(mergedAuthPath); err == nil {
		w.log.Info("ℹ️  Using existing authentication configuration: %s", mergedAuthPath)
		return mergedAuthPath, nil
	}

	// 创建合并认证配置
	w.log.Info("🔐 创建合并的认证配置文件...")

	pullSecretContent, err := os.ReadFile(pullSecretPath)
	if err != nil {
		return "", fmt.Errorf("读取 pull-secret.txt 失败: %v", err)
	}

	var pullSecretData map[string]interface{}
	if err := json.Unmarshal(pullSecretContent, &pullSecretData); err != nil {
		return "", fmt.Errorf("解析 pull-secret.txt JSON 失败: %v", err)
	}

	auths, ok := pullSecretData["auths"].(map[string]interface{})
	if !ok {
		return "", fmt.Errorf("pull-secret.txt 格式无效: 缺少 'auths' 字段")
	}

	// 添加私有仓库认证信息
	registryHostname := fmt.Sprintf("registry.%s.%s", cfg.ClusterInfo.ClusterID, cfg.ClusterInfo.Domain)
	registryURL := fmt.Sprintf("%s:8443", registryHostname)
	authString := fmt.Sprintf("%s:ztesoft123", cfg.Registry.RegistryUser)
	authBase64 := base64.StdEncoding.EncodeToString([]byte(authString))

	auths[registryURL] = map[string]interface{}{
		"auth":  authBase64,
		"email": "user@example.com",
	}

	mergedAuthContent, err := json.MarshalIndent(pullSecretData, "", "  ")
	if err != nil {
		return "", fmt.Errorf("序列化合并后的认证配置失败: %v", err)
	}

	// 创建registry目录
	registryDir := filepath.Dir(mergedAuthPath)
	if err := os.MkdirAll(registryDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create registry directory: %v", err)
	}

	// 保存合并认证文件
	if err := os.WriteFile(mergedAuthPath, mergedAuthContent, 0600); err != nil {
		return "", fmt.Errorf("保存合并后的认证配置失败: %v", err)
	}

	w.log.Info("✅ 认证配置已保存到: %s", mergedAuthPath)

	// 尝试设置CA证书信任（非阻塞）
	caCertPath := filepath.Join(clusterDir, "registry", "*.pem")
	if matches, err := filepath.Glob(caCertPath); err == nil && len(matches) > 0 {
		w.log.Info("ℹ️  检测到CA证书文件，建议手动配置证书信任")
		w.log.Info("   CA证书路径: %s", matches[0])
		// 这里可以添加自动配置CA证书的逻辑，但通常需要root权限
	}

	return mergedAuthPath, nil
}

// setupWorkspaceAndCache 设置工作空间和缓存目录，避免使用默认的 $HOME/.oc-mirror
func (w *MirrorWrapper) setupWorkspaceAndCache(cfg *config.ClusterConfig, clusterName string) (string, string, error) {
	// 获取当前工作目录，确保在当前项目目录内创建文件
	currentDir, err := os.Getwd()
	if err != nil {
		return "", "", fmt.Errorf("failed to get current working directory: %v", err)
	}

	// 确定集群目录，优先使用配置中的 ClusterID，否则使用传入的 clusterName
	clusterID := cfg.ClusterInfo.ClusterID
	if clusterID == "" {
		clusterID = clusterName
	}

	// 构建完整的集群目录路径
	clusterDir := filepath.Join(currentDir, clusterID)

	// 设置工作空间目录（在集群目录内）
	workspaceDir := filepath.Join(clusterDir, "images", "working-dir")
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create workspace directory: %v", err)
	}

	// 设置缓存目录（在集群目录内）
	cacheDir := filepath.Join(clusterDir, "images", "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return "", "", fmt.Errorf("failed to create cache directory: %v", err)
	}

	// 获取绝对路径，确保 oc-mirror 能正确识别
	workspaceAbsPath, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to get workspace absolute path: %v", err)
	}

	cacheAbsPath, err := filepath.Abs(cacheDir)
	if err != nil {
		return "", "", fmt.Errorf("failed to get cache directory absolute path: %v", err)
	}

	// 将路径转换为 file:// 格式（oc-mirror 工作空间要求）
	workspacePath := "file://" + workspaceAbsPath
	cachePath := cacheAbsPath // cache-dir 不需要 file:// 前缀

	w.log.Info("📁 Workspace directory: %s", workspaceAbsPath)
	w.log.Info("💾 Cache directory: %s", cacheAbsPath)
	w.log.Debug("oc-mirror workspace parameter: %s", workspacePath)
	w.log.Debug("oc-mirror cache-dir parameter: %s", cachePath)

	return workspacePath, cachePath, nil
}

// CleanCache 清理指定集群的缓存目录
func (w *MirrorWrapper) CleanCache(cfg *config.ClusterConfig, clusterName string) error {
	// 获取当前工作目录
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current working directory: %v", err)
	}

	// 确定集群目录
	clusterID := cfg.ClusterInfo.ClusterID
	if clusterID == "" {
		clusterID = clusterName
	}

	clusterDir := filepath.Join(currentDir, clusterID)
	cacheDir := filepath.Join(clusterDir, "images", "cache")

	// 检查缓存目录是否存在
	if _, err := os.Stat(cacheDir); os.IsNotExist(err) {
		w.log.Info("ℹ️  Cache directory does not exist: %s", cacheDir)
		return nil
	}

	// 计算缓存大小
	size, err := w.calculateDirectorySize(cacheDir)
	if err != nil {
		w.log.Warn("⚠️  Failed to calculate cache size: %v", err)
	} else {
		w.log.Info("📊 Current cache size: %s", w.formatBytes(size))
	}

	w.log.Info("🧹 Cleaning cache directory: %s", cacheDir)

	// 删除缓存目录内容
	err = os.RemoveAll(cacheDir)
	if err != nil {
		return fmt.Errorf("failed to clean cache directory: %v", err)
	}

	w.log.Info("✅ Cache cleaned successfully")
	return nil
}

// GetCacheInfo 获取缓存信息，包括大小和位置
func (w *MirrorWrapper) GetCacheInfo(cfg *config.ClusterConfig, clusterName string) (map[string]interface{}, error) {
	// 获取当前工作目录
	currentDir, err := os.Getwd()
	if err != nil {
		return nil, fmt.Errorf("failed to get current working directory: %v", err)
	}

	// 确定集群目录
	clusterID := cfg.ClusterInfo.ClusterID
	if clusterID == "" {
		clusterID = clusterName
	}

	clusterDir := filepath.Join(currentDir, clusterID)
	cacheDir := filepath.Join(clusterDir, "images", "cache")
	workspaceDir := filepath.Join(clusterDir, "images", "working-dir")

	info := map[string]interface{}{
		"cluster_id":    clusterID,
		"cache_dir":     cacheDir,
		"workspace_dir": workspaceDir,
	}

	// 检查缓存目录
	if stat, err := os.Stat(cacheDir); err == nil {
		size, err := w.calculateDirectorySize(cacheDir)
		if err == nil {
			info["cache_size_bytes"] = size
			info["cache_size_human"] = w.formatBytes(size)
		}
		info["cache_exists"] = true
		info["cache_modified"] = stat.ModTime()
	} else {
		info["cache_exists"] = false
		info["cache_size_bytes"] = 0
		info["cache_size_human"] = "0 B"
	}

	// 检查工作空间目录
	if stat, err := os.Stat(workspaceDir); err == nil {
		size, err := w.calculateDirectorySize(workspaceDir)
		if err == nil {
			info["workspace_size_bytes"] = size
			info["workspace_size_human"] = w.formatBytes(size)
		}
		info["workspace_exists"] = true
		info["workspace_modified"] = stat.ModTime()
	} else {
		info["workspace_exists"] = false
		info["workspace_size_bytes"] = 0
		info["workspace_size_human"] = "0 B"
	}

	return info, nil
}

// calculateDirectorySize 计算目录大小
func (w *MirrorWrapper) calculateDirectorySize(dirPath string) (int64, error) {
	var size int64
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			size += info.Size()
		}
		return nil
	})
	return size, err
}

// formatBytes 格式化字节数为人类可读格式
func (w *MirrorWrapper) formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
