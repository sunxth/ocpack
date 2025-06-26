package main

import (
	"fmt"
	"io"
	"time"

	"github.com/vbauerster/mpb/v8"

	"ocpack/pkg/mirror/spinners"
)

func main() {
	fmt.Println("=== UI 优化演示 - 方案三：极简专注式 ===\n")

	// 模拟镜像同步过程
	images := []string{
		"openshift-release:4.14.0 → cache",
		"ubi8-minimal:latest → registry.redhat.io/ubi8...",
		"operator-bundle:v1.2.3 → quay.io/operators...",
		"helm-chart:stable → cache",
	}

	fmt.Println("🚀 copying 4 images...")
	fmt.Println("📋 Loading config...")
	fmt.Println("💾 Cache: /path/to/cluster/images/cache\n")

	// 创建进度条容器
	p := mpb.New(mpb.WithOutput(io.Discard))

	// 创建整体进度条（极简版本）
	overallProgress := spinners.AddMinimalOverallProgress(p, len(images))

	// 创建每个镜像的spinner（极简版本）
	var bars []*mpb.Bar
	for _, img := range images {
		bar := spinners.AddMinimalSpinner(p, img)
		bars = append(bars, bar)
	}

	// 模拟同步过程
	for i, bar := range bars {
		// 模拟处理时间
		time.Sleep(500 * time.Millisecond)

		// 完成当前镜像
		bar.Increment()
		overallProgress.SetCurrent(int64(i + 1))

		time.Sleep(200 * time.Millisecond)
	}

	p.Wait()

	// 显示结果
	fmt.Println("\n✅ mirrored 4/4 images successfully")
	fmt.Println("\n镜像同步完成！")

	fmt.Println("\n=== 优化效果总结 ===")
	fmt.Println("1. ✓ 简化了 spinner 显示，移除了时间戳")
	fmt.Println("2. ✓ 优化了整体进度条，使用紧凑格式")
	fmt.Println("3. ✓ 简化了日志消息，减少视觉噪音")
	fmt.Println("4. ✓ 智能的结果汇总，突出关键信息")
}
