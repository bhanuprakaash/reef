package store

import (
	"context"
	"testing"
)

func TestDirectRequestState(t *testing.T) {
	ctx := context.Background()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = s.Close() }()
	if _, err := s.CreateProject(ctx, Project{Name: "shop", Path: "/tmp/shop"}); err != nil {
		t.Fatal(err)
	}
	ch, err := s.CreateSpace(ctx, Space{ProjectID: 1, Name: "chat"})
	if err != nil {
		t.Fatal(err)
	}
	state := func(id int64) string {
		t.Helper()
		m, err := s.DirectMessage(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return m.State
	}
	request := func(from, to string) DirectMessage {
		t.Helper()
		m, err := s.CreateDirectMessage(ctx, DirectMessage{SpaceID: ch.ID, From: from, To: to, Body: "status?"})
		if err != nil {
			t.Fatal(err)
		}
		return m
	}
	turn := func(role string, wake *int64) {
		t.Helper()
		if _, err := s.CreateTurn(ctx, Turn{
			SpaceID: ch.ID, Role: role, Backend: "claude", DirectMessageID: wake,
		}); err != nil {
			t.Fatal(err)
		}
	}

	req := request("developer", "tester")
	if got := state(req.ID); got != "waiting" {
		t.Fatalf("new request state = %q, want waiting", got)
	}
	turn("tester", &req.ID)
	if got := state(req.ID); got != "delivered" {
		t.Fatalf("state after the recipient's turn started = %q, want delivered", got)
	}
	resp, err := s.CreateDirectResponse(ctx, req.ID, "tester", "all green")
	if err != nil {
		t.Fatal(err)
	}
	if got := state(req.ID); got != "answered" || state(resp.ID) != "" {
		t.Fatalf("state after the response = %q (response %q), want answered and none", got, state(resp.ID))
	}

	if err := s.SetDirectRequestState(ctx, req.ID, DirectFailed); err != nil {
		t.Fatal(err)
	}
	turn("developer", &resp.ID)
	if got := state(req.ID); got != "answered" {
		t.Fatalf("state after a failed response wake was retried = %q, want answered", got)
	}

	failed, stopped := request("lead", "architect"), request("lead", "tester")
	if _, err := s.CreateFailedDirectResponse(ctx, failed.ID, "architect", "dropped"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CreateFailedDirectResponse(ctx, stopped.ID, "tester", "stopped"); err != nil {
		t.Fatal(err)
	}
	if got, want := state(failed.ID)+" "+state(stopped.ID), "failed stopped"; got != want {
		t.Fatalf("states after failed responses = %q, want %q", got, want)
	}
}
