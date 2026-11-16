package content

import (
	"bytes"
	"embed"
	"fmt"
	htmlutil "html"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	ghtml "github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// TOCEntry is one entry (H2 or H3) in a chapter's table of contents.
type TOCEntry struct {
	Title  string
	Anchor string
	Level  int
}

// Chapter is one rendered page of the regolamento.
type Chapter struct {
	Slug        string
	Order       int
	Title       string
	Group       string // "base" | "ambientazioni" | "materiali"
	DataChapter string
	Excerpt     string
	HTML        string
	Sections    []TOCEntry
	RawMarkdown string
	InPrompt    bool
}

var filenamePattern = regexp.MustCompile(`^(\d+)-(.+)\.md$`)

var dataChapterByOrder = map[int]string{
	0: "materiali", 1: "introduzione", 2: "eroi", 3: "giocare", 4: "evolvere",
	5: "fine", 6: "narrare", 13: "materiali",
}

func groupFor(order int) string {
	switch {
	case order == 0 || order == 13:
		return "materiali"
	case order >= 1 && order <= 6:
		return "base"
	default:
		return "ambientazioni"
	}
}

func dataChapterFor(order int) string {
	if v, ok := dataChapterByOrder[order]; ok {
		return v
	}
	return "ambientazioni"
}

// LoadAll reads every *.md file under regolamento/, renders it to HTML and
// returns the chapters sorted by their numeric filename prefix.
func LoadAll(fsys embed.FS) ([]Chapter, error) {
	entries, err := fs.ReadDir(fsys, "regolamento")
	if err != nil {
		return nil, fmt.Errorf("content: leggere regolamento/: %w", err)
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.Table, extension.Typographer),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(ghtml.WithUnsafe()),
	)

	var chapters []Chapter
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		m := filenamePattern.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("content: nome file inatteso %q (atteso NN-slug.md)", e.Name())
		}
		order, err := strconv.Atoi(m[1])
		if err != nil {
			return nil, fmt.Errorf("content: prefisso numerico non valido in %q: %w", e.Name(), err)
		}
		slug := m[2]

		raw, err := fs.ReadFile(fsys, path.Join("regolamento", e.Name()))
		if err != nil {
			return nil, fmt.Errorf("content: leggere %s: %w", e.Name(), err)
		}

		ch, err := parseChapter(md, order, slug, raw)
		if err != nil {
			return nil, fmt.Errorf("content: elaborare %s: %w", e.Name(), err)
		}
		chapters = append(chapters, ch)
	}

	if len(chapters) == 0 {
		return nil, fmt.Errorf("content: nessun capitolo trovato in regolamento/")
	}

	sort.Slice(chapters, func(i, j int) bool { return chapters[i].Order < chapters[j].Order })
	return chapters, nil
}

func parseChapter(md goldmark.Markdown, order int, slug string, raw []byte) (Chapter, error) {
	source := text.NewReader(raw)
	doc := md.Parser().Parse(source)

	title, err := extractAndRemoveTitle(doc, raw)
	if err != nil {
		return Chapter{}, err
	}

	toc := collectTOC(doc, raw)
	excerpt := extractExcerpt(doc, raw)

	var buf bytes.Buffer
	if err := md.Renderer().Render(&buf, raw, doc); err != nil {
		return Chapter{}, fmt.Errorf("render HTML: %w", err)
	}

	finalHTML, err := postProcessHTML(buf.String())
	if err != nil {
		return Chapter{}, fmt.Errorf("post-process HTML: %w", err)
	}

	order0 := order // keep for InPrompt check below
	return Chapter{
		Slug:        slug,
		Order:       order,
		Title:       title,
		Group:       groupFor(order),
		DataChapter: dataChapterFor(order),
		Excerpt:     excerpt,
		HTML:        finalHTML,
		Sections:    toc,
		RawMarkdown: string(raw),
		InPrompt:    order0 >= 1 && order0 <= 12,
	}, nil
}

// extractAndRemoveTitle finds the chapter's leading H1, returns its text and
// removes the node from the tree so it isn't rendered twice (the page
// template prints the title itself, in its own markup).
func extractAndRemoveTitle(doc ast.Node, source []byte) (string, error) {
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok {
			continue
		}
		if h.Level != 1 {
			return "", fmt.Errorf("primo titolo non è un H1 (livello %d)", h.Level)
		}
		title := nodeText(h, source)
		doc.RemoveChild(doc, h)
		return title, nil
	}
	return "", fmt.Errorf("nessun titolo H1 trovato")
}

// collectTOC walks the tree for H2/H3 headings (the auto-heading-id
// extension has already assigned their "id" attribute during parsing).
func collectTOC(doc ast.Node, source []byte) []TOCEntry {
	var toc []TOCEntry
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		h, ok := n.(*ast.Heading)
		if !ok || h.Level < 2 || h.Level > 3 {
			return ast.WalkContinue, nil
		}
		anchor := ""
		if v, ok := h.AttributeString("id"); ok {
			switch t := v.(type) {
			case []byte:
				anchor = string(t)
			case string:
				anchor = t
			}
		}
		toc = append(toc, TOCEntry{Title: nodeText(h, source), Anchor: anchor, Level: h.Level})
		return ast.WalkSkipChildren, nil
	})
	return toc
}

// extractExcerpt returns the first paragraph's plain text, truncated.
func extractExcerpt(doc ast.Node, source []byte) string {
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		p, ok := n.(*ast.Paragraph)
		if !ok {
			continue
		}
		return truncateWords(nodeText(p, source), 140)
	}
	return ""
}

// nodeText concatenates the plain text of every ast.Text descendant,
// inserting a space for soft/hard line breaks.
func nodeText(n ast.Node, source []byte) string {
	var buf bytes.Buffer
	var walk func(ast.Node)
	walk = func(n ast.Node) {
		for c := n.FirstChild(); c != nil; c = c.NextSibling() {
			switch t := c.(type) {
			case *ast.Text:
				buf.Write(t.Segment.Value(source))
				if t.SoftLineBreak() || t.HardLineBreak() {
					buf.WriteByte(' ')
				}
			case *ast.String:
				// Produced by extension.Typographer for smart quotes/dashes:
				// an HTML-entity replacement (e.g. "&rsquo;"), not a slice of
				// source — decode it back to the literal rune for plain text.
				buf.WriteString(htmlutil.UnescapeString(string(t.Value)))
			default:
				walk(c)
			}
		}
	}
	walk(n)
	return strings.TrimSpace(buf.String())
}

func truncateWords(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	runes := []rune(s)
	cut := runes[:max]
	if i := strings.LastIndexAny(string(cut), " \t"); i > 0 {
		cut = []rune(string(cut)[:i])
	}
	return strings.TrimSpace(string(cut)) + "…"
}

// --- HTML post-processing ------------------------------------------------
//
// goldmark renders plain, standard HTML. A second pass over the parsed DOM
// (golang.org/x/net/html) applies the three presentational touches from the
// design system: images become <figure>, tables get a scroll wrapper, and
// regolamento terminology is wrapped for small-caps styling. Doing this on
// a real DOM (rather than string/regexp surgery) means the glossary pass
// can never accidentally match inside a tag or attribute.

var gameTermsPattern = buildGameTermsPattern()

func buildGameTermsPattern() *regexp.Regexp {
	// Curated on purpose: only unambiguous rules jargon, not common words
	// ("storia", "scena", "turno"...) that would be highlighted too often
	// to stay tasteful.
	terms := []string{
		"uscire di scena",
		"personaggio secondario", "personaggi secondari",
		"complicazioni", "complicazione",
		"difficoltà",
		"profondità",
		"narratori", "narratore",
		"giocatori", "giocatore",
		"cicatrici", "cicatrice",
		"obiettivi", "obiettivo",
		"successi", "successo",
		"pericoli", "pericolo",
		"sacchetti", "sacchetto",
		"lezioni", "lezione",
		"tratti", "tratto",
		"prove", "prova",
		"sfide", "sfida",
		"riserva",
		"token",
		"eroi", "eroe",
	}
	// Longest-first so e.g. "uscire di scena" wins over a bare "scena" (not
	// in the list anyway, kept here for clarity of intent).
	sort.Slice(terms, func(i, j int) bool { return len(terms[i]) > len(terms[j]) })
	escaped := make([]string, len(terms))
	for i, t := range terms {
		escaped[i] = regexp.QuoteMeta(t)
	}
	return regexp.MustCompile(`(?i)(` + strings.Join(escaped, "|") + `)`)
}

// excludedFromGameTerms are elements whose text content must never be
// touched by the small-caps glossary pass (headings already look like a
// title, links/code have their own meaning, <pre>/<script>/<style> aren't
// prose at all).
var excludedFromGameTerms = map[string]bool{
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"a": true, "code": true, "pre": true, "script": true, "style": true,
	"figcaption": true, "th": true,
}

func postProcessHTML(fragment string) (string, error) {
	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(fragment), context)
	if err != nil {
		return "", err
	}

	// ParseFragment returns detached top-level nodes (Parent == nil).
	// Give them a throwaway parent so every transform below can rely on
	// n.Parent being non-nil, then unwrap before rendering.
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		root.AppendChild(n)
	}

	transformNode(root, false)

	var buf bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return "", err
		}
	}
	return buf.String(), nil
}

// transformNode walks the tree in place. excluded is true while inside an
// element listed in excludedFromGameTerms.
func transformNode(n *html.Node, excluded bool) {
	if n.Type == html.ElementNode {
		switch n.Data {
		case "img":
			wrapImageInFigure(n)
			return // the img itself has no children to recurse into
		case "table":
			wrapTableForScroll(n)
		}
		if excludedFromGameTerms[n.Data] {
			excluded = true
		}
	}

	// Collect children first: we may rewrite a text node into several
	// siblings while iterating, which would otherwise confuse NextSibling.
	var children []*html.Node
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		children = append(children, c)
	}
	for _, c := range children {
		if c.Type == html.TextNode && !excluded {
			applyGameTerms(n, c)
			continue
		}
		transformNode(c, excluded)
	}
}

// wrapImageInFigure replaces <img src="img/x.png" alt="..."> with
// <figure><img src="/regolamento/img/x.png" ... loading="lazy" ...><figcaption>alt</figcaption></figure>.
// Every image in this corpus is a hex-grid diagram exported at 1300x1210px.
func wrapImageInFigure(img *html.Node) {
	var alt string
	for i, a := range img.Attr {
		switch a.Key {
		case "alt":
			alt = a.Val
		case "src":
			if strings.HasPrefix(a.Val, "img/") {
				img.Attr[i].Val = "/regolamento/" + a.Val
			}
		}
	}
	img.Attr = append(img.Attr,
		html.Attribute{Key: "width", Val: "1300"},
		html.Attribute{Key: "height", Val: "1210"},
		html.Attribute{Key: "loading", Val: "lazy"},
		html.Attribute{Key: "decoding", Val: "async"},
	)

	figure := &html.Node{Type: html.ElementNode, Data: "figure", DataAtom: atom.Figure}
	parent := img.Parent
	parent.InsertBefore(figure, img)
	parent.RemoveChild(img)
	figure.AppendChild(img)

	if alt != "" {
		figcaption := &html.Node{Type: html.ElementNode, Data: "figcaption", DataAtom: atom.Figcaption}
		figcaption.AppendChild(&html.Node{Type: html.TextNode, Data: alt})
		figure.AppendChild(figcaption)
	}
}

func wrapTableForScroll(table *html.Node) {
	wrap := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div,
		Attr: []html.Attribute{{Key: "class", Val: "table-wrap"}}}
	parent := table.Parent
	parent.InsertBefore(wrap, table)
	parent.RemoveChild(table)
	wrap.AppendChild(table)
}

// applyGameTerms splits a text node into plain-text and
// <span class="game-term"> siblings wherever the glossary matches,
// inserting the result in place of the original node.
func applyGameTerms(parent *html.Node, textNode *html.Node) {
	s := textNode.Data
	matches := gameTermsPattern.FindAllStringIndex(s, -1)
	if matches == nil {
		return
	}

	// Keep only matches whose surrounding runes aren't letters, so e.g.
	// "Prova" inside "Approvazione" is never touched.
	var kept [][2]int
	for _, m := range matches {
		if !isLetterBefore(s, m[0]) && !isLetterAfter(s, m[1]) {
			kept = append(kept, [2]int{m[0], m[1]})
		}
	}
	if len(kept) == 0 {
		return
	}

	next := textNode.NextSibling
	pos := 0
	for _, m := range kept {
		if m[0] > pos {
			parent.InsertBefore(&html.Node{Type: html.TextNode, Data: s[pos:m[0]]}, next)
		}
		span := &html.Node{Type: html.ElementNode, Data: "span", DataAtom: atom.Span,
			Attr: []html.Attribute{{Key: "class", Val: "game-term"}}}
		span.AppendChild(&html.Node{Type: html.TextNode, Data: s[m[0]:m[1]]})
		parent.InsertBefore(span, next)
		pos = m[1]
	}
	if pos < len(s) {
		parent.InsertBefore(&html.Node{Type: html.TextNode, Data: s[pos:]}, next)
	}
	parent.RemoveChild(textNode)
}

func isLetterBefore(s string, idx int) bool {
	if idx == 0 {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:idx])
	return unicode.IsLetter(r)
}

func isLetterAfter(s string, idx int) bool {
	if idx >= len(s) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(s[idx:])
	return unicode.IsLetter(r)
}
