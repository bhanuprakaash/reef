// Package skill lists the skills a backend finds on disk. Reef keeps no copy of them.
package skill

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
)

// Skill is one SKILL.md: its name and the description from its frontmatter.
type Skill struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

var _name = regexp.MustCompile(`^[A-Za-z0-9:_-]+$`)

// ValidName reports whether name is safe to put in a skill permission rule: letters, digits,
// ":", "_" and "-".
func ValidName(name string) bool { return _name.MatchString(name) }

// Claude lists the skills claude finds when it runs in dir: the user's, the folder's and
// those of the plugins installed for the user or for dir. A plugin's skill is named
// "plugin:skill". When two skills share a name the first found wins, in that order.
func Claude(home, dir string) []Skill {
	return unique(slices.Concat(
		scan(filepath.Join(home, ".claude", "skills"), ""),
		scan(filepath.Join(dir, ".claude", "skills"), ""),
		pluginSkills(home, dir),
	))
}

// Codex lists the skills codex finds when it runs in dir: the user's and the folder's.
func Codex(home, dir string) []Skill {
	return unique(slices.Concat(
		scan(filepath.Join(home, ".agents", "skills"), ""),
		scan(filepath.Join(dir, ".agents", "skills"), ""),
	))
}

type installedPlugins struct {
	Plugins map[string][]struct {
		Scope       string `json:"scope"`
		ProjectPath string `json:"projectPath"`
		InstallPath string `json:"installPath"`
	} `json:"plugins"`
}

func pluginSkills(home, dir string) []Skill {
	path := filepath.Join(home, ".claude", "plugins", "installed_plugins.json")
	body, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	var installed installedPlugins
	if err == nil {
		err = json.Unmarshal(body, &installed)
	}
	if err != nil {
		log.Printf("skip plugin skills: installed_plugins.json: %v", err)
		return nil
	}
	var found []Skill
	for key, entries := range installed.Plugins {
		plugin, _, _ := strings.Cut(key, "@")
		for _, e := range entries {
			if e.Scope != "user" && (e.ProjectPath == "" || filepath.Clean(e.ProjectPath) != filepath.Clean(dir)) {
				continue
			}
			found = append(found, scan(filepath.Join(e.InstallPath, "skills"), plugin+":")...)
		}
	}
	return found
}

func scan(root, prefix string) []Skill {
	entries, err := os.ReadDir(root)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		log.Printf("skip skills in %s: %v", root, err)
		return nil
	}
	var found []Skill
	for _, e := range entries {
		body, err := os.ReadFile(filepath.Join(root, e.Name(), "SKILL.md"))
		if errors.Is(err, fs.ErrNotExist) || errors.Is(err, syscall.ENOTDIR) {
			continue
		}
		if err != nil {
			log.Printf("skip skill %s in %s: %v", e.Name(), root, err)
			continue
		}
		name, description := frontmatter(string(body))
		if name == "" {
			name = e.Name()
		}
		if !ValidName(prefix + name) {
			log.Printf("skip skill %s in %s: %q is not a valid skill name", e.Name(), root, prefix+name)
			continue
		}
		found = append(found, Skill{Name: prefix + name, Description: description})
	}
	return found
}

func unique(skills []Skill) []Skill {
	out := make([]Skill, 0, len(skills))
	for _, s := range skills {
		if !slices.ContainsFunc(out, func(o Skill) bool { return o.Name == s.Name }) {
			out = append(out, s)
		}
	}
	slices.SortFunc(out, func(a, b Skill) int { return strings.Compare(a.Name, b.Name) })
	return out
}

func frontmatter(body string) (name, description string) {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	if lines[0] != "---" {
		return "", ""
	}
	for i := 1; i < len(lines) && lines[i] != "---"; i++ {
		key, value, ok := strings.Cut(lines[i], ":")
		if !ok || strings.HasPrefix(key, " ") || strings.HasPrefix(key, "\t") {
			continue
		}
		value = strings.TrimSpace(value)
		if strings.HasPrefix(value, ">") || strings.HasPrefix(value, "|") {
			var block []string
			for ; i+1 < len(lines) && (lines[i+1] == "" || lines[i+1][0] == ' ' || lines[i+1][0] == '\t'); i++ {
				block = append(block, strings.TrimSpace(lines[i+1]))
			}
			value = strings.TrimSpace(strings.Join(block, " "))
		} else {
			value = scalar(value)
		}
		switch key {
		case "name":
			name = value
		case "description":
			description = value
		}
	}
	return name, description
}

func scalar(v string) string {
	if len(v) >= 2 && v[0] == '"' && v[len(v)-1] == '"' {
		if s, err := strconv.Unquote(v); err == nil {
			return s
		}
	}
	if len(v) >= 2 && v[0] == '\'' && v[len(v)-1] == '\'' {
		return strings.ReplaceAll(v[1:len(v)-1], "''", "'")
	}
	return v
}
