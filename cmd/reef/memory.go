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
	_memoryUsage = `usage: reef memory [--project P] [show|set FILE|-] [--data DIR]
       reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W]
       reef memory recall "<q>" [--scope S] [--limit N]
       reef memory show <id> | retire <id> | approve <id> | pending
       reef memory embed [none|openai|voyage|gemini [--key K]]`
	_memoryShowUsage    = "usage: reef memory [--project P] show [<id>] [--data DIR]"
	_memorySetUsage     = "usage: reef memory [--project P] set FILE|- [--data DIR]"
	_memoryAddUsage     = `usage: reef memory add <kind> "<body>" --scope S [--as ROLE] [--about X]... [--replaces ID]... [--why W] [--data DIR]`
	_memoryRecallUsage  = `usage: reef memory recall "<q>" [--scope S] [--limit N] [--data DIR]`
	_memoryRetireUsage  = "usage: reef memory retire <id> [--data DIR]"
	_memoryPendingUsage = "usage: reef memory pending [--data DIR]"
	_memoryApproveUsage = "usage: reef memory approve <id> [--data DIR]"
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

func memoryCmd(ctx context.Context, args []string, stdin io.Reader, stdout io.Writer) error {
	sub, rest := memorySub(args)
	switch sub {
	case "show":
		return memoryShow(ctx, rest, stdout)
	case "set":
		return memorySet(ctx, rest, stdin, stdout)
	case "add":
		return memoryAdd(ctx, rest, stdout)
	case "recall":
		return memoryRecall(ctx, rest, stdout)
	case "retire":
		return memoryRetire(ctx, rest, stdout)
	case "approve":
		return memoryApprove(ctx, rest, stdout)
	case "pending":
		return memoryPending(ctx, rest, stdout)
	case "embed":
		return memoryEmbed(ctx, rest, stdout)
	}
	return errors.New(_memoryUsage)
}

// memorySub splits the subcommand word off args; only --project and --data may come before
// it. With no word the subcommand is show.
func memorySub(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "-") {
			return args[i], slices.Concat(args[:i], args[i+1:])
		}
		name := strings.TrimLeft(args[i], "-")
		if (name == "project" || name == "data") && i+1 < len(args) {
			i++
		}
	}
	return "show", args
}

func memoryShow(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory show", _memoryShowUsage, 0, 1)
	project := c.fs.String("project", "", "")
	st, pos, err := c.open(args)
	if err != nil {
		return err
	}
	defer func() { _ = st.Close() }()

	if len(pos) == 1 {
		return memoryItem(ctx, st, pos[0], stdout)
	}
	p, err := memoryProject(ctx, st, *project)
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
	if _, err := st.PutProjectMemory(ctx,
		store.ProjectMemory{ProjectID: p.ID, Body: string(body)}); err != nil {
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

func memoryAdd(ctx context.Context, args []string, stdout io.Writer) error {
	c := newCommand("reef memory add", _memoryAddUsage, 2, 2)
	scope := c.fs.String("scope", "", "")
	role := c.fs.String("as", "", "")
	why := c.fs.String("why", "", "")
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
	if w.Scope, err = parseMemoryRef("--scope", *scope); err != nil {
		return err
	}
	n := memory.Note{Kind: pos[0], Body: pos[1], Why: *why}
	for _, a := range about {
		r, err := parseMemoryRef("--about", a)
		if err != nil {
			return err
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
	for _, s := range res.Similar {
		if _, err := fmt.Fprintf(stdout, "%d  similar  %s\n", s.ID, s.Body); err != nil {
			return err
		}
	}
	return nil
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
		r, err := parseMemoryRef("--scope", *scope)
		if err != nil {
			return err
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
	if err := st.SetSetting(ctx, _settingEmbeddingProvider, provider); err != nil {
		return err
	}
	if err := st.SetSetting(ctx, _settingEmbeddingKey, key); err != nil {
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
	if provider != prev {
		if err := ms.ClearVectors(ctx); err != nil {
			return err
		}
	}
	n, err := ms.Backfill(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(stdout, "embedding provider %s; embedded %d items\n", provider, n)
	return err
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

func parseMemoryRef(flag, s string) (memory.Ref, error) {
	kind, id, ok := strings.Cut(s, ":")
	if !ok || kind == "" || id == "" {
		return memory.Ref{}, fmt.Errorf("%w: %s %q is not kind:id", memory.ErrInvalid, flag, s)
	}
	return memory.Ref{Kind: kind, ID: id}, nil
}
