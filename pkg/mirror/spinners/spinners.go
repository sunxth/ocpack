package spinners

import (
	"fmt"
	"io"

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

// 完成状态图标选择 - 您可以选择喜欢的
const (
	// 选项1: 经典勾选
	CompletedIcon1 = "✓" // 简洁勾号
	CompletedIcon2 = "✔" // 粗勾号
	CompletedIcon3 = "✅" // 绿色方框勾号(当前)

	// 选项2: 圆形图标
	CompletedIcon4 = "🟢" // 绿色圆点
	CompletedIcon5 = "⚫" // 黑色圆点
	CompletedIcon6 = "●" // 实心圆点

	// 选项3: 方形图标
	CompletedIcon7 = "▣" // 方框勾号
	CompletedIcon8 = "■" // 实心方块
	CompletedIcon9 = "◼" // 中等方块

	// 选项4: 箭头图标
	CompletedIcon10 = "►" // 右箭头
	CompletedIcon11 = "▶" // 播放图标
	CompletedIcon12 = "→" // 简单箭头
)

// 当前使用的完成图标 - 您可以修改这里来选择不同的图标
var CompletedIcon = CompletedIcon1 // 默认使用简洁勾号

// 更和谐的动态spinner样式 - 使用圆形旋转动画
func ModernSpinnerLeft(original mpb.BarFiller) mpb.BarFiller {
	// 选项1: 圆形旋转动画 (用户要求)
	return mpb.SpinnerStyle("◐", "◓", "◑", "◒").PositionLeft().Build()

	// 选项2: 点状动画
	// return mpb.SpinnerStyle("⣀", "⣄", "⣤", "⣦", "⣶", "⣷", "⣿", "⡿", "⠿", "⠟", "⠛", "⠙", "⠈", "⠁").PositionLeft().Build()

	// 选项3: 经典旋转
	// return mpb.SpinnerStyle("|", "/", "-", "\\").PositionLeft().Build()

	// 选项4: 现代箭头
	// return mpb.SpinnerStyle("←", "↖", "↑", "↗", "→", "↘", "↓", "↙").PositionLeft().Build()
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

// 彩色增强版spinner - 固定风格，时间前置
func AddColorfulSpinner(progressBar *mpb.Progress, message string) *mpb.Bar {
	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(ModernSpinnerLeft),
		mpb.BarWidth(2),
		mpb.PrependDecorators(
			decor.OnComplete(EmptyDecorator(), ColorGreen+CompletedIcon+ColorReset),
			decor.OnAbort(EmptyDecorator(), ColorRed+"❌"+ColorReset),
		),
		mpb.AppendDecorators(
			// 时间前置，加括号和颜色
			decor.Name(ColorCyan+"("+ColorReset),
			decor.Elapsed(decor.ET_STYLE_MMSS),
			decor.Name(ColorCyan+")"+ColorReset+" "+message),
		),
		mpb.BarFillerClearOnComplete(),
		BarFillerClearOnAbort(),
	)
}

// 对齐美化版spinner - 固定风格，时间前置
func AddAlignedSpinner(progressBar *mpb.Progress, imageName, destination string, maxImageWidth, maxDestWidth int) *mpb.Bar {
	// 格式化对齐的消息
	alignedImage := fmt.Sprintf("%-*s", maxImageWidth, imageName)
	alignedDest := fmt.Sprintf("%-*s", maxDestWidth, destination)
	message := fmt.Sprintf("%s%s%s → %s%s%s", ColorBlue, alignedImage, ColorReset, ColorCyan, alignedDest, ColorReset)

	return progressBar.AddSpinner(
		1, mpb.BarFillerMiddleware(ModernSpinnerLeft),
		mpb.BarWidth(2),
		mpb.PrependDecorators(
			decor.OnComplete(EmptyDecorator(), ColorGreen+CompletedIcon+ColorReset),
			decor.OnAbort(EmptyDecorator(), ColorRed+"❌"+ColorReset),
		),
		mpb.AppendDecorators(
			// 时间前置，加括号和颜色
			decor.Name(ColorYellow+"("+ColorReset),
			decor.Elapsed(decor.ET_STYLE_MMSS),
			decor.Name(ColorYellow+")"+ColorReset+" "+message),
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
