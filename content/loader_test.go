package content

import (
	"strings"
	"testing"
)

func TestLoadAll(t *testing.T) {
	chapters, err := LoadAll(FS)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if len(chapters) != 14 {
		t.Fatalf("got %d chapters, want 14", len(chapters))
	}
	for i, ch := range chapters {
		if ch.Title == "" {
			t.Errorf("chapter %q (order %d) has empty title", ch.Slug, ch.Order)
		}
		if i > 0 && chapters[i-1].Order >= ch.Order {
			t.Errorf("chapters not sorted: %d before %d", chapters[i-1].Order, ch.Order)
		}
	}

	var intro, eroi *Chapter
	for i := range chapters {
		switch chapters[i].Slug {
		case "introduzione":
			intro = &chapters[i]
		case "eroi":
			eroi = &chapters[i]
		}
	}
	if intro == nil {
		t.Fatal("capitolo 'introduzione' non trovato")
	}
	if intro.Title != "Introduzione" {
		t.Errorf("titolo introduzione = %q, want %q", intro.Title, "Introduzione")
	}
	if intro.Group != "base" || intro.DataChapter != "introduzione" || !intro.InPrompt {
		t.Errorf("metadata introduzione inattesi: %+v", intro)
	}
	if strings.Contains(intro.HTML, "<h1>") {
		t.Errorf("l'H1 non dovrebbe comparire nel body renderizzato")
	}
	if !strings.Contains(intro.HTML, `class="game-term"`) {
		t.Errorf("atteso almeno un termine di gioco evidenziato in introduzione")
	}
	if intro.Excerpt == "" {
		t.Errorf("excerpt vuoto per introduzione")
	}
	if len(intro.Sections) == 0 {
		t.Errorf("TOC vuoto per introduzione")
	}

	if eroi == nil {
		t.Fatal("capitolo 'eroi' non trovato")
	}
	if !strings.Contains(eroi.HTML, "<figure>") || !strings.Contains(eroi.HTML, "/regolamento/img/eroi-lothar.png") {
		t.Errorf("atteso <figure> con immagine riscritta in eroi.HTML")
	}

	for _, ch := range chapters {
		wantInPrompt := ch.Order >= 1 && ch.Order <= 12
		if ch.InPrompt != wantInPrompt {
			t.Errorf("capitolo %q (order %d): InPrompt=%v, want %v", ch.Slug, ch.Order, ch.InPrompt, wantInPrompt)
		}
	}
}
