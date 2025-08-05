package output

import (
	"fmt"
	"html"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/liamg/guerrilla/pkg/guerrilla"
	"github.com/liamg/tml"
	"github.com/mattn/go-runewidth"
	"golang.org/x/term"
)

const (
	borderTopLeft     = '╭'
	borderTopRight    = '╮'
	borderBottomLeft  = '╰'
	borderBottomRight = '╯'
	borderVertical    = '│'
	borderHorizontal  = '─'
	borderLeftT       = '├'
	borderRightT      = '┤'
)

// HTMLConvertOptions HTML转换选项
type HTMLConvertOptions struct {
	ShowLinkURLs   bool   // 是否显示链接URL
	UseListBullets bool   // 是否使用列表项目符号
	TableSeparator string // 表格单元格分隔符
}

type Printer interface {
	PrintSummary(addresses []string)
	PrintEmail(email guerrilla.Email)
}

type printer struct {
	w           io.Writer
	width       int
	htmlOptions HTMLConvertOptions
}

func New(w io.Writer) Printer {
	width, _, err := term.GetSize(0)
	if err != nil {
		width = 80
	}

	return &printer{
		w:     w,
		width: width,
		htmlOptions: HTMLConvertOptions{
			ShowLinkURLs:   true,
			UseListBullets: true,
			TableSeparator: "\t",
		},
	}
}

var _ Printer = (*printer)(nil)

func (p *printer) printf(format string, args ...interface{}) {
	_ = tml.Fprintf(p.w, format, args...)
}

func (p *printer) printHeader(heading string) {
	p.printf(
		"\r\n<dim>%c%c%c</dim> <bold>%s</bold> <dim>%c%s%c</dim>\n",
		borderTopLeft,
		borderHorizontal,
		borderRightT,
		heading,
		borderLeftT,
		safeRepeat(string(borderHorizontal), p.width-7-runewidth.StringWidth(heading)),
		borderTopRight,
	)
	p.printBlank()
}

func (p *printer) printDivider(heading string) {
	p.printBlank()
	p.printf(
		"<dim>%c%c%c</dim> <bold>%s</bold> <dim>%c%s%c</dim>\n",
		borderLeftT,
		borderHorizontal,
		borderRightT,
		heading,
		borderLeftT,
		safeRepeat(string(borderHorizontal), p.width-7-runewidth.StringWidth(heading)),
		borderRightT,
	)
	p.printBlank()
}

func safeRepeat(input string, repeat int) string {
	if repeat <= 0 {
		return ""
	}
	return strings.Repeat(input, repeat)
}

func (p *printer) printIn(indent int, strip bool, format string, args ...interface{}) {
	var lines []string
	if strip {
		lines = p.limitSizeWithStrip(fmt.Sprintf(format, args...), p.width-indent-4)
	} else {
		lines = p.limitSize(fmt.Sprintf(format, args...), p.width-indent-4)
	}

	for _, line := range lines {
		realStr := line
		if strip {
			realStr = stripTags(line)
		}
		repeat := p.width - 4 - indent - runewidth.StringWidth(realStr)
		coloured := line
		if strip {
			coloured = tml.Sprintf(line)
		}
		padded := coloured + safeRepeat(" ", repeat)
		p.printf("<dim>%c</dim> %s", borderVertical, safeRepeat(" ", indent))
		p.printf("%s", padded)
		p.printf(" <dim>%c</dim>\n", borderVertical)
	}
}

func (p *printer) limitSizeWithStrip(input string, size int) []string {
	var word string
	var words []string
	var inTag bool
	for _, r := range []rune(input) {
		if inTag {
			word += string(r)
			if r == '>' {
				inTag = false
				if word != "" {
					words = append(words, word)
				}
				word = ""
			}
		} else {
			if r == '<' {
				if word != "" {
					words = append(words, word)
				}
				word = "<"
				inTag = true
			} else if r == ' ' {
				if word != "" {
					words = append(words, word)
					word = ""
				} else {
					words[len(words)-1] += " "
				}
			} else if r == '\n' {
				if word != "" {
					words = append(words, word)
					word = ""
				}
				words = append(words, "\n")
			} else {
				word += string(r)
			}
		}
	}
	if word != "" {
		words = append(words, word)
	}

	var line string
	var currentSize int
	var lines []string
	var hasContent bool

	for _, word := range words {
		if word == "\n" {
			lines = append(lines, line)
			line = ""
			continue
		}
		if word[0] == '<' {
			line += word
			continue
		}
		if currentSize+runewidth.StringWidth(word)+1 > size {
			lines = append(lines, line)
			line = word
			hasContent = true
			currentSize = runewidth.StringWidth(word)
		} else {
			if line != "" && hasContent {
				line += " "
				currentSize++
			}
			line += word
			hasContent = true
			currentSize += runewidth.StringWidth(word)
		}
	}

	if line != "" {
		lines = append(lines, line)
	}

	if len(lines) == 0 {
		return []string{""}
	}

	return lines
}

func (p *printer) limitSize(input string, max int) []string {
	lines := strings.Split(input, "\n")
	var output []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		for runewidth.StringWidth(line) > max {
			output = append(output, line[:max])
			line = line[max:]
		}
		until := max
		if until > len(line) {
			until = len(line)
		}
		output = append(output, line[:until])
	}
	return output
}

func (p *printer) printBlank() {
	p.printIn(0, true, "")
}

func (p *printer) printFooter() {
	p.printBlank()
	p.printf("<dim>%c%s%c</dim>\n", borderBottomLeft, safeRepeat(string(borderHorizontal), p.width-2), borderBottomRight)
}

var (
	rgxHtml        = regexp.MustCompile(`<[^>]+>`)
	rgxHtmlContent = regexp.MustCompile(`<\s*(?:html|body|div|p|br|a|table|tr|td|th|ul|ol|li|h[1-6]|img|input)\b[^>]*>`)

	// HTML转换用的预编译正则表达式
	rgxBr            = regexp.MustCompile(`<br\s*/?>`)
	rgxP             = regexp.MustCompile(`</?p[^>]*>`)
	rgxDiv           = regexp.MustCompile(`</?div[^>]*>`)
	rgxHeading       = regexp.MustCompile(`</?h[1-6][^>]*>`)
	rgxTable         = regexp.MustCompile(`</?table[^>]*>`)
	rgxTr            = regexp.MustCompile(`</?tr[^>]*>`)
	rgxTd            = regexp.MustCompile(`</?t[dh][^>]*>`)
	rgxUl            = regexp.MustCompile(`</?ul[^>]*>`)
	rgxOl            = regexp.MustCompile(`</?ol[^>]*>`)
	rgxLi            = regexp.MustCompile(`<li[^>]*>`)
	rgxLiEnd         = regexp.MustCompile(`</li>`)
	rgxLink          = regexp.MustCompile(`<a[^>]*href=["']([^"']*)["'][^>]*>([^<]*)</a>`)
	rgxAllTags       = regexp.MustCompile(`<[^>]+>`)
	rgxMultiNewlines = regexp.MustCompile(`\n\s*\n`)
)

func stripTags(input string) string {
	return rgxHtml.ReplaceAllString(input, "")
}

// isHTMLContent 检测内容是否包含HTML标签
func isHTMLContent(content string) bool {
	// 检测常见HTML标签
	if rgxHtmlContent.MatchString(content) {
		return true
	}

	// 检测HTML实体，如果包含多个HTML实体也认为是HTML内容
	entityCount := strings.Count(content, "&") + strings.Count(content, "&#")
	if entityCount >= 3 {
		return true
	}

	// 检测HTML文档声明
	if strings.Contains(strings.ToLower(content), "<!doctype html>") ||
		strings.Contains(strings.ToLower(content), "<html") {
		return true
	}

	return false
}

// processHTMLStructure 处理HTML结构标签
func processHTMLStructure(text string) string {
	// 处理换行标签
	text = rgxBr.ReplaceAllString(text, "\n")

	// 处理段落和div标签
	text = rgxP.ReplaceAllString(text, "\n")
	text = rgxDiv.ReplaceAllString(text, "\n")

	// 处理标题标签
	text = rgxHeading.ReplaceAllString(text, "\n")

	return text
}

// processHTMLTables 处理HTML表格
func processHTMLTables(text string, options HTMLConvertOptions) string {
	// 表格开始和结束添加换行
	text = rgxTable.ReplaceAllString(text, "\n")

	// 表格行添加换行
	text = rgxTr.ReplaceAllString(text, "\n")

	// 表格单元格用配置的分隔符分隔
	text = rgxTd.ReplaceAllString(text, options.TableSeparator)

	return text
}

// processHTMLLists 处理HTML列表
func processHTMLLists(text string, options HTMLConvertOptions) string {
	// 列表开始和结束添加换行
	text = rgxUl.ReplaceAllString(text, "\n")
	text = rgxOl.ReplaceAllString(text, "\n")

	// 根据配置决定是否使用项目符号
	if options.UseListBullets {
		text = rgxLi.ReplaceAllString(text, "\n• ")
	} else {
		text = rgxLi.ReplaceAllString(text, "\n")
	}
	text = rgxLiEnd.ReplaceAllString(text, "")

	return text
}

// processHTMLLinks 处理HTML链接
func processHTMLLinks(text string, options HTMLConvertOptions) string {
	if options.ShowLinkURLs {
		// 保留链接文本和URL
		return rgxLink.ReplaceAllString(text, "$2 ($1)")
	} else {
		// 只保留链接文本
		return rgxLink.ReplaceAllString(text, "$2")
	}
}

// cleanupText 清理文本格式
func cleanupText(text string) string {
	// 移除所有剩余的HTML标签
	text = rgxAllTags.ReplaceAllString(text, "")

	// 使用html包解码HTML实体
	text = html.UnescapeString(text)

	// 清理多余的空行
	text = rgxMultiNewlines.ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)

	return text
}

// convertHTMLToText 将HTML内容转换为格式化的纯文本
func convertHTMLToText(htmlContent string, options HTMLConvertOptions) (string, error) {
	text := htmlContent

	// 按顺序处理不同类型的HTML元素
	text = processHTMLStructure(text)
	text = processHTMLTables(text, options)
	text = processHTMLLists(text, options)
	text = processHTMLLinks(text, options)
	text = cleanupText(text)

	return text, nil
}

func (p *printer) PrintSummary(addresses []string) {
	p.printHeader("New Address Created")
	p.printIn(0, true, "Your disposable email addresses are:")
	p.printIn(0, true, "")

	for _, addr := range addresses {
		if strings.TrimSpace(addr) != "" {
			p.printIn(4, true, "<blue>%s</blue>", strings.TrimSpace(addr))
		}
	}

	p.printIn(0, true, "")
	p.printIn(0, true, "Emails will appear below as they are received.")
	p.printFooter()
}

func (p *printer) PrintEmail(email guerrilla.Email) {
	p.printHeader("Email #" + email.ID)
	p.printIn(0, true, "Subject:   <blue>%s", email.Subject)
	p.printIn(0, true, "From:      <blue>%s", email.From)
	p.printIn(0, true, "Time:      <blue>%s", email.Timestamp.Format(time.RFC1123))
	p.printDivider("Body")

	// 智能HTML检测和转换
	bodyContent := email.Body
	if isHTMLContent(bodyContent) {
		if convertedText, err := convertHTMLToText(bodyContent, p.htmlOptions); err == nil {
			bodyContent = convertedText
		}
		// 如果转换失败，使用原始内容（错误回退机制）
	}

	p.printIn(0, false, "%s", bodyContent)
	p.printFooter()
}
