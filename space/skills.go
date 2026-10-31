package space

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/bhanuprakaash/reef/skill"
	"github.com/bhanuprakaash/reef/store"
)

// BotSkillsDir is where the skills BotSession id adopted are copied, one folder per skill.
func BotSkillsDir(dataDir string, id int64) string {
	return filepath.Join(dataDir, "bot-sessions", strconv.FormatInt(id, 10), "skills")
}

// AdoptBotSession adopts the Bot's settings in the Space and copies the skills it picked into
// the new BotSession's own folder: a Reef skill at its current library version, a claude: or
// codex: skill as read from disk, which is never changed. The copies stay as they are until the
// Bot adopts again.
func AdoptBotSession(
	ctx context.Context, st *store.Store, dataDir string, spaceID, botID int64,
) (store.BotSession, error) {
	bs, err := st.AdoptBotSession(ctx, spaceID, botID)
	if err != nil {
		return store.BotSession{}, err
	}
	if len(bs.Skills) == 0 {
		return bs, nil
	}
	skills, err := copySkills(ctx, st, dataDir, spaceID, bs)
	if err == nil {
		return st.SetBotSessionSkills(ctx, bs.ID, skills)
	}
	if endErr := st.EndBotSession(ctx, bs.ID, store.BotSessionReplaced); endErr != nil {
		err = fmt.Errorf("%w; end bot session: %v", err, endErr)
	}
	return store.BotSession{}, fmt.Errorf("adopt skills: %w", err)
}

func copySkills(
	ctx context.Context, st *store.Store, dataDir string, spaceID int64, bs store.BotSession,
) ([]store.AdoptedSkill, error) {
	sp, err := st.Space(ctx, spaceID)
	if err != nil {
		return nil, err
	}
	p, err := st.Project(ctx, sp.ProjectID)
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("find home folder: %w", err)
	}
	library, err := st.Skills(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]store.AdoptedSkill, 0, len(bs.Skills))
	for _, picked := range bs.Skills {
		source, name, _ := strings.Cut(picked.Ref, ":")
		k := store.AdoptedSkill{Ref: picked.Ref}
		var pkg skill.Package
		switch source {
		case "reef":
			pkg, k.Version, err = libraryPackage(ctx, st, library, name)
		case "claude":
			pkg, err = diskPackage(skill.Claude(home, p.Path), name)
		default:
			pkg, err = diskPackage(skill.Codex(home, p.Path), name)
		}
		if err != nil {
			k.Reason = err.Error()
			out = append(out, k)
			continue
		}
		k.Description = pkg.Description
		if k.Version == "" {
			k.Version = pkg.Hash()
		}
		if k.Reason = pkg.Compat()[bs.Backend]; k.Reason == "" {
			k.Dir = filepath.Join(BotSkillsDir(dataDir, bs.ID), source, strings.ReplaceAll(name, ":", "-"))
			if err := writePackage(k.Dir, pkg); err != nil {
				return nil, err
			}
			k.Compatible = true
		}
		out = append(out, k)
	}
	return out, nil
}

func libraryPackage(
	ctx context.Context, st *store.Store, library []store.Skill, name string,
) (skill.Package, string, error) {
	i := slices.IndexFunc(library, func(k store.Skill) bool { return k.Name == name })
	if i < 0 {
		return skill.Package{}, "", fmt.Errorf("%q is not in the skill library", name)
	}
	files, err := st.SkillFiles(ctx, library[i].ID, library[i].Version)
	if err != nil {
		return skill.Package{}, "", err
	}
	pkg := skill.Package{Name: name, Description: library[i].Description, Files: files}
	return pkg, strconv.Itoa(library[i].Version), nil
}

func diskPackage(found []skill.Skill, name string) (skill.Package, error) {
	i := slices.IndexFunc(found, func(k skill.Skill) bool { return k.Name == name })
	if i < 0 {
		return skill.Package{}, fmt.Errorf("%q was not found on disk", name)
	}
	pkg, err := skill.ReadFolder(found[i].Dir)
	if err != nil {
		return skill.Package{}, err
	}
	return pkg, nil
}

func writePackage(dir string, pkg skill.Package) error {
	if err := os.RemoveAll(dir); err != nil {
		return fmt.Errorf("clear %s: %w", dir, err)
	}
	for _, f := range pkg.Files {
		path := filepath.Join(dir, filepath.FromSlash(f.Path))
		if !filepath.IsLocal(f.Path) {
			return fmt.Errorf("skill file %q leaves its folder", f.Path)
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			return fmt.Errorf("make %s: %w", filepath.Dir(path), err)
		}
		if err := os.WriteFile(path, f.Body, 0o600); err != nil {
			return fmt.Errorf("write %s: %w", path, err)
		}
	}
	return nil
}

// skillsPrompt lists the skills a BotSession adopted that its backend can use, to put in the
// session's standing instructions. A skill is a folder: the Bot reads SKILL.md when the skill applies.
func skillsPrompt(skills []store.AdoptedSkill) string {
	var b strings.Builder
	for _, k := range skills {
		if !k.Compatible {
			continue
		}
		name := k.Ref[strings.Index(k.Ref, ":")+1:]
		fmt.Fprintf(&b, "- %s: %s (%s)\n", name, k.Description, filepath.Join(k.Dir, "SKILL.md"))
	}
	if b.Len() == 0 {
		return ""
	}
	return "## Your skills\nRead a skill's SKILL.md when the skill applies; its supporting files are relative to it.\n" + b.String()
}
