// Package skills serves the in-app prompt library: the agent playbooks in
// docs/skills, embedded into the binary (P2 #17). files/ is a copy of
// docs/skills (the Docker build context is backend/); a test keeps them equal.
package skills

import (
	"embed"
	"io/fs"
	"regexp"
	"sort"
	"strings"
)

//go:embed files/*.md
var files embed.FS

// Skill is one prompt-library entry.
type Skill struct {
	Name        string   `json:"name"`
	Title       string   `json:"title"`
	Summary     string   `json:"summary"`
	Tools       []string `json:"tools"`
	Schedulable string   `json:"schedulable,omitempty"`
	Prompt      string   `json:"prompt"`
	Content     string   `json:"content"` // full markdown
}

// List returns every skill (README excluded), sorted by name.
func List() ([]Skill, error) {
	paths, err := fs.Glob(files, "files/*.md")
	if err != nil {
		return nil, err
	}
	out := []Skill{}
	for _, p := range paths {
		if strings.EqualFold(p, "files/README.md") {
			continue
		}
		b, err := files.ReadFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, parse(strings.TrimPrefix(p, "files/"), string(b)))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

var backticked = regexp.MustCompile("`([^`]+)`")

// parse reads the fixed skill layout: "# Skill: name", a one-line summary,
// "**Tools:**" / "**Schedulable:**" lines and the first fenced block as the prompt.
func parse(file, md string) Skill {
	s := Skill{Name: strings.TrimSuffix(file, ".md"), Content: md, Tools: []string{}}
	lines := strings.Split(md, "\n")
	for i := 0; i < len(lines); i++ {
		l := strings.TrimSpace(lines[i])
		switch {
		case strings.HasPrefix(l, "# ") && s.Title == "":
			s.Title = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(l, "# "), "Skill:"))
		case strings.HasPrefix(l, "**Tools:**"):
			for _, m := range backticked.FindAllStringSubmatch(l, -1) {
				s.Tools = append(s.Tools, m[1])
			}
		case strings.HasPrefix(l, "**Schedulable:**"):
			s.Schedulable = strings.TrimSpace(strings.TrimPrefix(l, "**Schedulable:**"))
		case strings.HasPrefix(l, "```") && s.Prompt == "":
			var body []string
			for i++; i < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[i]), "```"); i++ {
				body = append(body, lines[i])
			}
			s.Prompt = strings.Join(body, "\n")
		case l != "" && s.Summary == "" && s.Title != "" && !strings.HasPrefix(l, "#") && !strings.HasPrefix(l, "**"):
			s.Summary = l
		}
	}
	return s
}
