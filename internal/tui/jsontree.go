package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

type nodeKind int

const (
	nodeObject nodeKind = iota
	nodeArray
	nodeLeaf
)

type jsonNode struct {
	key       string
	arrayIdx  int // >= 0 for array elements, -1 otherwise
	valStr    string
	kind      nodeKind
	children  []*jsonNode
	parent    *jsonNode
	collapsed bool
}

type jsonTreeState struct {
	root   *jsonNode
	flat   []*jsonNode
	cursor int
}

func parseJSONTree(data []byte) (*jsonTreeState, error) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	switch v.(type) {
	case map[string]interface{}, []interface{}:
	default:
		return nil, fmt.Errorf("not an object or array")
	}
	root := buildJSONNode("", -1, v, nil)
	st := &jsonTreeState{root: root}
	st.rebuild()
	return st, nil
}

func buildJSONNode(key string, arrayIdx int, v interface{}, parent *jsonNode) *jsonNode {
	n := &jsonNode{key: key, arrayIdx: arrayIdx, parent: parent}
	switch val := v.(type) {
	case map[string]interface{}:
		n.kind = nodeObject
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			n.children = append(n.children, buildJSONNode(k, -1, val[k], n))
		}
	case []interface{}:
		n.kind = nodeArray
		for i, child := range val {
			n.children = append(n.children, buildJSONNode("", i, child, n))
		}
	default:
		n.kind = nodeLeaf
		b, _ := json.Marshal(v)
		n.valStr = string(b)
	}
	return n
}

func (s *jsonTreeState) rebuild() {
	s.flat = s.flat[:0]
	s.collectVisible(s.root)
}

func (s *jsonTreeState) collectVisible(n *jsonNode) {
	s.flat = append(s.flat, n)
	if n.collapsed {
		return
	}
	for _, c := range n.children {
		s.collectVisible(c)
	}
}

func (s *jsonTreeState) toggle() {
	if s.cursor >= len(s.flat) {
		return
	}
	n := s.flat[s.cursor]
	if n.kind == nodeLeaf {
		return
	}
	n.collapsed = !n.collapsed
	s.rebuild()
	if s.cursor >= len(s.flat) {
		s.cursor = len(s.flat) - 1
	}
}

func (s *jsonTreeState) collapseAll() {
	s.setCollapsed(s.root, true)
	s.root.collapsed = false
	s.rebuild()
	s.cursor = 0
}

func (s *jsonTreeState) expandAll() {
	saved := s.currentNode()
	s.setCollapsed(s.root, false)
	s.rebuild()
	if saved != nil {
		for i, n := range s.flat {
			if n == saved {
				s.cursor = i
				return
			}
		}
	}
	if s.cursor >= len(s.flat) {
		s.cursor = len(s.flat) - 1
	}
}

func (s *jsonTreeState) currentNode() *jsonNode {
	if s.cursor < len(s.flat) {
		return s.flat[s.cursor]
	}
	return nil
}

func (s *jsonTreeState) setCollapsed(n *jsonNode, v bool) {
	if n.kind != nodeLeaf {
		n.collapsed = v
	}
	for _, c := range n.children {
		s.setCollapsed(c, v)
	}
}

func (s *jsonTreeState) moveCursor(delta int) {
	s.cursor += delta
	if s.cursor < 0 {
		s.cursor = 0
	}
	if s.cursor >= len(s.flat) {
		s.cursor = len(s.flat) - 1
	}
}

func (s *jsonTreeState) jumpNextSibling() {
	if s.cursor >= len(s.flat) {
		return
	}
	n := s.flat[s.cursor]
	if n.parent == nil {
		return
	}
	depth := jsonNodeDepth(n)
	for i := s.cursor + 1; i < len(s.flat); i++ {
		d := jsonNodeDepth(s.flat[i])
		if d < depth {
			break
		}
		if d == depth && s.flat[i].parent == n.parent {
			s.cursor = i
			return
		}
	}
}

func (s *jsonTreeState) jumpPrevSibling() {
	if s.cursor >= len(s.flat) {
		return
	}
	n := s.flat[s.cursor]
	if n.parent == nil {
		return
	}
	depth := jsonNodeDepth(n)
	for i := s.cursor - 1; i >= 0; i-- {
		d := jsonNodeDepth(s.flat[i])
		if d < depth {
			break
		}
		if d == depth && s.flat[i].parent == n.parent {
			s.cursor = i
			return
		}
	}
}

func jsonNodeDepth(n *jsonNode) int {
	d := 0
	for n.parent != nil {
		d++
		n = n.parent
	}
	return d
}

func (s *jsonTreeState) render(width int) string {
	accentSt := lipgloss.NewStyle().Foreground(colorAccent)
	mutedSt := lipgloss.NewStyle().Foreground(colorMuted)
	cursorSt := lipgloss.NewStyle().Foreground(colorAccent).Bold(true)

	var sb strings.Builder
	for i, n := range s.flat {
		depth := jsonNodeDepth(n)
		indent := strings.Repeat("  ", depth)

		var pfx string
		if i == s.cursor {
			pfx = cursorSt.Render(">") + " "
		} else {
			pfx = "  "
		}

		var keyPart string
		switch {
		case n.key != "":
			keyPart = accentSt.Render(`"`+n.key+`"`) + ": "
		case n.arrayIdx >= 0:
			keyPart = mutedSt.Render(fmt.Sprintf("[%d]", n.arrayIdx)) + " "
		}

		var body string
		switch n.kind {
		case nodeLeaf:
			body = colorJSONValue(n.valStr)
		case nodeObject:
			if n.collapsed {
				body = mutedSt.Render(fmt.Sprintf("{ %d }", len(n.children)))
			} else {
				body = "{"
			}
		case nodeArray:
			if n.collapsed {
				body = mutedSt.Render(fmt.Sprintf("[ %d ]", len(n.children)))
			} else {
				body = "["
			}
		}

		sb.WriteString(pfx + indent + keyPart + body + "\n")
	}
	_ = width
	return strings.TrimRight(sb.String(), "\n")
}

func colorJSONValue(v string) string {
	mutedSt := lipgloss.NewStyle().Foreground(colorMuted)
	successSt := lipgloss.NewStyle().Foreground(colorSuccess)
	warnSt := lipgloss.NewStyle().Foreground(colorWarn)
	subtleSt := lipgloss.NewStyle().Foreground(colorSubtle)

	switch {
	case v == "null":
		return mutedSt.Render(v)
	case v == "true" || v == "false":
		return subtleSt.Render(v)
	case len(v) > 0 && v[0] == '"':
		return successSt.Render(v)
	default:
		return warnSt.Render(v)
	}
}
