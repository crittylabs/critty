// Package workspace prepares bounded, reviewable edits outside source control.
package workspace

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/gofrs/flock"
)

type File struct {
	Exists bool        `json:"exists"`
	Digest string      `json:"digest,omitempty"`
	Mode   fs.FileMode `json:"mode,omitempty"`
}

type Manifest struct {
	Version int             `json:"version"`
	ID      string          `json:"id"`
	Root    string          `json:"root"`
	Files   map[string]File `json:"files"`
}

type Snapshot struct {
	Manifest
	RequestVersion int             `json:"request_version,omitempty"`
	After          map[string]File `json:"after"`
	Diff           string          `json:"diff"`
}

type Workspace struct {
	Dir  string `json:"directory"`
	Tree string `json:"tree"`
	Manifest
}

func openRoot(path string) (*os.Root, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, err
	}
	if resolved != path {
		return nil, fmt.Errorf("directory redirected: %s", path)
	}
	return os.OpenRoot(path)
}

func identifier() string { return hex.EncodeToString(randomBytes()) }
func randomBytes() []byte {
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
func digest(b []byte) string { return fmt.Sprintf("sha256:%x", sha256.Sum256(b)) }

func validID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 12 && strings.ToLower(id) == id
}

func validPath(name string) bool {
	if !filepath.IsLocal(name) || name == "." || filepath.Clean(name) != name {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(name), "/") {
		if strings.EqualFold(part, ".git") || strings.EqualFold(part, ".critty") {
			return false
		}
	}
	return !strings.ContainsAny(name, "\x00\r\n")
}

// inspect rejects symlinks at every component, including absent destinations.
func inspect(root *os.Root, name string) (File, []byte, error) {
	if !validPath(name) {
		return File{}, nil, fmt.Errorf("invalid target: %q", name)
	}
	parts := strings.Split(filepath.ToSlash(name), "/")
	for i := range parts {
		p := filepath.FromSlash(strings.Join(parts[:i+1], "/"))
		info, err := root.Lstat(p)
		if errors.Is(err, fs.ErrNotExist) {
			return File{}, nil, nil
		}
		if err != nil {
			return File{}, nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return File{}, nil, fmt.Errorf("symlink target: %s", p)
		}
		if i < len(parts)-1 && !info.IsDir() {
			return File{}, nil, fmt.Errorf("not a directory: %s", p)
		}
		if i == len(parts)-1 && !info.Mode().IsRegular() {
			return File{}, nil, fmt.Errorf("not a regular file: %s", p)
		}
	}
	f, err := root.Open(name)
	if err != nil {
		return File{}, nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return File{}, nil, err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return File{}, nil, err
	}
	return File{true, digest(data), info.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)}, data, nil
}

func write(root *os.Root, name string, data []byte, mode fs.FileMode) error {
	if err := root.MkdirAll(filepath.Dir(name), 0700); err != nil {
		return err
	}
	f, err := root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	_, writeErr := f.Write(data)
	chmodErr := f.Chmod(mode)
	syncErr := f.Sync()
	return errors.Join(writeErr, chmodErr, syncErr, f.Close())
}

func save(path string, value any) error {
	b, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".state-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(append(b, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func readJSON(path string, value any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, value)
}

func git(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=" + os.DevNull, "-c", "commit.gpgSign=false", "-c", "core.attributesFile=" + os.DevNull, "-c", "user.name=Critty", "-c", "user.email=critty@localhost"}, args...)...)
	cmd.Dir = dir
	for _, e := range os.Environ() {
		if !strings.HasPrefix(e, "GIT_") {
			cmd.Env = append(cmd.Env, e)
		}
	}
	cmd.Env = append(cmd.Env, "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_ATTR_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, out)
	}
	return out, nil
}

func initTree(dir string) error {
	if _, err := git(dir, "init", "--quiet", "--template=", "--initial-branch=review"); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(dir, ".git", "info"), 0700); err != nil {
		return err
	}
	// Highest-precedence attributes keep the patch independent of reviewed attributes.
	return os.WriteFile(filepath.Join(dir, ".git", "info", "attributes"), []byte("* -text -crlf -ident -filter -working-tree-encoding !diff\n"), 0600)
}

func storage(cwd string) (string, error) {
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return "", err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return "", err
	}
	for _, path := range []string{filepath.Join(abs, ".critty"), filepath.Join(abs, ".critty", "tmp")} {
		if err := os.Mkdir(path, 0700); err != nil && !errors.Is(err, fs.ErrExist) {
			return "", err
		}
		st, err := os.Lstat(path)
		if err != nil {
			return "", err
		}
		if !st.IsDir() {
			return "", fmt.Errorf("not a directory: %s", path)
		}
	}
	dir := filepath.Join(abs, ".critty", "tmp")
	ignore := filepath.Join(dir, ".gitignore")
	f, err := os.OpenFile(ignore, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err == nil {
		_, e := f.WriteString("*\n")
		return dir, errors.Join(e, f.Close())
	}
	if !errors.Is(err, fs.ErrExist) {
		return "", err
	}
	info, err := os.Lstat(ignore)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", ignore)
	}
	b, err := os.ReadFile(ignore)
	if err != nil {
		return "", err
	}
	if string(b) != "*\n" {
		return "", fmt.Errorf("existing %s must contain only a * line", ignore)
	}
	return dir, nil
}

func Prepare(cwd, source string, names []string) (*Workspace, error) {
	rootPath, err := filepath.Abs(source)
	if err != nil {
		return nil, err
	}
	rootPath, err = filepath.EvalSymlinks(rootPath)
	if err != nil {
		return nil, err
	}
	root, err := openRoot(rootPath)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	files := map[string]File{}
	contents := map[string][]byte{}
	for _, name := range names {
		if _, ok := files[name]; ok {
			return nil, fmt.Errorf("duplicate target: %s", name)
		}
		f, b, err := inspect(root, name)
		if err != nil {
			return nil, err
		}
		files[name], contents[name] = f, b
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("at least one target is required")
	}
	for name := range files {
		for parent := filepath.Dir(name); parent != "."; parent = filepath.Dir(parent) {
			if _, ok := files[parent]; ok {
				return nil, fmt.Errorf("overlapping targets: %s and %s", parent, name)
			}
		}
	}
	base, err := storage(cwd)
	if err != nil {
		return nil, err
	}
	id := identifier()
	dir := filepath.Join(base, id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	w := &Workspace{Dir: dir, Tree: filepath.Join(dir, "tree"), Manifest: Manifest{Version: 1, ID: id, Root: rootPath, Files: files}}
	for _, sub := range []string{"tree", "originals", "snapshots"} {
		if err := os.Mkdir(filepath.Join(dir, sub), 0700); err != nil {
			return nil, err
		}
	}
	for _, sub := range []string{"tree", "originals"} {
		dest, err := openRoot(filepath.Join(dir, sub))
		if err != nil {
			return nil, err
		}
		for name, f := range files {
			if f.Exists {
				if err := write(dest, name, contents[name], f.Mode); err != nil {
					dest.Close()
					return nil, err
				}
			}
		}
		dest.Close()
	}
	if err := initTree(w.Tree); err != nil {
		return nil, err
	}
	if _, err := git(w.Tree, "add", "--force", "--all", "--", "."); err != nil {
		return nil, err
	}
	if _, err := git(w.Tree, "commit", "--quiet", "--allow-empty", "-m", "Record review originals"); err != nil {
		return nil, err
	}
	if err := save(filepath.Join(dir, "manifest.json"), w.Manifest); err != nil {
		return nil, err
	}
	return w, nil
}

func Load(cwd, id string) (*Workspace, error) {
	if !validID(id) {
		return nil, fmt.Errorf("invalid workspace ID")
	}
	abs, err := filepath.Abs(cwd)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(abs, ".critty", "tmp", id)
	for p := dir; p != abs; p = filepath.Dir(p) {
		st, err := os.Lstat(p)
		if err != nil {
			return nil, err
		}
		if !st.IsDir() {
			return nil, fmt.Errorf("not a directory: %s", p)
		}
	}
	w := &Workspace{Dir: dir, Tree: filepath.Join(dir, "tree")}
	if err := readJSON(filepath.Join(dir, "manifest.json"), &w.Manifest); err != nil {
		return nil, err
	}
	if w.Version != 1 || w.ID != id || !filepath.IsAbs(w.Root) || len(w.Files) == 0 {
		return nil, fmt.Errorf("invalid workspace manifest")
	}
	for name := range w.Files {
		if !validPath(name) {
			return nil, fmt.Errorf("invalid target: %s", name)
		}
	}
	return w, nil
}

func (w *Workspace) lock() (*flock.Flock, error) {
	l := flock.New(filepath.Join(w.Dir, "lock"))
	ok, err := l.TryLock()
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("workspace is busy")
	}
	return l, nil
}

func (w *Workspace) Snapshot() (string, error) {
	l, err := w.lock()
	if err != nil {
		return "", err
	}
	defer l.Unlock()
	tree, err := openRoot(w.Tree)
	if err != nil {
		return "", err
	}
	defer tree.Close()
	err = fs.WalkDir(tree.FS(), ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if name == ".git" {
			return fs.SkipDir
		}
		if d.IsDir() {
			return nil
		}
		if _, ok := w.Files[filepath.FromSlash(name)]; !ok {
			return fmt.Errorf("unregistered target: %s", name)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	s := Snapshot{Manifest: w.Manifest, RequestVersion: 1, After: map[string]File{}}
	contents := map[string][]byte{}
	for name := range w.Files {
		f, b, err := inspect(tree, name)
		if err != nil {
			return "", err
		}
		s.After[name], contents[name] = f, b
	}
	// Rebuild the patch from saved originals, independent of edits to tree/.git.
	id := identifier()
	dir := filepath.Join(w.Dir, "snapshots", id)
	if err := os.Mkdir(dir, 0700); err != nil {
		return "", err
	}
	patchTree := filepath.Join(dir, "candidate")
	if err := os.Mkdir(patchTree, 0700); err != nil {
		return "", err
	}
	dest, err := openRoot(patchTree)
	if err != nil {
		return "", err
	}
	defer dest.Close()
	originals, err := openRoot(filepath.Join(w.Dir, "originals"))
	if err != nil {
		return "", err
	}
	defer originals.Close()
	for name, before := range w.Files {
		f, b, err := inspect(originals, name)
		if err != nil {
			return "", err
		}
		if f != before {
			return "", fmt.Errorf("baseline changed: %s", name)
		}
		if f.Exists {
			if err := write(dest, name, b, f.Mode); err != nil {
				return "", err
			}
		}
	}
	if err := initTree(patchTree); err != nil {
		return "", err
	}
	if _, err := git(patchTree, "add", "--force", "--all", "--", "."); err != nil {
		return "", err
	}
	if _, err := git(patchTree, "commit", "--quiet", "--allow-empty", "-m", "Record review originals"); err != nil {
		return "", err
	}
	for name, before := range w.Files {
		if before.Exists {
			if err := dest.Remove(name); err != nil {
				return "", err
			}
		}
		after := s.After[name]
		if after.Exists {
			if err := write(dest, name, contents[name], after.Mode); err != nil {
				return "", err
			}
		}
	}
	if _, err := git(patchTree, "add", "--force", "--all", "--", "."); err != nil {
		return "", err
	}
	diff, err := git(patchTree, "diff", "--cached", "--binary", "--full-index", "--no-ext-diff", "--no-textconv", "--no-renames", "HEAD", "--")
	if err != nil {
		return "", err
	}
	s.Diff = string(diff)
	if err := save(filepath.Join(dir, "snapshot.json"), s); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "review.diff"), diff, 0600); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(dir, "APPLY.md"), []byte(s.Request()), 0600); err != nil {
		return "", err
	}
	return id, nil
}

func (s Snapshot) Request() string {
	if s.RequestVersion == 0 {
		return s.legacyRequest()
	}
	var b strings.Builder
	b.WriteString("# Apply reviewed files\n\nApply the attached changes to files in this folder:\n\n")
	fmt.Fprintf(&b, "    %s\n", strconv.Quote(s.Root))
	for _, op := range []string{"Create", "Update", "Delete"} {
		var files strings.Builder
		for _, name := range sortedNames(s.Files) {
			before, after := s.Files[name], s.After[name]
			if before == after {
				continue
			}
			kind := "Update"
			if !before.Exists {
				kind = "Create"
			} else if !after.Exists {
				kind = "Delete"
			}
			if kind != op {
				continue
			}
			fmt.Fprintf(&files, "    %s", strconv.Quote(name))
			if !before.Exists {
				fmt.Fprintf(&files, " (permissions: %s)", displayMode(after))
			} else if after.Exists && before.Mode != after.Mode {
				fmt.Fprintf(&files, " (permissions: %s -> %s)", displayMode(before), displayMode(after))
			}
			files.WriteByte('\n')
		}
		if files.Len() > 0 {
			fmt.Fprintf(&b, "\n## %s\n\n%s", op, files.String())
		}
	}
	b.WriteString("\nA rename appears as a deletion and a creation.  Files not listed above stay unchanged.\n\nBefore applying, Critty checks that the destination files still match their saved originals.\n\nNew folders use your process's default permissions (0777 filtered by umask).  Replaced files do not retain ownership, ACLs, extended attributes, or hard-link relationships.  Changes to configuration files may affect running tools.\n\n- [ ] Apply these changes to the listed destinations. <!-- form: id=apply consent -->\n\n## Machine-readable details\n\nThe JSON records the destination folder and exact before/after states, including unchanged files.  The patch digest identifies the reviewed diff.\n\n")
	b.WriteString(s.exactStates())
	fmt.Fprintf(&b, "\n\nPatch digest: %s\n", digest([]byte(s.Diff)))
	return b.String()
}

// Preserve requests attached to reviews created before request versioning.
func (s Snapshot) legacyRequest() string {
	var summary strings.Builder
	fmt.Fprintf(&summary, "    Root: %s\n", strconv.Quote(s.Root))
	for _, name := range sortedNames(s.Files) {
		before, after := s.Files[name], s.After[name]
		op := "update"
		switch {
		case before == after:
			op = "unchanged"
		case !before.Exists:
			op = "create"
		case !after.Exists:
			op = "delete"
		}
		fmt.Fprintf(&summary, "    %s %s (%s -> %s)\n", op, strconv.Quote(name), displayMode(before), displayMode(after))
	}
	return "# Apply reviewed files\n\nApply the attached diff and mode changes to these destinations.  A rename or split combines creation, updates, and deletion.\n\n" + summary.String() + "\nNew parent directories use 0777 subject to the process umask.  File replacement changes the inode; ownership, ACLs, extended attributes, and hard-link relationships are not preserved.  Configuration changes may affect running tools.\n\n## Exact states\n\n" + s.exactStates() + "\n\nPatch digest: " + digest([]byte(s.Diff)) + "\n\n- [ ] Apply these changes to the listed destinations. <!-- form: id=apply consent -->\n"
}

func (s Snapshot) exactStates() string {
	b, _ := json.MarshalIndent(struct {
		Root   string          `json:"root"`
		Before map[string]File `json:"before"`
		After  map[string]File `json:"after"`
	}{s.Root, s.Files, s.After}, "", "  ")
	// JSON is indented Markdown, so path text cannot introduce form controls.
	lines := strings.Split(string(b), "\n")
	for i := range lines {
		lines[i] = "    " + lines[i]
	}
	return strings.Join(lines, "\n")
}

func displayMode(f File) string {
	if !f.Exists {
		return "absent"
	}
	m := uint32(f.Mode.Perm())
	for bit, unix := range map[fs.FileMode]uint32{os.ModeSetuid: 04000, os.ModeSetgid: 02000, os.ModeSticky: 01000} {
		if f.Mode&bit != 0 {
			m |= unix
		}
	}
	return fmt.Sprintf("%04o", m)
}

func (w *Workspace) ReadSnapshot(id string) (Snapshot, error) {
	var s Snapshot
	if !validID(id) {
		return s, fmt.Errorf("invalid snapshot ID")
	}
	dir := filepath.Join(w.Dir, "snapshots", id)
	if err := readJSON(filepath.Join(dir, "snapshot.json"), &s); err != nil {
		return s, err
	}
	if s.RequestVersion < 0 || s.RequestVersion > 1 {
		return s, fmt.Errorf("unsupported application request version: %d", s.RequestVersion)
	}
	a, _ := json.Marshal(s.Manifest)
	b, _ := json.Marshal(w.Manifest)
	if string(a) != string(b) || len(s.After) != len(w.Files) {
		return s, fmt.Errorf("snapshot target changed")
	}
	for name := range s.After {
		if _, ok := w.Files[name]; !ok {
			return s, fmt.Errorf("snapshot target changed")
		}
	}
	return s, nil
}

func sortedNames(files map[string]File) []string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}
