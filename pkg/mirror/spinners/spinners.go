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

// 极简风格的spinner - 方案三实现
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
