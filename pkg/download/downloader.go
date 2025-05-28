package download

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"ocpack/pkg/config"
)

// Downloader 负责下载所需文件
type Downloader struct {
	config     *config.ClusterConfig
	downloadDir string
}

// NewDownloader 创建一个新的下载器
func NewDownloader(cfg *config.ClusterConfig, downloadDir string) *Downloader {
	return &Downloader{
		config:     cfg,
		downloadDir: downloadDir,
	}
}

// DownloadAll 下载所有需要的文件
func (d *Downloader) DownloadAll() error {
	// 确保下载目录存在
	if err := os.MkdirAll(d.downloadDir, 0755); err != nil {
		return fmt.Errorf("创建下载目录失败: %w", err)
	}

	// 获取OpenShift版本
	version := d.config.ClusterInfo.OpenShiftVersion

	// 下载 OpenShift 客户端工具
	if err := d.downloadOpenShiftClient(version); err != nil {
		return fmt.Errorf("下载 OpenShift 客户端失败: %w", err)
	}

	// 下载 OpenShift 安装程序
	if err := d.downloadOpenShiftInstaller(version); err != nil {
		return fmt.Errorf("下载 OpenShift 安装程序失败: %w", err)
	}

	// 下载其他所需文件...
	// 下载oc-mirror (仅支持 4.14.0 及以上版本)
	if d.supportsOcMirror(version) {
		if err := d.downloadOcMirror(version); err != nil {
			return fmt.Errorf("下载 oc-mirror 失败: %w", err)
		}
	} else {
		fmt.Printf("注意: OpenShift 版本 %s 不支持 oc-mirror 工具 (需要 4.14.0 及以上版本)\n", version)
	}

	// 下载 butane 工具
	if err := d.downloadButane(); err != nil {
		return fmt.Errorf("下载 butane 工具失败: %w", err)
	}

	// 下载 Quay 镜像仓库安装包
	if err := d.downloadMirrorRegistry(); err != nil {
		return fmt.Errorf("下载 Quay 镜像仓库安装包失败: %w", err)
	}

	// 提取工具到 bin 目录
	if err := d.extractTools(version); err != nil {
		return fmt.Errorf("提取工具失败: %w", err)
	}
	
	return nil
}

// downloadOpenShiftClient 下载 OpenShift 客户端
func (d *Downloader) downloadOpenShiftClient(version string) error {
	clientURL := fmt.Sprintf("https://mirror.openshift.com/pub/openshift-v4/clients/ocp/%s/openshift-client-linux-%s.tar.gz", version, version)
	clientPath := filepath.Join(d.downloadDir, fmt.Sprintf("openshift-client-linux-%s.tar.gz", version))
	
	return d.downloadFile(clientURL, clientPath)
}

// downloadOpenShiftInstaller 下载 OpenShift 安装程序
func (d *Downloader) downloadOpenShiftInstaller(version string) error {
	installerURL := fmt.Sprintf("https://mirror.openshift.com/pub/openshift-v4/clients/ocp/%s/openshift-install-linux-%s.tar.gz", version, version)
	installerPath := filepath.Join(d.downloadDir, fmt.Sprintf("openshift-install-linux-%s.tar.gz", version))
	
	return d.downloadFile(installerURL, installerPath)
}

// downloadOcMirror 下载 oc-mirror 工具
func (d *Downloader) downloadOcMirror(version string) error {
	// 获取系统架构
	arch := d.getSystemArch()
	
	// 构造下载URL和文件名
	fileName := fmt.Sprintf("oc-mirror-%s.tar.gz", version)
	ocMirrorURL := fmt.Sprintf("https://mirror.openshift.com/pub/openshift-v4/%s/clients/ocp/%s/oc-mirror.tar.gz", arch, version)

	ocMirrorPath := filepath.Join(d.downloadDir, fileName)

	return d.downloadFile(ocMirrorURL, ocMirrorPath)
}

// downloadButane 下载 butane 工具
func (d *Downloader) downloadButane() error {
	// 获取系统架构
	arch := d.getSystemArch()
	
	// 构造下载URL和文件名
	fileName := fmt.Sprintf("butane-%s", arch)
	butaneURL := fmt.Sprintf("https://mirror.openshift.com/pub/openshift-v4/clients/butane/latest/butane-%s", arch)
	
	butanePath := filepath.Join(d.downloadDir, fileName)
	
	return d.downloadFile(butaneURL, butanePath)
}

// downloadMirrorRegistry 下载 Quay 镜像仓库安装包
func (d *Downloader) downloadMirrorRegistry() error {
	// 构造下载URL和文件名
	fileName := "mirror-registry-amd64.tar.gz"
	mirrorRegistryURL := "https://mirror.openshift.com/pub/cgw/mirror-registry/latest/mirror-registry-amd64.tar.gz"
	
	mirrorRegistryPath := filepath.Join(d.downloadDir, fileName)
	
	return d.downloadFile(mirrorRegistryURL, mirrorRegistryPath)
}

// getSystemArch 获取系统架构
func (d *Downloader) getSystemArch() string {
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		// 默认返回 x86_64
		return "x86_64"
	}
}

// extractMajorVersion 提取主版本号
func (d *Downloader) extractMajorVersion(version string) string {
	// 从版本号中提取主版本（如 4.18.1 -> 4.18）
	parts := strings.Split(version, ".")
	if len(parts) >= 2 {
		return parts[0] + "." + parts[1]
	}
	// 如果版本号格式不正确，返回默认版本
	return "4.18"
}

// supportsOcMirror 检查版本是否支持 oc-mirror 工具
func (d *Downloader) supportsOcMirror(version string) bool {
	return d.compareVersion(version, "4.14.0") >= 0
}

// compareVersion 比较两个版本号
// 返回值: -1 表示 v1 < v2, 0 表示 v1 == v2, 1 表示 v1 > v2
func (d *Downloader) compareVersion(v1, v2 string) int {
	parts1 := d.parseVersion(v1)
	parts2 := d.parseVersion(v2)
	
	// 比较每个部分
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}
	
	for i := 0; i < maxLen; i++ {
		var p1, p2 int
		if i < len(parts1) {
			p1 = parts1[i]
		}
		if i < len(parts2) {
			p2 = parts2[i]
		}
		
		if p1 < p2 {
			return -1
		} else if p1 > p2 {
			return 1
		}
	}
	
	return 0
}

// parseVersion 解析版本号为整数数组
func (d *Downloader) parseVersion(version string) []int {
	parts := strings.Split(version, ".")
	result := make([]int, 0, len(parts))
	
	for _, part := range parts {
		// 简单的字符串转整数，忽略错误
		num := 0
		for _, char := range part {
			if char >= '0' && char <= '9' {
				num = num*10 + int(char-'0')
			} else {
				break // 遇到非数字字符就停止
			}
		}
		result = append(result, num)
	}
	
	return result
}

// downloadFile 下载文件到指定路径
func (d *Downloader) downloadFile(url string, destPath string) error {
	// 检查文件是否已存在
	if _, err := os.Stat(destPath); err == nil {
		fmt.Printf("文件已存在，跳过下载: %s\n", destPath)
		return nil
	}

	// 创建临时文件
	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}
	defer out.Close()

	// 发送HTTP请求
	fmt.Printf("正在下载: %s\n", url)
	resp, err := http.Get(url)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("下载请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 检查响应状态
	if resp.StatusCode != http.StatusOK {
		os.Remove(tmpPath)
		return fmt.Errorf("下载失败，HTTP状态码: %d", resp.StatusCode)
	}

	// 复制数据
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("保存文件失败: %w", err)
	}

	// 关闭文件以确保写入完成
	out.Close()

	// 重命名临时文件为目标文件
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("重命名文件失败: %w", err)
	}

	fmt.Printf("下载完成: %s\n", filepath.Base(destPath))
	return nil
}

// GetDownloadedFiles 获取已下载的文件列表
func (d *Downloader) GetDownloadedFiles() ([]string, error) {
	var files []string
	
	err := filepath.Walk(d.downloadDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(d.downloadDir, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("获取已下载文件列表失败: %w", err)
	}
	
	return files, nil
}

// extractTools 提取工具到 bin 目录
func (d *Downloader) extractTools(version string) error {
	// 创建 bin 目录
	binDir := filepath.Join(d.downloadDir, "bin")
	if err := os.MkdirAll(binDir, 0755); err != nil {
		return fmt.Errorf("创建 bin 目录失败: %w", err)
	}

	fmt.Println("正在提取工具到 bin 目录...")

	// 提取 OpenShift 客户端工具
	clientTarPath := filepath.Join(d.downloadDir, fmt.Sprintf("openshift-client-linux-%s.tar.gz", version))
	if err := d.extractTarGz(clientTarPath, binDir, []string{"oc", "kubectl"}); err != nil {
		return fmt.Errorf("提取 OpenShift 客户端工具失败: %w", err)
	}

	// 提取 OpenShift 安装程序
	installerTarPath := filepath.Join(d.downloadDir, fmt.Sprintf("openshift-install-linux-%s.tar.gz", version))
	if err := d.extractTarGz(installerTarPath, binDir, []string{"openshift-install"}); err != nil {
		return fmt.Errorf("提取 OpenShift 安装程序失败: %w", err)
	}

	// 提取 oc-mirror 工具 (如果支持)
	if d.supportsOcMirror(version) {
		ocMirrorTarPath := filepath.Join(d.downloadDir, fmt.Sprintf("oc-mirror-%s.tar.gz", version))
		if err := d.extractTarGz(ocMirrorTarPath, binDir, []string{"oc-mirror"}); err != nil {
			return fmt.Errorf("提取 oc-mirror 工具失败: %w", err)
		}
	}

	// 复制 butane 工具
	arch := d.getSystemArch()
	butaneSrcPath := filepath.Join(d.downloadDir, fmt.Sprintf("butane-%s", arch))
	butaneDstPath := filepath.Join(binDir, "butane")
	if err := d.copyFile(butaneSrcPath, butaneDstPath); err != nil {
		return fmt.Errorf("复制 butane 工具失败: %w", err)
	}

	// 设置可执行权限
	if err := d.makeExecutable(binDir); err != nil {
		return fmt.Errorf("设置可执行权限失败: %w", err)
	}

	fmt.Println("工具提取完成！")
	return nil
}

// extractTarGz 从 tar.gz 文件中提取指定的文件
func (d *Downloader) extractTarGz(tarPath, destDir string, targetFiles []string) error {
	// 检查文件是否存在
	if _, err := os.Stat(tarPath); os.IsNotExist(err) {
		fmt.Printf("跳过不存在的文件: %s\n", filepath.Base(tarPath))
		return nil
	}

	file, err := os.Open(tarPath)
	if err != nil {
		return fmt.Errorf("打开文件失败: %w", err)
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return fmt.Errorf("创建 gzip 读取器失败: %w", err)
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)

	// 用于跟踪硬链接
	hardLinks := make(map[string]string)

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("读取 tar 文件失败: %w", err)
		}

		// 检查是否是我们需要的文件
		fileName := filepath.Base(header.Name)
		if !d.isTargetFile(fileName, targetFiles) {
			continue
		}

		destPath := filepath.Join(destDir, fileName)

		// 处理硬链接
		if header.Typeflag == tar.TypeLink {
			// 这是一个硬链接
			linkTarget := filepath.Base(header.Linkname)
			if targetPath, exists := hardLinks[linkTarget]; exists {
				// 创建硬链接
				if err := os.Link(targetPath, destPath); err != nil {
					return fmt.Errorf("创建硬链接失败: %w", err)
				}
				fmt.Printf("提取: %s (硬链接到 %s)\n", fileName, linkTarget)
			} else {
				return fmt.Errorf("硬链接目标 %s 不存在", linkTarget)
			}
		} else {
			// 普通文件
			destFile, err := os.Create(destPath)
			if err != nil {
				return fmt.Errorf("创建目标文件失败: %w", err)
			}

			// 复制文件内容
			_, err = io.Copy(destFile, tr)
			destFile.Close()
			if err != nil {
				return fmt.Errorf("复制文件内容失败: %w", err)
			}

			// 记录文件路径，用于后续的硬链接
			hardLinks[fileName] = destPath
			fmt.Printf("提取: %s\n", fileName)
		}
	}

	return nil
}

// isTargetFile 检查文件名是否在目标文件列表中
func (d *Downloader) isTargetFile(fileName string, targetFiles []string) bool {
	for _, target := range targetFiles {
		if fileName == target {
			return true
		}
	}
	return false
}

// copyFile 复制文件
func (d *Downloader) copyFile(src, dst string) error {
	// 检查源文件是否存在
	if _, err := os.Stat(src); os.IsNotExist(err) {
		fmt.Printf("跳过不存在的文件: %s\n", filepath.Base(src))
		return nil
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("打开源文件失败: %w", err)
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("创建目标文件失败: %w", err)
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	if err != nil {
		return fmt.Errorf("复制文件失败: %w", err)
	}

	fmt.Printf("复制: %s\n", filepath.Base(dst))
	return nil
}

// makeExecutable 为目录中的所有文件设置可执行权限
func (d *Downloader) makeExecutable(dir string) error {
	return filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			if err := os.Chmod(path, 0755); err != nil {
				return fmt.Errorf("设置文件权限失败 %s: %w", path, err)
			}
		}
		return nil
	})
} 