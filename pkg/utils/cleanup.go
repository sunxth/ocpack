package utils

import (
	"fmt"
	"os"
	"path/filepath"
)

// CleanupIntermediateFiles 清理不必要的中间文件
func CleanupIntermediateFiles(clusterDir string) error {
	// 定义可以删除的中间文件
	filesToDelete := []string{
		// 格式化后的pull-secret副本（调试用）
		filepath.Join(clusterDir, "pull-secret-formatted.json"),
		// registry目录下的pull-secret副本（功能重复）
		filepath.Join(clusterDir, "registry", "pull-secret.json"),
		// 临时的ICSP配置文件
		filepath.Join(clusterDir, ".icsp.yaml"),
	}

	deletedCount := 0
	for _, filePath := range filesToDelete {
		if _, err := os.Stat(filePath); err == nil {
			if err := os.Remove(filePath); err != nil {
				// 静默处理删除失败，不输出错误
				continue
			} else {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		fmt.Printf("🧹 已清理 %d 个中间文件\n", deletedCount)
	}

	return nil
}

// CleanupOldExtractedBinaries 清理旧的提取的二进制文件
func CleanupOldExtractedBinaries(clusterDir string) error {
	// 查找所有以 openshift-install- 开头的文件
	entries, err := os.ReadDir(clusterDir)
	if err != nil {
		return fmt.Errorf("读取集群目录失败: %v", err)
	}

	deletedCount := 0
	var deletedSize int64
	for _, entry := range entries {
		if !entry.IsDir() &&
			(entry.Name() == "openshift-install" ||
				(len(entry.Name()) > 18 && entry.Name()[:18] == "openshift-install-")) {
			filePath := filepath.Join(clusterDir, entry.Name())

			// 获取文件大小
			if fileInfo, err := os.Stat(filePath); err == nil {
				deletedSize += fileInfo.Size()
			}

			if err := os.Remove(filePath); err == nil {
				deletedCount++
			}
		}
	}

	if deletedCount > 0 {
		fmt.Printf("🗑️  已清理 %d 个旧二进制文件 (释放 %s)\n", deletedCount, formatSize(deletedSize))
	}

	return nil
}

// ShowDiskUsage 显示各目录的磁盘使用情况
func ShowDiskUsage(clusterDir string) error {
	fmt.Println("📊 磁盘使用情况:")

	directories := []string{
		"downloads",
		"images",
		"oc-mirror-workspace",
		"registry",
		"installation",
	}

	totalSize := int64(0)
	for _, dir := range directories {
		dirPath := filepath.Join(clusterDir, dir)
		if _, err := os.Stat(dirPath); err == nil {
			size, err := calculateDirSize(dirPath)
			if err != nil {
				fmt.Printf("  %s: 计算失败\n", dir)
			} else {
				fmt.Printf("  %s: %s\n", dir, formatSize(size))
				totalSize += size
			}
		}
	}

	if totalSize > 0 {
		fmt.Printf("  总计: %s\n", formatSize(totalSize))
	}

	return nil
}

// calculateDirSize 计算目录大小
func calculateDirSize(dirPath string) (int64, error) {
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

// formatSize 格式化文件大小
func formatSize(size int64) string {
	const unit = 1024
	if size < unit {
		return fmt.Sprintf("%d B", size)
	}
	div, exp := int64(unit), 0
	for n := size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(size)/float64(div), "KMGTPE"[exp])
}
