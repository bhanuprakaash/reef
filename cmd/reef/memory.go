package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_memoryUsage = `usage: reef memory [--project P] [show|set FILE|-] [--data DIR]
       reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W]
       reef memory recall "<q>" [--scope S] [--limit N]
       reef memory show <id> | retire <id> | approve <id> | pending`
	_memoryShowUsage    = "usage: reef memory [--project P] show [<id>] [--data DIR]"
	_memorySetUsage     = "usage: reef memory [--project P] set FILE|- [--data DIR]"
	_memoryAddUsage     = `usage: reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W] [--data DIR]`
	_memoryRecallUsage  = `usage: reef memory recall "<q>" [--scope S] [--limit N] [--data DIR]`
	_memoryRetireUsage  = "usage: reef memory retire <id> [--data DIR]"
	_memoryPendingUsage = "usage: reef memory pending [--data DIR]"
	_memoryApproveUsage = "usage: reef memory approve <id> [--data DIR]"
)

// listFlag is a flag that may repeat; each use adds one value.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

// memoryCmd parses its flags and --data before looking for the subcommand word, so they can
// come before or after it; bare `reef memory` means show.
func memoryCmd(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	c := newCommand("reef memory", _memoryUsage, 0, 3)
	project := c.fs.String("project", "", "")
	scope := c.fs.String("scope", "", "")
	role := c.fs.String("as", "", "")
	why := c.fs.String("why", "", "")
	limit := c.fs.Int("limit", 0, "")
	var about, replaces listFlag
	c.fs.Var(&about, "about", "")
	c.fs.Var(&replaces, "replaces", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	sub, rest := "show", pos
	if len(pos) > 0 {
		sub, rest = pos[0], pos[1:]
	}
	switch sub {
	case "show":
		switch len(rest) {
		case 0:
			return memoryShow(ctx, st, *project, stdout)
		case 1:
			return memoryItem(ctx, st, rest[0], stdout)
		}
		return errors.New(_memoryShowUsage)
	case "set":
		if len(rest) != 1 {
			return errors.New(_memorySetUsage)
		}
		return memorySet(ctx, st, *project, rest[0], stdin, stdout)
	case "add":
		if len(rest) != 2 || *scope == "" {
			return errors.New(_memoryAddUsage)
		}
		return memoryAdd(ctx, st, rest[0], rest[1], *scope, *role, *why, about, replaces, stdout)
	case "recall":
		if len(rest) != 1 {
			return errors.New(_memoryRecallUsage)
		}
		return memoryRecall(ctx, st, rest[0], *scope, *limit, stdout)
	case "retire":
		if len(rest) != 1 {
			return errors.New(_memoryRetireUsage)
		}
		return memoryRetire(ctx, st, rest[0], stdout)
	case "approve":
		if len(rest) != 1 {
			return errors.New(_memoryApproveUsage)
		}
		return memoryApprove(ctx, st, rest[0], stdout)
	case "pending":
		if len(rest) != 0 {
			return errors.New(_memoryPendingUsage)
		}
		return memoryPending(ctx, st, stdout)
	}
	return errors.New(_memoryUsage)
}

func memoryShow(ctx context.Context, st *store.Store, project string, stdout io.Writer) error {
	p, err := memoryProject(ctx, st, project)
	if err != nil {
		return err
	}
	m, err := st.ProjectMemory(ctx, p.ID)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(stdout, m.Body)
	return err
}

func memorySet(ctx context.Context, st *store.Store, project, file string, stdin io.Reader, stdout io.Writer) error {
	p, err := memoryProject(ctx, st, project)
	if err != nil {
		return err
	}
	var body []byte
	if file == "-" {
		body, err = io.ReadAll(stdin)
	} else {
		body, err = os.ReadFile(file)
	}
	if err != nil {
		return fmt.Errorf("read memory: %w", err)
	}
	if _, err := st.PutProjectMemory(ctx, store.ProjectMemory{ProjectID: p.ID, Body: string(body)}); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "saved memory for project %s\n", p.Name)
	return err
}

// memoryProject is the project named by ref (its id), or the project of the current folder
// when ref is empty.
func memoryProject(ctx context.Context, st *store.Store, ref string) (store.Project, error) {
	if ref == "" {
		return cwdProject(ctx, st)
	}
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		return store.Project{}, fmt.Errorf("%w: --project %q is not a project id", store.ErrInvalid, ref)
	}
	return st.Project(ctx, id)
}

func memoryAdd(ctx context.Context, st *store.Store, kind, body, scope, role, why string,
	about, replaces []string, stdout io.Writer) error {
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		return err
	}
	w := memory.Writer{Author: _cliAuthor, Role: role}
	if w.Scope, err = parseMemoryRef("--scope", scope); err != nil {
		return err
	}
	n := memory.Note{Kind: kind, Body: body, Why: why}
	for _, a := range about {
		r, err := parseMemoryRef("--about", a)
		if err != nil {
			return err
		}
		n.About = append(n.About, r)
	}
	for _, r := range replaces {
		id, err := parseMemoryID(r)
		if err != nil {
			return err
		}
		n.Replaces = append(n.Replaces, id)
	}
	res, err := ms.Remember(ctx, w, n)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(stdout, memoryRow(res.Item)); err != nil {
		return err
	}
	for _, s := range res.Similar {
		if _, err := fmt.Fprintf(stdout, "%d  similar  %s\n", s.ID, s.Body); err != nil {
			return err
		}
	}
	return nil
}

func memoryRecall(ctx context.Context, st *store.Store, text, scope string, limit int, stdout io.Writer) error {
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		return err
	}
	q := memory.Query{Text: text, Limit: limit}
	if scope != "" {
		r, err := parseMemoryRef("--scope", scope)
		if err != nil {
			return err
		}
		q.Scope = &r
	}
	if q.Scopes, err = ms.Scopes(ctx); err != nil {
		return err
	}
	hits, err := ms.Recall(ctx, q)
	if err != nil {
		return err
	}
	for _, h := range hits {
		if _, err := fmt.Fprintf(stdout, "%d  %s  %s  %s:%s  %s  %s\n",
			h.ID, h.Status, h.Kind, h.Scope.Kind, h.Scope.ID, h.Via, h.Body); err != nil {
			return err
		}
	}
	return nil
}

func memoryItem(ctx context.Context, st *store.Store, ref string, stdout io.Writer) error {
	ms, id, err := memoryByRef(st, ref)
	if err != nil {
		return err
	}
	it, err := ms.Read(ctx, id)
	if err != nil {
		return err
	}
	lines := []string{memoryRow(it)}
	if it.Why != "" {
		lines = append(lines, "why  "+it.Why)
	}
	author := "author  " + it.Author
	if it.Role != "" {
		author += "  " + it.Role
	}
	lines = append(lines, author)
	for _, l := range it.Links {
		lines = append(lines, fmt.Sprintf("link  %s  %s:%s", l.Rel, l.ToKind, l.ToID))
	}
	if it.ReplacedBy != nil {
		lines = append(lines, fmt.Sprintf("replaced_by  %d", *it.ReplacedBy))
	}
	if it.RetiredAt != nil {
		lines = append(lines, "retired_at  "+it.RetiredAt.Format(time.RFC3339))
	}
	_, err = fmt.Fprintln(stdout, strings.Join(lines, "\n"))
	return err
}

func memoryRetire(ctx context.Context, st *store.Store, ref string, stdout io.Writer) error {
	ms, id, err := memoryByRef(st, ref)
	if err != nil {
		return err
	}
	if err := ms.Retire(ctx, id); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s\n", id, memory.StatusRetired)
	return err
}

func memoryApprove(ctx context.Context, st *store.Store, ref string, stdout io.Writer) error {
	ms, id, err := memoryByRef(st, ref)
	if err != nil {
		return err
	}
	if err := ms.Accept(ctx, id); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s\n", id, memory.StatusAccepted)
	return err
}

func memoryPending(ctx context.Context, st *store.Store, stdout io.Writer) error {
	ms, err := memory.Open(st.DB(), nil)
	if err != nil {
		return err
	}
	items, err := ms.Pending(ctx)
	if err != nil {
		return err
	}
	for _, it := range items {
		if _, err := fmt.Fprintln(stdout, memoryRow(it)); err != nil {
			return err
		}
	}
	return nil
}

// memoryByRef opens the memory tables and parses ref as an item id.
func memoryByRef(st *store.Store, ref string) (*memory.Store, int64, error) {
	id, err := parseMemoryID(ref)
	if err != nil {
		return nil, 0, err
	}
	ms, err := memory.Open(st.DB(), nil)
	return ms, id, err
}

func memoryRow(it memory.Item) string {
	return fmt.Sprintf("%d  %s  %s  %s:%s  %s", it.ID, it.Status, it.Kind, it.Scope.Kind, it.Scope.ID, it.Body)
}

func parseMemoryID(s string) (int64, error) {
	id, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: %q is not a memory id", memory.ErrInvalid, s)
	}
	return id, nil
}

// parseMemoryRef reads a kind:id value of the named flag.
func parseMemoryRef(flag, s string) (memory.Ref, error) {
	kind, id, ok := strings.Cut(s, ":")
	if !ok || kind == "" || id == "" {
		return memory.Ref{}, fmt.Errorf("%w: %s %q is not kind:id", memory.ErrInvalid, flag, s)
	}
	return memory.Ref{Kind: kind, ID: id}, nil
}
