package graphgloss

import (
	_ "embed"
	"os"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/srhopkins/iugum/contract"
)

//go:embed defaults.yaml
var defaultsYAML []byte

// File is a closed list of node kinds and relation words.
// Edit the YAML to add or remove a split. Same shape as glossaries/iugum.yaml.
type File struct {
	Name  string `yaml:"name"`
	Kinds []Term `yaml:"kinds"`
	Rels  []Term `yaml:"rels"`
}

// Term is one allowed word. Inflections are extra spellings of the same rel.
type Term struct {
	Word          string   `yaml:"word"`
	Meaning       string   `yaml:"approved_meaning"`
	Inflections   []string `yaml:"inflections"`
}

type relPat struct {
	canon string
	word  string
	re    *regexp.Regexp
}

var (
	sentSplit = regexp.MustCompile(`[.!?;\n]+`)
	articles  = regexp.MustCompile(`(?i)^(the|a|an)\s+`)
	copula    = regexp.MustCompile(`(?i)\s+(is|are|was|were|be)$`)
	slugBad   = regexp.MustCompile(`[^a-z0-9]+`)
)

func Default() File {
	f, err := Parse(defaultsYAML)
	if err != nil {
		return File{Name: "memory-graph"}
	}
	return f
}

func Load(path string) (File, error) {
	if path == "" {
		return Default(), nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Default(), nil
		}
		return File{}, err
	}
	return Parse(raw)
}

func Parse(raw []byte) (File, error) {
	var f File
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return File{}, err
	}
	return f, nil
}

func (f File) AllowedRel(rel string) bool {
	_, ok := f.CanonRel(rel)
	return ok
}

// CanonRel maps an inflection to the glossary word.
func (f File) CanonRel(rel string) (string, bool) {
	return f.canonRel(rel)
}

func (f File) canonRel(rel string) (string, bool) {
	r := strings.ToLower(strings.TrimSpace(rel))
	if r == "" {
		return "", false
	}
	for _, t := range f.Rels {
		if strings.ToLower(t.Word) == r {
			return t.Word, true
		}
		for _, inf := range t.Inflections {
			if strings.ToLower(inf) == r {
				return t.Word, true
			}
		}
	}
	return "", false
}

// Extract splits text on glossary rels: LEFT <rel> RIGHT.
// It does not call a model. Add a rel in YAML to teach a new split.
func (f File) Extract(text string) []contract.MemoryEdge {
	pats := f.relPats()
	var out []contract.MemoryEdge
	for _, sent := range sentSplit.Split(text, -1) {
		sent = strings.TrimSpace(sent)
		if sent == "" {
			continue
		}
		for _, p := range pats {
			m := p.re.FindStringSubmatch(sent)
			if m == nil {
				continue
			}
			from := slug(m[1])
			to := slug(m[2])
			if from == "" || to == "" || from == to {
				continue
			}
			out = append(out, contract.MemoryEdge{From: from, Rel: p.canon, To: to, Value: sent})
			break
		}
	}
	return out
}

func (f File) relPats() []relPat {
	type pair struct {
		canon, word string
	}
	var pairs []pair
	for _, t := range f.Rels {
		pairs = append(pairs, pair{t.Word, t.Word})
		for _, inf := range t.Inflections {
			pairs = append(pairs, pair{t.Word, inf})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		return len(pairs[i].word) > len(pairs[j].word)
	})
	out := make([]relPat, 0, len(pairs))
	for _, p := range pairs {
		w := regexp.QuoteMeta(p.word)
		re := regexp.MustCompile(`(?i)^(.+?)\s+` + w + `\s+(.+)$`)
		out = append(out, relPat{canon: p.canon, word: p.word, re: re})
	}
	return out
}

func slug(s string) string {
	s = strings.TrimSpace(s)
	s = copula.ReplaceAllString(s, "")
	s = articles.ReplaceAllString(s, "")
	s = strings.ToLower(s)
	s = slugBad.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
