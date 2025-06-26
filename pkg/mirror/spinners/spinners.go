package spinners

import (
	"fmt"
	"io"
	"strings"

	"github.com/vbauerster/mpb/v8"
	"github.com/vbauerster/mpb/v8/decor"

	"ocpack/pkg/mirror/emoji"
)

// ANSI 颜色代码
const (
	ColorReset  = "\033[0m"
	ColorBlue   = "\033[34m" // 进行中
	ColorGreen  = "\033[32m" // 成功
	ColorYellow = "\033[33m" // 警告
	ColorRed    = "\033[31m" // 错误
	ColorCyan   = "\033[36m" // 信息
	ColorPurple = "\033[35m" // 紫色
)

// 现代化spinner样式 - 使用更流畅的动画
func ModernSpinnerLeft(original mpb.BarFiller) mpb.BarFiller {
	return mpb.SpinnerStyle("⣾", "⣽", "⣻", "⢿", "⡿", "⣟", "⣯", "⣷", " ").PositionLeft().Build()
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

// 彩色增强版spinner - 固定风格
func AddColorfulSpinner(progressBar *mpb.Progress, message string) *mpb.Bar {
	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(ModernSpinnerLeft),
		mpb.BarWidth(2),
		mpb.PrependDecorators(
			decor.OnComplete(EmptyDecorator(), ColorGreen+"✅"+ColorReset),
			decor.OnAbort(EmptyDecorator(), ColorRed+"❌"+ColorReset),
		),
		mpb.AppendDecorators(
			decor.Name(" "+message+" "),
			decor.Any(func(s decor.Statistics) string {
				elapsed := s.Total - s.Current
				minutes := elapsed / 60
				seconds := elapsed % 60
				return fmt.Sprintf("%s%02d:%02d%s", ColorCyan, minutes, seconds, ColorReset)
			}),
		),
		mpb.BarFillerClearOnComplete(),
		BarFillerClearOnAbort(),
	)
}

// 对齐美化版spinner - 固定风格
func AddAlignedSpinner(progressBar *mpb.Progress, imageName, destination, timeStr string, maxImageWidth, maxDestWidth int) *mpb.Bar {
	// 格式化对齐的消息
	alignedImage := fmt.Sprintf("%-*s", maxImageWidth, imageName)
	alignedDest := fmt.Sprintf("%-*s", maxDestWidth, destination)
	message := fmt.Sprintf("%s%s%s → %s%s%s", ColorBlue, alignedImage, ColorReset, ColorCyan, alignedDest, ColorReset)

	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(ModernSpinnerLeft),
		mpb.BarWidth(2),
		mpb.PrependDecorators(
			decor.OnComplete(EmptyDecorator(), ColorGreen+"✅"+ColorReset),
			decor.OnAbort(EmptyDecorator(), ColorRed+"❌"+ColorReset),
		),
		mpb.AppendDecorators(
			decor.Name(" "+message+" "),
			decor.Any(func(s decor.Statistics) string {
				elapsed := s.Total - s.Current
				minutes := elapsed / 60
				seconds := elapsed % 60
				return fmt.Sprintf("%s%02d:%02d%s", ColorYellow, minutes, seconds, ColorReset)
			}),
		),
		mpb.BarFillerClearOnComplete(),
		BarFillerClearOnAbort(),
	)
}

// 彩色增强版整体进度条 - 固定风格
func AddColorfulOverallProgress(progressBar *mpb.Progress, total int) *mpb.Bar {
	return progressBar.AddBar(int64(total),
		mpb.PrependDecorators(
			decor.Name(ColorPurple+"📦 "+ColorReset),
			decor.Any(func(s decor.Statistics) string {
				return fmt.Sprintf("%s%d/%d%s", ColorCyan, s.Current, total, ColorReset)
			}),
		),
		mpb.AppendDecorators(
			decor.Name(" "),
			decor.Any(func(s decor.Statistics) string {
				percentage := int(100 * float64(s.Current) / float64(s.Total))
				var color string
				var icon string
				switch {
				case percentage >= 95:
					color = ColorGreen
					icon = "🎉"
				case percentage >= 80:
					color = ColorGreen
					icon = "⚡"
				case percentage >= 60:
					color = ColorCyan
					icon = "🔥"
				case percentage >= 40:
					color = ColorYellow
					icon = "📈"
				case percentage >= 20:
					color = ColorYellow
					icon = "⏳"
				default:
					color = ColorBlue
					icon = "🔄"
				}
				return fmt.Sprintf("%s%s %d%%%s", color, icon, percentage, ColorReset)
			}),
		),
		mpb.BarPriority(total+1),
	)
}

// 传统的spinner（保持向后兼容）
func AddSpinner(progressBar *mpb.Progress, message string) *mpb.Bar {
	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(ModernSpinnerLeft),
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

// 计算字符串显示宽度（考虑中文字符）
func stringDisplayWidth(s string) int {
	// 简单实现：中文字符按2个宽度计算，英文按1个
	width := 0
	for _, r := range s {
		if r > 127 {
			width += 2
		} else {
			width += 1
		}
	}
	return width
}

// 计算多个字符串的最大显示宽度
func calculateMaxWidth(strings []string) int {
	maxWidth := 0
	for _, s := range strings {
		width := stringDisplayWidth(s)
		if width > maxWidth {
			maxWidth = width
		}
	}
	return maxWidth
}

// 填充字符串到指定宽度（考虑中文字符）
func padString(s string, width int) string {
	currentWidth := stringDisplayWidth(s)
	if currentWidth >= width {
		return s
	}
	return s + strings.Repeat(" ", width-currentWidth)
}
