package spinners

import (
	"io"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"ocpack/pkg/mirror/emoji"
)

// MinimalSpinnerStyle - 极简spinner样式，只使用点和短横线
func MinimalSpinnerLeft(original mpb.BarFiller) mpb.BarFiller {
	return mpb.SpinnerStyle("⠙", "⠸", "⠼", "⠦", "⠇", "⠋", " ").PositionLeft().Build()
}

// 传统的spinner样式（保持向后兼容）
func PositionSpinnerLeft(original mpb.BarFiller) mpb.BarFiller {
	return mpb.SpinnerStyle("⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏", " ").PositionLeft().Build()
}

func EmptyDecorator() decor.Decorator {
	return decor.Any(func(s decor.Statistics) string {
		return ""
	})
}

func BarFillerClearOnAbort() mpb.BarOption {
	return mpb.BarFillerMiddleware(func(base mpb.BarFiller) mpb.BarFiller {
		return mpb.BarFillerFunc(func(w io.Writer, st decor.Statistics) error {
			if st.Aborted {
				_, err := io.WriteString(w, "")
				return err
			}
			return base.Fill(w, st)
		})
	})
}

// 极简风格的spinner - 只显示完成状态，没有进度信息
func AddMinimalSpinner(progressBar *mpb.Progress, message string) *mpb.Bar {
	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(MinimalSpinnerLeft),
		mpb.BarWidth(2), // 更窄的宽度
		mpb.PrependDecorators(
			decor.OnComplete(EmptyDecorator(), "✓"), // 简洁的完成标记
			decor.OnAbort(EmptyDecorator(), "✗"),    // 简洁的失败标记
		),
		mpb.AppendDecorators(
			decor.Name(" "+message), // 去掉括号和时间，只显示消息
		),
		mpb.BarFillerClearOnComplete(),
		BarFillerClearOnAbort(),
	)
}

// 增强版spinner - 显示下载进度和速度（改进版）
func AddProgressSpinner(progressBar *mpb.Progress, message string, totalSize int64) *mpb.Bar {
	if totalSize > 0 {
		// 如果知道总大小，显示进度条
		return progressBar.AddBar(totalSize,
			mpb.PrependDecorators(
				decor.OnComplete(EmptyDecorator(), "✓"),
				decor.OnAbort(EmptyDecorator(), "✗"),
			),
			mpb.AppendDecorators(
				decor.Name(" "+message+" "),
				decor.CountersKibiByte("% .1f/% .1f"),
				decor.Name(" "),
				decor.AverageSpeed(decor.SizeB1024(0), "% .1f"),
				decor.Name(" "),
				decor.AverageETA(decor.ET_STYLE_GO),
			),
		)
	} else {
		// 如果不知道总大小，显示增强的spinner
		return progressBar.AddSpinner(
			1, mpb.BarFillerMiddleware(MinimalSpinnerLeft),
			mpb.BarWidth(2),
			mpb.PrependDecorators(
				decor.OnComplete(EmptyDecorator(), "✓"),
				decor.OnAbort(EmptyDecorator(), "✗"),
			),
			mpb.AppendDecorators(
				decor.Name(" "+message+" "),
				decor.AverageSpeed(decor.SizeB1024(0), "% .1f"),
				decor.Name(" "),
				decor.Elapsed(decor.ET_STYLE_GO),
			),
			mpb.BarFillerClearOnComplete(),
			BarFillerClearOnAbort(),
		)
	}
}

// 紧凑版spinner - 显示关键进度信息但保持简洁
func AddCompactSpinner(progressBar *mpb.Progress, message string) *mpb.Bar {
	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(MinimalSpinnerLeft),
		mpb.BarWidth(2),
		mpb.PrependDecorators(
			decor.OnComplete(EmptyDecorator(), "✓"),
			decor.OnAbort(EmptyDecorator(), "✗"),
		),
		mpb.AppendDecorators(
			decor.Name(" "+message+" "),
			decor.AverageSpeed(decor.SizeB1024(0), "%.0f"), // 显示速度，但不显示单位
			decor.Name(" "),
			decor.Elapsed(decor.ET_STYLE_MMSS), // 显示经过时间，MM:SS格式
		),
		mpb.BarFillerClearOnComplete(),
		BarFillerClearOnAbort(),
	)
}

// 传统的spinner（保持向后兼容）
func AddSpinner(progressBar *mpb.Progress, message string) *mpb.Bar {
	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(PositionSpinnerLeft),
		mpb.BarWidth(3),
		mpb.PrependDecorators(
			decor.OnComplete(EmptyDecorator(), emoji.SpinnerCheckMark),
			decor.OnAbort(EmptyDecorator(), emoji.SpinnerCrossMark),
		),
		mpb.AppendDecorators(
			decor.Name("("),
			decor.Elapsed(decor.ET_STYLE_GO),
			decor.Name(") "+message+" "),
		),
		mpb.BarFillerClearOnComplete(),
		BarFillerClearOnAbort(),
	)
}

// 增强的整体进度条 - 显示速度和ETA
func AddEnhancedOverallProgress(progressBar *mpb.Progress, total int) *mpb.Bar {
	return progressBar.AddBar(int64(total),
		mpb.PrependDecorators(
			decor.Name("📦 "),
			decor.CountersNoUnit("%d/%d"),
		),
		mpb.AppendDecorators(
			decor.Name(" "),
			decor.Percentage(),
			decor.Name(" "),
			decor.AverageSpeed(decor.SizeB1024(0), "%.0f img/min"),
			decor.Name(" "),
			decor.AverageETA(decor.ET_STYLE_MMSS),
		),
		mpb.BarPriority(total+1),
	)
}

// 极简的整体进度条
func AddMinimalOverallProgress(progressBar *mpb.Progress, total int) *mpb.Bar {
	return progressBar.AddBar(int64(total),
		mpb.PrependDecorators(
			decor.Name("📦 "),              // 简单的前缀图标
			decor.CountersNoUnit("%d/%d"), // 紧凑的计数器
		),
		mpb.AppendDecorators(
			decor.Name(" "),
			decor.Percentage(), // 只显示百分比
		),
		mpb.BarPriority(total+1),
	)
}
