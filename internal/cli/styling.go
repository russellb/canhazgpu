package cli

import (
	"fmt"
	"strings"

	"github.com/fatih/color"
)

var (
	// Global flag to enable/disable colors
	noColor = false

	// Status colors
	colorAvailable  = color.New(color.FgGreen, color.Bold)
	colorInUse      = color.New(color.FgBlue)
	colorUnreserved = color.New(color.FgYellow, color.Bold)
	colorError      = color.New(color.FgRed, color.Bold)

	// UI element colors
	colorHeader  = color.New(color.FgCyan, color.Bold)
	colorHost    = color.New(color.FgMagenta, color.Bold)
	colorMetric  = color.New(color.FgWhite, color.Bold)
	colorDim     = color.New(color.Faint)
	colorSuccess = color.New(color.FgGreen)
	colorWarning = color.New(color.FgYellow)

	// Box drawing characters
	boxHorizontal  = "─"
	boxVertical    = "│"
	boxTopLeft     = "┌"
	boxTopRight    = "┐"
	boxBottomLeft  = "└"
	boxBottomRight = "┘"
	boxCross       = "┼"
	boxTDown       = "┬"
	boxTUp         = "┴"
	boxTRight      = "├"
	boxTLeft       = "┤"
)

func init() {
	// Disable colors if NO_COLOR environment variable is set
	if noColor {
		color.NoColor = true
	}
}

// SetNoColor enables or disables color output
func SetNoColor(value bool) {
	noColor = value
	color.NoColor = value
}

// FormatStatus returns a colored status string
func FormatStatus(status string) string {
	switch status {
	case "AVAILABLE":
		return colorAvailable.Sprint("● AVAILABLE")
	case "IN_USE":
		return colorInUse.Sprint("● IN_USE   ")
	case "UNRESERVED":
		return colorUnreserved.Sprint("⚠ UNRESERVED")
	case "ERROR":
		return colorError.Sprint("✗ ERROR    ")
	default:
		return "  " + status
	}
}

// FormatHeader returns a colored header string
func FormatHeader(text string) string {
	return colorHeader.Sprint(text)
}

// FormatHost returns a colored host name
func FormatHost(host string) string {
	return colorHost.Sprint(host)
}

// FormatMetric returns a colored metric number
func FormatMetric(value int) string {
	return colorMetric.Sprint(value)
}

// FormatDim returns dimmed text
func FormatDim(text string) string {
	return colorDim.Sprint(text)
}

// DrawBox draws a box around content
func DrawBox(title string, content []string) string {
	if len(content) == 0 {
		return ""
	}

	// Find max width
	maxWidth := len(title) + 2
	for _, line := range content {
		// Strip ANSI codes for length calculation
		plainLine := stripANSI(line)
		if len(plainLine) > maxWidth {
			maxWidth = len(plainLine)
		}
	}

	var sb strings.Builder

	// Top border with title
	sb.WriteString(boxTopLeft)
	if title != "" {
		sb.WriteString(" ")
		sb.WriteString(colorHost.Sprint(title))
		sb.WriteString(" ")
		remaining := maxWidth - len(title) - 2
		if remaining > 0 {
			sb.WriteString(strings.Repeat(boxHorizontal, remaining))
		}
	} else {
		sb.WriteString(strings.Repeat(boxHorizontal, maxWidth))
	}
	sb.WriteString(boxTopRight)
	sb.WriteString("\n")

	// Content
	for _, line := range content {
		sb.WriteString(boxVertical)
		sb.WriteString(" ")
		sb.WriteString(line)
		// Pad to max width (accounting for ANSI codes)
		plainLine := stripANSI(line)
		padding := maxWidth - len(plainLine) - 1
		if padding > 0 {
			sb.WriteString(strings.Repeat(" ", padding))
		}
		sb.WriteString(boxVertical)
		sb.WriteString("\n")
	}

	// Bottom border
	sb.WriteString(boxBottomLeft)
	sb.WriteString(strings.Repeat(boxHorizontal, maxWidth))
	sb.WriteString(boxBottomRight)
	sb.WriteString("\n")

	return sb.String()
}

// DrawSeparator draws a horizontal separator line
func DrawSeparator(width int) string {
	return colorDim.Sprint(strings.Repeat(boxHorizontal, width))
}

// DrawTableBorder draws a table border
func DrawTableBorder(columnWidths []int, position string) string {
	var sb strings.Builder

	switch position {
	case "top":
		sb.WriteString(boxTopLeft)
		for i, width := range columnWidths {
			sb.WriteString(strings.Repeat(boxHorizontal, width+2))
			if i < len(columnWidths)-1 {
				sb.WriteString(boxTDown)
			}
		}
		sb.WriteString(boxTopRight)
	case "middle":
		sb.WriteString(boxTRight)
		for i, width := range columnWidths {
			sb.WriteString(strings.Repeat(boxHorizontal, width+2))
			if i < len(columnWidths)-1 {
				sb.WriteString(boxCross)
			}
		}
		sb.WriteString(boxTLeft)
	case "bottom":
		sb.WriteString(boxBottomLeft)
		for i, width := range columnWidths {
			sb.WriteString(strings.Repeat(boxHorizontal, width+2))
			if i < len(columnWidths)-1 {
				sb.WriteString(boxTUp)
			}
		}
		sb.WriteString(boxBottomRight)
	}

	return colorDim.Sprint(sb.String())
}

// stripANSI removes ANSI color codes for length calculation
func stripANSI(str string) string {
	// Simple ANSI code stripper
	result := strings.Builder{}
	inEscape := false

	for _, r := range str {
		if r == '\x1b' {
			inEscape = true
			continue
		}
		if inEscape {
			if r == 'm' {
				inEscape = false
			}
			continue
		}
		result.WriteRune(r)
	}

	return result.String()
}

// FormatSummaryMetric formats a summary metric with color
func FormatSummaryMetric(label string, value int, total int) string {
	percentage := 0
	if total > 0 {
		percentage = (value * 100) / total
	}

	var colorFunc *color.Color
	switch label {
	case "AVAILABLE":
		if percentage > 50 {
			colorFunc = colorSuccess
		} else if percentage > 25 {
			colorFunc = colorWarning
		} else {
			colorFunc = colorError
		}
	case "IN_USE":
		if percentage < 50 {
			colorFunc = colorSuccess
		} else if percentage < 75 {
			colorFunc = colorWarning
		} else {
			colorFunc = colorError
		}
	default:
		colorFunc = color.New(color.FgWhite)
	}

	return fmt.Sprintf("%s: %s",
		colorHeader.Sprint(label),
		colorFunc.Sprintf("%d", value))
}
