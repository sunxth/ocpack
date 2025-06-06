package download

import (
	"archive/tar"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"ocpack/pkg/config"
	"ocpack/pkg/utils"
)

// Downloader 负责下载所需文件
type Downloader struct {
	config      *config.ClusterConfig
	downloadDir string
}

// ProgressReader 带进度显示的 Reader
type ProgressReader struct {
	io.Reader
	total      int64
	downloaded int64
	fileName   string
	startTime  time.Time
	lastUpdate time.Time
}

// Read 实现 io.Reader 接口，同时更新进度
func (pr *ProgressReader) Read(p []byte) (int, error) {
	n, err := pr.Reader.Read(p)
	pr.downloaded += int64(n)

	// 限制更新频率，避免输出过于频繁
	now := time.Now()
	if now.Sub(pr.lastUpdate) >= 100*time.Millisecond || err == io.EOF {
		pr.lastUpdate = now
		pr.printProgress()
	}

	return n, err
}

// printProgress 打印进度条
func (pr *ProgressReader) printProgress() {
	if pr.total <= 0 {
		// 如果不知道总大小，显示已下载大小
		fmt.Printf("\r⬇️  %s: %s downloaded",
			pr.fileName,
			pr.formatBytes(pr.downloaded))
		return
	}

	percent := float64(pr.downloaded) / float64(pr.total) * 100
	elapsed := time.Since(pr.startTime)

	// 计算下载速度
	speed := float64(pr.downloaded) / elapsed.Seconds()

	// 估算剩余时间
	var eta time.Duration
	if speed > 0 && pr.downloaded < pr.total {
		remaining := pr.total - pr.downloaded
		eta = time.Duration(float64(remaining)/speed) * time.Second
	}

	// 创建进度条
	barWidth := 30
	filled := int(percent * float64(barWidth) / 100)
	bar := strings.Repeat("█", filled) + strings.Repeat("░", barWidth-filled)

	// 格式化输出
	fmt.Printf("\r⬇️  %s: [%s] %.1f%% (%s/%s) %s/s",
		pr.fileName,
		bar,
		percent,
		pr.formatBytes(pr.downloaded),
		pr.formatBytes(pr.total),
		pr.formatBytes(int64(speed)))

	if eta > 0 {
		fmt.Printf(" ETA: %s", pr.formatDuration(eta))
	}
}

// formatBytes 格式化字节数为人类可读格式
func (pr *ProgressReader) formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}

	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}

	units := []string{"KB", "MB", "GB", "TB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// formatDuration 格式化时间为人类可读格式
func (pr *ProgressReader) formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm%ds", int(d.Minutes()), int(d.Seconds())%60)
	}
	return fmt.Sprintf("%dh%dm", int(d.Hours()), int(d.Minutes())%60)
}

type DownloadTask struct {
	Name       string
	URL        string
	FileName   string
	Required   bool
	VersionDep bool // 是否依赖版本
}

// NewDownloader 创建一个新的下载器
func NewDownloader(cfg *config.ClusterConfig, downloadDir string) *Downloader {
	return &Downloader{
		config:      cfg,
		downloadDir: downloadDir,
	}
}

// DownloadAll 下载所有需要的文件
func (d *Downloader) DownloadAll() error {
	// 确保下载目录存在
	if err := os.MkdirAll(d.downloadDir, 0755); err != nil {
		return fmt.Errorf("创建下载目录失败: %w", err)
	}

	version := d.config.ClusterInfo.OpenShiftVersion

	// 构建下载任务列表
	tasks := d.buildDownloadTasks(version)

	// 统计跳过的文件
	var skippedFiles []string
	var downloadedFiles []string

	// 执行下载任务
	for _, task := range tasks {
		if task.VersionDep && !d.supportsOcMirror(version) {
			fmt.Printf("跳过 %s: OpenShift 版本 %s 不支持 (需要 4.14.0 及以上版本)\n", task.Name, version)
			continue
		}

		// 检查文件是否已存在
		destPath := filepath.Join(d.downloadDir, task.FileName)
		if _, err := os.Stat(destPath); err == nil {
			skippedFiles = append(skippedFiles, task.FileName)
			continue
		}

		if err := d.downloadFile(task.URL, destPath); err != nil {
			if task.Required {
				return fmt.Errorf("下载 %s 失败: %w", task.Name, err)
			}
			fmt.Printf("警告: 下载 %s 失败: %v\n", task.Name, err)
		} else {
			downloadedFiles = append(downloadedFiles, task.FileName)
		}
	}

	// 显示跳过文件的汇总
	if len(skippedFiles) > 0 {
		fmt.Printf("✓ 跳过已存在文件 (%d个): %s\n", len(skippedFiles), strings.Join(skippedFiles, ", "))
	}

	// 提取工具到 bin 目录
	if err := d.extractTools(version); err != nil {
		return fmt.Errorf("提取工具失败: %w", err)
	}

	return nil
}

// buildDownloadTasks 构建下载任务列表
func (d *Downloader) buildDownloadTasks(version string) []DownloadTask {
	arch := d.getSystemArch()
	butaneArch := d.getSystemArchForButane()

	return []DownloadTask{
		{
			Name:     "OpenShift 客户端",
			URL:      fmt.Sprintf("https://mirror.openshift.com/pub/openshift-v4/clients/ocp/%s/openshift-client-linux-%s.tar.gz", version, version),
			FileName: fmt.Sprintf("openshift-client-linux-%s.tar.gz", version),
			Required: true,
		},
		{
			Name:     "OpenShift 安装程序",
			URL:      fmt.Sprintf("https://mirror.openshift.com/pub/openshift-v4/clients/ocp/%s/openshift-install-linux-%s.tar.gz", version, version),
			FileName: fmt.Sprintf("openshift-install-linux-%s.tar.gz", version),
			Required: true,
		},
		{
			Name:       "oc-mirror 工具",
			URL:        fmt.Sprintf("https://mirror.openshift.com/pub/openshift-v4/%s/clients/ocp/%s/oc-mirror.tar.gz", arch, version),
			FileName:   fmt.Sprintf("oc-mirror-%s.tar.gz", version),
			Required:   false,
			VersionDep: true,
		},
		{
			Name:     "butane 工具",
			URL:      fmt.Sprintf("https://mirror.openshift.com/pub/openshift-v4/clients/butane/latest/butane-%s", butaneArch),
			FileName: fmt.Sprintf("butane-%s", butaneArch),
			Required: true,
		},
		{
			Name:     "Quay 镜像仓库安装包",
			URL:      "https://mirror.openshift.com/pub/cgw/mirror-registry/latest/mirror-registry-amd64.tar.gz",
			FileName: "mirror-registry-amd64.tar.gz",
			Required: true,
		},
	}
}

// getSystemArch 获取系统架构
func (d *Downloader) getSystemArch() string {
	archMap := map[string]string{
		"amd64": "x86_64",
		"arm64": "aarch64",
	}

	if mapped, exists := archMap[runtime.GOARCH]; exists {
		return mapped
	}
	return "x86_64" // 默认值
}

// getSystemArchForButane 获取适用于 butane 的系统架构
func (d *Downloader) getSystemArchForButane() string {
	osArchMap := map[string]map[string]string{
		"linux": {
			"amd64":   "amd64",
			"arm64":   "aarch64",
			"ppc64le": "ppc64le",
			"s390x":   "s390x",
		},
		"darwin": {
			"amd64": "darwin-amd64",
			"arm64": "darwin-amd64", // 暂时使用 amd64 版本
		},
		"windows": {
			"amd64": "windows-amd64.exe",
		},
	}

	if osMap, exists := osArchMap[runtime.GOOS]; exists {
		if arch, exists := osMap[runtime.GOARCH]; exists {
			return arch
		}
	}
	return "amd64" // 默认值
}

// supportsOcMirror 检查版本是否支持 oc-mirror 工具
func (d *Downloader) supportsOcMirror(version string) bool {
	return utils.SupportsOcMirror(version)
}

// compareVersion 比较两个版本号 - 优化版本
func (d *Downloader) compareVersion(v1, v2 string) int {
	return utils.CompareVersion(v1, v2)
}

// parseVersion 解析版本号为整数数组 - 优化版本
func (d *Downloader) parseVersion(version string) []int {
	return utils.ParseVersion(version)
}

// downloadFile 下载文件到指定路径 - 带进度条版本
func (d *Downloader) downloadFile(url, destPath string) error {
	fileName := filepath.Base(destPath)
	fmt.Printf("🚀 开始下载: %s\n", fileName)

	// 创建临时文件
	tmpPath := destPath + ".tmp"

	// 清理函数
	cleanup := func() {
		if _, err := os.Stat(tmpPath); err == nil {
			os.Remove(tmpPath)
		}
	}
	defer cleanup()

	// 发送 HEAD 请求获取文件大小
	headResp, err := http.Head(url)
	var contentLength int64
	if err == nil {
		headResp.Body.Close()
		contentLength = headResp.ContentLength
	}

	// 下载文件
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("下载失败，HTTP状态码: %d", resp.StatusCode)
	}

	// 如果 HEAD 请求失败，从 GET 响应获取大小
	if contentLength <= 0 {
		contentLength = resp.ContentLength
	}

	// 创建目标文件
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer out.Close()

	// 创建进度读取器
	progressReader := &ProgressReader{
		Reader:    resp.Body,
		total:     contentLength,
		fileName:  fileName,
		startTime: time.Now(),
	}

	// 复制数据（带进度显示）
	_, err = io.Copy(out, progressReader)

	// 完成后换行，避免进度条被覆盖
	fmt.Println()

	if err != nil {
		return fmt.Errorf("保存文件失败: %w", err)
	}

	// 关闭文件
	out.Close()

	// 重命名临时文件为目标文件
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("重命名文件失败: %w", err)
	}

	// 计算总下载时间
	elapsed := time.Since(progressReader.startTime)
	avgSpeed := float64(progressReader.downloaded) / elapsed.Seconds()

	fmt.Printf("✅ 下载完成: %s (%s, 平均速度: %s/s, 用时: %s)\n",
		fileName,
		progressReader.formatBytes(progressReader.downloaded),
		progressReader.formatBytes(int64(avgSpeed)),
		progressReader.formatDuration(elapsed))

	return nil
}

// extractTools 提取工具到 bin 目录 - 优化版本
func (d *Downloader) extractTools(version string) error {
	binDir := filepath.Join(d.downloadDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("创建 bin 目录失败: %w", err)
	}

	fmt.Println("🔧 正在提取工具到 bin 目录...")

	// 清理已存在的二进制文件以避免冲突
	if err := d.cleanupBinDir(binDir); err != nil {
		fmt.Printf("⚠️  清理 bin 目录警告: %v\n", err)
	}

	// 定义提取任务
	extractTasks := []struct {
		name      string
		tarPath   string
		files     []string
		condition func() bool
	}{
		{
			name:      "OpenShift 客户端工具",
			tarPath:   fmt.Sprintf("openshift-client-linux-%s.tar.gz", version),
			files:     []string{"oc", "kubectl"},
			condition: func() bool { return true },
		},
		{
			name:      "OpenShift 安装程序",
			tarPath:   fmt.Sprintf("openshift-install-linux-%s.tar.gz", version),
			files:     []string{"openshift-install"},
			condition: func() bool { return true },
		},
		{
			name:      "oc-mirror 工具",
			tarPath:   fmt.Sprintf("oc-mirror-%s.tar.gz", version),
			files:     []string{"oc-mirror"},
			condition: func() bool { return d.supportsOcMirror(version) },
		},
	}

	// 执行提取任务
	for _, task := range extractTasks {
		if !task.condition() {
			continue
		}

		fullPath := filepath.Join(d.downloadDir, task.tarPath)
		if err := d.extractTarGz(fullPath, binDir, task.files); err != nil {
			return fmt.Errorf("提取 %s 失败: %w", task.name, err)
		}
	}

	// 复制 butane 工具
	if err := d.copyButaneTool(binDir); err != nil {
		return fmt.Errorf("复制 butane 工具失败: %w", err)
	}

	// 设置可执行权限
	if err := d.makeExecutable(binDir); err != nil {
		return fmt.Errorf("设置可执行权限失败: %w", err)
	}

	fmt.Println("✅ 工具提取完成！")
	return nil
}

// copyButaneTool 复制 butane 工具
func (d *Downloader) copyButaneTool(binDir string) error {
	arch := d.getSystemArchForButane()
	srcPath := filepath.Join(d.downloadDir, fmt.Sprintf("butane-%s", arch))
	dstPath := filepath.Join(binDir, "butane")

	return d.copyFile(srcPath, dstPath)
}

// cleanupBinDir 清理 bin 目录中可能冲突的文件
func (d *Downloader) cleanupBinDir(binDir string) error {
	// 定义需要清理的文件列表
	filesToClean := []string{"oc", "kubectl", "openshift-install", "oc-mirror", "butane"}

	for _, fileName := range filesToClean {
		filePath := filepath.Join(binDir, fileName)
		if _, err := os.Stat(filePath); err == nil {
			if err := os.Remove(filePath); err != nil {
				return fmt.Errorf("删除文件 %s 失败: %w", fileName, err)
			}
			fmt.Printf("🗑️  清理已存在的文件: %s\n", fileName)
		}
	}

	return nil
}

// extractTarGz 从 tar.gz 文件中提取指定的文件 - 优化版本
func (d *Downloader) extractTarGz(tarPath, destDir string, targetFiles []string) error {
	if _, err := os.Stat(tarPath); os.IsNotExist(err) {
		fmt.Printf("⚠️  跳过不存在的文件: %s\n", filepath.Base(tarPath))
		return nil
	}

	return utils.ExtractTarGz(tarPath, destDir, targetFiles)
}

// extractFile 提取单个文件 - 处理已存在的文件
func (d *Downloader) extractFile(tr *tar.Reader, destPath string) error {
	return utils.ExtractFile(tr, destPath)
}

// copyFile 复制文件 - 优化版本
func (d *Downloader) copyFile(src, dst string) error {
	if _, err := os.Stat(src); os.IsNotExist(err) {
		fmt.Printf("⚠️  跳过不存在的文件: %s\n", filepath.Base(src))
		return nil
	}

	if err := utils.CopyFile(src, dst); err != nil {
		return err
	}

	fmt.Printf("📋 复制: %s\n", filepath.Base(dst))
	return nil
}

// makeExecutable 为目录中的所有文件设置可执行权限
func (d *Downloader) makeExecutable(dir string) error {
	return utils.MakeExecutable(dir)
}

// GetDownloadedFiles 获取已下载的文件列表
func (d *Downloader) GetDownloadedFiles() ([]string, error) {
	var files []string

	err := filepath.Walk(d.downloadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if relPath, err := filepath.Rel(d.downloadDir, path); err == nil {
				files = append(files, relPath)
			}
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("获取已下载文件列表失败: %w", err)
	}

	return files, nil
}
