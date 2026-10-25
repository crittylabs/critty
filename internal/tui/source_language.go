package tui

import (
	"path/filepath"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/mattn/go-shellwords"
)

// Prefer known filenames, then use a bounded shebang for unrecognized or plain
// text files.  Never analyse the whole document just to guess its language.
func codeLexer(filename, source string) chroma.Lexer {
	ext := filepath.Ext(filename)
	if ext == ".C" {
		return lexers.Get("cpp")
	}
	switch strings.ToLower(ext) {
	case ".m", ".mm":
		return lexers.Get("objective-c")
	case ".h":
		if outlineObjCType.MatchString(source) {
			return lexers.Get("objective-c")
		}
		return lexers.Get("cpp")
	case ".dash":
		return lexers.Get("bash")
	}
	base := filepath.Base(filename)
	switch base {
	case "Brewfile", "Guardfile", "Podfile", "Capfile", "Thorfile", "Berksfile", ".irbrc":
		return lexers.Get("ruby")
	case ".emacs":
		return lexers.Get("emacs-lisp")
	case ".profile", ".kshrc", ".zprofile", ".zlogin", ".zlogout", ".zshenv":
		return lexers.Get("bash")
	}
	if (strings.HasPrefix(base, "Gemfile.") || strings.HasPrefix(base, "Rakefile.")) && !strings.HasSuffix(base, ".lock") {
		return lexers.Get("ruby")
	}
	lexer := lexers.Match(filename)
	if lexer != nil && lexer.Config().Name != "plaintext" {
		return lexer
	}
	if command := shebangCommand(source); command != "" {
		command = filepath.Base(command)
		switch strings.TrimRight(command, "0123456789.") {
		case "python", "ruby", "perl", "lua":
			command = strings.TrimRight(command, "0123456789.")
		}
		switch command {
		case "dash", "ash":
			command = "bash"
		case "node", "nodejs", "deno", "bun":
			command = "javascript"
		case "ts-node", "tsx":
			command = "typescript"
		}
		if detected := lexers.Get(command); detected != nil {
			return detected
		}
	}
	return lexer
}

func shebangCommand(source string) string {
	if !strings.HasPrefix(source, "#!") {
		return ""
	}
	line, _, _ := strings.Cut(source[:min(len(source), 4097)], "\n")
	if len(line) > 4096 {
		return ""
	}
	parser := shellwords.NewParser()
	parser.ParseEnv, parser.ParseBacktick = false, false
	args, err := parser.Parse(strings.TrimSpace(line[2:]))
	if err != nil || len(args) == 0 {
		return ""
	}
	if filepath.Base(args[0]) != "env" {
		return args[0]
	}
	args = args[1:]
	for len(args) > 0 {
		arg := args[0]
		args = args[1:]
		switch {
		case arg == "--":
			if len(args) > 0 {
				return args[0]
			}
		case arg == "-S" || arg == "--split-string":
			continue
		case strings.HasPrefix(arg, "--split-string="):
			split, err := parser.Parse(strings.TrimPrefix(arg, "--split-string="))
			if err != nil {
				return ""
			}
			args = append(split, args...)
		case arg == "-u" || arg == "--unset" || arg == "-C" || arg == "--chdir" || arg == "-a" || arg == "--argv0":
			if len(args) == 0 {
				return ""
			}
			args = args[1:]
		case arg == "-" || arg == "-i" || arg == "--ignore-environment" || arg == "-v" || arg == "--debug":
		case strings.HasPrefix(arg, "--unset="), strings.HasPrefix(arg, "--chdir="), strings.HasPrefix(arg, "--argv0="):
		case !strings.HasPrefix(arg, "-") && strings.Contains(arg, "="):
		case strings.HasPrefix(arg, "-"):
			return ""
		default:
			return arg
		}
	}
	return ""
}
