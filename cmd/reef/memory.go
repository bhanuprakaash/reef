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
	_memoryUsage     = "usage: reef memory show|set FILE|- [--project P] [--data DIR]"
	_memoryShowUsage = "usage: reef memory show [--project P] [--data DIR]"
	_memorySetUsage  = "usage: reef memory set FILE|- [--project P] [--data DIR]"
)

func memoryCmd(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(_memoryUsage)
	}
	switch args[0] {
	case "show":
		return memoryShow(ctx, args[1:], stdout)
	case "set":
		return memorySet(ctx, args[1:], stdin, stdout)
	}
	return errors.New(_memoryUsage)
}

func memoryShow(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory show", _memoryShowUsage, 0, 0)
	project := c.fs.String("project", "", "")
	st, _, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	p, err := memoryProject(ctx, st, *project)
	if err != nil {
		return err
	}
	m, err := st.ProjectMemory(ctx, p.ID)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(stdout, m.Body)
	return err
}

func memorySet(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	c := newCommand("reef memory set", _memorySetUsage, 1, 1)
	project := c.fs.String("project", "", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	p, err := memoryProject(ctx, st, *project)
	if err != nil {
		return err
	}
	var body []byte
	if pos[0] == "-" {
		body, err = io.ReadAll(stdin)
	} else {
		body, err = os.ReadFile(pos[0])
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
