package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bhanuprakaash/reef/memory"
	"github.com/bhanuprakaash/reef/store"
)

const (
	_memoryUsage = `usage: reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W] [--needs-approval]
       reef memory recall "<q>" [--scope S] [--limit N]
       reef memory show <id> | retire <id> | approve <id> | reject <id> | pending
       reef memory embed [none|openai|voyage|gemini [--key K]]`
	_memoryShowUsage    = "usage: reef memory show <id> [--data DIR]"
	_memoryAddUsage     = `usage: reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W] [--needs-approval] [--data DIR]`
	_memoryRecallUsage  = `usage: reef memory recall "<q>" [--scope S] [--limit N] [--data DIR]`
	_memoryRetireUsage  = "usage: reef memory retire <id> [--data DIR]"
	_memoryPendingUsage = "usage: reef memory pending [--data DIR]"
	_memoryApproveUsage = "usage: reef memory approve <id> [--data DIR]"
	_memoryRejectUsage  = "usage: reef memory reject <id> [--data DIR]"
	_memoryEmbedUsage   = "usage: reef memory embed [none|openai|voyage|gemini [--key K]] [--data DIR]"
)

const (
	_settingEmbeddingProvider = "embedding.provider"
	_settingEmbeddingKey      = "embedding.key"
)

type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	*l = append(*l, v)
	return nil
}

func memoryCmd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	sub, rest := memorySub(args)
	switch sub {
	case "show":
		return memoryShow(ctx, rest, stdout)
	case "add":
		return memoryAdd(ctx, rest, stdout, stderr)
	case "recall":
		return memoryRecall(ctx, rest, stdout)
	case "retire":
		return memoryRetire(ctx, rest, stdout)
	case "approve":
		return memoryApprove(ctx, rest, stdout)
	case "reject":
		return memoryReject(ctx, rest, stdout)
	case "pending":
		return memoryPending(ctx, rest, stdout)
	case "embed":
		return memoryEmbed(ctx, rest, stdout)
	}
	return errors.New(_memoryUsage)
}

// memorySub splits the subcommand word off args; only --data may come before it.
func memorySub(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			return args[i], slices.Concat(args[:i], args[i+1:])
		}
		if strings.TrimLeft(args[i], "-") == "data" && i+1 < len(args) {
			i++
		}
	}
	return "", args
}

func memoryShow(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory show", _memoryShowUsage, 1, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	return memoryItem(ctx, st, pos[0], stdout)
}

func memoryAdd(ctx context.Context, args []string, stdout, stderr io.Writer) error {
	c := newCommand("reef memory add", _memoryAddUsage, 2, 2)
	scope := c.fs.String("scope", "", "")
	role := c.fs.String("as", "", "")
	why := c.fs.String("why", "", "")
	needs := c.fs.Bool("needs-approval", false, "")
	var about, replaces listFlag
	c.fs.Var(&about, "about", "")
	c.fs.Var(&replaces, "replaces", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if *scope == "" {
		return errors.New(_memoryAddUsage)
	}
	ms, err := openMemory(ctx, st)
	if err != nil {
		return err
	}
	w := memory.Writer{Author: _cliAuthor, Role: *role}
	if w.Scope, err = memory.ParseRef(*scope); err != nil {
		return fmt.Errorf("--scope: %w", err)
	}
	n := memory.Note{Kind: pos[0], Body: pos[1], Why: *why, NeedsApproval: *needs}
	for _, a := range about {
		r, err := memory.ParseRef(a)
		if err != nil {
			return fmt.Errorf("--about: %w", err)
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
	return reportRemembered(ctx, st, res, stdout, stderr)
}

// reportRemembered says what looks like the item just written and why it has no vector.
func reportRemembered(
	ctx context.Context, st *store.Store, res memory.Result, stdout, stderr io.Writer,
) error {
	for _, s := range res.Similar {
		if _, err := fmt.Fprintf(stdout, "%d  similar  %s\n", s.ID, s.Body); err != nil {
			return err
		}
	}
	if res.EmbedErr == nil {
		return nil
	}
	provider, err := st.Setting(ctx, _settingEmbeddingProvider)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stderr, "vector not stored: %v; run reef memory embed %s to retry\n",
		res.EmbedErr, provider)
	return err
}

func memoryRecall(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory recall", _memoryRecallUsage, 1, 1)
	scope := c.fs.String("scope", "", "")
	limit := c.fs.Int("limit", 0, "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if *limit < 0 {
		return errors.New(_memoryRecallUsage)
	}
	ms, err := openMemory(ctx, st)
	if err != nil {
		return err
	}
	q := memory.Query{Text: pos[0], Limit: *limit}
	if *scope != "" {
		r, err := memory.ParseRef(*scope)
		if err != nil {
			return fmt.Errorf("--scope: %w", err)
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
	ms, id, err := memoryByRef(ctx, st, ref)
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

func memoryRetire(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory retire", _memoryRetireUsage, 1, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	ms, id, err := memoryByRef(ctx, st, pos[0])
	if err != nil {
		return err
	}
	if err := ms.Retire(ctx, id); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s\n", id, memory.StatusRetired)
	return err
}

func memoryApprove(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory approve", _memoryApproveUsage, 1, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	ms, id, err := memoryByRef(ctx, st, pos[0])
	if err != nil {
		return err
	}
	if err := ms.Accept(ctx, id); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s\n", id, memory.StatusAccepted)
	return err
}

func memoryReject(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory reject", _memoryRejectUsage, 1, 1)
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	ms, id, err := memoryByRef(ctx, st, pos[0])
	if err != nil {
		return err
	}
	if err := ms.Reject(ctx, id); err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "%d  %s\n", id, memory.StatusRejected)
	return err
}

func memoryPending(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory pending", _memoryPendingUsage, 0, 0)
	st, _, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	ms, err := openMemory(ctx, st)
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

func memoryByRef(ctx context.Context, st *store.Store, ref string) (*memory.Store, int64, error) {
	id, err := parseMemoryID(ref)
	if err != nil {
		return nil, 0, err
	}
	ms, err := openMemory(ctx, st)
	return ms, id, err
}

func openMemory(ctx context.Context, st *store.Store) (*memory.Store, error) {
	provider, err := st.Setting(ctx, _settingEmbeddingProvider)
	if err != nil {
		return nil, err
	}
	key, err := st.Setting(ctx, _settingEmbeddingKey)
	if err != nil {
		return nil, err
	}
	e, err := memory.NewEmbedder(memory.EmbedConfig{Provider: provider, Key: key}, nil)
	if err != nil {
		return nil, err
	}
	return memory.Open(st.DB(), e)
}

func memoryEmbedShow(ctx context.Context, st *store.Store, stdout io.Writer) error {
	provider, err := st.Setting(ctx, _settingEmbeddingProvider)
	if err != nil {
		return err
	}
	if provider == "" {
		provider = memory.ProviderNone
	}
	_, err = fmt.Fprintln(stdout, provider)
	return err
}

func memoryEmbed(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory embed", _memoryEmbedUsage, 0, 1)
	key := c.fs.String("key", "", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if len(pos) == 0 {
		if *key != "" {
			return errors.New(_memoryEmbedUsage)
		}
		return memoryEmbedShow(ctx, st, stdout)
	}
	return memoryEmbedSet(ctx, st, pos[0], *key, stdout)
}

func memoryEmbedSet(
	ctx context.Context, st *store.Store, provider, key string, stdout io.Writer,
) error {
	prev, err := st.Setting(ctx, _settingEmbeddingProvider)
	if err != nil {
		return err
	}
	switch provider {
	case memory.ProviderNone:
		key = ""
	case memory.ProviderOpenAI, memory.ProviderVoyage, memory.ProviderGemini:
		if key == "" && prev == provider {
			if key, err = st.Setting(ctx, _settingEmbeddingKey); err != nil {
				return err
			}
		}
		if key == "" {
			return fmt.Errorf("%w: embedding provider %s needs --key", memory.ErrInvalid, provider)
		}
	default:
		return fmt.Errorf("%w: embedding provider %q is not none, openai, voyage or gemini",
			memory.ErrInvalid, provider)
	}
	if provider != prev {
		cleared, err := memory.Open(st.DB(), nil)
		if err != nil {
			return err
		}
		if err := cleared.ClearVectors(ctx); err != nil {
			return err
		}
	}
	if key != "" {
		if err := restrictDB(ctx, st); err != nil {
			return err
		}
	}
	err = st.SetSettings(ctx, map[string]string{
		_settingEmbeddingProvider: provider,
		_settingEmbeddingKey:      key,
	})
	if err != nil {
		return err
	}
	if provider == memory.ProviderNone {
		_, err = fmt.Fprintf(stdout, "embedding provider %s\n", provider)
		return err
	}
	ms, err := openMemory(ctx, st)
	if err != nil {
		return err
	}
	n, err := ms.Backfill(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "embedding provider %s; embedded %d items\n", provider, n)
	return err
}

// restrictDB makes reef.db and any -wal/-shm beside it owner-only. SQLite
// creates later -wal/-shm files with the main file's mode.
func restrictDB(ctx context.Context, st *store.Store) error {
	var path string
	err := st.DB().QueryRowContext(ctx,
		"SELECT file FROM pragma_database_list WHERE name = 'main'").Scan(&path)
	if err != nil {
		return fmt.Errorf("find db file: %w", err)
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Chmod(path+suffix, 0o600); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("restrict db file: %w", err)
		}
	}
	return nil
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
