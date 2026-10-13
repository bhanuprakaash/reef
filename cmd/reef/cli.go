package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_cliAuthor     = "user"
	_projectUsage  = "usage: reef project add [--name N] [--data DIR]"
	_featureUsage  = "usage: reef feature add <slug> <name> [--goal G] [--data DIR]"
	_briefUsage    = "usage: reef brief <feature> [--data DIR]"
	_decideUsage   = `usage: reef decide <feature> "<body>" [--why W] [--data DIR]`
	_taskUsage     = "usage: reef task add|list [<feature>] [--data DIR]"
	_taskAddUsage  = `usage: reef task add [<feature>] "<title>" [--data DIR]`
	_taskListUsage = "usage: reef task list [<feature>] [--status S] [--data DIR]"
	_contextUsage  = "usage: reef context <feature> --role R [--data DIR]"
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

func featureCmd(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 || args[0] != "add" {
		return errors.New(_featureUsage)
	}
	c := newCommand("reef feature add", _featureUsage, 2, 2)
	goal := c.fs.String("goal", "", "")
	st, pos, err := c.open(args[1:])
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	p, err := cwdProject(ctx, st)
	if err != nil {
		return err
	}
	f, err := st.CreateFeature(ctx, store.Feature{
		ProjectID: p.ID, Slug: pos[0], Name: pos[1], Goal: *goal,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s  %s\n", f.ID, f.Slug, f.Name)
	return err
}

func briefCmd(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef brief", _briefUsage, 1, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	f, err := st.FeatureByRef(ctx, pos[0])
	if err != nil {
		return err
	}
	b, err := st.Brief(ctx, f.ID)
	if errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("feature %s has no brief yet", pos[0])
	}
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, b.Body)
	return err
}

func decideCmd(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef decide", _decideUsage, 2, 2)
	why := c.fs.String("why", "", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	f, err := st.FeatureByRef(ctx, pos[0])
	if err != nil {
		return err
	}
	d, err := st.CreateDecision(ctx, store.Decision{
		FeatureID: f.ID, Body: pos[1], Why: *why, Author: _cliAuthor,
	})
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s\n", d.ID, d.Body)
	return err
}

func taskCmd(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(_taskUsage)
	}
	switch args[0] {
	case "add":
		return taskAdd(ctx, args[1:], stdout)
	case "list":
		return taskList(ctx, args[1:], stdout)
	}
	return errors.New(_taskUsage)
}

func taskAdd(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef task add", _taskAddUsage, 1, 2)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	t := store.Task{Title: pos[len(pos)-1]}
	if len(pos) == 2 {
		f, err := st.FeatureByRef(ctx, pos[0])
		if err != nil {
			return err
		}
		t.ProjectID, t.FeatureID = f.ProjectID, &f.ID
	} else {
		p, err := cwdProject(ctx, st)
		if err != nil {
			return err
		}
		t.ProjectID = p.ID
	}
	t, err = st.CreateTask(ctx, t)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s  %s\n", t.ID, t.Status, t.Title)
	return err
}

func taskList(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef task list", _taskListUsage, 0, 1)
	status := c.fs.String("status", "", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	filter := store.TaskFilter{Status: *status}
	if len(pos) == 1 {
		f, err := st.FeatureByRef(ctx, pos[0])
		if err != nil {
			return err
		}
		filter.FeatureID = &f.ID
	} else {
		p, err := cwdProject(ctx, st)
		if err != nil {
			return err
		}
		filter.ProjectID = &p.ID
	}
	tasks, err := st.Tasks(ctx, filter)
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if _, err := fmt.Fprintf(stdout, "%d  %s  %s\n", t.ID, t.Status, t.Title); err != nil {
			return err
		}
	}
	return nil
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
	f, err := st.FeatureByRef(ctx, pos[0])
	if err != nil {
		return err
	}
	out, err := channel.Context(ctx, st, f.ID, *role)
	if err != nil {
		return err
	}
	_, err = io.WriteString(stdout, out)
	return err
}

// cwdProject is the project of the current folder or its nearest parent.
func cwdProject(ctx context.Context, st *store.Store) (store.Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return store.Project{}, fmt.Errorf("find current folder: %w", err)
	}
	p, err := st.ProjectByPath(ctx, cwd)
	if errors.Is(err, store.ErrNotFound) {
		return store.Project{}, fmt.Errorf("no reef project for %s; run reef project add", cwd)
	}
	return p, err
}
