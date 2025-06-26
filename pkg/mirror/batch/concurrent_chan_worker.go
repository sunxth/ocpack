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
			loop:
				for {
					select {
					case <-cancelCtx.Done():
						spinner.Abort(false)
						break loop
					default:
						if !triggered {
							triggered = true
							timeoutCtx, _ := opts.Global.CommandTimeoutContext()

							options := opts
							if img.Type.IsOperatorCatalog() && img.RebuiltTag != "" {
								options.RemoveSignatures = true
							}

							err = o.Mirror.Run(timeoutCtx, img.Source, img.Destination, mirror.Mode(opts.Function), &options) //nolint:contextcheck

							switch {
							case err == nil:
								spinner.Increment()
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
							break loop
						}

					}
				}
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
			m.Unlock()

			logImageError(o.Log, &res.img, &opts)
			// 改进：不再因为单个 release 镜像失败就终止整个流程
			// 让所有镜像都有机会下载，最后再统一处理错误
		}

		completed++
		progressCh <- 1
	}
	close(progressCh)

	p.Wait()

	logResults(o.Log, opts.Function, &copiedImages, &collectorSchema)

	if len(errArray) > 0 {
		// 计算成功率
		totalImages := len(collectorSchema.AllImages)
		successImages := len(copiedImages.AllImages)
		successRate := float64(successImages) / float64(totalImages) * 100

		batchErr := &BatchError{
			releaseCountDiff:       collectorSchema.TotalReleaseImages - copiedImages.TotalReleaseImages,
			operatorCountDiff:      collectorSchema.TotalOperatorImages - copiedImages.TotalOperatorImages,
			additionalImgCountDiff: collectorSchema.TotalAdditionalImages - copiedImages.TotalAdditionalImages,
			helmCountDiff:          collectorSchema.TotalHelmImages - copiedImages.TotalHelmImages,
		}

		filename, err := saveErrors(o.Log, o.LogsDir, errArray)
		if err != nil {
			batchErr.source = fmt.Errorf(errMsgHeader+" - unable to log these errors in %s/%s: %w", workerPrefix, o.LogsDir, filename, err)
		} else {
			batchErr.source = fmt.Errorf(errMsg, workerPrefix, o.LogsDir, filename)
		}

		// 改进：如果成功率足够高(比如 >= 80%)，则将错误视为警告而不是致命错误
		if successRate >= 80.0 {
			o.Log.Warn("⚠️  镜像同步部分失败，但成功率达到 %.1f%% (%d/%d)，继续执行", successRate, successImages, totalImages)
			o.Log.Warn("   失败的镜像列表请查看: %s/%s", o.LogsDir, filename)
			return copiedImages, nil // 返回 nil 而不是错误
		}

		// 如果成功率太低，才返回错误
		o.Log.Error("❌ 镜像同步成功率过低: %.1f%% (%d/%d)，建议检查网络或重试", successRate, successImages, totalImages)
		return copiedImages, batchErr
	}
	o.Log.Debug("concurrent channel worker time     : %v", time.Since(startTime))
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
	// 极简显示：只显示镜像名称和目标
	imageName := path.Base(img.Origin)
	var destination string
	if strings.Contains(img.Destination, localStorage) {
		destination = "cache"
	} else {
		destination = hostNamespace(img.Destination)
		// 简化目标显示，只保留关键部分
		if len(destination) > 30 {
			destination = destination[:27] + "..."
		}
	}

	imageText := imageName + " → " + destination

	return spinners.AddMinimalSpinner(p, imageText)
}

func newOverallProgress(p *mpb.Progress, total int) *mpb.Bar {
	return spinners.AddMinimalOverallProgress(p, total)
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
