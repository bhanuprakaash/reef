package space

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/store"
	"github.com/bhanuprakaash/reef/worktree"
)

var _notSlug = regexp.MustCompile(`[^a-z0-9]+`)

// WorkRoot is the one folder the agents of the Work item with tree key key work from; each
// target's workspace is a folder inside it.
func WorkRoot(dataDir, key string) string {
	return filepath.Join(dataDir, "worktrees", "item-"+key)
}

// WorkspaceDir is where the workspace of the Work item with tree key key for project p lives.
func WorkspaceDir(dataDir, key string, p store.Project) string {
	return filepath.Join(WorkRoot(dataDir, key), strconv.FormatInt(p.ID, 10)+"-"+projectSlug(p))
}

func projectSlug(p store.Project) string {
	slug := strings.Trim(_notSlug.ReplaceAllString(strings.ToLower(p.Name), "-"), "-")
	if slug == "" {
		slug = "project"
	}
	return slug
}

// EnsureWorkspaces makes the workspace of every target of item, one per repository, and returns
// their rows. Each row is recorded as `creating` before git runs, so a cut creation is found
// again and finished in the same folder, keeping its edits. A target Project being deleted or
// deleted gets none. A target whose base branch or
// repository cannot be found is `paused` with a note, and the others go on. A base that could
// only be taken from the local branch is `ready` with a note saying origin was not refreshed.
// Notes are also posted in the item's active Space. A failure that is not about the repository
// leaves the row `creating` and is returned after the other targets are done.
func EnsureWorkspaces(ctx context.Context, st *store.Store, trees *Trees, dataDir string, item store.WorkItem) ([]store.Workspace, error) {
	topID, err := strconv.ParseInt(item.TreeKey(), 10, 64)
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, t := range item.Targets {
		if !authorized(t) {
			continue
		}
		p, err := st.Project(ctx, t.ProjectID)
		if err != nil {
			return nil, err
		}
		if !p.Active() {
			continue
		}
		w, err := st.EnsureWorkspace(ctx, store.Workspace{
			ProjectID: p.ID, WorkItemID: &topID, Dir: WorkspaceDir(dataDir, item.TreeKey(), p),
			Branch: worktree.Branch(item.TreeKey()), Base: p.BaseBranch,
		})
		if err != nil {
			return nil, err
		}
		note := func(ctx context.Context, body string) error { return noteWorkspace(ctx, st, topID, body) }
		if err := ensureWorkspace(ctx, st, trees, p, w, note); err != nil {
			if ctx.Err() != nil {
				return nil, err
			}
			errs = append(errs, fmt.Errorf("workspace of %s: %w", p.Name, err))
		}
	}
	rows, err := st.WorkItemWorkspaces(ctx, topID)
	if err != nil {
		return nil, err
	}
	return rows, errors.Join(errs...)
}

// authorized reports whether the user, or an approval, gave t: a target still proposed by a Bot
// is no write permission.
func authorized(t store.Target) bool {
	return t.AuthorizedBy != "" && !strings.HasPrefix(t.AuthorizedBy, "proposed by")
}

// targetLines tells an agent where each repository of item f is, relative to its working
// folder, on which branch and from which base. A pull request you open targets that base.
func targetLines(ctx context.Context, st *store.Store, b *strings.Builder, f store.WorkItem) error {
	topID, err := strconv.ParseInt(f.TreeKey(), 10, 64)
	if err != nil {
		return err
	}
	all, err := st.WorkItemWorkspaces(ctx, topID)
	if err != nil {
		return err
	}
	var ws []store.Workspace
	for _, w := range all {
		if slices.ContainsFunc(f.Targets, func(t store.Target) bool { return t.ProjectID == w.ProjectID && authorized(t) }) {
			ws = append(ws, w)
		}
	}
	if len(ws) == 0 {
		return nil
	}
	fmt.Fprintf(b, "Your folder %s is not a repository, and changes made in it are lost. Make every change "+
		"inside one of these repository folders (a pull request targets the base branch):\n", filepath.Dir(ws[0].Dir))
	for _, w := range ws {
		p, err := st.Project(ctx, w.ProjectID)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "- %s: %s/ on branch %s", p.Name, filepath.Base(w.Dir), w.Branch)
		base := cmp.Or(w.Base, w.BaseRef)
		switch {
		case w.State == store.WorkspacePaused:
			fmt.Fprintf(b, ", paused: %s", w.Note)
		case w.BaseSource != "":
			fmt.Fprintf(b, ", base %s (%s)", base, w.BaseSource)
			if w.Note != "" {
				fmt.Fprintf(b, ": %s", w.Note)
			}
		default:
			fmt.Fprintf(b, ", base %s", base)
		}
		b.WriteString("\n")
	}
	return nil
}

// readyWorkspaces lists the workspaces of the item with tree key key that can be worked in.
func readyWorkspaces(ctx context.Context, st *store.Store, key string) ([]store.Workspace, error) {
	topID, err := strconv.ParseInt(key, 10, 64)
	if err != nil {
		return nil, err
	}
	all, err := st.WorkItemWorkspaces(ctx, topID)
	if err != nil {
		return nil, err
	}
	return activeReady(ctx, st, all)
}

// readyFixWorkspaces lists the quick-fix workspaces of conversation spaceID that can be worked in.
func readyFixWorkspaces(ctx context.Context, st *store.Store, spaceID int64) ([]store.Workspace, error) {
	all, err := st.SpaceWorkspaces(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	return activeReady(ctx, st, all)
}

// activeReady keeps the workspaces that can be worked in and whose Project is not being deleted or
// deleted: the row of a deleted Project stays as a record, but no agent writes there.
func activeReady(ctx context.Context, st *store.Store, all []store.Workspace) ([]store.Workspace, error) {
	var ready []store.Workspace
	for _, w := range all {
		if w.State != store.WorkspaceReady {
			continue
		}
		p, err := st.Project(ctx, w.ProjectID)
		if err != nil {
			return nil, err
		}
		if p.Active() {
			ready = append(ready, w)
		}
	}
	return ready, nil
}

// ensureWorkspace makes the worktree of w, recording how it went, and calls note with the
// visible line when the note of w changed.
func ensureWorkspace(
	ctx context.Context, st *store.Store, trees *Trees, p store.Project, w store.Workspace,
	note func(ctx context.Context, body string) error,
) error {
	if _, err := os.Stat(filepath.Join(w.Dir, ".git")); err == nil && w.State == store.WorkspaceReady {
		return nil
	}
	unlock, err := trees.Lock(ctx, "add "+w.Dir)
	if err != nil {
		return err
	}
	defer unlock()
	if w, err = st.Workspace(ctx, w.ID); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(w.Dir, ".git")); err == nil && w.State == store.WorkspaceReady {
		return nil
	}
	before := w.Note
	w.Base = p.BaseBranch
	b, err := worktree.Create(ctx, p.Path, w.Dir, w.Branch, p.BaseBranch)
	switch {
	case errors.Is(err, worktree.ErrNoBase) || errors.Is(err, worktree.ErrNoRepo):
		w.State, w.Note = store.WorkspacePaused, oneLine(err)
		err = nil
	case err != nil:
		w.State, w.Note = store.WorkspaceCreating, oneLine(err)
		if serr := st.SetWorkspace(ctx, w); serr != nil {
			return errors.Join(err, serr)
		}
		return err
	default:
		w.State = store.WorkspaceReady
		if b.Ref == "" && w.BaseRef == "" {
			b, _ = worktree.ResolveBase(ctx, p.Path, p.BaseBranch)
		}
		if b.Ref != "" {
			w.BaseRef, w.BaseSource, w.Note = b.Ref, b.Source, b.Note
		} else if w.BaseRef == "" {
			w.Note = ""
		}
	}
	if err := st.SetWorkspace(ctx, w); err != nil {
		return err
	}
	if w.Note != "" && w.Note != before {
		return note(ctx, fmt.Sprintf("Workspace for %s: %s", p.Name, w.Note))
	}
	return nil
}

func noteWorkspace(ctx context.Context, st *store.Store, topID int64, body string) error {
	sp, err := st.WorkItemActiveSpace(ctx, topID)
	if errors.Is(err, store.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	_, err = st.CreateMessage(ctx, store.Message{
		SpaceID: sp.ID, Author: store.ReefAuthor, Body: body,
	})
	return err
}

var _notFixName = regexp.MustCompile(`[^a-z0-9-]+`)

// FixName turns what a Bot calls a fix into the name its folder and branch carry, or "" when
// nothing usable is left.
func FixName(name string) string {
	return strings.Trim(_notFixName.ReplaceAllString(strings.ToLower(name), "-"), "-")
}

// FixDir is where the quick fix fix of Space spaceID for project p lives, inside the Space's
// own folder, so the agent of that Space, and no other, reaches it from its working folder.
func FixDir(dataDir string, spaceID int64, fix string, p store.Project) string {
	return filepath.Join(ChatDir(dataDir, spaceID), fix+"-"+projectSlug(p))
}

// EnsureFixWorkspace makes, or finds again, the workspace of the quick fix fix of the
// conversation spaceID in project p, on branch fix/<space>-<fix> from the project's base branch,
// and returns its row. The fix is the Space's own: another Space naming the same fix gets its
// own folder and branch, and never this one. A row made earlier keeps its folder, so a retry or
// a restart continues it with its edits. A base that cannot be found leaves the row `paused`
// with a note. Notes are also posted in the conversation.
func EnsureFixWorkspace(
	ctx context.Context, st *store.Store, trees *Trees, dataDir string, spaceID int64, fix string, p store.Project,
) (store.Workspace, error) {
	w, err := st.EnsureWorkspace(ctx, store.Workspace{
		ProjectID: p.ID, SpaceID: &spaceID, Name: fix, Dir: FixDir(dataDir, spaceID, fix, p),
		Branch: fmt.Sprintf("fix/%d-%s", spaceID, fix), Base: p.BaseBranch,
	})
	if err != nil {
		return w, err
	}
	note := func(ctx context.Context, body string) error {
		_, err := st.CreateMessage(ctx, store.Message{SpaceID: spaceID, Author: store.ReefAuthor, Body: body})
		return err
	}
	if err := ensureWorkspace(ctx, st, trees, p, w, note); err != nil {
		return w, err
	}
	rows, err := st.SpaceWorkspaces(ctx, spaceID)
	if err != nil {
		return w, err
	}
	for _, r := range rows {
		if r.ID == w.ID {
			return r, nil
		}
	}
	return w, nil
}

// fixLines tells the agent of a conversation which fixes it holds and how to choose between
// them. The tool enforces the ambiguous case.
func fixLines(ctx context.Context, st *store.Store, b *strings.Builder, spaceID int64) error {
	ws, err := st.SpaceWorkspaces(ctx, spaceID)
	if err != nil {
		return err
	}
	var open []store.Workspace
	for _, w := range ws {
		if w.State != store.WorkspaceRemoved {
			open = append(open, w)
		}
	}
	if len(open) == 0 {
		return nil
	}
	b.WriteString("Fixes of this conversation (open_workspace), each in its own folder under your working " +
		"folder. A follow-up to a fix goes in the same fix (pass its name); clearly independent work " +
		"gets a new fix name; when it is unclear, ask the user whether to continue a fix or start a new one. " +
		"One fix across Projects takes the same fix name in each. Finish a fix by verifying it, committing " +
		"it in its folder and calling open_repo_request with its workspace id: that pushes the branch and " +
		"opens or links the pull or merge request, and calling it again after a follow-up commit updates the " +
		"same request. When a request must merge after others of the fix, pass their request ids as " +
		"dependsOn. Never merge: the user merges.\n")
	for _, w := range open {
		p, err := st.Project(ctx, w.ProjectID)
		if err != nil {
			return err
		}
		fmt.Fprintf(b, "- %s in %s: %s/ on branch %s (%s), workspace %d", w.Name, p.Name, filepath.Base(w.Dir), w.Branch, w.State, w.ID)
		if w.Note != "" {
			fmt.Fprintf(b, ": %s", w.Note)
		}
		b.WriteString("\n")
		reqs, err := st.WorkspaceRequests(ctx, w.ID)
		if err != nil {
			return err
		}
		for _, r := range reqs {
			fmt.Fprintf(b, "  - request %d: %s (%s)", r.ID, r.URL, r.State)
			if len(r.DependsOn) > 0 {
				fmt.Fprintf(b, ", merges after request %v", r.DependsOn)
			}
			b.WriteString("\n")
		}
	}
	return nil
}

// requireReadyWorkspace is nil when any of ws can be worked in: a target that failed is noted
// and the others run. Otherwise it names each repository and why it waits. err is what
// EnsureWorkspaces returned.
func requireReadyWorkspace(ctx context.Context, st *store.Store, ws []store.Workspace, err error) error {
	if slices.ContainsFunc(ws, func(w store.Workspace) bool { return w.State == store.WorkspaceReady }) {
		return nil
	}
	if len(ws) == 0 {
		return cmp.Or(err, errors.New("no repository of this Work item is ready"))
	}
	if len(ws) == 1 && ws[0].Note != "" {
		return errors.New(ws[0].Note)
	}
	var lines []string
	for _, w := range ws {
		p, perr := st.Project(ctx, w.ProjectID)
		if perr != nil {
			return perr
		}
		lines = append(lines, fmt.Sprintf("%s: %s", p.Name, cmp.Or(w.Note, "not ready")))
	}
	return fmt.Errorf("no repository of this Work item is ready: %s", strings.Join(lines, "; "))
}

func oneLine(err error) string {
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}
