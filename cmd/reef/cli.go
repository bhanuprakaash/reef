package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/space"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_cliAuthor     = "user"
	_projectUsage  = "usage: reef project add [--name N] [--data DIR]"
	_workItemUsage = `usage: reef work-item add "<title>" [--type T] [--description D] [--data DIR]`
	_briefUsage    = "usage: reef brief <work-item> [--data DIR]"
	_decideUsage   = `usage: reef decide <work-item> "<body>" [--why W] [--data DIR]`
	_contextUsage  = "usage: reef context <work-item> --role R [--data DIR]"
)

type command struct {
	fs       *flag.FlagSet
	data     *string
	usage    string
	min, max int
}

func newCommand(name, usage string, minArgs, maxArgs int) command {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return command{fs: fs, data: fs.String("data", "", ""), usage: usage, min: minArgs, max: maxArgs}
}

// open parses args, flags may come before or after the positional args, and
// opens the store.
func (c command) open(args []string) (*store.Store, []string, error) {
	var pos []string
	for {
		if err := c.fs.Parse(args); err != nil {
			return nil, nil, fmt.Errorf("%w\n%s", err, c.usage)
		}
		rest := c.fs.Args()
		if n := len(args) - len(rest); n > 0 && args[n-1] == "--" {
			pos = append(pos, rest...)
			break
		}
		if len(rest) == 0 {
			break
		}
		pos = append(pos, rest[0])
		args = rest[1:]
	}
	if len(pos) < c.min || len(pos) > c.max {
		return nil, nil, errors.New(c.usage)
	}
	dir, err := dataDir(*c.data)
	if err != nil {
		return nil, nil, err
	}
	st, err := store.Open(dir)
	if err != nil {
		return nil, nil, fmt.Errorf("open store: %w", err)
	}
	return st, pos, nil
}

func projectCmd(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "add" {
		return errors.New(_projectUsage)
	}
	c := newCommand("reef project add", _projectUsage, 0, 0)
	name := c.fs.String("name", "", "")
	st, _, err := c.open(args[1:])
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("find current folder: %w", err)
	}
	cwd, err = store.ResolvePath(cwd)
	if err != nil {
		return fmt.Errorf("resolve current folder: %w", err)
	}
	existing, err := st.ProjectByPath(ctx, cwd)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return err
	}
	if err == nil && existing.Path == cwd {
		return fmt.Errorf("%s is already project %d", cwd, existing.ID)
	}
	if *name == "" {
		*name = filepath.Base(cwd)
	}
	p, err := st.CreateProject(ctx, store.Project{Name: *name, Path: cwd})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s  %s\n", p.ID, p.Name, p.Path)
	return err
}

func workItemCmd(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "add" {
		return errors.New(_workItemUsage)
	}
	c := newCommand("reef work-item add", _workItemUsage, 1, 1)
	typ := c.fs.String("type", "", "")
	description := c.fs.String("description", "", "")
	st, pos, err := c.open(args[1:])
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	p, err := cwdProject(ctx, st)
	if err != nil {
		return err
	}
	w, err := st.CreateWorkItem(ctx, store.WorkItem{
		ProjectID: p.ID, Type: *typ, Title: pos[0], Description: *description,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s  %s\n", w.ID, w.Type, w.Title)
	return err
}

// workItemArg is the work item whose id is ref.
func workItemArg(ctx context.Context, st *store.Store, ref string) (store.WorkItem, error) {
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil {
		return store.WorkItem{}, fmt.Errorf("work item %q is not an id", ref)
	}
	return st.WorkItem(ctx, id)
}

func briefCmd(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef brief", _briefUsage, 1, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	f, err := workItemArg(ctx, st, pos[0])
	if err != nil {
		return err
	}
	b, err := st.Brief(ctx, f.ID)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("work item %s has no brief yet", pos[0])
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, b.Body)
	return err
}

func decideCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c := newCommand("reef decide", _decideUsage, 2, 2)
	why := c.fs.String("why", "", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	f, err := workItemArg(ctx, st, pos[0])
	if err != nil {
		return err
	}
	ms, err := openMemory(ctx, st)
	if err != nil {
		return err
	}
	res, err := ms.Remember(ctx,
		memory.Writer{
			Author: _cliAuthor, Role: _cliAuthor,
			Scope: memory.Ref{Kind: memory.ScopeWorkItem, ID: strconv.FormatInt(f.ID, 10)},
		},
		memory.Note{Kind: memory.KindDecision, Body: pos[1], Why: *why})
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(stdout, "%d  %s\n", res.Item.ID, res.Item.Body); err != nil {
		return err
	}
	return reportRemembered(ctx, st, res, stdout, stderr)
}

func contextCmd(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef context", _contextUsage, 1, 1)
	role := c.fs.String("role", "", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if *role == "" {
		return errors.New(_contextUsage)
	}
	f, err := workItemArg(ctx, st, pos[0])
	if err != nil {
		return err
	}
	ms, err := openMemory(ctx, st)
	if err != nil {
		return err
	}
	out, err := space.Context(ctx, st, ms, f.ID, *role)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, out)
	return err
}

// cwdProject is the project of the current folder or its nearest parent.
func cwdProject(ctx context.Context, st *store.Store) (store.Project, error) {
	raw, err := os.Getwd()
	if err != nil {
		return store.Project{}, fmt.Errorf("find current folder: %w", err)
	}
	cwd, err := store.ResolvePath(raw)
	if err != nil {
		return store.Project{}, fmt.Errorf("resolve current folder: %w", err)
	}
	p, err := st.ProjectByPath(ctx, cwd)
	if errors.Is(err, store.ErrNotFound) && raw != cwd {
		p, err = st.ProjectByPath(ctx, raw)
	}
	if errors.Is(err, store.ErrNotFound) {
		return store.Project{}, fmt.Errorf("no reef project for %s; run reef project add", cwd)
	}
	return p, err
}
