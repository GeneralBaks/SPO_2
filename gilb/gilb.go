package gilb

import (
	"crypto/sha1"
	"encoding/hex"
	"sort"
	"strings"
	"sync"
	"unicode"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/scala"
)

type Entry struct {
	Name  string
	Count int
}

type Branch struct {
	Line         int
	Kind         string
	Text         string
	Contribution int
	Level        int
}

type Result struct {
	CL           int
	NOps         int
	Cl           float64
	CLI          int
	Operators    []Entry
	Branches     []Branch
	Lines        int
	SyntaxErrors int
}

var wordOps = map[string]bool{
	"to":    true,
	"until": true,
	"max":   true,
	"min":   true,
}

var (
	cacheMu sync.Mutex
	cache   = map[string]*Result{}
)

func AnalyzeCached(source string) *Result {
	sum := sha1.Sum([]byte(source))
	key := hex.EncodeToString(sum[:])

	cacheMu.Lock()
	if r, ok := cache[key]; ok {
		cacheMu.Unlock()
		return r
	}
	cacheMu.Unlock()

	r := Analyze(source)

	cacheMu.Lock()
	cache[key] = r
	cacheMu.Unlock()
	return r
}

func Analyze(source string) *Result {
	src := []byte(source)

	parser := sitter.NewParser()
	defer parser.Close()
	parser.SetLanguage(scala.GetLanguage())

	tree := parser.Parse(nil, src)
	defer tree.Close()
	root := tree.RootNode()

	w := &walker{src: src, ops: map[string]int{}}
	w.walk(root, 0)

	res := &Result{
		CL:           w.cl,
		CLI:          w.maxLevel,
		Branches:     w.branches,
		SyntaxErrors: w.errors,
	}
	if source != "" {
		res.Lines = strings.Count(source, "\n") + 1
	}
	if root.HasError() && res.SyntaxErrors == 0 {
		res.SyntaxErrors = 1
	}
	for k, v := range w.ops {
		res.Operators = append(res.Operators, Entry{k, v})
		res.NOps += v
	}
	sort.Slice(res.Operators, func(i, j int) bool {
		a, b := res.Operators[i], res.Operators[j]
		if a.Count != b.Count {
			return a.Count > b.Count
		}
		return a.Name < b.Name
	})
	if res.NOps > 0 {
		res.Cl = float64(res.CL) / float64(res.NOps)
	}
	return res
}

type walker struct {
	src      []byte
	ops      map[string]int
	cl       int
	maxLevel int
	errors   int
	branches []Branch
}

func (w *walker) text(n *sitter.Node) string { return n.Content(w.src) }
func (w *walker) add(op string)              { w.ops[op]++ }

func (w *walker) record(kind string, n *sitter.Node, contribution, level int) {
	w.cl += contribution
	if level > w.maxLevel {
		w.maxLevel = level
	}
	w.branches = append(w.branches, Branch{
		Line:         int(n.StartPoint().Row) + 1,
		Kind:         kind,
		Text:         firstLine(w.text(n)),
		Contribution: contribution,
		Level:        level,
	})
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " …"
	}
	if r := []rune(s); len(r) > 80 {
		s = string(r[:80]) + "…"
	}
	return s
}

func sameNode(a, b *sitter.Node) bool {
	return a != nil && b != nil &&
		a.StartByte() == b.StartByte() && a.EndByte() == b.EndByte() && a.Type() == b.Type()
}

func hasTok(n *sitter.Node, tok string) bool {
	for i := 0; i < int(n.ChildCount()); i++ {
		if c := n.Child(i); c != nil && !c.IsNamed() && c.Type() == tok {
			return true
		}
	}
	return false
}

func isWordOp(s string) bool {
	for _, r := range s {
		return unicode.IsLetter(r)
	}
	return false
}

func (w *walker) kids(n *sitter.Node, depthOf func(*sitter.Node) int) {
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c == nil || !c.IsNamed() {
			continue
		}
		w.walk(c, depthOf(c))
	}
}

func (w *walker) walk(n *sitter.Node, depth int) {
	if n == nil {
		return
	}
	same := func(*sitter.Node) int { return depth }
	deeper := func(*sitter.Node) int { return depth + 1 }

	switch n.Type() {
	case "comment", "block_comment":
		return
	case "ERROR":
		w.errors++

	case "if_expression":
		w.add("if")
		w.record("if", n, 1, depth)
		cond := n.ChildByFieldName("condition")
		w.kids(n, func(c *sitter.Node) int {
			if sameNode(c, cond) {
				return depth
			}
			return depth + 1
		})
		return
	case "while_expression":
		w.add("while")
		w.record("while", n, 1, depth)
		w.kids(n, deeper)
		return
	case "do_while_expression":
		w.add("do...while")
		w.record("do-while", n, 1, depth)
		w.kids(n, deeper)
		return
	case "for_expression":
		w.add("for")
		w.record("for", n, 1, depth)
		w.kids(n, deeper)
		return
	case "match_expression":
		w.match(n, depth)
		return

	case "val_definition", "var_definition":
		if hasTok(n, "=") {
			w.add("=")
		}
	case "return_expression":
		w.add("return")
	case "throw_expression":
		w.add("throw")
	case "assignment_expression":
		if p := n.Parent(); p == nil || p.Type() != "arguments" {
			w.add("=")
		}
	case "infix_expression":
		op := n.ChildByFieldName("operator")
		if op == nil && n.NamedChildCount() >= 3 {
			op = n.NamedChild(1)
		}
		if op != nil {
			name := w.text(op)
			if !isWordOp(name) || wordOps[name] {
				w.add(name)
			}
		}
	case "prefix_expression":
		if c := n.Child(0); c != nil {
			w.add(w.text(c))
		}
	case "alternative_pattern":
		for i := 0; i < int(n.ChildCount()); i++ {
			if c := n.Child(i); c != nil && !c.IsNamed() && c.Type() == "|" {
				w.add("|")
			}
		}
	}

	w.kids(n, same)
}

func isDefaultCase(c *sitter.Node, src []byte) bool {
	for i := 0; i < int(c.NamedChildCount()); i++ {
		if c.NamedChild(i).Type() == "guard" {
			return false
		}
	}
	p := c.ChildByFieldName("pattern")
	if p == nil {
		p = c.NamedChild(0)
	}
	return p != nil && strings.TrimSpace(p.Content(src)) == "_"
}

func (w *walker) match(n *sitter.Node, depth int) {
	w.add("match")
	var clauses []*sitter.Node
	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c == nil {
			continue
		}
		switch c.Type() {
		case "case_clause":
			clauses = append(clauses, c)
		case "case_block":
			for j := 0; j < int(c.ChildCount()); j++ {
				if cc := c.Child(j); cc != nil && cc.Type() == "case_clause" {
					clauses = append(clauses, cc)
				}
			}
		}
	}

	ordinary := 0
	for _, c := range clauses {
		if !isDefaultCase(c, w.src) {
			ordinary++
		}
	}

	for i := 0; i < int(n.ChildCount()); i++ {
		c := n.Child(i)
		if c != nil && c.IsNamed() && c.Type() != "case_block" && c.Type() != "case_clause" {
			w.walk(c, depth)
		}
	}

	if ordinary > 0 {
		w.record("match", n, ordinary, depth+ordinary-1)
	}

	k := 0
	for _, c := range clauses {
		var headDepth, bodyDepth int
		if isDefaultCase(c, w.src) {
			headDepth = depth + maxInt(ordinary-1, 0)
			bodyDepth = depth + maxInt(ordinary, 1)
		} else {
			headDepth = depth + k
			bodyDepth = depth + k + 1
			k++
		}
		body := c.ChildByFieldName("body")
		if body == nil && c.NamedChildCount() > 0 {
			body = c.NamedChild(int(c.NamedChildCount()) - 1)
		}
		w.kids(c, func(ch *sitter.Node) int {
			if sameNode(ch, body) {
				return bodyDepth
			}
			return headDepth
		})
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
