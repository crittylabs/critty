package cli

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"golang.org/x/term"

	"github.com/crittylabs/critty/internal/tui"
)

// helpWidth bounds the styled help so long descriptions stay readable on
// wide terminals.
const helpWidth = 100

// showHelp prints Cobra's plain help, or a styled page through the pager
// when stdout is a color-capable terminal.
func showHelp(cmd *cobra.Command, _ []string) {
	out := cmd.OutOrStdout()
	style := detectOutputStyle(out)
	width, paged := pageWidth(out, style)
	if !paged {
		if _, err := io.WriteString(out, plainHelp(cmd)); err != nil {
			cmd.PrintErrln(err)
		}
		return
	}
	var buf strings.Builder
	if _, err := io.WriteString(style.writer(&buf), styledHelp(cmd, style.dark, width)); err != nil {
		cmd.PrintErrln(err)
		return
	}
	if err := pageOrPrint(out, buf.String()); err != nil {
		cmd.PrintErrln(err)
	}
}

// pageWidth reports whether out is a color-capable terminal whose output
// should be styled and paged, and the width to wrap that output to.
func pageWidth(out io.Writer, style outputStyle) (width int, paged bool) {
	file, ok := out.(*os.File)
	if !ok || !style.color() || !term.IsTerminal(int(file.Fd())) {
		return 0, false
	}
	width = 80
	if w, _, err := term.GetSize(int(file.Fd())); err == nil && w > 0 {
		width = w
	}
	return min(width, helpWidth), true
}

// pageOrPrint shows text through the pager, or writes it to out when no
// pager is available.
func pageOrPrint(out io.Writer, text string) error {
	if showInPager(text) {
		return nil
	}
	_, err := io.WriteString(out, text)
	return err
}

// plainHelp mirrors Cobra's default help output.
func plainHelp(cmd *cobra.Command) string {
	var b strings.Builder
	if usage := strings.TrimRight(cmp(cmd.Long, cmd.Short), " \t\n"); usage != "" {
		b.WriteString(usage + "\n\n")
	}
	if cmd.Runnable() || cmd.HasSubCommands() {
		b.WriteString(cmd.UsageString())
	}
	return b.String()
}

func cmp(long, short string) string {
	if long != "" {
		return long
	}
	return short
}

// helpStyles holds the help page palette for a dark or light background.
type helpStyles struct {
	heading, name, code, meta lipgloss.Style
}

func newHelpStyles(dark bool) helpStyles {
	ld := lipgloss.LightDark(dark)
	return helpStyles{
		heading: lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#D29600"), lipgloss.BrightYellow)),
		name:    lipgloss.NewStyle().Bold(true).Foreground(ld(lipgloss.Color("#00768A"), lipgloss.BrightCyan)),
		code:    lipgloss.NewStyle().Foreground(ld(lipgloss.Color("#2ea043"), lipgloss.Color("#3fb950"))),
		meta:    lipgloss.NewStyle().Foreground(lipgloss.BrightBlack),
	}
}

var backquoted = regexp.MustCompile("`([^`]+)`")

// prose styles backquoted commands in help text and wraps unindented lines
// to width; indented lines are preformatted and stay as written.
func (s helpStyles) prose(text string, width int) string {
	lines := strings.Split(strings.TrimRight(text, " \t\n"), "\n")
	for i, line := range lines {
		styled := backquoted.ReplaceAllStringFunc(line, func(m string) string {
			return s.code.Render(m[1 : len(m)-1])
		})
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			styled = wrapWords(styled, width)
		}
		lines[i] = styled
	}
	return strings.Join(lines, "\n")
}

// styledHelp renders the help page: the brand, the description, and the
// usage sections Cobra prints, with names and headings colored.
func styledHelp(cmd *cobra.Command, dark bool, width int) string {
	s := newHelpStyles(dark)
	var b strings.Builder
	b.WriteString(tui.BrandVersion(cmd.Root().Version, dark) + "\n\n")
	if text := cmp(cmd.Long, cmd.Short); text != "" {
		b.WriteString(s.prose(text, width) + "\n\n")
	}
	section := func(title string) { b.WriteString(s.heading.Render(title) + "\n") }
	section("Usage")
	if cmd.Runnable() {
		fmt.Fprintf(&b, "  %s\n", s.useLine(cmd))
	}
	if cmd.HasAvailableSubCommands() {
		fmt.Fprintf(&b, "  %s %s\n", s.name.Render(cmd.CommandPath()), "[command]")
	}
	if len(cmd.Aliases) > 0 {
		b.WriteString("\n")
		section("Aliases")
		fmt.Fprintf(&b, "  %s\n", s.name.Render(cmd.NameAndAliases()))
	}
	if cmd.HasExample() {
		b.WriteString("\n")
		section("Examples")
		b.WriteString(s.prose(cmd.Example, width) + "\n")
	}
	if cmd.HasAvailableSubCommands() {
		b.WriteString("\n")
		s.commands(&b, cmd, width)
	}
	if cmd.HasAvailableLocalFlags() {
		b.WriteString("\n")
		section("Flags")
		b.WriteString(s.flags(cmd.LocalFlags(), width))
	}
	if cmd.HasAvailableInheritedFlags() {
		b.WriteString("\n")
		section("Global Flags")
		b.WriteString(s.flags(cmd.InheritedFlags(), width))
	}
	if cmd.HasAvailableSubCommands() {
		hint := "Use " + s.code.Render(cmd.CommandPath()+" [command] --help") + " for more information about a command."
		b.WriteString("\n" + wrapWords(hint, width) + "\n")
	}
	return b.String()
}

// useLine colors the command path of Cobra's use line and dims the rest.
func (s helpStyles) useLine(cmd *cobra.Command) string {
	line := cmd.UseLine()
	path := cmd.CommandPath()
	if !strings.HasPrefix(line, path) {
		return line
	}
	return s.name.Render(path) + line[len(path):]
}

func (s helpStyles) commands(b *strings.Builder, cmd *cobra.Command, width int) {
	groups := cmd.Groups()
	if len(groups) == 0 {
		s.commandGroup(b, "Commands", cmd, "", width)
		return
	}
	for i, group := range groups {
		if i > 0 {
			b.WriteString("\n")
		}
		s.commandGroup(b, strings.TrimSuffix(group.Title, ":"), cmd, group.ID, width)
	}
	if !cmd.AllChildCommandsHaveGroup() {
		b.WriteString("\n")
		s.commandGroup(b, "Additional Commands", cmd, "", width)
	}
}

func (s helpStyles) commandGroup(b *strings.Builder, title string, cmd *cobra.Command, groupID string, width int) {
	b.WriteString(s.heading.Render(title) + "\n")
	for _, sub := range cmd.Commands() {
		if sub.GroupID != groupID || !sub.IsAvailableCommand() && sub.Name() != "help" {
			continue
		}
		name := s.name.Width(sub.NamePadding() + 1).Render(sub.Name())
		b.WriteString("  " + name + hanging(s.prose(sub.Short, width-sub.NamePadding()-3), sub.NamePadding()+3) + "\n")
	}
}

// flags lays out a flag set like pflag, with names colored, value names
// dimmed, and usage wrapped under its own column.
func (s helpStyles) flags(set *pflag.FlagSet, width int) string {
	type row struct{ head, usage string }
	var rows []row
	headWidth := 0
	set.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		head := "    " + s.name.Render("--"+f.Name)
		if f.Shorthand != "" && f.ShorthandDeprecated == "" {
			head = s.name.Render("-"+f.Shorthand) + ", " + s.name.Render("--"+f.Name)
		}
		varname, usage := pflag.UnquoteUsage(f)
		if varname != "" {
			head += " " + s.meta.Render(varname)
		}
		if optional := noOptDefault(f); optional != "" {
			head += s.meta.Render(optional)
		}
		if !isZeroDefault(f) {
			if f.Value.Type() == "string" {
				usage += s.meta.Render(fmt.Sprintf(" (default %q)", f.DefValue))
			} else {
				usage += s.meta.Render(" (default " + f.DefValue + ")")
			}
		}
		if f.Deprecated != "" {
			usage += s.meta.Render(" (DEPRECATED: " + f.Deprecated + ")")
		}
		headWidth = max(headWidth, lipgloss.Width(head))
		rows = append(rows, row{head, usage})
	})
	var b strings.Builder
	column := headWidth + 5
	for _, r := range rows {
		padding := strings.Repeat(" ", headWidth-lipgloss.Width(r.head)+3)
		usage := backquoted.ReplaceAllStringFunc(r.usage, func(m string) string { return s.code.Render(m[1 : len(m)-1]) })
		b.WriteString("  " + r.head + padding + hanging(wrapWords(usage, max(20, width-column)), column) + "\n")
	}
	return b.String()
}

func noOptDefault(f *pflag.Flag) string {
	if f.NoOptDefVal == "" {
		return ""
	}
	switch f.Value.Type() {
	case "string":
		return fmt.Sprintf("[=%q]", f.NoOptDefVal)
	case "bool", "boolfunc":
		if f.NoOptDefVal == "true" {
			return ""
		}
	case "count":
		if f.NoOptDefVal == "+1" {
			return ""
		}
	}
	return "[=" + f.NoOptDefVal + "]"
}

// isZeroDefault mirrors pflag's unexported defaultIsZeroValue.
func isZeroDefault(f *pflag.Flag) bool {
	switch f.Value.Type() {
	case "bool":
		return f.DefValue == "false"
	case "duration":
		return f.DefValue == "0" || f.DefValue == "0s"
	case "int", "int8", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "count", "float32", "float64":
		return f.DefValue == "0"
	case "string":
		return f.DefValue == ""
	case "ip", "ipMask", "ipNet":
		return f.DefValue == "<nil>"
	case "intSlice", "stringSlice", "stringArray":
		return f.DefValue == "[]"
	default:
		switch f.DefValue {
		case "false", "<nil>", "", "0":
			return true
		}
		return false
	}
}

// wrapWords wraps text at spaces so flag values and hyphenated names stay
// whole; a word longer than the limit keeps its own line.
func wrapWords(text string, limit int) string {
	var lines []string
	var line strings.Builder
	width := 0
	for _, word := range strings.Split(text, " ") {
		w := ansi.StringWidth(word)
		if width > 0 && width+1+w > limit {
			lines = append(lines, line.String())
			line.Reset()
			width = 0
		}
		if width > 0 {
			line.WriteString(" ")
			width++
		}
		line.WriteString(word)
		width += w
	}
	return strings.Join(append(lines, line.String()), "\n")
}

// hanging indents every line after the first by indent columns.
func hanging(text string, indent int) string {
	return strings.ReplaceAll(text, "\n", "\n"+strings.Repeat(" ", indent))
}

func init() {
	rootCmd.SetHelpFunc(showHelp)
}
