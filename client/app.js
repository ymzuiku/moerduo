(function () {
  "use strict";

  var app = document.getElementById("app");
  var state = {
    view: "list", // "list" | "player"
    books: [],
    currentBook: null,
    sentenceIndex: 0,
    playing: false,
    showText: false,
    currentAudio: null,
  };

  // ---- Storage (AppShell native or fallback) ----

  function saveProgress(bookId, index) {
    AppShell.storage.set("progress:" + bookId, String(index));
  }

  function loadProgress(bookId) {
    return AppShell.storage.get("progress:" + bookId).then(function (r) {
      return r.value ? parseInt(r.value, 10) : 0;
    });
  }

  // ---- API ----

  function getApiBase() {
    return localStorage.getItem("appshell_api_base") || "http://192.168.1.13:20300";
  }

  function fetchBooks() {
    return fetch(getApiBase() + "/api/books").then(function (r) { return r.json(); });
  }

  function fetchBook(id) {
    return fetch(getApiBase() + "/api/books/" + id).then(function (r) { return r.json(); });
  }

  // ---- Audio playback ----

  // Try to play pre-generated audio file; fall back to Web Speech TTS
  function playSentence(bookId, index, text, onEnd) {
    stopPlaying();
    var audioUrl = getApiBase() + "/data/audio/" + bookId + "/" + pad4(index) + ".mp3";

    var audio = new Audio(audioUrl);
    state.currentAudio = audio;

    audio.onended = function () {
      state.currentAudio = null;
      if (onEnd) onEnd();
    };
    audio.onerror = function () {
      // No pre-generated audio, fall back to TTS
      state.currentAudio = null;
      speakTTS(text, onEnd);
    };
    audio.play();
  }

  function speakTTS(text, onEnd) {
    window.speechSynthesis.cancel();
    var u = new SpeechSynthesisUtterance(text);
    u.lang = "en-US";
    u.rate = 0.85;
    u.onend = function () { if (onEnd) onEnd(); };
    u.onerror = function () { if (onEnd) onEnd(); };
    window.speechSynthesis.speak(u);
  }

  function stopPlaying() {
    if (state.currentAudio) {
      state.currentAudio.pause();
      state.currentAudio = null;
    }
    window.speechSynthesis.cancel();
    state.playing = false;
  }

  // ---- Auto-play logic ----

  function playCurrentSentence() {
    if (!state.currentBook || !state.playing) return;
    var sentences = state.currentBook.sentences;
    if (state.sentenceIndex >= sentences.length) {
      state.playing = false;
      render();
      return;
    }
    state.showText = false;
    render();
    playSentence(state.currentBook.id, state.sentenceIndex, sentences[state.sentenceIndex], function () {
      if (!state.playing) return;
      setTimeout(function () {
        if (!state.playing) return;
        state.sentenceIndex++;
        saveProgress(state.currentBook.id, state.sentenceIndex);
        playCurrentSentence();
      }, 800);
    });
  }

  function pad4(n) {
    var s = String(n);
    while (s.length < 4) s = "0" + s;
    return s;
  }

  // ---- Render ----

  function render() {
    if (state.view === "list") {
      renderBookList();
    } else {
      renderPlayer();
    }
  }

  function renderBookList() {
    var base = getApiBase();
    var html = '<div class="view">';
    html += '<div class="header"><h1>先搞定听力</h1></div>';
    html += '<div class="book-list">';
    for (var i = 0; i < state.books.length; i++) {
      var b = state.books[i];
      var coverUrl = b.cover ? base + b.cover : "";
      html += '<div class="book-item" data-id="' + b.id + '">';
      if (coverUrl) {
        html += '<img class="book-cover" src="' + coverUrl + '" alt="">';
      }
      html += '<div class="book-info"><div class="book-title">' + escapeHtml(b.title) + '</div>';
      html += '<div class="book-progress" id="progress-' + b.id + '"></div></div>';
      html += '<span class="book-arrow">\u203A</span>';
      html += '</div>';
    }
    html += '</div></div>';
    app.innerHTML = html;

    // Load progress for each book
    state.books.forEach(function (b) {
      loadProgress(b.id).then(function (idx) {
        var el = document.getElementById("progress-" + b.id);
        if (el && idx > 0) {
          el.textContent = "已听到第 " + (idx + 1) + " 句";
        }
      });
    });

    // Bind click
    var items = app.querySelectorAll(".book-item");
    for (var j = 0; j < items.length; j++) {
      items[j].addEventListener("click", function () {
        openBook(this.getAttribute("data-id"));
      });
    }
  }

  function renderPlayer() {
    var book = state.currentBook;
    if (!book) return;
    var sentences = book.sentences;
    var idx = state.sentenceIndex;
    var sentence = sentences[idx] || "";
    var total = sentences.length;

    var html = '<div class="view' + (state.playing ? " speaking" : "") + '">';

    // Header
    html += '<div class="header-with-back">';
    html += '<button class="back-btn" id="back-btn">返回</button>';
    html += '<span class="header-title">' + escapeHtml(book.title) + '</span>';
    html += '</div>';

    // Body
    html += '<div class="player-body">';
    html += '<div class="sentence-counter">' + (idx + 1) + ' / ' + total + '</div>';
    html += '<div class="sentence-card" id="sentence-card">';
    if (state.showText) {
      html += '<div class="sentence-text">' + escapeHtml(sentence) + '</div>';
    } else {
      html += '<div class="sentence-hidden">点击显示文字</div>';
    }
    html += '</div>';
    html += '</div>';

    // Controls
    html += '<div class="controls">';
    html += '<div class="progress-bar-container"><div class="progress-bar">';
    html += '<div class="progress-bar-fill" style="width:' + ((idx / Math.max(total - 1, 1)) * 100) + '%"></div>';
    html += '</div></div>';
    html += '<div class="main-controls">';

    // Prev
    html += '<button class="ctrl-btn" id="prev-btn"><svg viewBox="0 0 24 24"><path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/></svg></button>';

    // Play/Pause
    if (state.playing) {
      html += '<button class="play-btn" id="play-btn"><svg viewBox="0 0 24 24"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg></button>';
    } else {
      html += '<button class="play-btn" id="play-btn"><svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg></button>';
    }

    // Next
    html += '<button class="ctrl-btn" id="next-btn"><svg viewBox="0 0 24 24"><path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z"/></svg></button>';

    html += '</div></div></div>';
    app.innerHTML = html;

    // Bind events
    document.getElementById("back-btn").addEventListener("click", function () {
      stopPlaying();
      state.view = "list";
      state.currentBook = null;
      render();
    });

    document.getElementById("sentence-card").addEventListener("click", function () {
      state.showText = !state.showText;
      render();
    });

    document.getElementById("play-btn").addEventListener("click", function () {
      if (state.playing) {
        stopPlaying();
        render();
      } else {
        state.playing = true;
        render();
        playCurrentSentence();
      }
    });

    document.getElementById("prev-btn").addEventListener("click", function () {
      if (state.sentenceIndex > 0) {
        stopPlaying();
        state.sentenceIndex--;
        saveProgress(book.id, state.sentenceIndex);
        state.showText = false;
        render();
      }
    });

    document.getElementById("next-btn").addEventListener("click", function () {
      if (state.sentenceIndex < sentences.length - 1) {
        stopPlaying();
        state.sentenceIndex++;
        saveProgress(book.id, state.sentenceIndex);
        state.showText = false;
        render();
      }
    });
  }

  function escapeHtml(s) {
    var div = document.createElement("div");
    div.textContent = s;
    return div.innerHTML;
  }

  function openBook(id) {
    fetchBook(id).then(function (book) {
      state.currentBook = book;
      return loadProgress(id);
    }).then(function (idx) {
      state.sentenceIndex = idx;
      state.view = "player";
      state.playing = false;
      state.showText = false;
      render();
    });
  }

  // ---- Init ----

  function init() {
    app.innerHTML = '<div class="view"><div class="header"><h1>先搞定听力</h1></div><div class="book-list" style="display:flex;align-items:center;justify-content:center;color:var(--text2)">加载中...</div></div>';

    fetchBooks().then(function (books) {
      state.books = books;
      render();
    }).catch(function () {
      setTimeout(function () {
        fetchBooks().then(function (books) {
          state.books = books;
          render();
        }).catch(function () {
          app.innerHTML = '<div class="view"><div class="header"><h1>先搞定听力</h1></div><div class="book-list" style="display:flex;flex-direction:column;align-items:center;justify-content:center;color:var(--text2);gap:12px"><div>无法连接服务器</div><button onclick="location.reload()" style="padding:8px 24px;background:var(--accent);color:#fff;border:none;border-radius:8px;font-size:15px">重试</button></div></div>';
        });
      }, 1500);
    });
  }

  init();
})();
