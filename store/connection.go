package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"time"

	"github.com/bhanuprakaash/reef/agent"
)

// Connection is a tool connection set up once in Reef: an MCP server a Bot is offered when it
// selects the connection. It is a local process (Command, Args, Env) or an HTTP server (URL), and
// it names the server the backend shows the Bot's tools under. It listens for nothing and
// authorises nothing.
type Connection struct {
	ID        int64             `json:"id"`
	Name      string            `json:"name"`
	Command   string            `json:"command,omitempty"`
	Args      []string          `json:"args,omitempty"`
	Env       map[string]string `json:"env,omitempty"`
	URL       string            `json:"url,omitempty"`
	CreatedAt time.Time         `json:"createdAt"`
}

const _connectionCols = "id, name, command, args, env, url, created_at"

// Server is the MCP server the backend is given for c.
func (c Connection) Server() agent.MCPServer {
	return agent.MCPServer{Command: c.Command, Args: c.Args, Env: c.Env, URL: c.URL}
}

func (c Connection) validate() error {
	if c.Name == "" || c.Name == "reef" || !_roleName.MatchString(c.Name) {
		return fmt.Errorf("%w: connection name %q must be letters, digits, _ or -, and not reef",
			ErrInvalid, c.Name)
	}
	if (c.Command == "") == (c.URL == "") {
		return fmt.Errorf("%w: connection %s needs a command or a url, not both", ErrInvalid, c.Name)
	}
	if c.URL != "" {
		if u, err := url.Parse(c.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return fmt.Errorf("%w: connection %s: url %q must be http or https", ErrInvalid, c.Name, c.URL)
		}
	}
	return nil
}

// CreateConnection adds c, or fails with ErrConflict when the name is taken.
func (s *Store) CreateConnection(ctx context.Context, c Connection) (Connection, error) {
	if err := c.validate(); err != nil {
		return Connection{}, err
	}
	args, env, err := encodeConnection(c)
	if err != nil {
		return Connection{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO connection (name, command, args, env, url, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		c.Name, c.Command, args, env, c.URL, time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		if isUnique(err) {
			return Connection{}, fmt.Errorf("%w: a connection named %q already exists", ErrConflict, c.Name)
		}
		return Connection{}, fmt.Errorf("insert connection: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Connection{}, fmt.Errorf("insert connection: %w", err)
	}
	return s.Connection(ctx, id)
}

// Connection returns the connection with the id, or ErrNotFound.
func (s *Store) Connection(ctx context.Context, id int64) (Connection, error) {
	c, err := scanConnection(s.db.QueryRowContext(ctx,
		"SELECT "+_connectionCols+" FROM connection WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return Connection{}, fmt.Errorf("connection %d: %w", id, ErrNotFound)
	}
	return c, err
}

// Connections lists every connection by name.
func (s *Store) Connections(ctx context.Context) ([]Connection, error) {
	return listConnections(ctx, s.db)
}

// UpdateConnection replaces every field of the connection with c.ID. Sessions that already
// adopted it keep the old settings.
func (s *Store) UpdateConnection(ctx context.Context, c Connection) (Connection, error) {
	if err := c.validate(); err != nil {
		return Connection{}, err
	}
	args, env, err := encodeConnection(c)
	if err != nil {
		return Connection{}, err
	}
	res, err := s.db.ExecContext(ctx,
		"UPDATE connection SET name = ?, command = ?, args = ?, env = ?, url = ? WHERE id = ?",
		c.Name, c.Command, args, env, c.URL, c.ID)
	if err != nil {
		if isUnique(err) {
			return Connection{}, fmt.Errorf("%w: a connection named %q already exists", ErrConflict, c.Name)
		}
		return Connection{}, fmt.Errorf("update connection: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return Connection{}, fmt.Errorf("connection %d: %w", c.ID, ErrNotFound)
	}
	return s.Connection(ctx, c.ID)
}

// DeleteConnection removes the connection and takes it out of every Bot's selection. Sessions
// that already adopted it keep it.
func (s *Store) DeleteConnection(ctx context.Context, id int64) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin delete connection: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	res, err := tx.ExecContext(ctx, "DELETE FROM connection WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete connection: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("connection %d: %w", id, ErrNotFound)
	}
	rows, err := tx.QueryContext(ctx, "SELECT id, connections FROM bot")
	if err != nil {
		return fmt.Errorf("read bot connections: %w", err)
	}
	type pick struct {
		id  int64
		ids string
	}
	picks, err := scanAll(rows, func(r interface{ Scan(...any) error }) (pick, error) {
		var p pick
		return p, r.Scan(&p.id, &p.ids)
	})
	if err != nil {
		return err
	}
	for _, p := range picks {
		var ids []int64
		if err := json.Unmarshal([]byte(p.ids), &ids); err != nil {
			return fmt.Errorf("bot %d: bad connections: %w", p.id, err)
		}
		if !slices.Contains(ids, id) {
			continue
		}
		left, err := json.Marshal(slices.DeleteFunc(ids, func(i int64) bool { return i == id }))
		if err != nil {
			return fmt.Errorf("encode connections: %w", err)
		}
		if _, err := tx.ExecContext(ctx, "UPDATE bot SET connections = ? WHERE id = ?", string(left), p.id); err != nil {
			return fmt.Errorf("update bot connections: %w", err)
		}
	}
	return tx.Commit()
}

// checkConnections fails with ErrInvalid when an id names no connection.
func (s *Store) checkConnections(ctx context.Context, ids []int64) error {
	all, err := s.Connections(ctx)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if !slices.ContainsFunc(all, func(c Connection) bool { return c.ID == id }) {
			return fmt.Errorf("%w: unknown connection %d", ErrInvalid, id)
		}
	}
	return nil
}

type connectionQuerier interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func listConnections(ctx context.Context, q connectionQuerier) ([]Connection, error) {
	rows, err := q.QueryContext(ctx, "SELECT "+_connectionCols+" FROM connection ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("list connections: %w", err)
	}
	return scanAll(rows, scanConnection)
}

// pickConnections returns the connections with the ids, in the order of ids.
func pickConnections(all []Connection, ids []int64) []Connection {
	out := make([]Connection, 0, len(ids))
	for _, id := range ids {
		if i := slices.IndexFunc(all, func(c Connection) bool { return c.ID == id }); i >= 0 {
			out = append(out, all[i])
		}
	}
	return out
}

func encodeConnection(c Connection) (string, string, error) {
	args, err := json.Marshal(nonNil(c.Args))
	if err != nil {
		return "", "", fmt.Errorf("encode args: %w", err)
	}
	env := c.Env
	if env == nil {
		env = map[string]string{}
	}
	e, err := json.Marshal(env)
	if err != nil {
		return "", "", fmt.Errorf("encode env: %w", err)
	}
	return string(args), string(e), nil
}

func scanConnection(r interface{ Scan(...any) error }) (Connection, error) {
	var c Connection
	var args, env, created string
	if err := r.Scan(&c.ID, &c.Name, &c.Command, &args, &env, &c.URL, &created); err != nil {
		return Connection{}, err
	}
	if err := json.Unmarshal([]byte(args), &c.Args); err != nil {
		return Connection{}, fmt.Errorf("connection %s: bad args: %w", c.Name, err)
	}
	if err := json.Unmarshal([]byte(env), &c.Env); err != nil {
		return Connection{}, fmt.Errorf("connection %s: bad env: %w", c.Name, err)
	}
	var err error
	if c.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return Connection{}, fmt.Errorf("connection %s: bad created_at: %w", c.Name, err)
	}
	return c, nil
}
