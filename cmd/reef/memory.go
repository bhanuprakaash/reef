package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"github.com/bhanuprakaash/reef/store"
)

const (
	_memoryUsage     = "usage: reef memory [--project P] [show|set FILE|-] [--data DIR]"
	_memoryShowUsage = "usage: reef memory [--project P] show [--data DIR]"
	_memorySetUsage  = "usage: reef memory [--project P] set FILE|- [--data DIR]"
)

// memoryCmd parses --project and --data before looking for the show|set word, so either can
// come before or after it; bare `reef memory` means show.
func memoryCmd(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	c := newCommand("reef memory", _memoryUsage, 0, 2)
	project := c.fs.String("project", "", "")
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
		if len(rest) != 0 {
			return errors.New(_memoryShowUsage)
		}
		return memoryShow(ctx, st, *project, stdout)
	case "set":
		if len(rest) != 1 {
			return errors.New(_memorySetUsage)
		}
		return memorySet(ctx, st, *project, rest[0], stdin, stdout)
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
