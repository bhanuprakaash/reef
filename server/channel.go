package server

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"unicode/utf8"

	"github.com/bhanuprakaash/reef/channel"
	"github.com/bhanuprakaash/reef/store"
)

const _fileCap = 512 << 10

func (s *server) channelRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /channels", s.listChannels)
	mux.HandleFunc("POST /channels", s.createChannel)
	mux.HandleFunc("/channels", methodNotAllowed)
	mux.HandleFunc("GET /channels/{id}", s.getChannel)
	mux.HandleFunc("PUT /channels/{id}", s.updateChannel)
	mux.HandleFunc("DELETE /channels/{id}", s.deleteChannel)
	mux.HandleFunc("/channels/{id}", methodNotAllowed)
	mux.HandleFunc("POST /channels/{id}/reopen", s.reopenChannel)
	mux.HandleFunc("/channels/{id}/reopen", methodNotAllowed)
	mux.HandleFunc("GET /channels/{id}/inspector", s.getInspector)
	mux.HandleFunc("/channels/{id}/inspector", methodNotAllowed)
	mux.HandleFunc("GET /channels/{id}/direct-messages", s.listDirectMessages)
	mux.HandleFunc("/channels/{id}/direct-messages", methodNotAllowed)
	mux.HandleFunc("GET /channels/{id}/file", s.getChannelFile)
	mux.HandleFunc("/channels/{id}/file", methodNotAllowed)
	mux.HandleFunc("GET /messages", s.listMessages)
	mux.HandleFunc("POST /messages", s.createMessage)
	mux.HandleFunc("/messages", methodNotAllowed)
	mux.HandleFunc("/messages/{id}", methodNotAllowed)
}

func (s *server) listDirectMessages(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	after, err := queryID(r, "after")
	if err != nil {
		writeError(w, err)
		return
	}
	var afterID int64
	if after != nil {
		afterID = *after
	}
	msgs, err := s.st.DirectMessages(r.Context(), id, afterID)
	if err != nil {
		writeError(w, err)
		return
	}
	if msgs == nil {
		msgs = []store.DirectMessage{}
	}
	writeJSON(w, http.StatusOK, msgs)
}

// getChannelFile returns the text of a file inside the channel's working folder: its
// feature's worktree when it exists, else the project folder.
func (s *server) getChannelFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	p := r.URL.Query().Get("path")
	if p == "" {
		writeError(w, fmt.Errorf("%w: path is required", store.ErrInvalid))
		return
	}
	if strings.ContainsRune(p, 0) {
		writeError(w, fmt.Errorf("%w: path has a NUL byte", store.ErrInvalid))
		return
	}
	dir, err := channel.FileDir(r.Context(), s.st, s.dataDir, id)
	if err != nil {
		writeError(w, err)
		return
	}
	realDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		writeError(w, fmt.Errorf("channel %d folder: %w", id, err))
		return
	}
	outside := fmt.Errorf("%w: path is outside the channel's folder", store.ErrInvalid)
	rel := filepath.Clean(p)
	if filepath.IsAbs(rel) {
		switch {
		case within(dir, rel):
			rel, _ = filepath.Rel(dir, rel)
		case within(realDir, rel):
			rel, _ = filepath.Rel(realDir, rel)
		default:
			writeError(w, outside)
			return
		}
	}
	root, err := os.OpenRoot(realDir)
	if err != nil {
		writeError(w, fmt.Errorf("channel %d folder: %w", id, err))
		return
	}
	defer func() { _ = root.Close() }()
	const flag = os.O_RDONLY | syscall.O_NONBLOCK
	f, err := root.OpenFile(rel, flag, 0)
	if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
		writeError(w, fmt.Errorf("file: %w", store.ErrNotFound))
		return
	}
	if err != nil {
		// os.Root refuses an absolute symlink even when it points inside the root.
		target, terr := filepath.EvalSymlinks(filepath.Join(realDir, rel))
		if terr != nil || !within(realDir, target) {
			writeError(w, outside)
			return
		}
		target, _ = filepath.Rel(realDir, target)
		if f, err = root.OpenFile(target, flag, 0); err != nil {
			writeError(w, fmt.Errorf("open channel %d file: %w", id, err))
			return
		}
	}
	defer func() { _ = f.Close() }()
	fi, err := f.Stat()
	if err != nil {
		writeError(w, fmt.Errorf("stat channel %d file: %w", id, err))
		return
	}
	if !fi.Mode().IsRegular() {
		writeError(w, fmt.Errorf("%w: path is not a regular file", store.ErrInvalid))
		return
	}
	data, err := io.ReadAll(io.LimitReader(f, _fileCap+1))
	if err != nil {
		writeError(w, fmt.Errorf("read channel %d file: %w", id, err))
		return
	}
	if bytes.IndexByte(data[:min(len(data), 8<<10)], 0) >= 0 {
		writeJSON(w, http.StatusUnsupportedMediaType, map[string]string{"error": "binary file"})
		return
	}
	truncated := len(data) > _fileCap
	if truncated {
		data = data[:_fileCap]
		i := len(data) - 1
		for i > 0 && i > len(data)-utf8.UTFMax && !utf8.RuneStart(data[i]) {
			i--
		}
		if !utf8.FullRune(data[i:]) {
			data = data[:i]
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"path": filepath.ToSlash(rel), "content": string(data), "size": fi.Size(),
		"truncated": truncated,
	})
}

func (s *server) listChannels(w http.ResponseWriter, r *http.Request) {
	var f store.ChannelFilter
	var err error
	if f.ProjectID, err = queryID(r, "project"); err != nil {
		writeError(w, err)
		return
	}
	if f.FeatureID, err = queryID(r, "feature"); err != nil {
		writeError(w, err)
		return
	}
	cs, err := s.st.ChannelPreviews(r.Context(), f)
	if err != nil {
		writeError(w, err)
		return
	}
	if cs == nil {
		cs = []store.ChannelPreview{}
	}
	writeJSON(w, http.StatusOK, cs)
}

func (s *server) createChannel(w http.ResponseWriter, r *http.Request) {
	var in store.Channel
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	if in.Role == "" {
		in.Role = "lead"
	}
	if in.Role != "lead" {
		writeError(w, fmt.Errorf("%w: role must be lead", store.ErrInvalid))
		return
	}
	if in.Team == "" {
		in.Team = store.SoloTeam
	}
	team, err := s.st.Team(r.Context(), in.Team)
	if errors.Is(err, store.ErrNotFound) {
		err = fmt.Errorf("%w: unknown team %s", store.ErrInvalid, in.Team)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	lead := team.Roles[0]
	in.Role = lead.Name
	if in.Backend == "" {
		in.Backend = lead.Backend
	}
	if in.PermissionMode == "" && in.Backend == lead.Backend {
		in.PermissionMode = lead.PermissionMode
	}
	if err := store.ValidateAgent(in.Backend, in.PermissionMode); err != nil {
		writeError(w, err)
		return
	}
	c, err := s.st.CreateChannel(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, c)
}

func (s *server) getChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	c, err := s.st.Channel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) updateChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Name           string  `json:"name"`
		PermissionMode *string `json:"permissionMode"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	current, err := s.st.Channel(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if in.PermissionMode != nil {
		current.PermissionMode = *in.PermissionMode
		if err := store.ValidateAgent(current.Backend, current.PermissionMode); err != nil {
			writeError(w, err)
			return
		}
	}
	current.Name = in.Name
	c, err := s.st.UpdateChannel(r.Context(), current)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, c)
}

func (s *server) deleteChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	defer s.stopForDelete(id)()
	if err := s.st.DeleteChannel(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *server) reopenChannel(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.ReopenChannel(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// stopChannels stops the agent of every channel f selects, as stopForDelete does. The
// caller calls the returned func once the store delete is done.
func (s *server) stopChannels(ctx context.Context, f store.ChannelFilter) (func(), error) {
	cs, err := s.st.Channels(ctx, f)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(cs))
	for i, c := range cs {
		ids[i] = c.ID
	}
	return s.stopForDelete(ids...), nil
}

// stopForDelete marks the channels as being deleted, so a send to them is refused with
// 404, stops their agents and drops their idle entries. The caller calls the returned
// func once the store delete is done, which lifts the marks.
func (s *server) stopForDelete(ids ...int64) (lift func()) {
	s.mu.Lock()
	for _, id := range ids {
		s.deleting[id]++
	}
	s.mu.Unlock()
	for _, id := range ids {
		if l := s.lookup(id); l != nil {
			s.stopLive(l)
			s.dropIdle(id, l, true)
		}
	}
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		for _, id := range ids {
			if s.deleting[id]--; s.deleting[id] == 0 {
				delete(s.deleting, id)
			}
		}
	}
}

func (s *server) listMessages(w http.ResponseWriter, r *http.Request) {
	channelID, err := queryID(r, "channel")
	if err != nil {
		writeError(w, err)
		return
	}
	if channelID == nil {
		writeError(w, fmt.Errorf("%w: channel is required", store.ErrInvalid))
		return
	}
	after, err := queryID(r, "after")
	if err != nil {
		writeError(w, err)
		return
	}
	var afterID int64
	if after != nil {
		afterID = *after
	}
	ms, err := s.st.Messages(r.Context(), *channelID, afterID)
	if err != nil {
		writeError(w, err)
		return
	}
	if ms == nil {
		ms = []store.Message{}
	}
	writeJSON(w, http.StatusOK, ms)
}

func (s *server) createMessage(w http.ResponseWriter, r *http.Request) {
	var in store.Message
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	m, err := s.st.CreateMessage(r.Context(), in)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, m)
}

// within reports whether path is dir or inside it. Both must be clean.
func within(dir, path string) bool {
	return path == dir || strings.HasPrefix(path, dir+string(filepath.Separator))
}
