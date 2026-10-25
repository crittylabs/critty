package tui

import (
	"regexp"
	"strings"
)

var outlineZig = regexp.MustCompile(`^(?:(?:pub|export|extern|inline|noinline)\s+)*(fn)\s+(` + outlineIdentifier + `)`)
var outlineZigType = regexp.MustCompile(`^(?:pub\s+)?const\s+(` + outlineIdentifier + `)\s*=\s*(?:(?:packed|extern)\s+)?(?:struct|enum|union|opaque)\b`)
var outlinePerl = regexp.MustCompile(`^(?:(?:my|our|state)\s+)?(package|sub)\s+(` + outlineIdentifier + `(?:::` + outlineIdentifier + `)*)`)
var outlineElisp = regexp.MustCompile(`^\((use-package|(?:cl-)?def(?:un|macro|subst|var(?:-local|alias)?|const|custom|group|face|alias|class|method|generic|struct|type|setf)|define-(?:minor|derived|globalized-minor|global-minor|generic)-mode)\s+([^\s()]+)`)
var outlineLua = regexp.MustCompile(`^(?:local\s+)?(function)\s+(` + outlineIdentifier + `(?:(?:\.|:)` + outlineIdentifier + `)*)`)
var outlineLuaValue = regexp.MustCompile(`^(?:local\s+)?(` + outlineIdentifier + `(?:\.` + outlineIdentifier + `)*)\s*=\s*function\s*\(`)
var outlineVim = regexp.MustCompile(`^:?(?:(?:export|legacy)\s+)?(fu(?:n(?:c(?:t(?:i(?:o(?:n)?)?)?)?)?)?!?|def!?|class|interface|enum)\s+((?:<SID>)?[^\s(]+)`)
var outlineObjCType = regexp.MustCompile(`(?m)^\s*(@interface|@implementation|@protocol)\s+(` + outlineIdentifier + `(?:\s*\([^\n)]*\))?)`)
var outlineObjCMethod = regexp.MustCompile(`^[-+]\s*\([^)]*\)\s*(` + outlineIdentifier + `)`)
var outlineObjCPart = regexp.MustCompile(`(` + outlineIdentifier + `)\s*:`)

func outlineObjCSelector(lines []string, first int) codeDeclaration {
	var signature strings.Builder
	for i := first; i < min(len(lines), first+16); i++ {
		line := strings.TrimSpace(lines[i])
		if i > first && (outlineObjCMethod.MatchString(line) || strings.HasPrefix(line, "@")) {
			break
		}
		if signature.Len()+len(line) > 8192 {
			break
		}
		if end := strings.IndexAny(line, "{;"); end >= 0 {
			signature.WriteString(line[:end])
			break
		}
		signature.WriteString(line)
		signature.WriteByte(' ')
	}
	text := signature.String()
	m := outlineObjCMethod.FindStringSubmatchIndex(text)
	if m == nil {
		return codeDeclaration{}
	}
	name := text[m[2]:m[3]]
	if parts := outlineObjCPart.FindAllStringSubmatch(text[m[2]:], -1); len(parts) > 0 {
		name = ""
		for _, part := range parts {
			name += part[1] + ":"
		}
	}
	return codeDeclaration{label: text[:1] + " " + name, kind: "function", body: -1}
}
