package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/bhanuprakaash/reef/store"
)

const (
	_teamUsage      = "usage: reef team apply FILE.json | list [--data DIR]"
	_teamApplyUsage = "usage: reef team apply FILE.json [--data DIR]"
	_teamListUsage  = "usage: reef team list [--data DIR]"
)

func teamCmd(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(_teamUsage)
	}
	switch args[0] {
	case "apply":
		return teamApply(ctx, args[1:], stdout)
	case "list":
		return teamList(ctx, args[1:], stdout)
	}
	return errors.New(_teamUsage)
}

func teamApply(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef team apply", _teamApplyUsage, 1, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	body, err := os.ReadFile(pos[0])
	if err != nil {
		return fmt.Errorf("read team file: %w", err)
	}
	var t store.Team
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&t); err != nil {
		return fmt.Errorf("parse %s: %w", pos[0], err)
	}
	t, err = st.ApplyTeam(ctx, t)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "applied team %s: %s\n", t.Name, roleList(t))
	return err
}

func teamList(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef team list", _teamListUsage, 0, 0)
	st, _, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	teams, err := st.Teams(ctx)
	if err != nil {
		return err
	}
	for _, t := range teams {
		if _, err := fmt.Fprintf(stdout, "%s  %s\n", t.Name, roleList(t)); err != nil {
			return err
		}
	}
	return nil
}

// roleList is "name(backend)" per role, the lead marked with a star.
func roleList(t store.Team) string {
	parts := make([]string, 0, len(t.Roles))
	for _, r := range t.Roles {
		lead := ""
		if r.Lead {
			lead = "*"
		}
		parts = append(parts, fmt.Sprintf("%s%s(%s)", r.Name, lead, r.Backend))
	}
	return strings.Join(parts, " ")
}
