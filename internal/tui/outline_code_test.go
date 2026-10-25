package tui

import (
	"reflect"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/crittylabs/critty/internal/document"
)

func TestCodeOutlineDeclarations(t *testing.T) {
	for _, tc := range []struct {
		path, source string
		want         []outlineHeading
	}{
		{"sample.js", "export class Service {\n  async run(value) {\n    if (value) {\n      send(value);\n    }\n  }\n}\nconst start = (\n  value,\n) => {\n  function normalize() {}\n};\nfunction* items() {}\n", []outlineHeading{{1, 1, "class Service"}, {2, 2, "run()"}, {8, 1, "const start"}, {11, 2, "function normalize"}, {13, 1, "function items"}}},
		{"sample.ts", "export interface Store {\n  get(key: string): string;\n}\ntype Key = string;\nexport class Memory implements Store {\n  get(key: string): string {\n    return lookup(key);\n  }\n}\n", []outlineHeading{{1, 1, "interface Store"}, {2, 2, "get()"}, {4, 1, "type Key"}, {5, 1, "class Memory"}, {6, 2, "get()"}}},
		{"sample.tsx", "export function Panel() {\n  return <div onClick={() => act()} />;\n}\nconst Button = () => <button />;\n", []outlineHeading{{1, 1, "function Panel"}, {4, 1, "const Button"}}},
		{"fields.ts", "class Service {\n  readonly run = async () => {\n    send();\n  };\n}\nconst api = {\n  stop: () => {},\n  start() {},\n};\nconst count = (2 + 3);\nconst list = values.map(value => value);\n", []outlineHeading{{1, 1, "class Service"}, {2, 2, "run"}, {6, 1, "const api"}, {7, 2, "stop"}, {8, 2, "start()"}}},
		{"sample.rs", "pub struct Service;\nimpl Service {\n    pub async fn run(&self) {\n        if ready() { send(); }\n    }\n}\nimpl Drop for Service {\n    fn drop(&mut self) {}\n}\npub mod nested {\n    pub trait Store {\n        fn get(&self);\n    }\n}\n", []outlineHeading{{1, 1, "struct Service"}, {2, 1, "impl Service"}, {3, 2, "fn run"}, {7, 1, "impl Drop for Service"}, {8, 2, "fn drop"}, {10, 1, "mod nested"}, {11, 2, "trait Store"}, {12, 3, "fn get"}}},
		{"sample.rb", "module Example\n  class Service\n    def self.build\n      new\n    end\n    def run!\n      send_request()\n    end\n  end\nend\ndef finish\nend\n", []outlineHeading{{1, 1, "module Example"}, {2, 2, "class Service"}, {3, 3, "def self.build"}, {6, 3, "def run!"}, {11, 1, "def finish"}}},
		{"sample.py", "class Service:\n  async def run(self):\n    def nested():\n      pass\n    return nested()\n\nif enabled:\n  def conditional():\n    pass\ndef finish():\n  pass\n", []outlineHeading{{1, 1, "class Service"}, {2, 2, "def run"}, {3, 3, "def nested"}, {8, 1, "def conditional"}, {10, 1, "def finish"}}},
		{"unicode.py", "class Delta:\n\tdef cafe\u0301(self):\n\t\tpass\n", []outlineHeading{{1, 1, "class Delta"}, {2, 2, "def cafe\u0301"}}},
		{"sample.swift", "public struct Service {\n    init() {}\n    public func run(\n        value: Int\n    ) {\n        if value > 0 { send() }\n    }\n}\nextension Service {\n    func stop() {}\n}\n", []outlineHeading{{1, 1, "struct Service"}, {2, 2, "init"}, {3, 2, "func run"}, {9, 1, "extension Service"}, {10, 2, "func stop"}}},
		{"sample.kt", "data class Entry(val name: String)\nclass Service {\n    suspend fun run(value: Int) {\n        if (value > 0) send()\n    }\n    companion object {\n        fun create() = Service()\n    }\n}\nfun stop() {}\n", []outlineHeading{{1, 1, "class Entry"}, {2, 1, "class Service"}, {3, 2, "fun run"}, {6, 2, "companion object"}, {7, 3, "fun create"}, {10, 1, "fun stop"}}},
		{"extensions.kt", "fun interface Handler {\n  fun handle()\n}\nfun <T> List<T>.items() = this\nfun next() {}\n", []outlineHeading{{1, 1, "interface Handler"}, {2, 2, "fun handle"}, {4, 1, "fun List<T>.items"}, {5, 1, "fun next"}}},
		{"sample.c", "typedef unsigned int Index;\nstruct Service {\n    int value;\n};\nstatic int\nrun(\n    const int value\n)\n{\n    if (value) { send(value); }\n    return value;\n}\nvoid stop(void);\n", []outlineHeading{{1, 1, "typedef Index"}, {2, 1, "struct Service"}, {6, 1, "run()"}, {13, 1, "stop()"}}},
		{"sample.cpp", "namespace example {\nclass Service {\npublic:\n    Service() {}\n    void run() {\n        if (ready()) { send(); }\n    }\n};\nint finish() { return 0; }\n}\nvoid done() {}\n", []outlineHeading{{1, 1, "namespace example"}, {2, 2, "class Service"}, {4, 3, "Service()"}, {5, 3, "run()"}, {9, 2, "finish()"}, {11, 1, "done()"}}},
		{"Sample.java", "public class Service {\n    public Service() {}\n    @Override\n    public String run(\n        String value\n    ) {\n        if (value != null) { send(value); }\n        return value;\n    }\n    private static class Nested {\n        void stop() {}\n    }\n}\n", []outlineHeading{{1, 1, "class Service"}, {2, 2, "Service()"}, {4, 2, "run()"}, {10, 2, "class Nested"}, {11, 3, "stop()"}}},
		{"sample.zig", "pub const Service = struct {\n    pub fn run(self: *Service) void {\n        send();\n    }\n};\nconst Kind = enum { ready, done };\npub fn finish() void {}\n", []outlineHeading{{1, 1, "const Service"}, {2, 2, "fn run"}, {6, 1, "const Kind"}, {7, 1, "fn finish"}}},
		{"sample.pm", "package First;\nsub new { bless {}, shift }\nsub run {\n    send();\n}\npackage Second {\n    sub stop {}\n}\nsub finish {}\n", []outlineHeading{{1, 1, "package First"}, {2, 2, "sub new"}, {3, 2, "sub run"}, {6, 1, "package Second"}, {7, 2, "sub stop"}, {9, 2, "sub finish"}}},
		{"packages.pl", "package First;\nsub first {}\npackage Second;\nsub second {}\n", []outlineHeading{{1, 1, "package First"}, {2, 2, "sub first"}, {3, 1, "package Second"}, {4, 2, "sub second"}}},
		{"Service.h", "@protocol Forward;\n@interface Service : NSObject\n- (void)run;\n@end\nvoid finish(void);\n", []outlineHeading{{1, 1, "@protocol Forward"}, {2, 1, "@interface Service"}, {3, 2, "- run"}, {5, 1, "finish()"}}},
		{"sample.el", "(use-package example\n  :config\n  (defun example-run ()\n    (message \"ready\")))\n(cl-defstruct example-entry value)\n(define-minor-mode example-mode\n  \"Example mode.\"\n  :lighter \" Ex\")\n(defcustom example-enabled t \"Enabled.\" :type 'boolean)\n(define-key example-map (kbd \"x\") #'example-run)\n(default-value 'example-enabled)\n(cl-defun example-stop (&key force) force)\n", []outlineHeading{{1, 1, "use-package example"}, {3, 2, "defun example-run"}, {5, 1, "cl-defstruct example-entry"}, {6, 1, "define-minor-mode example-mode"}, {9, 1, "defcustom example-enabled"}, {12, 1, "cl-defun example-stop"}}},
		{"sample.lua", "local function run()\n  local function nested() end\n  nested()\nend\nfunction Service:stop()\n  send()\nend\nService.start = function() end\n", []outlineHeading{{1, 1, "function run"}, {2, 2, "function nested"}, {5, 1, "function Service:stop"}, {8, 1, "function Service.start"}}},
		{"sample.vim", "function! s:Run(value) abort\n  call Send(a:value)\nendfunction\nfu! example#Stop()\nendfunction\nexport def Start(): void\nenddef\nclass Service\n  def Stop()\n  enddef\nendclass\n", []outlineHeading{{1, 1, "function! s:Run"}, {4, 1, "fu! example#Stop"}, {6, 1, "def Start"}, {8, 1, "class Service"}, {9, 2, "def Stop"}}},
		{"sample.m", "@interface Service : NSObject\n- (void)run:(int)value\n    completion:(Handler)handler;\n+ (instancetype)shared;\n@end\n@implementation Service (Extra)\n- (void)stop {\n    [self send];\n}\n@end\n@protocol Store\n- (id)get;\n@end\nvoid finish(void) {}\n", []outlineHeading{{1, 1, "@interface Service"}, {2, 2, "- run:completion:"}, {4, 2, "+ shared"}, {6, 1, "@implementation Service (Extra)"}, {7, 2, "- stop"}, {11, 1, "@protocol Store"}, {12, 2, "- get"}, {14, 1, "finish()"}}},
		{"sample.mm", "namespace example {\nclass Worker {\n  void run() {}\n};\n}\n@implementation Service\n- (void)run {\n  example::Worker worker;\n}\n@end\n", []outlineHeading{{1, 1, "namespace example"}, {2, 2, "class Worker"}, {3, 3, "run()"}, {6, 1, "@implementation Service"}, {7, 2, "- run"}}},
		{"sample.sh", "#!/bin/sh\nrun()\n{\n    if ready; then\n        send\n    fi\n}\nfunction finish {\n    :\n}\n", []outlineHeading{{2, 1, "function run"}, {8, 1, "function finish"}}},
	} {
		t.Run(tc.path, func(t *testing.T) {
			language := codeOutlineLanguage(tc.path, tc.source)
			if got := parseCodeOutline(language, tc.source, 0); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("outline:\ngot  %+v\nwant %+v\nmasked:\n%s", got, tc.want, outlineCode(language, tc.source))
			}
		})
	}
}

func BenchmarkCodeOutline(b *testing.B) {
	for _, tc := range []struct{ language, declaration string }{
		{"typescript", "export function run(value: number) { return value; }\n"},
		{"python", "def run(value):\n  return value\n"},
		{"cpp", "int run(int value) { return value; }\n"},
	} {
		b.Run(tc.language, func(b *testing.B) {
			source := strings.Repeat(tc.declaration, 1000)
			b.ReportAllocs()
			b.SetBytes(int64(len(source)))
			for b.Loop() {
				parseCodeOutline(tc.language, source, 0)
			}
		})
	}
}

func TestCodeOutlineIgnoresCommentsAndStrings(t *testing.T) {
	for _, tc := range []struct{ path, source string }{
		{"sample.zig", "// fn hidden() void {}\nconst text =\n    \\\\fn hidden() void {}\n;\nfn visible() void {}\n"},
		{"sample.pl", "=pod\nsub hidden {}\n=cut\nmy $text = <<'TEXT';\nsub hidden {}\nTEXT\nsub visible {}\n"},
		{"sample.el", "; (defun hidden () t)\n(defvar visible \"\n(defun hidden () t)\n\")\n"},
		{"sample.lua", "--[[\nfunction hidden() end\n]]\nlocal text = [=[\nfunction hidden() end\n]=]\nfunction visible() end\n"},
		{"sample.vim", "\" function! Hidden()\nlet text = 'function! Hidden()'\nfunction! visible()\nendfunction\n"},
		{"sample.m", "/*\n@interface Hidden\n@end\n*/\nconst char *text = \"@implementation Hidden\";\nvoid visible() {}\n"},
		{"sample.ts", "/*\nclass Hidden {\n  run() {}\n}\n*/\nconst text = `\nfunction hidden() {}\n`;\nfunction visible() {}\n"},
		{"sample.rs", "/*\nfn hidden() {}\n*/\nlet text = r#\"\nfn hidden() {}\n\"#;\nfn visible() {}\n"},
		{"sample.py", "\"\"\"\nclass Hidden:\n  def hidden(): pass\n\"\"\"\n# def hidden(): pass\ndef visible(): pass\n"},
		{"sample.rb", "=begin\nclass Hidden\nend\n=end\ntext = <<~TEXT\ndef hidden\nend\nTEXT\ndef visible\nend\n"},
		{"sample.sh", "cat <<'TEXT'\nhidden() { :; }\nTEXT\n# hidden() { :; }\nvisible() { :; }\n"},
		{"sample.swift", "let text = \"\"\"\nclass Hidden {}\n\"\"\"\nfunc visible() {}\n"},
		{"sample.kt", "val text = \"\"\"\nclass Hidden {}\n\"\"\"\nfun visible() {}\n"},
		{"Sample.java", "String text = \"\"\"\nclass Hidden {}\n\"\"\";\nvoid visible() {}\n"},
		{"sample.cpp", "const char *text = R\"tag(\nclass Hidden {};\n)tag\";\n/* void hidden() {} */\nvoid visible() {}\n"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			got := parseCodeOutline(codeOutlineLanguage(tc.path, tc.source), tc.source, 0)
			if len(got) != 1 || !strings.Contains(got[0].label, "visible") {
				t.Fatalf("comments or strings leaked into outline: %+v\n%s", got, outlineCode(codeOutlineLanguage(tc.path, tc.source), tc.source))
			}
		})
	}
}

func TestCodeOutlineNavigation(t *testing.T) {
	app := outlineTestApp(t, "class Service:\n  def run(self):\n    pass\n\ndef stop():\n  pass\n")
	app.tab().path, app.tab().isMarkdown = "sample.py", false
	app.tab().chromaLines = nil
	app.rebuildContent()
	if len(app.outline.rows) != 3 {
		t.Fatalf("source outline not connected to the sidebar: %+v", app.outline.rows)
	}
	app.jumpOutline(1)
	if app.tab().cursorLine != 2 || app.outline.active != 1 {
		t.Fatal("method navigation lost the physical source line")
	}
	app.outline.focused = true
	app.handleFilesSidebarKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if app.tab().cursorLine != 1 {
		t.Fatal("parent navigation did not reach the class")
	}
	app.tab().doc = document.FromContent("sample.py", []byte("def changed(): pass\n"))
	app.rebuildContent()
	if len(app.outline.rows) != 1 || app.outline.rows[0].label != "def changed" {
		t.Fatal("reload did not replace the cached outline")
	}
}

func TestCodeOutlineCacheAndFragments(t *testing.T) {
	doc := document.FromContent("sample.py", []byte("class Missing:\n  def first():\nmissing\nmissing\ndef second():\n  pass\n"))
	doc.Known = map[int]bool{2: true, 5: true, 6: true}
	tab := FileTab{path: doc.Path, doc: doc}
	cache := tab.ensureCodeOutline()
	if want := []outlineHeading{{2, 1, "def first"}, {5, 1, "def second"}}; !reflect.DeepEqual(cache.rows, want) {
		t.Fatalf("fragment lines or hierarchy changed: %+v", cache.rows)
	}
	if tab.ensureCodeOutline() != cache {
		t.Fatal("unchanged code outline was reparsed")
	}
	tab.path = "sample.txt"
	if tab.ensureCodeOutline() != nil {
		t.Fatal("unsupported extension retained an outline")
	}
	tab.path = "sample.py"
	doc.Content, doc.Known = "def changed(): pass", nil
	if next := tab.ensureCodeOutline(); next == cache || len(next.rows) != 1 || next.rows[0].label != "def changed" {
		t.Fatal("source change retained a cached outline")
	}
	for _, path := range []string{"script.sh", "script.bash", "script.ksh", "script.zsh", ".profile", ".bashrc"} {
		if codeOutlineLanguage(path, "") != "bash" {
			t.Fatalf("shell file not recognized: %s", path)
		}
	}
	if codeOutlineLanguage("script", "#!/usr/bin/env -S bash -eu\n") != "bash" {
		t.Fatal("shell shebang not recognized")
	}
}
