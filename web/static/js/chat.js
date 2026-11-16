(() => {
  "use strict";

  const launcher = document.getElementById("chat-launcher");
  const panel = document.getElementById("chat-panel");
  const closeBtn = document.getElementById("chat-close");
  const log = document.getElementById("chat-log");
  const emptyState = document.getElementById("chat-empty-state");
  const form = document.getElementById("chat-form");
  const input = document.getElementById("chat-input");
  const sendBtn = document.getElementById("chat-send");

  if (!launcher || !panel || !form || !input || !log) return;

  /** @type {{role: "user"|"assistant", content: string}[]} */
  const history = [];

  function openPanel() {
    panel.hidden = false;
    launcher.setAttribute("aria-expanded", "true");
    input.focus();
  }

  function closePanel() {
    panel.hidden = true;
    launcher.setAttribute("aria-expanded", "false");
    launcher.focus();
  }

  launcher.addEventListener("click", () => {
    if (panel.hidden) openPanel();
    else closePanel();
  });
  closeBtn?.addEventListener("click", closePanel);
  document.addEventListener("keydown", (e) => {
    if (e.key === "Escape" && !panel.hidden) closePanel();
  });

  document.querySelectorAll("[data-open-chat]").forEach((el) => {
    el.addEventListener("click", openPanel);
  });

  document.querySelectorAll(".chat-suggestion").forEach((btn) => {
    btn.addEventListener("click", () => {
      input.value = btn.dataset.question || "";
      form.requestSubmit();
    });
  });

  // Enter sends, Shift+Enter inserts a newline; the textarea grows a little
  // with the content instead of scrolling internally right away.
  input.addEventListener("keydown", (e) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault();
      form.requestSubmit();
    }
  });
  input.addEventListener("input", () => {
    input.style.height = "auto";
    input.style.height = Math.min(input.scrollHeight, 128) + "px";
  });

  function appendUserBubble(text) {
    emptyState?.remove();
    const div = document.createElement("div");
    div.className = "msg msg--user";
    div.textContent = text;
    log.appendChild(div);
    log.scrollTop = log.scrollHeight;
  }

  function renderAssistantText(container, text) {
    // Plain-text rendering: paragraphs only, no markdown/small-caps — the
    // full regolamento typography is reserved for the static pages.
    container.innerHTML = "";
    text.split(/\n{2,}/).forEach((para) => {
      if (!para.trim()) return;
      const p = document.createElement("p");
      p.textContent = para.trim();
      container.appendChild(p);
    });
  }

  function appendAssistantContainer() {
    emptyState?.remove();
    const div = document.createElement("div");
    div.className = "msg msg--assistant";
    log.appendChild(div);
    return div;
  }

  function appendTyping() {
    const div = document.createElement("div");
    div.className = "typing";
    div.innerHTML = '<span></span><span></span><span></span><span class="sr-only">Sto consultando il regolamento…</span>';
    log.appendChild(div);
    log.scrollTop = log.scrollHeight;
    return div;
  }

  async function sendMessage(text) {
    history.push({ role: "user", content: text });
    appendUserBubble(text);

    input.value = "";
    input.style.height = "auto";
    input.disabled = true;
    sendBtn.disabled = true;

    const typing = appendTyping();
    let assistantEl = null;
    let assistantText = "";

    try {
      const resp = await fetch("/api/chat", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ messages: history }),
      });

      if (!resp.ok) {
        const body = await resp.json().catch(() => ({}));
        if (resp.status === 429) {
          throw new Error("Troppe domande in poco tempo: riprova tra un momento.");
        }
        throw new Error(body.message || "Il servizio non è al momento disponibile.");
      }

      const reader = resp.body.getReader();
      const decoder = new TextDecoder();
      let buffer = "";

      for (;;) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });

        let sepIndex;
        while ((sepIndex = buffer.indexOf("\n\n")) !== -1) {
          const rawEvent = buffer.slice(0, sepIndex);
          buffer = buffer.slice(sepIndex + 2);
          const { event, data } = parseSSEEvent(rawEvent);
          if (!event) continue;

          if (event === "delta") {
            if (!assistantEl) {
              typing.remove();
              assistantEl = appendAssistantContainer();
            }
            assistantText += data.text || "";
            renderAssistantText(assistantEl, assistantText);
            log.scrollTop = log.scrollHeight;
          } else if (event === "error") {
            typing.remove();
            if (!assistantEl) assistantEl = appendAssistantContainer();
            assistantEl.classList.add("msg--error");
            renderAssistantText(assistantEl, data.message || "Si è verificato un errore.");
          } else if (event === "done") {
            // nothing to do; loop will exit when the stream closes
          }
        }
      }

      if (assistantText) {
        history.push({ role: "assistant", content: assistantText });
      }
    } catch (err) {
      typing.remove();
      if (!assistantEl) assistantEl = appendAssistantContainer();
      assistantEl.classList.add("msg--error");
      renderAssistantText(assistantEl, err.message || "Si è verificato un errore imprevisto.");
    } finally {
      input.disabled = false;
      sendBtn.disabled = false;
      input.focus();
      log.scrollTop = log.scrollHeight;
    }
  }

  /** Parses one "event: x\ndata: {...}" block into {event, data}. */
  function parseSSEEvent(block) {
    let event = null;
    let dataLine = "";
    for (const line of block.split("\n")) {
      if (line.startsWith("event: ")) event = line.slice(7).trim();
      else if (line.startsWith("data: ")) dataLine += line.slice(6);
    }
    let data = {};
    try {
      data = dataLine ? JSON.parse(dataLine) : {};
    } catch {
      /* ignore malformed payloads */
    }
    return { event, data };
  }

  form.addEventListener("submit", (e) => {
    e.preventDefault();
    const text = input.value.trim();
    if (!text) return;
    sendMessage(text);
  });
})();
