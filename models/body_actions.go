package models

// Fork (integrations SAFE-ACTION-BODY-SPEC D1) -- the report of every template action in a
// campaign body that is not a plain value, decided on Go's own parse of the text the compile
// parses. Report-only: nothing here saves, starts, sends or executes a template, and nothing in
// the compile or the send path calls it. It is served by POST /api/campaigns/actions
// (cmd/body_actions.go) to the integrations campaign review, which turns a non-empty report
// into the never-vouched `unread:action` unit.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"html/template"
	"sort"
	"strings"
	"text/template/parse"
	"unicode/utf8"
)

// BodyAction is one finding: its kind and the node as Go prints it (cut to 200 bytes).
type BodyAction struct {
	Kind string `json:"kind"`
	Text string `json:"text"`
}

// BodyActionReport is the whole report (wire version 1).
type BodyActionReport struct {
	V       int          `json:"v"`       // 1
	Total   int          `json:"total"`   // every finding, before the cap
	Digest  string       `json:"digest"`  // sha256 hex over every finding, uncut and uncapped; "" when Total is 0
	Actions []BodyAction `json:"actions"` // the first 50, in report order; never null
}

const (
	bodyActionsCap     = 50
	bodyActionTextCut  = 200
	bodyActionsVersion = 1
)

// bodyActionEngineNames are the field/method names through which an action reaches the
// compiled template itself (rule 3, `engine`). The data a campaign executes against holds the
// compiled template, shared by every recipient, and these names read or rewrite it. Pinned
// against the data types by internal/manager's TestBodyActionsDataPin.
var bodyActionEngineNames = []string{"Tpl", "SubjectTpl", "PreheaderTpl", "AltBodyTpl", "HeaderTpls", "CompileTemplate"}

// BodyActionEngineNames returns a copy of rule 3's name list (for the data pin test).
func BodyActionEngineNames() []string {
	return append([]string(nil), bodyActionEngineNames...)
}

// BodyActionsFailure is the report of exactly one `error` finding with the given text, digested
// as any finding (the endpoint's over-the-cap answer).
func BodyActionsFailure(text string) BodyActionReport {
	return finishBodyActions([]BodyAction{{Kind: "error", Text: text}})
}

// BodyActions reports every action of a stored campaign body that is not a plain value — a
// value Go's html/template prints, escaped, in place. It is the rule's one home.
//
// Steps, in order (an early return reports exactly the one finding named):
//
//  1. A copy is built: Campaign{Body: body, ContentType: contentType} and nothing else.
//  2. The existing CompileTemplate runs on the copy, unchanged. An error is `compile-error`.
//  3. The text the compile parsed is rebuilt with the compile's own helpers in the compile's
//     order: markdown.Convert for markdown, then TransformTrackLinks, then every regTplFuncs
//     entry. An error is `error`.
//  4. Agreement check (fail closed): that text is parsed with text/template/parse and the
//     printed root of its `content` tree must equal the printed root of the compiled
//     template's `content` tree; otherwise `mismatch`. Step 3 is a second copy of the compile's
//     rewrites, so if CompileTemplate ever gains or reorders one, every report says so instead
//     of reading the wrong text.
//  5. The text is parsed again with comments kept, and walked:
//     - every tree other than `content` (a define or block), by name: `define` (the name);
//     - in the content tree (then each defined tree, by name), in document order: an action is
//     `variable` (it declares or assigns), else `safe` (the identifier Safe appears anywhere
//     in it, unless it is the literal form below in a visual body), else `engine` (a field
//     chain names the compiled template: Tpl, SubjectTpl, PreheaderTpl, AltBodyTpl,
//     HeaderTpls, CompileTemplate), else `string` (a string literal whose quoted source holds
//     `}}` or `<` — it is longer than the stored `{{ … }}` or hides markup — except an
//     interpreted string that is TrackLink's first argument, where the href wraps put a
//     personalised link's expression), else nothing (a plain value); if/range/with are
//     reported with their keyword and pipeline (and their pipeline also tested for `engine`,
//     then `string`), template, comment, break and continue by kind; any other node is
//     `node` (fail closed on a Go release that adds one).
//  6. Everything runs under one recover: a panic is `error`. No template is executed, so no
//     template function is called and nothing is read or written.
//
// The literal form is `{{ Safe "…" }}`: no declaration, one command, whose arguments are
// exactly the identifier Safe and one interpreted string (a raw-string literal is reported).
// It is the builder's wrapper around its own Outlook markup, and in a visual body only the
// integrations review decodes it and reads that markup; in any other content type nothing
// reads it, so there it is `safe`.
//
// A body that is only `{{define "content"}}…{{end}}` in white space is not reported: Go lets
// that define supply the root tree, so the markup sent is the markup stored in place.
func BodyActions(body, contentType string, f template.FuncMap) (rep BodyActionReport) {
	defer func() {
		if r := recover(); r != nil {
			rep = BodyActionsFailure(fmt.Sprint(r))
		}
	}()

	// 1-2. The existing compile, on a copy.
	c := Campaign{Body: body, ContentType: contentType}
	if err := c.CompileTemplate(f); err != nil {
		return finishBodyActions([]BodyAction{{Kind: "compile-error", Text: err.Error()}})
	}

	// 3. The text the compile parsed.
	text := body
	if contentType == CampaignContentTypeMarkdown {
		var b bytes.Buffer
		if err := markdown.Convert([]byte(body), &b); err != nil {
			return BodyActionsFailure(err.Error())
		}
		text = b.String()
	}
	text = TransformTrackLinks(text, contentType)
	for _, r := range regTplFuncs {
		text = r.apply(text)
	}

	// 4. Agreement check.
	set, err := parseBodyActionText(text, parse.SkipFuncCheck)
	if err != nil {
		return BodyActionsFailure(err.Error())
	}
	compiled := ""
	if c.Tpl != nil {
		if ct := c.Tpl.Lookup(ContentTpl); ct != nil {
			compiled = printedRoot(ct.Tree)
		}
	}
	if printedRoot(set[ContentTpl]) != compiled {
		return finishBodyActions([]BodyAction{{Kind: "mismatch", Text: "the report's parse is not the compile's"}})
	}

	// 5. The walk, comments kept.
	set, err = parseBodyActionText(text, parse.SkipFuncCheck|parse.ParseComments)
	if err != nil {
		return BodyActionsFailure(err.Error())
	}
	w := bodyActionWalker{visual: contentType == CampaignContentTypeVisual}
	var names []string
	for name := range set {
		if name != ContentTpl {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		w.add("define", name)
	}
	if t := set[ContentTpl]; t != nil && t.Root != nil {
		w.walk(t.Root)
	}
	for _, name := range names {
		if t := set[name]; t != nil && t.Root != nil {
			w.walk(t.Root)
		}
	}
	return finishBodyActions(w.found)
}

// parseBodyActionText parses text as the compile does (default delimiters, a fresh tree set,
// no functions — SkipFuncCheck) and returns the tree set.
func parseBodyActionText(text string, mode parse.Mode) (map[string]*parse.Tree, error) {
	set := map[string]*parse.Tree{}
	t := parse.New(ContentTpl)
	t.Mode = mode
	if _, err := t.Parse(text, "", "", set); err != nil {
		return nil, err
	}
	return set, nil
}

func printedRoot(t *parse.Tree) string {
	if t == nil || t.Root == nil {
		return ""
	}
	return t.Root.String()
}

// finishBodyActions builds the report from every finding: total, digest over the uncut texts,
// the first 50 with their texts cut.
func finishBodyActions(found []BodyAction) BodyActionReport {
	rep := BodyActionReport{V: bodyActionsVersion, Total: len(found), Actions: []BodyAction{}}
	if len(found) == 0 {
		return rep
	}
	h := sha256.New()
	for _, a := range found {
		h.Write([]byte(a.Kind))
		h.Write([]byte{0})
		h.Write([]byte(a.Text))
		h.Write([]byte{'\n'})
	}
	rep.Digest = hex.EncodeToString(h.Sum(nil))
	for i, a := range found {
		if i == bodyActionsCap {
			break
		}
		rep.Actions = append(rep.Actions, BodyAction{Kind: a.Kind, Text: cutBodyActionText(a.Text)})
	}
	return rep
}

// cutBodyActionText cuts s to at most 200 bytes on a rune boundary.
func cutBodyActionText(s string) string {
	if len(s) <= bodyActionTextCut {
		return s
	}
	n := bodyActionTextCut
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

type bodyActionWalker struct {
	visual bool
	found  []BodyAction
}

func (w *bodyActionWalker) add(kind, text string) {
	w.found = append(w.found, BodyAction{Kind: kind, Text: text})
}

func (w *bodyActionWalker) walk(n parse.Node) {
	switch n := n.(type) {
	case nil:
		return
	case *parse.ListNode:
		if n == nil {
			return
		}
		for _, c := range n.Nodes {
			w.walk(c)
		}
	case *parse.TextNode:
	case *parse.ActionNode:
		if kind := w.actionKind(n.Pipe); kind != "" {
			w.add(kind, n.String())
		}
	case *parse.IfNode:
		w.branch("if", &n.BranchNode)
	case *parse.RangeNode:
		w.branch("range", &n.BranchNode)
	case *parse.WithNode:
		w.branch("with", &n.BranchNode)
	case *parse.TemplateNode:
		w.add("template", n.String())
	case *parse.CommentNode:
		w.add("comment", n.String())
	case *parse.BreakNode:
		w.add("break", n.String())
	case *parse.ContinueNode:
		w.add("continue", n.String())
	default:
		w.add("node", n.String())
	}
}

// branch reports an if/range/with with its keyword and pipeline, tests the pipeline for
// `engine` then `string`, then walks its list and else-list.
func (w *bodyActionWalker) branch(kw string, b *parse.BranchNode) {
	text := "{{" + kw + " " + b.Pipe.String() + "}}"
	w.add(kw, text)
	if pipeHasEngine(b.Pipe) {
		w.add("engine", text)
	} else if pipeHasString(b.Pipe, nil) {
		w.add("string", text)
	}
	if b.List != nil {
		w.walk(b.List)
	}
	if b.ElseList != nil {
		w.walk(b.ElseList)
	}
}

// actionKind is rules 1-4 for an action's pipeline: the first that holds, else "".
func (w *bodyActionWalker) actionKind(p *parse.PipeNode) string {
	if p == nil {
		return ""
	}
	if len(p.Decl) > 0 {
		return "variable"
	}
	literal := w.visual && isSafeLiteral(p)
	if !literal && pipeHas(p, isSafeIdent) {
		return "safe"
	}
	if pipeHasEngine(p) {
		return "engine"
	}
	var skip *parse.StringNode
	if literal {
		skip = p.Cmds[0].Args[1].(*parse.StringNode)
	}
	if pipeHasString(p, skip) {
		return "string"
	}
	return ""
}

// isSafeLiteral is the builder's literal form: no declaration, exactly one command whose
// arguments are exactly the identifier Safe and one interpreted (double-quoted) string.
func isSafeLiteral(p *parse.PipeNode) bool {
	if len(p.Decl) > 0 || len(p.Cmds) != 1 || len(p.Cmds[0].Args) != 2 {
		return false
	}
	if !isSafeIdent(p.Cmds[0].Args[0]) {
		return false
	}
	s, ok := p.Cmds[0].Args[1].(*parse.StringNode)
	return ok && strings.HasPrefix(s.Quoted, `"`)
}

func isSafeIdent(n parse.Node) bool {
	id, ok := n.(*parse.IdentifierNode)
	return ok && id.Ident == "Safe"
}

// pipeHas reports whether any node in the pipeline (commands, arguments, parenthesised
// pipelines, chains — recursively) satisfies pred.
func pipeHas(p *parse.PipeNode, pred func(parse.Node) bool) bool {
	if p == nil {
		return false
	}
	for _, c := range p.Cmds {
		for _, a := range c.Args {
			if nodeHas(a, pred) {
				return true
			}
		}
	}
	return false
}

func nodeHas(n parse.Node, pred func(parse.Node) bool) bool {
	if n == nil {
		return false
	}
	if pred(n) {
		return true
	}
	switch n := n.(type) {
	case *parse.PipeNode:
		return pipeHas(n, pred)
	case *parse.ChainNode:
		return nodeHas(n.Node, pred)
	case *parse.CommandNode:
		for _, a := range n.Args {
			if nodeHas(a, pred) {
				return true
			}
		}
	}
	return false
}

func pipeHasEngine(p *parse.PipeNode) bool {
	return pipeHas(p, func(n parse.Node) bool {
		var names []string
		switch n := n.(type) {
		case *parse.FieldNode:
			names = n.Ident
		case *parse.ChainNode:
			names = n.Field
		case *parse.VariableNode:
			if len(n.Ident) > 1 {
				names = n.Ident[1:]
			}
		}
		for _, name := range names {
			for _, e := range bodyActionEngineNames {
				if name == e {
					return true
				}
			}
		}
		return false
	})
}

// pipeHasString is rule 4: a string literal anywhere in the pipeline whose quoted source holds
// `}}` or `<`, other than skip (the visual literal form's own string) and the interpreted-string
// first argument of a command whose first word is the identifier TrackLink.
func pipeHasString(p *parse.PipeNode, skip *parse.StringNode) bool {
	exempt := map[*parse.StringNode]bool{}
	if skip != nil {
		exempt[skip] = true
	}
	var collect func(parse.Node)
	collectPipe := func(p *parse.PipeNode) {
		for _, c := range p.Cmds {
			collect(c)
		}
	}
	collect = func(n parse.Node) {
		switch n := n.(type) {
		case *parse.CommandNode:
			if len(n.Args) >= 2 {
				if id, ok := n.Args[0].(*parse.IdentifierNode); ok && id.Ident == "TrackLink" {
					if s, ok := n.Args[1].(*parse.StringNode); ok && strings.HasPrefix(s.Quoted, `"`) {
						exempt[s] = true
					}
				}
			}
			for _, a := range n.Args {
				collect(a)
			}
		case *parse.PipeNode:
			collectPipe(n)
		case *parse.ChainNode:
			collect(n.Node)
		}
	}
	if p == nil {
		return false
	}
	collectPipe(p)
	return pipeHas(p, func(n parse.Node) bool {
		s, ok := n.(*parse.StringNode)
		if !ok || exempt[s] {
			return false
		}
		return strings.Contains(s.Quoted, "}}") || strings.Contains(s.Quoted, "<")
	})
}
