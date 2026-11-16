# Not the End — Companion

Sito web del regolamento di [Not the End](https://www.fumblegdr.it/giochi/not-the-end/),
gioco di ruolo italiano di Fumble: una versione statica e consultabile del
manuale, affiancata da un assistente che risponde in italiano alle domande
sulle regole citando direttamente il testo del manuale.

Non è un prodotto ufficiale: è un progetto amatoriale, realizzato nel
rispetto della licenza con cui Fumble ha scelto di condividere il gioco.

## Cosa contiene

- **Sito statico del regolamento**, generato dai file Markdown in
  `content/regolamento/` (14 capitoli: regole base, ambientazioni di
  esempio, licenza e materiali), con un design system ispirato al manuale
  originale (font self-hosted, un colore d'accento per capitolo, piccole
  maiuscole per i termini di gioco).
- **Un chatbot** (`POST /api/chat`, streaming SSE) che riceve l'intero
  regolamento come contesto e risponde via [OpenRouter](https://openrouter.ai),
  con fallback automatico su altri modelli economici se quello preferito è
  irraggiungibile o rate-limited.
- **Nessun database**: la cronologia della chat vive solo nel browser, un
  rate-limiter per IP in memoria protegge l'endpoint da abusi.
- **Un solo binario Go**, autosufficiente: contenuti, template, CSS/JS e
  font sono tutti incorporati tramite `go:embed`, pronto per un'immagine
  Docker minimale (~20 MB) da deployare ovunque, ad es. su Coolify.

## Struttura del progetto

```
site/
├── cmd/server/main.go          Entrypoint: configurazione, avvio, shutdown
├── content/                    Markdown del regolamento + immagini + loader
│   ├── embed.go / loader.go     Parsing markdown → HTML (goldmark), piccole
│   │                            maiuscole sui termini di gioco, estrazione TOC
│   └── regolamento/*.md        I 14 capitoli (licenza CC BY-NC-SA 3.0 IT)
├── internal/
│   ├── config/                 Lettura configurazione da env var
│   ├── chat/                   System prompt, client OpenRouter, handler HTTP
│   ├── ratelimit/              Rate limiter per IP (in memoria)
│   ├── clientip/               Estrazione IP client dal contesto della request
│   └── httpserver/             Routing, pagine statiche, middleware
├── web/                        Template HTML, CSS, JS del widget di chat, font
├── Dockerfile                  Build multi-stage, immagine finale ~20MB
└── .env.example                Variabili di configurazione disponibili
```

## Requisiti

- Go 1.27 o superiore (versione dichiarata in `go.mod`)
- Una chiave API [OpenRouter](https://openrouter.ai/keys)
- Docker, solo per la build dell'immagine di produzione (facoltativo in locale)

## Configurazione

Tutta la configurazione passa da variabili d'ambiente, nessun file di
config. L'unica obbligatoria è `OPENROUTER_API_KEY`: il server non si
avvia senza.

| Variabile | Obbligatoria | Default | Note |
|---|---|---|---|
| `OPENROUTER_API_KEY` | Sì | — | |
| `OPENROUTER_MODEL` | No | `google/gemini-2.5-flash-lite` | |
| `OPENROUTER_FALLBACK_MODELS` | No | `anthropic/claude-haiku-5.5,openai/gpt-4o-mini` | CSV, massimo 2 (OpenRouter accetta al più 3 modelli in tutto) |
| `OPENROUTER_MAX_PRICE_PROMPT` | No | `0.50` | $/milione di token, tetto di sicurezza |
| `OPENROUTER_MAX_PRICE_COMPLETION` | No | `2.00` | $/milione di token, tetto di sicurezza |
| `OPENROUTER_BASE_URL` | No | `https://openrouter.ai/api/v1` | |
| `PORT` | No | `8080` | |
| `MAX_TOKENS` | No | `700` | Limite output per risposta |
| `RATE_LIMIT_PER_MINUTE` | No | `10` | Per IP |
| `RATE_LIMIT_PER_DAY` | No | `80` | Per IP |
| `REQUEST_TIMEOUT_SECONDS` | No | `60` | Timeout verso OpenRouter |
| `TRUST_PROXY_HEADERS` | No | `true` | Fidarsi di `X-Forwarded-For` (vero dietro un proxy come Coolify/Traefik) |

Copia `.env.example` in `.env`, inserisci la tua chiave, poi caricale nella
shell prima di avviare il server:

```bash
export $(grep -v '^#' .env | xargs)
```

## Sviluppo locale

```bash
go run ./cmd/server
```

Il sito è servito su `http://localhost:8080`. Endpoint principali:

- `GET /` — homepage con l'indice dei capitoli
- `GET /regolamento/{slug}` — pagina di un capitolo
- `POST /api/chat` — chat (SSE), body `{"messages":[{"role":"user","content":"..."}]}`
- `GET /healthz` — healthcheck

## Test

```bash
go build ./...
go vet ./...
go test ./...
```

## Build Docker

```bash
docker build -t nte-companion .
docker run --rm -p 8080:8080 -e OPENROUTER_API_KEY=sk-or-... nte-companion
```

Immagine finale basata su `distroless/static-debian12:nonroot` (include i
certificati CA necessari per le chiamate HTTPS a OpenRouter). Nessun volume
da montare: tutto il contenuto è nel binario.

## Licenza

Questo repository contiene due licenze distinte (vedi [NOTICE](NOTICE)):

- Il codice sorgente (tutto tranne `content/`) è rilasciato con licenza
  **MIT** — vedi [LICENSE](LICENSE).
- I contenuti del regolamento in `content/` restano protetti dalla licenza
  originale dell'opera, **CC BY-NC-SA 3.0 IT** — vedi
  [content/LICENSE](content/LICENSE).

## Crediti

Grazie a [Fumble](https://fumblegdr.it) e a
[Claudio Pustorino](https://claudiopustorino.com), autore di Not the End,
per aver creato questo gioco e averlo reso disponibile con licenza
Creative Commons. Se il gioco ti piace,
[acquista il manuale originale](https://www.fumblegdr.it/giochi/not-the-end/):
ogni copia venduta contribuisce a finanziare la creazione di nuovi giochi.
