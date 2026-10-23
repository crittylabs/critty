package history

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode"

	"github.com/gofrs/flock"
	"github.com/yuin/goldmark/v2/ast"
	"github.com/yuin/goldmark/v2/parser"
	nethtml "golang.org/x/net/html"

	"github.com/crittylabs/critty/internal/form"
	"github.com/crittylabs/critty/internal/ipc"
	"github.com/crittylabs/critty/internal/review"
	"github.com/crittylabs/critty/internal/xdg"
)

var ErrBusy = errors.New("review is active or busy")

type snapshotOptions struct {
	AllowMissing bool
	MetadataOnly bool
}

func withSessions(task Task, options snapshotOptions, fn func([]*review.Session, []string) error) (err error) {
	var locks []*flock.Flock
	defer func() {
		for _, lock := range slices.Backward(locks) {
			err = errors.Join(err, lock.Unlock())
		}
	}()
	sessions := make([]*review.Session, 0, len(task.Sessions))
	var missing []string
	for _, id := range task.Sessions {
		if !review.ValidSessionKey(id) {
			return fmt.Errorf("invalid session ID %q", id)
		}
		var pending *review.SessionEntry
		if task.Pruning {
			for _, entry := range task.Pending {
				if entry.Key == id {
					copy := entry
					pending = &copy
					break
				}
			}
			if pending == nil {
				continue
			}
		}
		entry, err := review.ReadSessionEntry(id)
		if err != nil && pending != nil {
			entry, err = pending, nil
		}
		if err != nil {
			if _, statErr := os.Stat(filepath.Join(review.SessionsDir(), id+".json")); os.IsNotExist(statErr) && options.AllowMissing {
				missing = append(missing, id)
				continue
			}
			return err
		}
		if entry.CWD != task.CWD || entry.Key != id || filepath.Base(entry.ReviewPath) != "review.json" {
			return fmt.Errorf("review %s has inconsistent metadata", id)
		}
		if !filepath.IsAbs(entry.ReviewPath) {
			entry.ReviewPath = filepath.Join(entry.CWD, entry.ReviewPath)
		}
		dir := filepath.Dir(entry.ReviewPath)
		if relative, err := filepath.Rel(dir, task.CWD); err == nil && (relative == "." || filepath.IsLocal(relative)) {
			return fmt.Errorf("review directory contains the task working directory: %s", dir)
		}
		if member, err := readMembership(entry); err != nil {
			return err
		} else if member != "" && member != task.ID {
			return fmt.Errorf("review %s belongs to another task", id)
		}
		if ipc.Alive(review.SocketPathFor(id)) {
			return fmt.Errorf("%w: review %s; stop it before organizing", ErrBusy, id)
		}
		if options.AllowMissing {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				missing = append(missing, id)
				if options.MetadataOnly {
					sessions = append(sessions, &review.Session{Key: id, Dir: dir, Meta: *entry})
				}
				continue
			}
		}
		if task.Pruning {
			if _, err := os.Stat(dir); os.IsNotExist(err) {
				sessions = append(sessions, &review.Session{Key: id, Dir: dir, Meta: *entry})
				continue
			}
		}
		if info, err := os.Lstat(dir); err != nil || !info.IsDir() {
			return fmt.Errorf("review %s requires an existing directory without a symlink", id)
		}
		if ipc.Alive(review.SocketPathFor(id)) {
			return fmt.Errorf("%w: review %s; stop it before organizing", ErrBusy, id)
		}
		for _, path := range []string{filepath.Join(dir, "run.lock"), entry.ReviewPath + ".lock"} {
			lock := flock.New(path)
			locked, err := lock.TryLock()
			if err != nil {
				return err
			}
			if !locked {
				return fmt.Errorf("%w: review %s; retry after it stops", ErrBusy, id)
			}
			locks = append(locks, lock)
		}
		if _, err := os.Stat(entry.ReviewPath); err != nil {
			if os.IsNotExist(err) && options.AllowMissing {
				missing = append(missing, id)
				if options.MetadataOnly {
					sessions = append(sessions, &review.Session{Key: id, Dir: dir, Meta: *entry})
				}
				continue
			}
			return err
		}
		if options.MetadataOnly {
			sessions = append(sessions, &review.Session{Key: id, Dir: dir, Meta: *entry})
			continue
		}
		session, err := review.OpenSessionFromEntry(*entry)
		if err != nil {
			return err
		}
		sessions = append(sessions, session)
	}
	return fn(sessions, missing)
}

type archiveOptions struct {
	AllowIncomplete bool
	Missing         []string
}

type manifest struct {
	Version  int                     `json:"version"`
	SavedAt  string                  `json:"saved_at"`
	Task     Task                    `json:"task"`
	Sessions []review.SessionEntry   `json:"sessions"`
	Files    map[string]string       `json:"files"`
	Warnings []string                `json:"warnings,omitempty"`
	Assets   map[string]assetCapture `json:"assets,omitempty"`
}

type assetCapture struct {
	Source     string `json:"source"`
	Origin     string `json:"origin"`
	CapturedAt string `json:"captured_at"`
}

type archiveWriter struct {
	dir             string
	files           map[string]string
	links           map[string]string
	allowIncomplete bool
	warnings        []string
	assets          map[string]assetCapture
	savedAt         string
}

func (w *archiveWriter) write(path string, data []byte) error {
	if !filepath.IsLocal(path) {
		return fmt.Errorf("invalid archive path %q", path)
	}
	full := filepath.Join(w.dir, path)
	hash := fmt.Sprintf("%x", sha256.Sum256(data))
	if previous, exists := w.files[filepath.ToSlash(path)]; exists && previous != hash {
		return fmt.Errorf("archive path collision: %s", path)
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
		return err
	}
	if err := os.WriteFile(full, data, 0o600); err != nil {
		return err
	}
	w.files[filepath.ToSlash(path)] = hash
	return nil
}

func archive(task Task, sessions []*review.Session, root string, now time.Time, options archiveOptions) (path string, err error) {
	if root == "" {
		root = filepath.Join(xdg.StateHome(), "archives")
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return "", err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	for _, session := range sessions {
		dir, err := filepath.EvalSymlinks(session.Dir)
		if err != nil {
			return "", err
		}
		if rel, err := filepath.Rel(dir, root); err == nil && (rel == "." || filepath.IsLocal(rel)) {
			return "", fmt.Errorf("archive destination is inside review %s", session.Key)
		}
	}
	stage, err := os.MkdirTemp(root, ".saving-")
	if err != nil {
		return "", err
	}
	defer func() {
		if cleanupErr := os.RemoveAll(stage); cleanupErr != nil {
			err = errors.Join(err, cleanupErr)
		}
	}()
	w := &archiveWriter{dir: stage, files: map[string]string{}, allowIncomplete: options.AllowIncomplete, assets: map[string]assetCapture{}, savedAt: now.Format(time.RFC3339Nano)}
	for _, dir := range []string{"attachments", "assets", "sessions"} {
		if err := os.Mkdir(filepath.Join(stage, dir), 0o700); err != nil {
			return "", err
		}
	}
	task.Missing = options.Missing
	m := manifest{Version: 1, SavedAt: now.Format(time.RFC3339Nano), Task: task, Files: w.files, Assets: w.assets, Sessions: []review.SessionEntry{}}
	var main strings.Builder
	fmt.Fprintf(&main, "# %s\n\nProject: %s  \nTask: %s  \nStarted: %s  \nSaved: %s  \nDirectory: %s\n\n", markdownText(task.Name), markdownText(task.Project), task.ID, task.StartedAt, m.SavedAt, markdownText(task.CWD))
	main.WriteString("This archive contains saved review state, not a transcript of every edit or independent proof of authorization.  Original session files are under `sessions/`; `manifest.json` records their SHA-256 hashes.  Local images are captured at export and may differ from the reviewed revision; the manifest records their provenance.\n\n")
	for _, id := range options.Missing {
		w.warnings = append(w.warnings, "Missing review session: "+id)
	}
	for _, session := range sessions {
		m.Sessions = append(m.Sessions, session.Meta)
		if err := w.session(&main, session); err != nil {
			return "", fmt.Errorf("archiving review %s: %w", session.Key, err)
		}
	}
	m.Warnings = w.warnings
	if len(m.Warnings) > 0 {
		main.WriteString("## Incomplete archive\n\n")
		for _, warning := range m.Warnings {
			main.WriteString("- " + markdownText(warning) + "\n")
		}
		main.WriteByte('\n')
	}
	if err := w.write("review.md", []byte(main.String())); err != nil {
		return "", err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(stage, "manifest.json"), append(data, '\n'), 0o600); err != nil {
		return "", err
	}
	if err := Verify(stage); err != nil {
		return "", err
	}
	name := now.Format("2006-01-02 150405 - ") + safeName(task.Project) + " - " + safeName(task.Name) + " - " + task.ID + "-" + strings.TrimPrefix(filepath.Base(stage), ".saving-")
	path = filepath.Join(root, name)
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		return "", fmt.Errorf("archive destination already exists or is unreadable")
	}
	if err := os.Rename(stage, path); err != nil {
		return "", err
	}
	return path, nil
}

// Verify checks every recorded file before an archive can replace saved data.
func Verify(dir string) error {
	data, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var m manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	if m.Version != 1 || len(m.Files) == 0 {
		return fmt.Errorf("invalid history archive manifest")
	}
	for path, hash := range m.Files {
		if !filepath.IsLocal(path) {
			return fmt.Errorf("invalid manifest path %q", path)
		}
		full := filepath.Join(dir, filepath.FromSlash(path))
		resolved, err := filepath.EvalSymlinks(full)
		if err != nil {
			return err
		}
		root, err := filepath.EvalSymlinks(dir)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || !filepath.IsLocal(rel) {
			return fmt.Errorf("archive file escapes its directory: %s", path)
		}
		data, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != hash {
			return fmt.Errorf("archive checksum mismatch: %s", path)
		}
	}
	return nil
}

func safeName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || strings.ContainsRune(" .-_", r) {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
		if b.Len() >= 80 {
			break
		}
	}
	name = strings.Trim(b.String(), " .-_")
	if name == "" {
		return "review"
	}
	return name
}

func markdownText(text string) string {
	text = strings.Join(strings.Fields(text), " ")
	return strings.NewReplacer("&", "&amp;", "\\", "\\\\", "`", "\\`", "*", "\\*", "_", "\\_", "[", "\\[", "]", "\\]", "<", "&lt;", ">", "&gt;", "#", "\\#", "!", "\\!").Replace(text)
}

func fenced(text string) string {
	fence := "```"
	for strings.Contains(text, fence) {
		fence += "`"
	}
	return fence + "\n" + text + "\n" + fence + "\n\n"
}

func archiveLink(label, path string) string {
	return "[" + markdownText(label) + "](<" + (&url.URL{Path: filepath.ToSlash(path)}).String() + ">)"
}

func imageFile(path string) bool {
	return slices.Contains([]string{".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg"}, strings.ToLower(filepath.Ext(path)))
}

func readImage(path string) ([]byte, error) {
	if !imageFile(path) {
		return nil, fmt.Errorf("unsupported local image: %s", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("image is not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if slices.Contains([]string{"image/png", "image/jpeg", "image/gif", "image/webp"}, http.DetectContentType(data)) {
		return data, nil
	}
	if strings.EqualFold(filepath.Ext(path), ".svg") {
		decoder := xml.NewDecoder(bytes.NewReader(data))
		for {
			token, err := decoder.Token()
			if err != nil {
				break
			}
			if element, ok := token.(xml.StartElement); ok {
				if element.Name.Local == "svg" {
					return data, nil
				}
				break
			}
		}
	}
	return nil, fmt.Errorf("unsupported image contents: %s", path)
}

func (w *archiveWriter) session(main *strings.Builder, session *review.Session) error {
	docs, err := session.LoadDocuments()
	if err != nil {
		return err
	}
	w.links = map[string]string{}
	documentPaths := map[string]bool{}
	for _, doc := range docs.Documents {
		target := filepath.Join("attachments", session.Key, safeName(doc.ID), safeName(filepath.Base(doc.Source)))
		w.links[doc.Source] = target
		w.links[filepath.Join(session.Dir, filepath.FromSlash(doc.Path()))] = target
		documentPaths[doc.Path()] = true
	}
	for path := range session.CJ.RoundState.Files {
		if documentPaths[path] {
			continue
		}
		source := path
		if !filepath.IsAbs(source) {
			source = filepath.Join(session.Meta.WorkingDirectory(), source)
		}
		if _, attached := w.links[source]; !attached {
			w.links[source] = snapshotTarget(session.Key, path)
		}
	}
	raw := filepath.Join("sessions", session.Key)
	if err := filepath.WalkDir(session.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if path == filepath.Join(session.Dir, "run.lock") || path == session.Path()+".lock" {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("unsupported session file: %s", path)
		}
		rel, err := filepath.Rel(session.Dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return w.write(filepath.Join(raw, rel), data)
	}); err != nil {
		return err
	}
	label := session.Key
	if session.Meta.Topic != nil && *session.Meta.Topic != "" {
		label = *session.Meta.Topic
	}
	fmt.Fprintf(main, "## %s\n\nSession: %s  \nRound: %d  \nVerdict: %s\n\n%s\n\n", markdownText(label), session.Key, session.CJ.ReviewRound, session.CJ.RoundState.Verdict, archiveLink("Original review JSON", filepath.Join(raw, "review.json")))
	for _, comment := range session.CJ.ReviewComments {
		if err := w.thread(main, session, "Overall", comment); err != nil {
			return err
		}
	}
	if draft := session.CJ.RoundState.ReviewCommentDraft; draft != "" {
		main.WriteString("### Unsubmitted overall draft\n\n" + fenced(draft))
	}
	for _, path := range session.SortedFiles() {
		for _, comment := range session.CJ.Files[path].Comments {
			if err := w.thread(main, session, path, comment); err != nil {
				return err
			}
		}
	}
	forms, err := session.LoadForms()
	if err != nil {
		return err
	}
	for _, doc := range docs.Documents {
		target := w.links[doc.Source]
		body := doc.Content
		if ext := strings.ToLower(filepath.Ext(doc.Source)); ext == ".md" || ext == ".markdown" {
			body, err = w.markdown(body, filepath.Dir(doc.Source), filepath.Dir(target))
			if err != nil {
				return err
			}
		}
		if err := w.write(target, []byte(body)); err != nil {
			return err
		}
		fmt.Fprintf(main, "Attachment: %s  \nSource: %s  \nRevision: %s  \nRemoved: %t\n\n", archiveLink(doc.Name, target), markdownText(doc.Source), doc.Revision, doc.Removed)
		if d := forms.Document(doc.Path()); d != nil && !doc.Removed {
			if err := w.answers(main, session, d); err != nil {
				return err
			}
		}
	}
	paths := make([]string, 0, len(session.CJ.RoundState.Files))
	for path := range session.CJ.RoundState.Files {
		paths = append(paths, path)
	}
	slices.Sort(paths)
	for _, path := range paths {
		if documentPaths[path] {
			continue
		}
		snapshot := session.CJ.RoundState.Files[path]
		target := snapshotTarget(session.Key, path)
		body := snapshot.Content
		if ext := strings.ToLower(filepath.Ext(path)); ext == ".md" || ext == ".markdown" {
			base := filepath.Dir(path)
			if !filepath.IsAbs(base) {
				base = filepath.Join(session.Meta.WorkingDirectory(), base)
			}
			body, err = w.markdown(body, base, filepath.Dir(target))
			if err != nil {
				return err
			}
		}
		if err := w.write(target, []byte(body)); err != nil {
			return err
		}
		fmt.Fprintf(main, "Snapshot: %s (partial: %t)\n\n", archiveLink(path, target), snapshot.Partial)
	}
	return nil
}

// answers lists the saved form answers of an attachment once per field.
// Text is Markdown, so its images are captured like those in comments.
func (w *archiveWriter) answers(main *strings.Builder, session *review.Session, d *review.FormDocument) error {
	var fields []form.Item
	for _, item := range d.Items {
		if item.SelectAll == nil && !slices.ContainsFunc(fields, func(field form.Item) bool { return field.ID == item.ID }) {
			fields = append(fields, item)
		}
	}
	if len(fields) == 0 {
		return nil
	}
	main.WriteString("Form answers:\n\n")
	for _, field := range fields {
		answer := d.Answers[field.ID]
		state, label, note := "☐", field.Label, ""
		switch {
		case field.Rank:
			state, label = "○", field.ID
			if order, ranked := form.RankOrder(d.Items, field.ID, answer); ranked {
				state = "✓"
				var labels []string
				for i, value := range order {
					for _, item := range d.Items {
						if item.ID == field.ID && item.Value == value {
							labels = append(labels, fmt.Sprintf("%d. %s", i+1, item.Label))
						}
					}
				}
				label = field.ID + ": " + strings.Join(labels, ", ")
			}
		case field.Choice:
			state = "○"
			for _, item := range d.Items {
				if item.ID == field.ID && form.Selected(item, answer) {
					state, label = "●", item.Label
					if item.Text {
						note = answer.Text
					}
				}
			}
		case field.Text:
			if form.Selected(field, answer) {
				state = "✎"
				note, _ = answer.Value.(string)
			}
		case form.Selected(field, answer):
			state = "✓"
		}
		fmt.Fprintf(main, "- %s %s (`%s`)", state, markdownText(label), strings.ReplaceAll(field.ID, "`", ""))
		if answer.Human {
			fmt.Fprintf(main, " — %s, round %d", markdownText(answer.Author), answer.Round)
			if answer.Via != "" {
				fmt.Fprintf(main, " via `%s`", strings.ReplaceAll(answer.Via, "`", ""))
			}
		} else if state != "☐" && state != "○" {
			main.WriteString(" — initial value")
		}
		main.WriteString("\n")
		if note != "" {
			body, err := w.rewriteMarkdown(note, session.Meta.WorkingDirectory(), ".", session.Dir)
			if err != nil {
				return err
			}
			main.WriteString("\n  > " + strings.ReplaceAll(body, "\n", "\n  > ") + "\n\n")
		}
	}
	main.WriteString("\n")
	return nil
}

func snapshotTarget(sessionID, path string) string {
	hash := sha256.Sum256([]byte(path))
	return filepath.Join("attachments", sessionID, fmt.Sprintf("source-%x", hash[:6]), safeName(filepath.Base(path)))
}

func (w *archiveWriter) thread(main *strings.Builder, session *review.Session, path string, comment review.Comment) error {
	fmt.Fprintf(main, "### %s\n\nComment: %s  \n", markdownText(path), markdownText(comment.ID))
	if comment.StartLine > 0 {
		fmt.Fprintf(main, "Lines: %d–%d (%s)  \n", comment.StartLine, comment.EndLine, markdownText(comment.Side))
	}
	fmt.Fprintf(main, "Author: %s  \nCreated: %s  \nResolved: %t\n\n", markdownText(comment.Author), markdownText(comment.CreatedAt), comment.Resolved)
	if comment.Anchor != "" {
		main.WriteString(fenced(comment.Anchor))
	}
	write := func(body string) error {
		body, err := w.rewriteMarkdown(body, session.Meta.WorkingDirectory(), ".", session.Dir)
		if err != nil {
			return err
		}
		main.WriteString("> " + strings.ReplaceAll(body, "\n", "\n> ") + "\n\n")
		return nil
	}
	if err := write(comment.Body); err != nil {
		return err
	}
	for _, reply := range comment.Replies {
		fmt.Fprintf(main, "Reply by %s (%s):\n\n", markdownText(reply.Author), markdownText(reply.CreatedAt))
		if err := write(reply.Body); err != nil {
			return err
		}
	}
	return nil
}

// markdown rewrites parsed image destinations, including reference-style
// images, while leaving fenced code and the rest of the source unchanged.
func (w *archiveWriter) markdown(source, base, destinationDir string) (string, error) {
	return w.rewriteMarkdown(source, base, destinationDir, "")
}

func (w *archiveWriter) rewriteMarkdown(source, base, destinationDir, sessionDir string) (string, error) {
	data := []byte(source)
	root := parser.New().Parse(data)
	type edit struct {
		start, end int
		value      string
	}
	var edits []edit
	asset := func(value string) (string, error) {
		unavailable := func(err error, managed bool) (string, error) {
			if w.allowIncomplete && !managed {
				w.warnings = append(w.warnings, fmt.Sprintf("Image %q was not captured: %v", value, err))
				return value, nil
			}
			return "", err
		}
		u, err := url.Parse(value)
		if err != nil {
			return unavailable(err, sessionDir != "" && strings.HasPrefix(value, "attachments/"))
		}
		if u.Scheme != "" || u.Host != "" || u.Path == "" {
			return value, nil
		}
		path := filepath.FromSlash(u.Path)
		if !filepath.IsAbs(path) {
			imageBase := base
			if sessionDir != "" && strings.HasPrefix(u.Path, "attachments/") {
				imageBase = sessionDir
			}
			path = filepath.Join(imageBase, path)
		}
		managed := false
		if sessionDir != "" {
			rel, err := filepath.Rel(sessionDir, path)
			managed = err == nil && filepath.IsLocal(rel)
		}
		content, err := readImage(path)
		if err != nil {
			return unavailable(fmt.Errorf("reading referenced image %s: %w", path, err), managed)
		}
		hash := sha256.Sum256(content)
		target := filepath.Join(destinationDir, "assets", fmt.Sprintf("%x", hash), safeName(filepath.Base(path)))
		if err := w.write(target, content); err != nil {
			return "", err
		}
		origin := "local file at export"
		if managed {
			origin = "session attachment"
		}
		w.assets[filepath.ToSlash(target)] = assetCapture{Source: path, Origin: origin, CapturedAt: w.savedAt}
		rel, err := filepath.Rel(destinationDir, target)
		if err != nil {
			return "", err
		}
		u.Path, u.RawPath = filepath.ToSlash(rel), ""
		return u.String(), nil
	}
	readHTML := func(start, end int) error {
		z := nethtml.NewTokenizer(strings.NewReader(source[start:end]))
		offset := start
		for {
			kind := z.Next()
			if kind == nethtml.ErrorToken {
				return nil
			}
			raw := z.Raw()
			if kind == nethtml.StartTagToken || kind == nethtml.SelfClosingTagToken {
				token := z.Token()
				if token.Data == "img" {
					changed := false
					for i := range token.Attr {
						if token.Attr[i].Key != "src" {
							continue
						}
						value, err := asset(token.Attr[i].Val)
						if err != nil {
							return err
						}
						changed = changed || value != token.Attr[i].Val
						token.Attr[i].Val = value
					}
					if changed {
						edits = append(edits, edit{offset, offset + len(raw), token.String()})
					}
				}
			}
			offset += len(raw)
		}
	}
	err := ast.Walk(root, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Image:
			value := node.Destination.Str(data)
			next, err := asset(value)
			if err != nil {
				return ast.WalkStop, err
			}
			if next == value {
				return ast.WalkContinue, nil
			}
			indices := node.Destination.Indices()
			if len(indices) != 1 {
				return ast.WalkStop, fmt.Errorf("unsupported image destination %q", value)
			}
			edits = append(edits, edit{indices[0].Start, indices[0].Stop, next})
		case *ast.Link:
			value := node.Destination.Str(data)
			u, err := url.Parse(value)
			if err != nil {
				return ast.WalkContinue, nil
			}
			next := value
			path := filepath.FromSlash(u.Path)
			if !filepath.IsAbs(path) {
				linkBase := base
				if sessionDir != "" && strings.HasPrefix(u.Path, "attachments/") {
					linkBase = sessionDir
				}
				path = filepath.Join(linkBase, path)
			}
			if target, found := w.links[path]; found && u.Scheme == "" && u.Host == "" && u.Path != "" {
				rel, err := filepath.Rel(destinationDir, target)
				if err != nil {
					return ast.WalkStop, err
				}
				u.Path, u.RawPath = filepath.ToSlash(rel), ""
				next = u.String()
			} else if imageFile(u.Path) {
				next, err = asset(value)
			}
			if err != nil {
				return ast.WalkStop, err
			}
			if next == value {
				return ast.WalkContinue, nil
			}
			indices := node.Destination.Indices()
			if len(indices) != 1 {
				return ast.WalkStop, fmt.Errorf("unsupported image link destination %q", value)
			}
			edits = append(edits, edit{indices[0].Start, indices[0].Stop, next})
		case *ast.RawHTML:
			segments := node.Value.Indices()
			if len(segments) > 0 {
				if err := readHTML(segments[0].Start, segments[len(segments)-1].Stop); err != nil {
					return ast.WalkStop, err
				}
			}
		case *ast.HTMLBlock:
			segments := node.Value.Segments()
			if len(segments) > 0 {
				if err := readHTML(segments[0].Start, segments[len(segments)-1].Stop); err != nil {
					return ast.WalkStop, err
				}
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	slices.SortFunc(edits, func(a, b edit) int { return b.start - a.start })
	last := len(data)
	for _, edit := range edits {
		if edit.end > last {
			continue
		} // Shared reference definition.
		source = source[:edit.start] + edit.value + source[edit.end:]
		last = edit.start
	}
	return source, nil
}
