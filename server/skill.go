package server

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/store"
)

func (s *server) skillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /skills", s.listSkills)
	mux.HandleFunc("/skills", methodNotAllowed)
	mux.HandleFunc("GET /skills/library", s.skillLibrary)
	mux.HandleFunc("POST /skills/import", s.importSkill)
	mux.HandleFunc("GET /skills/{id}", s.getSkill)
	mux.HandleFunc("POST /skills/{id}/update", s.updateSkill)
	mux.HandleFunc("/skills/{id}/update", methodNotAllowed)
}

// skillEntry is one pickable skill. Ref is "reef:name", "claude:name" or "codex:name"; Version is
// the library version of a Reef skill.
type skillEntry struct {
	Ref         string `json:"ref"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Source      string `json:"source"`
	Version     int    `json:"version,omitempty"`
}

type skillFileView struct {
	Path string `json:"path"`
	Body string `json:"body"`
}

type skillView struct {
	store.Skill
	Files []skillFileView `json:"files"`
}

func (s *server) listSkills(w http.ResponseWriter, r *http.Request) {
	id, err := queryID(r, "project")
	if err == nil && id == nil {
		err = fmt.Errorf("%w: project is required", store.ErrInvalid)
	}
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := s.st.Project(r.Context(), *id)
	if err != nil {
		writeError(w, err)
		return
	}
	home, err := os.UserHomeDir()
	if err != nil {
		writeError(w, fmt.Errorf("find home folder: %w", err))
		return
	}
	library, err := s.st.Skills(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	entries := []skillEntry{}
	for _, k := range library {
		entries = append(entries, skillEntry{"reef:" + k.Name, k.Name, k.Description, "reef", k.Version})
	}
	for _, k := range skill.Claude(home, p.Path) {
		entries = append(entries, skillEntry{"claude:" + k.Name, k.Name, k.Description, "claude", 0})
	}
	for _, k := range skill.Codex(home, p.Path) {
		entries = append(entries, skillEntry{"codex:" + k.Name, k.Name, k.Description, "codex", 0})
	}
	writeJSON(w, http.StatusOK, entries)
}

func (s *server) skillLibrary(w http.ResponseWriter, r *http.Request) {
	library, err := s.st.Skills(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	if library == nil {
		library = []store.Skill{}
	}
	writeJSON(w, http.StatusOK, library)
}

func (s *server) importSkill(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Path string `json:"path"`
		Repo string `json:"repo"`
		Ref  string `json:"ref"`
		Dir  string `json:"dir"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	var kind, source string
	switch {
	case in.Path != "" && in.Repo == "":
		kind, source = "folder", in.Path
	case in.Repo != "" && in.Path == "":
		kind, source = "git", in.Repo
	default:
		writeError(w, fmt.Errorf("%w: give a path or a repo", store.ErrInvalid))
		return
	}
	p, err := readSkillPackage(r.Context(), kind, source, in.Ref, in.Dir)
	if err != nil {
		writeError(w, err)
		return
	}
	k, err := s.st.ImportSkill(r.Context(), kind, source, in.Ref, in.Dir, p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

func (s *server) getSkill(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	k, err := s.st.Skill(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	files, err := s.st.SkillFiles(r.Context(), id, k.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	view := skillView{Skill: k, Files: make([]skillFileView, 0, len(files))}
	for _, f := range files {
		view.Files = append(view.Files, skillFileView{f.Path, string(f.Body)})
	}
	writeJSON(w, http.StatusOK, view)
}

func (s *server) updateSkill(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	k, err := s.st.Skill(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	p, err := readSkillPackage(r.Context(), k.SourceKind, k.Source, k.Ref, k.Dir)
	if err != nil {
		writeError(w, err)
		return
	}
	k, err = s.st.UpdateSkill(r.Context(), id, p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

// readSkillPackage turns a package that cannot be read into ErrInvalid, so the caller sees why.
func readSkillPackage(ctx context.Context, kind, source, ref, dir string) (skill.Package, error) {
	var p skill.Package
	var err error
	if kind == "git" {
		p, err = skill.ReadRepo(ctx, source, ref, dir)
	} else {
		p, err = skill.ReadFolder(source)
	}
	if err != nil {
		return skill.Package{}, fmt.Errorf("%w: %v", store.ErrInvalid, err)
	}
	return p, nil
}
