package server

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"

	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/store"
)

func (s *server) skillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /skills", s.listSkills)
	mux.HandleFunc("POST /skills", s.createSkill)
	mux.HandleFunc("/skills", methodNotAllowed)
	mux.HandleFunc("GET /skills/library", s.skillLibrary)
	mux.HandleFunc("POST /skills/import", s.importSkill)
	mux.HandleFunc("GET /skills/{id}", s.getSkill)
	mux.HandleFunc("PUT /skills/{id}", s.editSkill)
	mux.HandleFunc("GET /skill-drafts", s.listSkillDrafts)
	mux.HandleFunc("POST /skill-drafts/{id}/save", s.saveSkillDraft)
	mux.HandleFunc("POST /skill-drafts/{id}/reject", s.rejectSkillDraft)
	mux.HandleFunc("POST /skills/{id}/update", s.updateSkill)
	mux.HandleFunc("/skills/{id}/update", methodNotAllowed)
}

// skillEntry is one pickable skill. Ref is "reef:name", "claude:name" or "codex:name"; Version is
// the library version of a Reef skill. Compat says for each backend why it cannot use the skill,
// or "" when it can.
type skillEntry struct {
	Ref         string            `json:"ref"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Source      string            `json:"source"`
	Version     int               `json:"version,omitempty"`
	Compat      map[string]string `json:"compat"`
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
	if err != nil {
		writeError(w, err)
		return
	}
	var projectPath string
	if id != nil {
		p, err := s.st.Project(r.Context(), *id)
		if err != nil {
			writeError(w, err)
			return
		}
		projectPath = p.Path
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
		files, err := s.st.SkillFiles(r.Context(), k.ID, k.Version)
		if err != nil {
			writeError(w, err)
			return
		}
		entries = append(entries, skillEntry{Ref: "reef:" + k.Name, Name: k.Name, Description: k.Description, Source: "reef", Version: k.Version, Compat: skill.Package{Files: files}.Compat()})
	}
	for _, k := range skill.Claude(home, projectPath) {
		entries = append(entries, skillEntry{Ref: "claude:" + k.Name, Name: k.Name, Description: k.Description, Source: "claude", Compat: diskCompat(k.Dir)})
	}
	for _, k := range skill.Codex(home, projectPath) {
		entries = append(entries, skillEntry{Ref: "codex:" + k.Name, Name: k.Name, Description: k.Description, Source: "codex", Compat: diskCompat(k.Dir)})
	}
	writeJSON(w, http.StatusOK, entries)
}

// diskCompat is the compatibility of the skill folder at dir; a folder Reef cannot read is
// incompatible with both backends, for the reason it cannot be read.
func diskCompat(dir string) map[string]string {
	pkg, err := skill.ReadFolder(dir)
	if err != nil {
		return map[string]string{"claude": err.Error(), "codex": err.Error()}
	}
	return pkg.Compat()
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
		view.Files = append(view.Files, skillFileView{Path: f.Path, Body: string(f.Body)})
	}
	writeJSON(w, http.StatusOK, view)
}

// buildSkillPackage turns files that do not make a skill into ErrInvalid, so the caller sees why.
func buildSkillPackage(name string, files []skillFileView) (skill.Package, error) {
	in := make([]skill.File, 0, len(files))
	for _, f := range files {
		in = append(in, skill.File{Path: f.Path, Body: []byte(f.Body)})
	}
	p, err := skill.Build(name, in)
	if err != nil {
		return skill.Package{}, fmt.Errorf("%w: %v", store.ErrInvalid, err)
	}
	return p, nil
}

func (s *server) createSkill(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string          `json:"name"`
		Files []skillFileView `json:"files"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	p, err := buildSkillPackage(in.Name, in.Files)
	if err != nil {
		writeError(w, err)
		return
	}
	k, err := s.st.ImportSkill(r.Context(), "reef", "", "", "", p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, k)
}

// editSkill saves the files as the next version of a skill written in Reef. The name stays.
func (s *server) editSkill(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Files []skillFileView `json:"files"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	k, err := s.st.Skill(r.Context(), id)
	if err != nil {
		writeError(w, err)
		return
	}
	if k.SourceKind != "reef" {
		writeError(w, fmt.Errorf("%w: %q was imported; update it from its source", store.ErrInvalid, k.Name))
		return
	}
	p, err := buildSkillPackage(k.Name, in.Files)
	if err != nil {
		writeError(w, err)
		return
	}
	current, err := s.st.SkillFiles(r.Context(), id, k.Version)
	if err != nil {
		writeError(w, err)
		return
	}
	for i, f := range p.Files {
		p.Files[i].Executable = slices.ContainsFunc(current, func(c skill.File) bool { return c.Path == f.Path && c.Executable })
	}
	k, err = s.st.UpdateSkill(r.Context(), id, p)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *server) listSkillDrafts(w http.ResponseWriter, r *http.Request) {
	space, err := queryID(r, "space")
	if err != nil {
		writeError(w, err)
		return
	}
	var spaceID int64
	if space != nil {
		spaceID = *space
	}
	drafts, err := s.st.PendingSkillDrafts(r.Context(), spaceID)
	if err != nil {
		writeError(w, err)
		return
	}
	if drafts == nil {
		drafts = []store.SkillDraft{}
	}
	writeJSON(w, http.StatusOK, drafts)
}

func (s *server) saveSkillDraft(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	var in struct {
		Update bool `json:"update"`
	}
	if err := readJSON(r, &in); err != nil {
		writeError(w, err)
		return
	}
	k, err := s.st.SaveSkillDraft(r.Context(), id, in.Update)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, k)
}

func (s *server) rejectSkillDraft(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		writeError(w, err)
		return
	}
	if err := s.st.RejectSkillDraft(r.Context(), id); err != nil {
		writeError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
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
	if k.SourceKind == "reef" {
		writeError(w, fmt.Errorf("%w: %q was written in Reef; edit it instead", store.ErrInvalid, k.Name))
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
