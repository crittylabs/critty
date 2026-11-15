package tui

import (
	"regexp"
	"strings"

	"github.com/crittylabs/critty/v2/internal/icons"
)

const outlineIdentifier = `[\pL_$][\pL\pM\pN_$]*`
const outlineQualified = `~?` + outlineIdentifier + `(?:(?:::|\.)~?` + outlineIdentifier + `)*`

var outlinePython = regexp.MustCompile(`^(?:async\s+)?(class|def)\s+(` + outlineIdentifier + `)`)
var outlineRuby = regexp.MustCompile(`^(class|module|def)\s+(` + outlineQualified + `[!?=]?)`)
var outlineRust = regexp.MustCompile(`^(?:pub(?:\([^)]*\))?\s+)?(?:(?:unsafe|async|const|extern)\s+)*(fn|struct|enum|trait|mod|type|union)\s+((?:r#)?` + outlineIdentifier + `)`)
var outlineRustImpl = regexp.MustCompile(`^(?:unsafe\s+)?impl\b`)
var outlineJS = regexp.MustCompile(`^(?:(?:export|default|declare|abstract|async)\s+)*(class|interface|enum|type|function)\s*\*?\s+(` + outlineIdentifier + `)`)
var outlineJSVariable = regexp.MustCompile(`^(?:export\s+)?(const|let|var)\s+(` + outlineIdentifier + `)(?:\s*:[^=]+)?\s*=\s*`)
var outlineJSField = regexp.MustCompile(`^(?:(?:public|private|protected|static|readonly|override)\s+)*(` + outlineIdentifier + `)\s*(?:\??\s*:[^=]+)?=\s*`)
var outlineJSProperty = regexp.MustCompile(`^(` + outlineIdentifier + `)\s*:\s*`)
var outlineJSArrow = regexp.MustCompile(`^(?:async\s+)?` + outlineIdentifier + `\s*=>`)
var outlineSwift = regexp.MustCompile(`^(?:(?:public|private|fileprivate|internal|open|final|static|class|override|required|convenience|mutating|nonmutating|nonisolated|distributed|indirect)\s+)*(class|struct|enum|protocol|extension|actor|typealias|func)\s+(` + outlineIdentifier + `)`)
var outlineSwiftInit = regexp.MustCompile(`^(?:(?:public|private|fileprivate|internal|required|convenience|override)\s+)*(init[!?]?|deinit|subscript)\b`)
var outlineKotlin = regexp.MustCompile(`^(?:(?:public|private|protected|internal|open|final|abstract|override|suspend|inline|tailrec|operator|infix|external|expect|actual|data|sealed|enum|annotation|value)\s+)*(class|interface|object|fun|typealias)\s+(?:<[^>]*>\s*)?(` + outlineQualified + `)`)
var outlineKotlinFunction = regexp.MustCompile(`^(?:(?:public|private|protected|internal|open|final|abstract|override|suspend|inline|tailrec|operator|infix|external|expect|actual)\s+)*(fun)\s+(?:<[^>]*>\s*)?([^\s(]+)`)
var outlineCompanion = regexp.MustCompile(`^companion\s+object(?:\s+(` + outlineIdentifier + `))?`)
var outlineCType = regexp.MustCompile(`^(?:(?:public|private|protected|static|final|abstract|sealed|non-sealed|typedef|inline)\s+)*(@interface|class|struct|union|enum|namespace|record)\s+(?:class\s+|struct\s+)?(` + outlineQualified + `)`)
var outlineTypedef = regexp.MustCompile(`^typedef\s+[^{}();]+\b(` + outlineIdentifier + `)\s*;`)
var outlineShell = regexp.MustCompile(`^(?:function\s+(` + outlineIdentifier + `)|(` + outlineIdentifier + `)\s*\(\s*\))`)
var outlineCall = regexp.MustCompile(`(` + outlineQualified + `)\s*\(`)
var outlineBoundary = regexp.MustCompile(`^(?:(?:public|private|protected|internal|export|static|async|abstract|override|pub(?:\([^)]*\))?)\s+)*(?:func|fun|fn|def|class|struct|enum|interface|trait|impl|namespace|const|let|var|type|module)\b`)

func outlineMatch(pattern *regexp.Regexp, text string) (codeDeclaration, bool) {
	m := pattern.FindStringSubmatchIndex(text)
	if m == nil {
		return codeDeclaration{}, false
	}
	kind, name := text[m[2]:m[3]], text[m[4]:m[5]]
	category := "type"
	if kind == "sub" || kind == "def" || kind == "fn" || kind == "func" || kind == "fun" || kind == "function" {
		category = "function"
	}
	return codeDeclaration{label: kind + " " + name, name: name, kind: category, symbol: outlineKeywordIcon(kind), start: m[1], body: -1}, true
}

func outlineKeywordIcon(keyword string) icons.Kind {
	switch keyword {
	case "struct", "union", "record", "cl-defstruct":
		return icons.Struct
	case "interface", "trait", "protocol", "@protocol":
		return icons.Interface
	case "enum":
		return icons.Enum
	case "namespace", "module", "mod", "package", "use-package":
		return icons.Namespace
	case "defconst":
		return icons.Constant
	case "defvar", "defvar-local", "defvaralias", "defcustom", "defgroup", "defface":
		return icons.Variable
	case "defmethod", "cl-defmethod":
		return icons.Method
	case "sub", "def", "fn", "func", "fun", "function", "defun", "cl-defun", "defmacro", "cl-defmacro", "defsubst", "cl-defsubst", "defalias", "cl-defgeneric", "defgeneric", "defsetf", "cl-defsetf":
		return icons.Function
	}
	if strings.HasPrefix(keyword, "define-") || strings.HasPrefix(keyword, "fu") || keyword == "def!" {
		return icons.Function
	}
	return icons.Class
}

func outlineDeclaration(language, text string) (codeDeclaration, bool) {
	switch language {
	case "zig":
		if decl, ok := outlineMatch(outlineZig, text); ok {
			return decl, true
		}
		if m := outlineZigType.FindStringSubmatchIndex(text); m != nil {
			name := text[m[2]:m[3]]
			return codeDeclaration{label: "const " + name, name: name, kind: "type", symbol: outlineKeywordIcon(text[m[4]:m[5]]), start: m[1], body: -1}, true
		}
		return codeDeclaration{}, false
	case "perl":
		return outlineMatch(outlinePerl, text)
	case "emacs-lisp":
		return outlineMatch(outlineElisp, text)
	case "lua":
		if decl, ok := outlineMatch(outlineLua, text); ok {
			return decl, true
		}
		if m := outlineLuaValue.FindStringSubmatchIndex(text); m != nil {
			name := text[m[2]:m[3]]
			return codeDeclaration{label: "function " + name, name: name, kind: "function", body: -1}, true
		}
		return codeDeclaration{}, false
	case "vim":
		return outlineMatch(outlineVim, text)
	case "objective-c":
		if m := outlineObjCType.FindStringSubmatchIndex(text); m != nil {
			name := strings.TrimSpace(text[m[4]:m[5]])
			return codeDeclaration{label: strings.TrimSpace(text[m[2]:m[3]]) + " " + name, name: name, kind: "type", symbol: outlineKeywordIcon(text[m[2]:m[3]]), body: -1}, true
		}
		return outlineDeclaration("cpp", text)
	case "python":
		return outlineMatch(outlinePython, text)
	case "ruby":
		if strings.HasPrefix(text, "class << ") {
			return codeDeclaration{label: text, name: strings.TrimPrefix(text, "class "), kind: "type", body: -1}, true
		}
		return outlineMatch(outlineRuby, text)
	case "rust":
		if match := outlineRustImpl.FindStringIndex(text); match != nil {
			end := len(text)
			if i := strings.IndexAny(text, "{\n"); i >= 0 {
				end = i
			}
			return codeDeclaration{label: strings.TrimSpace(text[:end]), name: strings.TrimSpace(text[match[1]:end]), kind: "type", body: -1}, true
		}
		return outlineMatch(outlineRust, text)
	case "javascript", "typescript":
		if decl, ok := outlineMatch(outlineJS, text); ok {
			return decl, true
		}
		if m := outlineJSVariable.FindStringSubmatchIndex(text); m != nil {
			return outlineJSValue(text, m[1], text[m[2]:m[3]]+" "+text[m[4]:m[5]], text[m[4]:m[5]])
		}
		for _, pattern := range []*regexp.Regexp{outlineJSField, outlineJSProperty} {
			if m := pattern.FindStringSubmatchIndex(text); m != nil {
				if decl, ok := outlineJSValue(text, m[1], text[m[2]:m[3]], text[m[2]:m[3]]); ok {
					return decl, true
				}
			}
		}
	case "swift":
		if decl, ok := outlineMatch(outlineSwift, text); ok {
			return decl, true
		}
		if m := outlineSwiftInit.FindStringSubmatchIndex(text); m != nil {
			return codeDeclaration{label: text[m[2]:m[3]], kind: "function", start: m[1], body: -1}, true
		}
		return codeDeclaration{}, false
	case "kotlin":
		if i := strings.Index(text, "fun interface "); i >= 0 {
			decl, ok := outlineMatch(outlineKotlin, text[:i]+text[i+4:])
			decl.start += 4
			return decl, ok
		}
		if m := outlineCompanion.FindStringSubmatchIndex(text); m != nil {
			name := "companion"
			if m[2] >= 0 {
				name = text[m[2]:m[3]]
			}
			return codeDeclaration{label: strings.TrimSpace(text[:m[1]]), name: name, kind: "type", start: m[1], body: -1}, true
		}
		if decl, ok := outlineMatch(outlineKotlinFunction, text); ok && decl.label != "fun interface" {
			return decl, true
		}
		if decl, ok := outlineMatch(outlineKotlin, text); ok {
			return decl, true
		}
		return codeDeclaration{}, false
	case "c", "cpp", "java":
		if decl, ok := outlineMatch(outlineCType, text); ok {
			return decl, true
		}
		if m := outlineTypedef.FindStringSubmatchIndex(text); m != nil {
			name := text[m[2]:m[3]]
			return codeDeclaration{label: "typedef " + name, name: name, kind: "type", body: -1}, true
		}
	case "bash":
		if m := outlineShell.FindStringSubmatchIndex(text); m != nil {
			name := ""
			for i := 2; i < len(m); i += 2 {
				if m[i] >= 0 {
					name = text[m[i]:m[i+1]]
				}
			}
			return codeDeclaration{label: "function " + name, name: name, kind: "function", start: m[1], body: -1}, true
		}
		return codeDeclaration{}, false
	}
	if strings.HasPrefix(text, "#") {
		return codeDeclaration{}, false
	}
	for _, m := range outlineCall.FindAllStringSubmatchIndex(text, -1) {
		name := text[m[2]:m[3]]
		prefix := strings.TrimSpace(text[:m[2]])
		if strings.ContainsAny(prefix, "=.@") {
			continue
		}
		switch name {
		case "if", "for", "while", "switch", "catch", "when", "with", "synchronized", "sizeof", "alignof", "decltype", "static_assert", "void", "return", "throw", "new", "super", "this":
			continue
		}
		if fields := strings.Fields(prefix); len(fields) > 0 {
			switch fields[0] {
			case "return", "throw", "new", "case", "else", "await":
				return codeDeclaration{}, false
			}
		}
		return codeDeclaration{label: name + "()", kind: "function", start: m[3], body: -1, generic: true, prefix: prefix != ""}, true
	}
	return codeDeclaration{}, false
}

func outlineJSValue(text string, start int, label, name string) (codeDeclaration, bool) {
	value := text[start:]
	decl := codeDeclaration{label: label, name: name, kind: "function", start: start, body: -1}
	switch {
	case strings.HasPrefix(value, "{"):
		decl.kind = "type"
	case strings.HasPrefix(value, "function"), strings.HasPrefix(value, "async function"):
	case outlineJSArrow.MatchString(value), strings.HasPrefix(value, "("), strings.HasPrefix(value, "async "), strings.HasPrefix(value, "<"):
		if arrow := strings.Index(value, "=>"); arrow >= 0 {
			decl.start += arrow + 2
		} else {
			decl.needsArrow = true
		}
	default:
		return codeDeclaration{}, false
	}
	return decl, true
}
