package httpserver

import (
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"

	"github.com/Procionegobbo/nte-companion/content"
)

var groupLabels = map[string]string{
	"base":          "Parte I: Regole base",
	"ambientazioni": "Parte II: Ambientazioni",
	"materiali":     "Materiali",
}

var groupOrder = []string{"base", "ambientazioni", "materiali"}

// chapterView adapts content.Chapter for html/template: its HTML field
// shadows the embedded string with a template.HTML of the same content, so
// the rendered markup is written out verbatim instead of escaped.
type chapterView struct {
	content.Chapter
	HTML template.HTML
}

func newChapterView(ch content.Chapter) chapterView {
	// ch.HTML is rendered by our own content/loader.go from vendored
	// markdown, never from user input, so writing it out unescaped is safe.
	return chapterView{Chapter: ch, HTML: template.HTML(ch.HTML)}
}

// PageHandler serves the static regolamento pages: the homepage index and
// one page per chapter.
type PageHandler struct {
	chapters    []content.Chapter
	bySlug      map[string]int // index into chapters
	homeTmpl    *template.Template
	chapterTmpl *template.Template
	privacyTmpl *template.Template
}

func NewPageHandler(chapters []content.Chapter, webFS fs.FS) (*PageHandler, error) {
	homeTmpl, err := template.ParseFS(webFS, "templates/layout.html", "templates/home.html")
	if err != nil {
		return nil, fmt.Errorf("parse home template: %w", err)
	}
	chapterTmpl, err := template.ParseFS(webFS, "templates/layout.html", "templates/chapter.html")
	if err != nil {
		return nil, fmt.Errorf("parse chapter template: %w", err)
	}
	privacyTmpl, err := template.ParseFS(webFS, "templates/layout.html", "templates/privacy.html")
	if err != nil {
		return nil, fmt.Errorf("parse privacy template: %w", err)
	}

	bySlug := make(map[string]int, len(chapters))
	for i, ch := range chapters {
		bySlug[ch.Slug] = i
	}

	return &PageHandler{
		chapters:    chapters,
		bySlug:      bySlug,
		homeTmpl:    homeTmpl,
		chapterTmpl: chapterTmpl,
		privacyTmpl: privacyTmpl,
	}, nil
}

type homeGroup struct {
	Key      string
	Label    string
	Chapters []content.Chapter
}

type homeData struct {
	Groups          []homeGroup
	PageTitle       string
	BodyDataChapter string
}

func (h *PageHandler) Home(w http.ResponseWriter, r *http.Request) {
	byGroup := make(map[string][]content.Chapter, len(groupOrder))
	for _, ch := range h.chapters {
		byGroup[ch.Group] = append(byGroup[ch.Group], ch)
	}

	data := homeData{}
	for _, key := range groupOrder {
		chs := byGroup[key]
		if len(chs) == 0 {
			continue
		}
		data.Groups = append(data.Groups, homeGroup{Key: key, Label: groupLabels[key], Chapters: chs})
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.homeTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("homepage: errore di rendering: %v", err)
	}
}

type chapterPageData struct {
	Chapter         chapterView
	GroupLabel      string
	Prev, Next      *content.Chapter
	PageTitle       string
	BodyDataChapter string
}

func (h *PageHandler) Chapter(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	i, ok := h.bySlug[slug]
	if !ok {
		http.NotFound(w, r)
		return
	}
	ch := h.chapters[i]

	data := chapterPageData{
		Chapter:         newChapterView(ch),
		GroupLabel:      groupLabels[ch.Group],
		PageTitle:       ch.Title,
		BodyDataChapter: ch.DataChapter,
	}
	if i > 0 {
		data.Prev = &h.chapters[i-1]
	}
	if i < len(h.chapters)-1 {
		data.Next = &h.chapters[i+1]
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.chapterTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("capitolo %q: errore di rendering: %v", slug, err)
	}
}

type simplePageData struct {
	PageTitle       string
	BodyDataChapter string
}

func (h *PageHandler) Privacy(w http.ResponseWriter, r *http.Request) {
	data := simplePageData{PageTitle: "Privacy", BodyDataChapter: "materiali"}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.privacyTmpl.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("privacy: errore di rendering: %v", err)
	}
}
