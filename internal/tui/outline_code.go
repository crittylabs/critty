package tui

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/crittylabs/critty/internal/document"
)

type codeOutlineCache struct {
	doc           *document.Document
	path, content string
	partial       bool
	rows          []outlineHeading
}

var outlineShellShebang = regexp.MustCompile(`^#!\s*(?:\S*/)?(?:env(?:\s+-S)?\s+)?(?:sh|bash|dash|ksh|zsh)(?:\s|$)`)
var outlineRubyHeredoc = regexp.MustCompile("^<<-?[\"'`]?([a-zA-Z_][a-zA-Z0-9_]*)")
var outlineCppRaw = regexp.MustCompile(`(?:u8|u|U|L)?R"([^\s()\\]{0,16})\(`)

func codeOutlineLanguage(path, source string) string {
	if filepath.Ext(path) == ".C" {
		return "cpp"
	}
	switch strings.ToLower(filepath.Ext(path)) {
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".ts", ".tsx", ".mts", ".cts":
		return "typescript"
	case ".rs":
		return "rust"
	case ".rb", ".rake", ".gemspec":
		return "ruby"
	case ".py", ".pyi":
		return "python"
	case ".swift":
		return "swift"
	case ".kt", ".kts":
		return "kotlin"
	case ".m", ".mm":
		return "objective-c"
	case ".zig":
		return "zig"
	case ".pl", ".pm", ".t":
		return "perl"
	case ".el":
		return "emacs-lisp"
	case ".lua", ".wlua":
		return "lua"
	case ".vim":
		return "vim"
	case ".c":
		return "c"
	case ".h":
		if outlineObjCType.MatchString(source) {
			return "objective-c"
		}
		return "cpp"
	case ".cc", ".cpp", ".cxx", ".hh", ".hpp", ".hxx":
		return "cpp"
	case ".java":
		return "java"
	case ".sh", ".bash", ".zsh", ".ksh", ".dash":
		return "bash"
	}
	switch filepath.Base(path) {
	case ".emacs":
		return "emacs-lisp"
	case ".vimrc", ".gvimrc", ".exrc", "_vimrc", "_gvimrc", "_exrc", "vimrc", "gvimrc":
		return "vim"
	case "Gemfile", "Rakefile":
		return "ruby"
	case ".profile", ".bashrc", ".bash_profile", ".zshrc", ".zprofile", ".kshrc":
		return "bash"
	}
	if outlineShellShebang.MatchString(source) {
		return "bash"
	}
	return ""
}

func (t *FileTab) ensureCodeOutline() *codeOutlineCache {
	if c := t.codeOutline; c != nil && c.doc == t.doc && c.path == t.path && c.content == t.doc.Content && c.partial == (t.doc.Known != nil) {
		return c
	}
	language := codeOutlineLanguage(t.path, t.doc.Content)
	if language == "" {
		return nil
	}
	c := &codeOutlineCache{doc: t.doc, path: t.path, content: t.doc.Content, partial: t.doc.Known != nil}
	c.rows = parseOutlineDocument(t.doc, func(source string, offset int) []outlineHeading {
		return parseCodeOutline(language, source, offset)
	})
	t.codeOutline = c
	return c
}

// Gaps in a supplied diff must not carry lexical or nesting state across runs.
func parseOutlineDocument(doc *document.Document, parse func(string, int) []outlineHeading) []outlineHeading {
	if doc.Known == nil {
		return parse(doc.Content, 0)
	}
	var rows []outlineHeading
	for start := 0; start < len(doc.Lines); {
		if !doc.HasLine(start + 1) {
			start++
			continue
		}
		end := start + 1
		for end < len(doc.Lines) && doc.HasLine(end+1) {
			end++
		}
		rows = append(rows, parse(strings.Join(doc.Lines[start:end], "\n"), start)...)
		start = end
	}
	return rows
}

// Keep byte offsets and physical lines while hiding comments and string data.
// The existing lexers handle common raw strings, docstrings, and heredocs.
func outlineCode(language, source string) string {
	if language == "ruby" {
		// Use the lexer's recognized opener for Ruby's squiggly spelling.
		// This changes neither byte offsets nor the declarations we extract.
		source = strings.ReplaceAll(source, "<<~", "<<-")
	}
	lexer := lexers.Get(language)
	if lexer == nil {
		return ""
	}
	tokens, err := lexer.Tokenise(&chroma.TokeniseOptions{State: "root", EnsureLF: false}, source)
	if err != nil {
		return ""
	}
	var b strings.Builder
	b.Grow(len(source))
	heredocs := map[int]string{}
	line := 0
	for token := tokens(); token != chroma.EOF; token = tokens() {
		line += strings.Count(token.Value, "\n")
		if language == "ruby" && token.Type == chroma.LiteralString {
			if m := outlineRubyHeredoc.FindStringSubmatch(token.Value); m != nil {
				heredocs[line] = m[1]
			}
		}
		if token.Type.InCategory(chroma.Comment) || token.Type.InSubCategory(chroma.LiteralString) {
			for _, c := range []byte(token.Value) {
				if c != '\n' && c != '\r' && c != '\t' {
					c = ' '
				}
				b.WriteByte(c)
			}
		} else {
			b.WriteString(token.Value)
		}
	}
	code := b.String()
	if language == "cpp" || language == "objective-c" {
		masked := []byte(code)
		end := 0
		for _, m := range outlineCppRaw.FindAllStringSubmatchIndex(source, -1) {
			if m[0] < end || m[1] > len(masked) || masked[m[0]] == ' ' {
				continue
			}
			terminator := ")" + source[m[2]:m[3]] + "\""
			end = len(masked)
			if i := strings.Index(source[m[1]:], terminator); i >= 0 {
				end = min(end, m[1]+i+len(terminator))
			}
			for j := m[0]; j < end; j++ {
				if masked[j] != '\n' && masked[j] != '\r' {
					masked[j] = ' '
				}
			}
		}
		code = string(masked)
	}
	if len(heredocs) > 0 {
		// Chroma marks Ruby heredoc openers but does not consume their bodies.
		original, masked := strings.Split(source, "\n"), strings.Split(code, "\n")
		end := ""
		for i := range min(len(original), len(masked)) {
			if end == "" {
				end = heredocs[i]
			}
			if end != "" {
				masked[i] = strings.Repeat(" ", len(masked[i]))
				if strings.TrimSpace(original[i]) == end {
					end = ""
				}
			}
		}
		code = strings.Join(masked, "\n")
	}
	return code
}

type codeDeclaration struct {
	label, kind string
	start, body int
	generic     bool
	prototype   bool
	prefix      bool
	needsArrow  bool
}

func parseCodeOutline(language, source string, offset int) []outlineHeading {
	code := outlineCode(language, source)
	lines := strings.Split(code, "\n")
	var rows []outlineHeading
	type scope struct {
		depth int
		kind  string
	}
	var parents []scope
	depth, base := 0, 0
	opens := map[int]string{}
	indented := language == "python" || language == "ruby" || language == "lua" || language == "vim"
	open, close := byte('{'), byte('}')
	if language == "emacs-lisp" {
		open, close = '(', ')'
	}
	// Semicolon packages and Objective-C containers have no enclosing braces.
	region, regionDepth := 0, 0
	for i, line := range lines {
		text := strings.TrimSpace(line)
		indent := outlineIndent(line)
		if indented && text != "" {
			for len(parents) > 0 && parents[len(parents)-1].depth >= indent {
				parents = parents[:len(parents)-1]
			}
		}
		if language == "objective-c" && strings.HasPrefix(text, "@end") {
			region = 0
		}
		decl, ok := outlineDeclaration(language, text)
		container := language == "objective-c" && outlineObjCType.MatchString(text)
		packageLine := language == "perl" && strings.HasPrefix(text, "package ")
		if language == "objective-c" && outlineObjCMethod.MatchString(text) {
			decl = outlineObjCSelector(lines, i)
			ok = decl.label != ""
		}
		if ok && !indented && !container && language != "emacs-lisp" {
			var arrow bool
			decl.body, decl.prototype, arrow = outlineBody(lines, i, len(line)-len(strings.TrimLeft(line, " \t"))+decl.start, base)
			ok = !decl.needsArrow || arrow
			if decl.generic {
				inFunction := len(parents) > 0 && parents[len(parents)-1].kind == "function"
				ok = !inFunction && (decl.body >= 0 || decl.prototype && (decl.prefix || len(parents) > 0))
			}
		}
		if ok {
			parentRegion := region
			for _, parent := range parents {
				if parent.kind == "package" {
					parentRegion = 0
				}
			}
			level := len(parents) + parentRegion + 1
			if container || packageLine {
				level = len(parents) + 1
			}
			rows = append(rows, outlineHeading{line: i + 1 + offset, level: level, label: decl.label})
			if container && !strings.Contains(text, ";") || packageLine && decl.prototype {
				region, regionDepth = 1, depth
			}
			if language == "emacs-lisp" {
				decl.body = base + len(line) - len(strings.TrimLeft(line, " \t"))
			}
			if packageLine {
				decl.kind = "package"
			}
			if indented {
				parents = append(parents, scope{indent, decl.kind})
			} else if decl.body >= 0 {
				opens[decl.body] = decl.kind
			}
		}
		if !indented {
			for j := range len(line) {
				switch line[j] {
				case open:
					depth++
					if kind, ok := opens[base+j]; ok {
						parents = append(parents, scope{depth, kind})
						delete(opens, base+j)
					}
				case close:
					for len(parents) > 0 && parents[len(parents)-1].depth >= depth {
						parents = parents[:len(parents)-1]
					}
					depth = max(0, depth-1)
					if depth < regionDepth {
						region = 0
					}
				}
			}
		}
		base += len(line) + 1
	}
	return rows
}

func outlineIndent(line string) int {
	indent := 0
	for _, c := range line {
		switch c {
		case ' ':
			indent++
		case '\t':
			indent = (indent/8 + 1) * 8
		default:
			return indent
		}
	}
	return indent
}

// Bounded lookahead handles ordinary multiline signatures and Allman braces.
// Stop before another declaration instead of claiming its body as our own.
func outlineBody(lines []string, first, start, base int) (body int, prototype, arrow bool) {
	parens, brackets, scanned := 0, 0, 0
	for i := first; i < min(len(lines), first+16); i++ {
		line := lines[i]
		if i > first {
			trimmed := strings.TrimSpace(line)
			if parens == 0 && brackets == 0 && (outlineBoundary.MatchString(trimmed) || outlineCall.MatchString(trimmed) && !strings.HasPrefix(trimmed, ":")) {
				return -1, false, arrow
			}
			start = 0
		}
		for j := start; j < len(line); j++ {
			scanned++
			if scanned > 8192 {
				return -1, false, arrow
			}
			switch line[j] {
			case '(':
				parens++
			case ')':
				parens = max(0, parens-1)
			case '[':
				brackets++
			case ']':
				brackets = max(0, brackets-1)
			case '{', '}', ';', '=', ',':
				if parens != 0 || brackets != 0 {
					continue
				}
				if line[j] == '{' {
					return base + j, false, arrow
				}
				if line[j] == '=' && j+1 < len(line) && line[j+1] == '>' {
					arrow = true
					j++
					continue
				}
				return -1, line[j] == ';', arrow
			}
		}
		base += len(line) + 1
	}
	return -1, false, arrow
}
