package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/bhanuprakaash/reef/server"
)

const (
	_wrapUsage     = "usage: reef wrap list|show|approve|reject [<work-item>|<id>] [--data DIR]"
	_wrapListUsage = "usage: reef wrap list [<work-item>] [--data DIR]"
	_wrapIDUsage   = "usage: reef wrap show|approve|reject <id> [--data DIR]"
)

func wrapCmd(ctx context.Context, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New(_wrapUsage)
	}
	switch args[0] {
	case "list":
		return wrapList(ctx, args[1:], stdout)
	case "show", "approve", "reject":
		return wrapDecide(ctx, args[0], args[1:], stdout)
	}
	return errors.New(_wrapUsage)
}

func wrapList(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef wrap list", _wrapListUsage, 0, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	var workItemID *int64
	var projectID *int64
	if len(pos) == 1 {
		f, err := workItemArg(ctx, st, pos[0])
		if err != nil {
			return err
		}
		workItemID = &f.ID
	} else {
		p, err := cwdProject(ctx, st)
		if err != nil {
			return err
		}
		projectID = &p.ID
	}
	ps, err := st.PendingProposals(ctx, workItemID, projectID)
	if err != nil {
		return err
	}
	for _, p := range ps {
		ch, err := st.Space(ctx, p.SpaceID)
		if err != nil {
			return err
		}
		f, err := st.WorkItem(ctx, *ch.WorkItemID)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "%d  %d  %d  %s  %s  %s\n",
			p.ID, f.ID, p.SpaceID, p.Role, p.Kind, p.CreatedAt.Format(time.DateOnly)); err != nil {
			return err
		}
	}
	return nil
}

func wrapDecide(ctx context.Context, verb string, args []string, stdout io.Writer) error {
	c := newCommand("reef wrap "+verb, _wrapIDUsage, 1, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	id, err := strconv.ParseInt(pos[0], 10, 64)
	if err != nil {
		return errors.New(_wrapIDUsage)
	}
	switch verb {
	case "show":
		p, err := st.Proposal(ctx, id)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "%s\nstatus: %s\n\n%s",
			proposalTitle(p), p.Status, proposalBodyText(p))
		return err
	case "approve":
		ms, err := openMemory(ctx, st)
		if err != nil {
			return err
		}
		a, err := server.ApproveProposal(ctx, st, ms, id)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintln(stdout, approvedText(a))
		return err
	}
	if _, err := st.RejectProposal(ctx, id); err != nil {
		return err
	}
	_, err = io.WriteString(stdout, "rejected: nothing changed\n")
	return err
}
