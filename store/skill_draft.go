package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/bhanuprakaash/reef/skill"
)

// SkillDraftFile is one file of a skill a Bot proposed.
type SkillDraftFile struct {
	Path string `json:"path"`
	Body string `json:"body"`
}

// SkillDraft is a skill a Bot proposed in a Space. It stays out of the library, and out of every
// Bot's skills, until the user saves it. Status is pending, saved or rejected.
type SkillDraft struct {
	ID        int64            `json:"id"`
	SpaceID   int64            `json:"spaceId"`
	BotID     int64            `json:"botId"`
	Name      string           `json:"name"`
	Files     []SkillDraftFile `json:"files"`
	Status    string           `json:"status"`
	SkillID   *int64           `json:"skillId"`
	CreatedAt time.Time        `json:"createdAt"`
}

const _skillDraftCols = "id, space_id, bot_id, name, files, status, skill_id, created_at"

// CreateSkillDraft stores p as a pending draft proposed by the Bot in the Space.
func (s *Store) CreateSkillDraft(ctx context.Context, spaceID, botID int64, p skill.Package) (SkillDraft, error) {
	files := make([]SkillDraftFile, 0, len(p.Files))
	for _, f := range p.Files {
		files = append(files, SkillDraftFile{f.Path, string(f.Body)})
	}
	encoded, err := json.Marshal(files)
	if err != nil {
		return SkillDraft{}, fmt.Errorf("encode skill draft files: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		"INSERT INTO skill_draft (space_id, bot_id, name, files, created_at) VALUES (?, ?, ?, ?, ?)",
		spaceID, botID, p.Name, string(encoded), time.Now().UTC().Format(time.RFC3339Nano))
	if err != nil {
		return SkillDraft{}, fmt.Errorf("insert skill draft: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return SkillDraft{}, fmt.Errorf("skill draft id: %w", err)
	}
	return s.SkillDraft(ctx, id)
}

// SkillDraft returns the draft with this id.
func (s *Store) SkillDraft(ctx context.Context, id int64) (SkillDraft, error) {
	d, err := scanSkillDraft(s.db.QueryRowContext(ctx, "SELECT "+_skillDraftCols+" FROM skill_draft WHERE id = ?", id))
	if errors.Is(err, sql.ErrNoRows) {
		return SkillDraft{}, fmt.Errorf("skill draft %d: %w", id, ErrNotFound)
	}
	return d, err
}

// PendingSkillDrafts returns the drafts nobody has decided yet, newest first; spaceID, when not
// 0, keeps those proposed in that Space.
func (s *Store) PendingSkillDrafts(ctx context.Context, spaceID int64) ([]SkillDraft, error) {
	rows, err := s.db.QueryContext(ctx,
		"SELECT "+_skillDraftCols+" FROM skill_draft WHERE status = 'pending' AND (? = 0 OR space_id = ?) ORDER BY id DESC",
		spaceID, spaceID)
	if err != nil {
		return nil, fmt.Errorf("list skill drafts: %w", err)
	}
	return scanAll(rows, scanSkillDraft)
}

// SaveSkillDraft puts a pending draft in the library: as a new skill, or as the next version
// of a skill written in Reef when update is true and the name is taken. A name that is taken
// while update is false, or by an imported skill, is ErrConflict. Only this skill changes; no
// Bot's skills do.
func (s *Store) SaveSkillDraft(ctx context.Context, id int64, update bool) (Skill, error) {
	d, err := s.SkillDraft(ctx, id)
	if err != nil {
		return Skill{}, err
	}
	if d.Status != "pending" {
		return Skill{}, fmt.Errorf("%w: skill draft %d is already %s", ErrConflict, id, d.Status)
	}
	files := make([]skill.File, 0, len(d.Files))
	for _, f := range d.Files {
		files = append(files, skill.File{Path: f.Path, Body: []byte(f.Body)})
	}
	p, err := skill.Build(d.Name, files)
	if err != nil {
		return Skill{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	library, err := s.Skills(ctx)
	if err != nil {
		return Skill{}, err
	}
	var existing Skill
	if i := slices.IndexFunc(library, func(k Skill) bool { return k.Name == d.Name }); i >= 0 {
		existing = library[i]
	}
	var saved Skill
	switch {
	case existing.ID == 0:
		saved, err = s.ImportSkill(ctx, "reef", "", "", "", p)
	case !update:
		return Skill{}, fmt.Errorf("%w: a skill named %q is already in the library; save it as a new version instead", ErrConflict, d.Name)
	case existing.SourceKind != "reef":
		return Skill{}, fmt.Errorf("%w: %q was imported; update it from its source", ErrConflict, d.Name)
	default:
		saved, err = s.UpdateSkill(ctx, existing.ID, p)
	}
	if err != nil {
		return Skill{}, err
	}
	if err := s.decideSkillDraft(ctx, id, "saved", &saved.ID); err != nil {
		return Skill{}, err
	}
	return saved, nil
}

// RejectSkillDraft marks a pending draft rejected. Nothing is saved.
func (s *Store) RejectSkillDraft(ctx context.Context, id int64) error {
	return s.decideSkillDraft(ctx, id, "rejected", nil)
}

func (s *Store) decideSkillDraft(ctx context.Context, id int64, status string, skillID *int64) error {
	res, err := s.db.ExecContext(ctx,
		"UPDATE skill_draft SET status = ?, skill_id = ?, decided_at = ? WHERE id = ? AND status = 'pending'",
		status, skillID, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		return fmt.Errorf("decide skill draft: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return fmt.Errorf("decide skill draft: %w", err)
	} else if n == 0 {
		if _, err := s.SkillDraft(ctx, id); err != nil {
			return err
		}
		return fmt.Errorf("%w: skill draft %d is already decided", ErrConflict, id)
	}
	return nil
}

func scanSkillDraft(r interface{ Scan(...any) error }) (SkillDraft, error) {
	var d SkillDraft
	var files, created string
	if err := r.Scan(&d.ID, &d.SpaceID, &d.BotID, &d.Name, &files, &d.Status, &d.SkillID, &created); err != nil {
		return SkillDraft{}, err
	}
	if err := json.Unmarshal([]byte(files), &d.Files); err != nil {
		return SkillDraft{}, fmt.Errorf("skill draft %d: bad files: %w", d.ID, err)
	}
	var err error
	if d.CreatedAt, err = time.Parse(time.RFC3339Nano, created); err != nil {
		return SkillDraft{}, fmt.Errorf("skill draft %d: bad created_at: %w", d.ID, err)
	}
	return d, nil
}
