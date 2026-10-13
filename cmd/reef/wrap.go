package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"
)

const (
	_wrapUsage     = "usage: reef wrap list|show|approve|reject [<feature>|<id>] [--data DIR]"
	_wrapListUsage = "usage: reef wrap list [<feature>] [--data DIR]"
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

	var featureID *int64
	var projectID *int64
	if len(pos) == 1 {
		f, err := st.FeatureByRef(ctx, pos[0])
		if err != nil {
			return err
		}
		featureID = &f.ID
	} else {
		p, err := cwdProject(ctx, st)
		if err != nil {
			return err
		}
		projectID = &p.ID
	}
	ps, err := st.PendingProposals(ctx, featureID, projectID)
	if err != nil {
		return err
	}
	for _, p := range ps {
		ch, err := st.Channel(ctx, p.ChannelID)
		if err != nil {
			return err
		}
		f, err := st.Feature(ctx, *ch.FeatureID)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(stdout, "%d  %s  %d  %s  %s\n",
			p.ID, f.Slug, p.ChannelID, p.Role, p.CreatedAt.Format(time.DateOnly)); err != nil {
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
		_, err = fmt.Fprintf(stdout, "proposal %d from %s\nstatus: %s\n\n%s",
			p.ID, p.Role, p.Status, proposalBodyText(p))
		return err
	case "approve":
		_, added, err := st.ApproveProposal(ctx, id)
		if err != nil {
			return err
		}
		_, err = fmt.Fprintf(stdout, "approved: brief, notes and %d decisions saved\n", added)
		return err
	}
	if _, err := st.RejectProposal(ctx, id); err != nil {
		return err
	}
	_, err = io.WriteString(stdout, "rejected: nothing changed\n")
	return err
}
