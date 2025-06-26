package batch

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/vbauerster/mpb/v8"

	"ocpack/pkg/mirror/api/v2alpha1"
	"ocpack/pkg/mirror/emoji"
	clog "ocpack/pkg/mirror/log"
	"ocpack/pkg/mirror/mirror"
	"ocpack/pkg/mirror/spinners"
)

const (
	skippingMsg = "skipping operator bundle %s because one of its related images failed to mirror"

	errMsgHeader = "%ssome errors occurred during the mirroring"
	errMsg       = errMsgHeader + ".\n" +
		"\t Please review %s/%s for a list of mirroring errors.\n" +
		"\t You may consider:\n" +
		"\t * removing images or operators that cause the error from the image set config, and retrying\n" +
		"\t * keeping the image set config (images are mandatory for you), and retrying\n" +
		"\t * mirroring the failing images manually, if retries also fail."
)

type ChannelConcurrentBatch struct {
	Log           clog.PluggableLoggerInterface
	LogsDir       string
	Mirror        mirror.MirrorInterface
	MaxGoroutines uint
}

type GoroutineResult struct {
	err     *mirrorErrorSchema
	imgType v2alpha1.ImageType
	img     v2alpha1.CopyImageSchema
}

// Worker - the main batch processor
func (o *ChannelConcurrentBatch) Worker(ctx context.Context, collectorSchema v2alpha1.CollectorSchema, opts mirror.CopyOptions) (v2alpha1.CollectorSchema, error) {
	startTime := time.Now()

	copiedImages := v2alpha1.CollectorSchema{
		AllImages: []v2alpha1.CopyImageSchema{},
	}

	var errArray []mirrorErrorSchema

	var m sync.RWMutex
	var wg sync.WaitGroup

	var mirrorMsg string
	switch {
	case opts.IsCopy():
		mirrorMsg = "copying"
	case opts.IsDelete():
		mirrorMsg = "deleting"
	}

	opts.PreserveDigests = true

	total := len(collectorSchema.AllImages)

	o.Log.Info("🚀 "+mirrorMsg+" %d images...", total)

	p := mpb.New(mpb.PopCompletedMode(), mpb.ContainerOptional(mpb.WithOutput(io.Discard), !opts.Global.IsTerminal))
	results := make(chan GoroutineResult, total)
	progressCh := make(chan int, total)
	semaphore := make(chan struct{}, o.MaxGoroutines)

	cancelCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	// 错误统计
	var criticalErrors int
	var skipCount int
	const maxCriticalErrors = 10 // 最大允许的关键错误数量

	go func() {
		defer close(results)
		defer close(semaphore)

		for _, img := range collectorSchema.AllImages {

			select {
			case <-cancelCtx.Done():
				wg.Wait()
				return
			default:
			}

			semaphore <- struct{}{}

			sp := newSpinner(img, opts.LocalStorageFQDN, p)

			wg.Add(1)
			go func(cancelCtx context.Context, semaphore chan struct{}, results chan<- GoroutineResult, spinner *mpb.Bar) {
				defer wg.Done()
				defer func() { <-semaphore }()
				result := GoroutineResult{imgType: img.Type, img: img}

				m.Lock()
				skip, reason := shouldSkipImage(img, opts, errArray)
				m.Unlock()
				if skip {
					if reason != nil {
						result.err = &mirrorErrorSchema{image: img, err: reason}
					}

					switch img.Type {
					case v2alpha1.TypeOperatorBundle:
						spinner.Abort(false)
					case v2alpha1.TypeCincinnatiGraph:
						spinner.Increment()
					}

					results <- result
					return
				}

				var err error
				var triggered bool

				// 添加重试机制
				maxRetries := 3
				for attempt := 0; attempt < maxRetries; attempt++ {
					select {
					case <-cancelCtx.Done():
						spinner.Abort(false)
						return
					default:
						if !triggered || attempt > 0 {
							triggered = true
							timeoutCtx, _ := opts.Global.CommandTimeoutContext()

							options := opts
							if img.Type.IsOperatorCatalog() && img.RebuiltTag != "" {
								options.RemoveSignatures = true
							}

							err = o.Mirror.Run(timeoutCtx, img.Source, img.Destination, mirror.Mode(opts.Function), &options) //nolint:contextcheck

							if err == nil {
								spinner.Increment()
								results <- result
								return
							}

							// 如果是最后一次尝试或者是不可重试的错误
							if attempt == maxRetries-1 || isNonRetryableError(err) {
								break
							}

							// 短暂延迟后重试
							time.Sleep(time.Duration(attempt+1) * time.Second)
						}
					}
				}

				// 处理最终错误
				switch {
				case img.Type.IsOperator():
					operators := collectorSchema.CopyImageSchemaMap.OperatorsByImage[img.Origin]
					bundles := collectorSchema.CopyImageSchemaMap.BundlesByImage[img.Origin]
					result.err = &mirrorErrorSchema{image: img, err: err, operators: operators, bundles: bundles}
					spinner.Abort(false)
				case img.Type.IsRelease() || img.Type.IsAdditionalImage() || img.Type.IsHelmImage():
					result.err = &mirrorErrorSchema{image: img, err: err}
					spinner.Abort(false)
				}
				results <- result

			}(cancelCtx, semaphore, results, sp)
		}
		wg.Wait()
	}()

	overallProgress := newOverallProgress(p, total)

	go runOverallProgress(overallProgress, cancelCtx, progressCh)

	completed := 0
	for completed < len(collectorSchema.AllImages) {
		res := <-results
		err := res.err
		if err == nil {
			logImageSuccess(o.Log, &res.img, &opts)
			copiedImages.AllImages = append(copiedImages.AllImages, res.img)
			incrementTotals(res.imgType, &copiedImages)
		} else {
			m.Lock()
			errArray = append(errArray, *err)

			// 统计错误类型
			if isCriticalError(err.err) {
				criticalErrors++
			} else {
				skipCount++
			}
			m.Unlock()

			logImageError(o.Log, &res.img, &opts)

			// 智能错误处理：如果关键错误太多，考虑提前终止
			if criticalErrors > maxCriticalErrors {
				o.Log.Warn("⚠️  检测到过多关键错误 (%d)，建议检查网络连接和配置", criticalErrors)
				// 不直接取消，让用户决定是否继续
			}
		}

		completed++
		progressCh <- 1
	}
	close(progressCh)

	p.Wait()

	// 增强的结果统计
	duration := time.Since(startTime)
	successCount := len(copiedImages.AllImages)
	failureCount := len(errArray)

	logResults(o.Log, opts.Function, &copiedImages, &collectorSchema)

	// 显示详细的执行摘要
	o.Log.Info("📊 执行摘要: 总计 %d 个镜像, 成功 %d, 失败 %d, 跳过 %d, 用时 %v",
		total, successCount, failureCount, skipCount, duration.Round(time.Second))

	if successCount > 0 {
		avgTime := duration / time.Duration(successCount)
		o.Log.Info("⚡ 平均每个镜像用时: %v", avgTime.Round(time.Millisecond))
	}

	if len(errArray) > 0 {
		// 计算成功率
		successRate := float64(successCount) / float64(total) * 100

		// 根据成功率给出不同的提示
		if successRate >= 95.0 {
			o.Log.Info("✅ 成功率: %.1f%% - 表现优秀!", successRate)
		} else if successRate >= 85.0 {
			o.Log.Warn("⚠️  成功率: %.1f%% - 有少量错误，建议查看日志", successRate)
		} else if successRate >= 70.0 {
			o.Log.Warn("⚠️  成功率: %.1f%% - 存在较多错误，请检查配置", successRate)
		} else {
			o.Log.Error("❌ 成功率: %.1f%% - 存在严重问题，请检查网络和配置", successRate)
		}

		// 分类错误原因
		networkErrors := 0
		authErrors := 0
		otherErrors := 0

		for _, err := range errArray {
			if isNetworkError(err.err) {
				networkErrors++
			} else if isAuthError(err.err) {
				authErrors++
			} else {
				otherErrors++
			}
		}

		if networkErrors > 0 {
			o.Log.Error("🌐 网络相关错误: %d 个", networkErrors)
		}
		if authErrors > 0 {
			o.Log.Error("🔐 认证相关错误: %d 个", authErrors)
		}
		if otherErrors > 0 {
			o.Log.Error("❓ 其他错误: %d 个", otherErrors)
		}

		// 错误写入文件
		if errorsFilePath := o.writeErrorsToFile(errArray); errorsFilePath != "" {
			o.Log.Info(errMsgHeader+"，详细信息已保存到: %s", emoji.SpinnerCrossMark, errorsFilePath)
			o.Log.Info("💡 建议操作:")
			o.Log.Info("   • 检查网络连接和DNS配置")
			o.Log.Info("   • 验证镜像仓库的访问权限")
			o.Log.Info("   • 考虑重新运行以重试失败的镜像")
			o.Log.Info("   • 或从镜像集配置中移除有问题的镜像")
		} else {
			o.Log.Error(errMsg, emoji.SpinnerCrossMark, o.LogsDir, "ERRORS")
		}

		// 返回错误，但包含成功的镜像信息
		return copiedImages, fmt.Errorf("镜像复制过程中出现 %d 个错误，成功率: %.1f%%", len(errArray), successRate)
	}

	o.Log.Info("🎉 所有镜像复制完成！用时 %v", duration.Round(time.Second))
	return copiedImages, nil
}

func hostNamespace(input string) string {
	parsedURL, err := url.Parse(input)
	if err != nil {
		return ""
	}

	host := parsedURL.Host
	pathSegments := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")

	if len(pathSegments) > 1 {
		namespacePath := strings.Join(pathSegments[:len(pathSegments)-1], "/")
		hostAndNamespace := path.Join(host, namespacePath) + "/"
		return hostAndNamespace
	} else if len(pathSegments) == 1 {
		return path.Join(host, pathSegments[0]) + "/"
	} else {
		return host
	}
}

func logResults(log clog.PluggableLoggerInterface, copyMode string, copiedImages, collectorSchema *v2alpha1.CollectorSchema) {
	var copyModeMsg string
	if copyMode == string(mirror.CopyMode) {
		copyModeMsg = "mirrored"
	} else {
		copyModeMsg = "deleted"
	}

	total := copiedImages.TotalReleaseImages + copiedImages.TotalOperatorImages + copiedImages.TotalAdditionalImages + copiedImages.TotalHelmImages
	expected := collectorSchema.TotalReleaseImages + collectorSchema.TotalOperatorImages + collectorSchema.TotalAdditionalImages + collectorSchema.TotalHelmImages

	if total == expected {
		log.Info("✅ %s %d/%d images successfully", copyModeMsg, total, expected)
	} else {
		log.Info("⚠️  %s %d/%d images (some failed)", copyModeMsg, total, expected)
		// 只在有失败时显示详细分解
		if copiedImages.TotalReleaseImages != collectorSchema.TotalReleaseImages {
			logResult(log, copyModeMsg, "release", copiedImages.TotalReleaseImages, collectorSchema.TotalReleaseImages)
		}
		if copiedImages.TotalOperatorImages != collectorSchema.TotalOperatorImages {
			logResult(log, copyModeMsg, "operator", copiedImages.TotalOperatorImages, collectorSchema.TotalOperatorImages)
		}
		if copiedImages.TotalAdditionalImages != collectorSchema.TotalAdditionalImages {
			logResult(log, copyModeMsg, "additional", copiedImages.TotalAdditionalImages, collectorSchema.TotalAdditionalImages)
		}
		if copiedImages.TotalHelmImages != collectorSchema.TotalHelmImages {
			logResult(log, copyModeMsg, "helm", copiedImages.TotalHelmImages, collectorSchema.TotalHelmImages)
		}
	}
}

func logResult(log clog.PluggableLoggerInterface, copyMode, imageType string, copied, total int) {
	if total != 0 {
		if copied == total {
			log.Info(emoji.SpinnerCheckMark+" %d / %d %s images %s successfully", copied, total, imageType, copyMode)
		} else {
			log.Info(emoji.SpinnerCrossMark+" %d / %d %s images %s: Some %s images failed to be %s - please check the logs", copied, total, imageType, copyMode, imageType, copyMode)
		}
	}
}

func logImageSuccess(log clog.PluggableLoggerInterface, image *v2alpha1.CopyImageSchema, opts *mirror.CopyOptions) {
	if opts.Global.IsTerminal {
		// It'll be printed by the spinner
		return
	}

	var dest string
	if strings.Contains(image.Destination, opts.LocalStorageFQDN) {
		dest = "cache"
	} else {
		dest = hostNamespace(image.Destination)
	}

	action := "copying"
	if opts.IsDelete() {
		action = "deleting"
	}

	log.Info("Success %s %s %s %s", action, image.Origin, emoji.RightArrow, dest)
}

func logImageError(log clog.PluggableLoggerInterface, image *v2alpha1.CopyImageSchema, opts *mirror.CopyOptions) {
	if opts.Global.IsTerminal {
		// It'll be printed by the spinner
		return
	}

	action := "copy"
	if opts.IsDelete() {
		action = "delete"
	}

	log.Error("Failed to %s %s %s", action, image.Type, image.Origin)
}

func newSpinner(img v2alpha1.CopyImageSchema, localStorage string, p *mpb.Progress) *mpb.Bar {
	// 显示镜像名称和目标，使用彩色增强UI风格
	imageName := path.Base(img.Origin)
	var destination string
	if strings.Contains(img.Destination, localStorage) {
		destination = "cache"
	} else {
		destination = hostNamespace(img.Destination)
		// 简化目标显示，只保留关键部分
		if len(destination) > 25 {
			destination = destination[:22] + "..."
		}
	}

	imageText := imageName + " → " + destination

	// 根据镜像类型使用不同的彩色显示（移除emoji图标）
	switch img.Type {
	case v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent:
		// Release镜像使用蓝色显示
		return spinners.AddColorfulSpinner(p, spinners.ColorBlue+imageText+spinners.ColorReset)

	case v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorCatalog:
		// Operator镜像使用紫色显示
		return spinners.AddColorfulSpinner(p, spinners.ColorPurple+imageText+spinners.ColorReset)

	case v2alpha1.TypeGeneric:
		// 通用镜像使用绿色显示
		return spinners.AddColorfulSpinner(p, spinners.ColorGreen+imageText+spinners.ColorReset)

	default:
		// 其他镜像使用默认彩色风格
		return spinners.AddColorfulSpinner(p, imageText)
	}
}

// 彩色增强版spinner - 方案一（已优化）
func newColorfulSpinner(img v2alpha1.CopyImageSchema, localStorage string, p *mpb.Progress) *mpb.Bar {
	imageName := path.Base(img.Origin)
	var destination string
	if strings.Contains(img.Destination, localStorage) {
		destination = "cache"
	} else {
		destination = hostNamespace(img.Destination)
		if len(destination) > 25 {
			destination = destination[:22] + "..."
		}
	}

	imageText := imageName + " → " + destination

	return spinners.AddColorfulSpinner(p, imageText)
}

// 对齐美化版spinner - 方案二（已优化）
func newAlignedSpinner(img v2alpha1.CopyImageSchema, localStorage string, p *mpb.Progress, maxImageWidth, maxDestWidth int) *mpb.Bar {
	imageName := path.Base(img.Origin)
	var destination string
	if strings.Contains(img.Destination, localStorage) {
		destination = "cache"
	} else {
		destination = hostNamespace(img.Destination)
		if len(destination) > 25 {
			destination = destination[:22] + "..."
		}
	}

	// 计算对齐宽度时限制最大宽度
	if len(imageName) > maxImageWidth {
		maxImageWidth = len(imageName)
	}
	if len(destination) > maxDestWidth {
		maxDestWidth = len(destination)
	}

	return spinners.AddAlignedSpinner(p, imageName, destination, "", maxImageWidth, maxDestWidth)
}

func newOverallProgress(p *mpb.Progress, total int) *mpb.Bar {
	// 使用彩色增强的整体进度条
	return spinners.AddColorfulOverallProgress(p, total)
}

// 彩色增强版整体进度条 - 方案一（已优化）
func newColorfulOverallProgress(p *mpb.Progress, total int) *mpb.Bar {
	return spinners.AddColorfulOverallProgress(p, total)
}

// 对齐美化版整体进度条 - 方案二（保持兼容）
func newAlignedOverallProgress(p *mpb.Progress, total int) *mpb.Bar {
	return spinners.AddColorfulOverallProgress(p, total)
}

func runOverallProgress(overallProgress *mpb.Bar, cancelCtx context.Context, progressCh chan int) {
	var progress int

	for {
		select {
		case <-cancelCtx.Done():
			overallProgress.Abort(false)
			return
		case _, ok := <-progressCh:
			if !ok {
				// channel closed (end of progress)
				overallProgress.Abort(false)
				return
			}
			progress++
			overallProgress.SetCurrent(int64(progress))
		}
	}
}

func incrementTotals(imgType v2alpha1.ImageType, copiedImages *v2alpha1.CollectorSchema) {
	switch imgType {
	case v2alpha1.TypeCincinnatiGraph, v2alpha1.TypeOCPRelease, v2alpha1.TypeOCPReleaseContent:
		copiedImages.TotalReleaseImages++
	case v2alpha1.TypeGeneric:
		copiedImages.TotalAdditionalImages++
	case v2alpha1.TypeOperatorBundle, v2alpha1.TypeOperatorCatalog, v2alpha1.TypeOperatorRelatedImage:
		copiedImages.TotalOperatorImages++
	case v2alpha1.TypeHelmImage:
		copiedImages.TotalHelmImages++
	}
}

// shouldSkipImage helps determine whether the batch should perform the mirroring of the image
// or if the image should be skipped.
func shouldSkipImage(img v2alpha1.CopyImageSchema, opts mirror.CopyOptions, errArray []mirrorErrorSchema) (bool, error) {
	// In MirrorToMirror and MirrorToDisk, the release collector will generally build and push the graph image
	// to the destination registry (disconnected registry or cache resp.)
	// Therefore this image can be skipped.
	// OCPBUGS-38037: The only exception to this is in the enclave environment. Enclave environment is detected by the presence
	// of env var UPDATE_URL_OVERRIDE.
	// When in enclave environment, release collector cannot build nor push the graph image. Therefore graph image
	// should not be skipped.
	updateURLOverride := os.Getenv("UPDATE_URL_OVERRIDE")
	if img.Type == v2alpha1.TypeCincinnatiGraph && (opts.Mode == mirror.MirrorToDisk || opts.Mode == mirror.MirrorToMirror) && len(updateURLOverride) == 0 {
		return true, nil
	}

	if img.Type == v2alpha1.TypeOperatorBundle {
		for _, err := range errArray {
			bundleImage := img.Origin
			if strings.Contains(bundleImage, "://") {
				bundleImage = strings.Split(img.Origin, "://")[1]
			}

			if err.bundles != nil && err.bundles.Has(bundleImage) {
				return true, fmt.Errorf(skippingMsg, img.Origin)
			}
		}
	}

	return false, nil
}

// 错误处理辅助函数

// isNonRetryableError 判断是否为不可重试的错误
func isNonRetryableError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// 认证错误通常不可重试
	if strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "login") {
		return true
	}

	// 镜像不存在错误
	if strings.Contains(errStr, "not found") ||
		strings.Contains(errStr, "does not exist") {
		return true
	}

	// 配置错误
	if strings.Contains(errStr, "invalid") ||
		strings.Contains(errStr, "malformed") {
		return true
	}

	return false
}

// isCriticalError 判断是否为关键错误
func isCriticalError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	// 网络和连接问题通常是关键错误
	if strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "dns") {
		return true
	}

	// 存储空间问题
	if strings.Contains(errStr, "no space") ||
		strings.Contains(errStr, "disk full") {
		return true
	}

	return false
}

// isNetworkError 判断是否为网络相关错误
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	return strings.Contains(errStr, "connection") ||
		strings.Contains(errStr, "timeout") ||
		strings.Contains(errStr, "network") ||
		strings.Contains(errStr, "dns") ||
		strings.Contains(errStr, "dial") ||
		strings.Contains(errStr, "unreachable")
}

// isAuthError 判断是否为认证相关错误
func isAuthError(err error) bool {
	if err == nil {
		return false
	}

	errStr := strings.ToLower(err.Error())

	return strings.Contains(errStr, "unauthorized") ||
		strings.Contains(errStr, "forbidden") ||
		strings.Contains(errStr, "authentication") ||
		strings.Contains(errStr, "login") ||
		strings.Contains(errStr, "token") ||
		strings.Contains(errStr, "credential")
}

// writeErrorsToFile 将错误信息写入文件
func (o *ChannelConcurrentBatch) writeErrorsToFile(errArray []mirrorErrorSchema) string {
	if len(errArray) == 0 {
		return ""
	}

	// 使用现有的错误保存逻辑
	filename, err := saveErrors(o.Log, o.LogsDir, errArray)
	if err != nil {
		o.Log.Error("无法保存错误信息到文件: %v", err)
		return ""
	}

	return fmt.Sprintf("%s/%s", o.LogsDir, filename)
}
