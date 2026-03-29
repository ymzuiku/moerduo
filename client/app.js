(function () {
  "use strict";

  var app = document.getElementById("app");
  var API = location.origin;

  var state = {
    view: "login", // login | list | player
    token: null,
    user: null,
    books: [],
    currentBook: null,
    sentenceIndex: 0,
    playing: false,
    showText: true,
    showTranslation: true,
    currentAudio: null,
    phraseTranslations: [], // per-phrase translations for current paragraph
    wordPopup: null, // { word, translation, x, y }
  };

  // ---- Storage ----

  function showToast(msg) {
    var old = document.getElementById("toast");
    if (old) old.remove();
    var div = document.createElement("div");
    div.id = "toast";
    div.className = "toast";
    div.textContent = msg;
    document.body.appendChild(div);
    setTimeout(function () { div.remove(); }, 3000);
  }

  function saveLocal(key, val) {
    try { localStorage.setItem(key, val); } catch (e) {}
  }

  function loadLocal(key) {
    try { return localStorage.getItem(key); } catch (e) { return null; }
  }

  function saveProgress(bookId, index) {
    saveLocal("progress:" + bookId, String(index));
  }

  function loadProgress(bookId) {
    var v = loadLocal("progress:" + bookId);
    return v ? parseInt(v, 10) : 0;
  }

  // ---- API helpers ----

  function api(method, path, body) {
    var opts = {
      method: method,
      headers: { "Content-Type": "application/json" },
    };
    if (state.token) opts.headers["Authorization"] = "Bearer " + state.token;
    if (body) opts.body = JSON.stringify(body);
    return fetch(API + path, opts).then(function (r) {
      if (r.status === 401) {
        state.token = null;
        state.user = null;
        saveLocal("token", "");
        state.view = "login";
        render();
        return Promise.reject(new Error("unauthorized"));
      }
      if (r.status === 402) {
        showToast("阅读币不足，请充值后继续使用");
        return Promise.reject(new Error("insufficient_balance"));
      }
      return r;
    });
  }

  function apiJSON(method, path, body) {
    return api(method, path, body).then(function (r) { return r.json(); });
  }

  // ---- Data ----

  function fetchBooks() {
    return apiJSON("GET", "/api/books").then(function (books) {
      state.books = books || [];
    });
  }

  function fetchBook(id) {
    return apiJSON("GET", "/api/books/" + id);
  }

  function fetchMe() {
    return apiJSON("GET", "/api/me").then(function (u) {
      state.user = u;
    });
  }

  // ---- Audio ----

  function audioURL(bookId, index) {
    var pad = String(index);
    while (pad.length < 4) pad = "0" + pad;
    return API + "/data/audio/" + bookId + "/" + pad + ".mp3";
  }

  function playSentence(bookId, index, text, onEnd) {
    stopAudio();
    var url = audioURL(bookId, index);
    var audio = new Audio(url);
    state.currentAudio = audio;
    audio.onended = function () {
      state.currentAudio = null;
      if (onEnd) onEnd();
    };
    audio.onerror = function () {
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

  function stopAudio() {
    if (state.currentAudio) { state.currentAudio.pause(); state.currentAudio = null; }
    window.speechSynthesis.cancel();
  }

  function playCurrentSentence() {
    if (!state.currentBook || !state.playing) return;
    var paragraphs = state.currentBook.paragraphs;
    if (state.sentenceIndex >= paragraphs.length) {
      state.playing = false;
      render();
      return;
    }
    render();
    playSentence(state.currentBook.id, state.sentenceIndex, paragraphs[state.sentenceIndex], function () {
      if (!state.playing) return;
      setTimeout(function () {
        if (!state.playing) return;
        state.sentenceIndex++;
        saveProgress(state.currentBook.id, state.sentenceIndex);
        loadPhraseTranslations().then(playCurrentSentence);
      }, 1000);
    });
  }

  function replayCurrentSentence() {
    if (!state.currentBook) return;
    var paragraphs = state.currentBook.paragraphs;
    var text = paragraphs[state.sentenceIndex] || "";
    stopAudio();
    var wasPlaying = state.playing;
    playSentence(state.currentBook.id, state.sentenceIndex, text, function () {
      if (wasPlaying) {
        state.playing = true;
        playCurrentSentence();
      }
    });
  }

  // ---- Word click: pronounce + translate ----

  function onWordClick(word, contextSentence, x, y) {
    var clean = word.replace(/[^a-zA-Z'-]/g, "").toLowerCase();
    if (!clean) return;

    // Pronounce via OpenAI TTS
    api("POST", "/api/word-pronounce", { word: clean })
      .then(function (r) { return r.blob(); })
      .then(function (blob) {
        var url = URL.createObjectURL(blob);
        new Audio(url).play();
      })
      .catch(function () {
        // Fallback to browser TTS
        var u = new SpeechSynthesisUtterance(clean);
        u.lang = "en-US";
        window.speechSynthesis.speak(u);
      });

    // Translate
    apiJSON("POST", "/api/word-translate", { word: clean, context_sentence: contextSentence })
      .then(function (data) {
        state.wordPopup = { word: clean, translation: data.translation, x: x, y: y };
        renderWordPopup();
      })
      .catch(function () {});
  }

  function renderWordPopup() {
    // Remove existing
    var old = document.getElementById("word-popup");
    if (old) old.remove();

    if (!state.wordPopup) return;
    var p = state.wordPopup;
    var div = document.createElement("div");
    div.id = "word-popup";
    div.className = "word-popup";
    div.innerHTML = '<div class="word-popup-word">' + escapeHtml(p.word) + '</div>' +
      '<div class="word-popup-trans">' + escapeHtml(p.translation) + '</div>';
    div.style.left = Math.min(p.x, window.innerWidth - 160) + "px";
    div.style.top = (p.y - 70) + "px";
    document.body.appendChild(div);

    // Auto close
    setTimeout(function () {
      state.wordPopup = null;
      var el = document.getElementById("word-popup");
      if (el) el.remove();
    }, 3000);
  }

  // ---- Phrase splitting & translation ----

  var SKIP_WORDS = new Set([
    "a", "an", "the", "is", "am", "are", "was", "were", "be", "been", "being",
    "i", "he", "she", "it", "we", "they", "me", "him", "her", "us", "them",
    "my", "his", "its", "our", "your", "their",
    "in", "on", "at", "to", "of", "for", "with", "by", "from", "as",
    "and", "or", "but", "not", "no", "so",
    "do", "did", "does", "had", "has", "have",
    "that", "this", "who", "which", "what",
    "if", "than",
  ]);

  // Split paragraph into phrases (by commas, semicolons, "and", or every ~8 words)
  function splitPhrases(text) {
    // Split by punctuation delimiters
    var parts = text.split(/([,;:]\s+|\s+—\s+)/);
    var phrases = [];
    var current = "";
    for (var i = 0; i < parts.length; i++) {
      current += parts[i];
      // If this part is a delimiter or we've accumulated enough words
      var wordCount = current.trim().split(/\s+/).length;
      if (wordCount >= 5 || i === parts.length - 1) {
        var trimmed = current.trim();
        if (trimmed) phrases.push(trimmed);
        current = "";
      }
    }
    if (current.trim()) {
      if (phrases.length > 0) {
        phrases[phrases.length - 1] += " " + current.trim();
      } else {
        phrases.push(current.trim());
      }
    }
    return phrases.length > 0 ? phrases : [text];
  }

  function loadPhraseTranslations() {
    if (!state.currentBook) return Promise.resolve();
    var paragraph = state.currentBook.paragraphs[state.sentenceIndex] || "";
    var phrases = splitPhrases(paragraph);

    return apiJSON("POST", "/api/phrase-translate", {
      phrases: phrases,
      context_paragraph: paragraph,
    }).then(function (data) {
      state.phraseTranslations = data.translations || [];
    }).catch(function () {
      state.phraseTranslations = [];
    });
  }

  // ---- Render ----

  function render() {
    if (state.view === "login") renderLogin();
    else if (state.view === "list") renderBookList();
    else renderPlayer();
  }

  function renderLogin() {
    var html = '<div class="login-view">';
    html += '<div class="login-logo">磨耳朵</div>';
    html += '<div class="login-subtitle">沉浸式英语听读</div>';
    html += '<div class="login-buttons">';
    html += '<button class="login-btn login-btn-apple" id="login-apple">';
    html += '<svg viewBox="0 0 24 24" width="20" height="20"><path fill="currentColor" d="M17.05 20.28c-.98.95-2.05.88-3.08.4-1.09-.5-2.08-.48-3.24 0-1.44.62-2.2.44-3.06-.4C2.79 15.25 3.51 7.59 9.05 7.31c1.35.07 2.29.74 3.08.8 1.18-.24 2.31-.93 3.57-.84 1.51.12 2.65.72 3.4 1.8-3.12 1.87-2.38 5.98.48 7.13-.57 1.5-1.31 2.99-2.54 4.09zM12.03 7.25c-.15-2.23 1.66-4.07 3.74-4.25.29 2.58-2.34 4.5-3.74 4.25z"/></svg>';
    html += ' 使用 Apple 登录</button>';
    html += '<button class="login-btn login-btn-google" id="login-google">';
    html += '<svg viewBox="0 0 24 24" width="20" height="20"><path fill="#4285F4" d="M22.56 12.25c0-.78-.07-1.53-.2-2.25H12v4.26h5.92a5.06 5.06 0 01-2.2 3.32v2.77h3.57c2.08-1.92 3.28-4.74 3.28-8.1z"/><path fill="#34A853" d="M12 23c2.97 0 5.46-.98 7.28-2.66l-3.57-2.77c-.98.66-2.23 1.06-3.71 1.06-2.86 0-5.29-1.93-6.16-4.53H2.18v2.84C3.99 20.53 7.7 23 12 23z"/><path fill="#FBBC05" d="M5.84 14.09c-.22-.66-.35-1.36-.35-2.09s.13-1.43.35-2.09V7.07H2.18C1.43 8.55 1 10.22 1 12s.43 3.45 1.18 4.93l2.85-2.22.81-.62z"/><path fill="#EA4335" d="M12 5.38c1.62 0 3.06.56 4.21 1.64l3.15-3.15C17.45 2.09 14.97 1 12 1 7.7 1 3.99 3.47 2.18 7.07l3.66 2.84c.87-2.6 3.3-4.53 6.16-4.53z"/></svg>';
    html += ' 使用 Google 登录</button>';
    html += '</div>';
    html += '<div class="login-hint">登录后即可开始阅读</div>';
    html += '</div>';
    app.innerHTML = html;

    // Demo: for now, create a dev user
    document.getElementById("login-apple").addEventListener("click", function () {
      doLogin("apple");
    });
    document.getElementById("login-google").addEventListener("click", function () {
      doLogin("google");
    });
  }

  function doLogin(provider) {
    if (typeof AppShell === "undefined" || !AppShell.callNative) {
      alert("请在 App 内登录");
      return;
    }

    var adapterName = provider === "apple" ? "appleSignIn" : "googleSignIn";

    AppShell.callNative(adapterName, {}).then(function (result) {
      return apiJSON("POST", "/api/login", {
        provider: provider,
        id_token: result.id_token,
        name: result.name || "",
      });
    }).then(handleLoginResponse).catch(function (e) {
      if (e && e.message && e.message.indexOf("cancel") >= 0) return; // user cancelled
      alert("登录失败: " + (e.message || e));
    });
  }

  function handleLoginResponse(data) {
    if (data.token) {
      state.token = data.token;
      state.user = data.user;
      saveLocal("token", data.token);
      state.view = "list";
      fetchBooks().then(render);
    }
  }

  function renderBookList() {
    var html = '<div class="view">';
    // User bar
    if (state.user) {
      html += '<div class="user-bar">';
      html += '<span class="user-name">' + escapeHtml(state.user.name || state.user.email || "用户") + '</span>';
      html += '<span class="user-balance">' + (state.user.balance_coins || 0) + ' 阅读币</span>';
      html += '</div>';
    }
    html += '<div class="bookshelf">';
    for (var i = 0; i < state.books.length; i++) {
      var b = state.books[i];
      html += '<div class="book-3d" data-id="' + b.id + '">';
      html += '<div class="book-3d-inner">';
      html += '<div class="book-3d-spine"></div>';
      html += '<div class="book-3d-front">';
      if (b.cover) html += '<img src="' + API + '/data/' + b.cover + '" alt="">';
      else html += '<div class="book-3d-placeholder">' + escapeHtml(b.title) + '</div>';
      html += '</div>';
      html += '<div class="book-3d-top"></div>';
      html += '<div class="book-3d-right"></div>';
      html += '</div>';
      html += '<div class="book-3d-label">' + escapeHtml(b.title) + '</div>';
      html += '<div class="book-3d-progress" id="progress-' + b.id + '"></div>';
      html += '</div>';
    }
    html += '</div></div>';
    app.innerHTML = html;

    state.books.forEach(function (b) {
      var idx = loadProgress(b.id);
      var el = document.getElementById("progress-" + b.id);
      if (el && idx > 0) el.textContent = "已听到第 " + (idx + 1) + " 句";
    });

    var items = app.querySelectorAll(".book-3d");
    for (var j = 0; j < items.length; j++) {
      items[j].addEventListener("click", function () { openBook(this.getAttribute("data-id")); });
    }
  }

  function renderPlayer() {
    var book = state.currentBook;
    if (!book) return;
    var paragraphs = book.paragraphs;
    var idx = state.sentenceIndex;
    var paragraph = paragraphs[idx] || "";
    var total = paragraphs.length;

    var html = '<div class="view">';

    // Header
    html += '<div class="header-with-back">';
    html += '<button class="back-btn" id="back-btn">返回</button>';
    html += '<span class="header-title">' + escapeHtml(book.title) + '</span>';
    html += '<button class="text-toggle-btn' + (state.showText ? " active" : "") + '" id="text-toggle">文</button>';
    html += '<button class="text-toggle-btn' + (state.showTranslation ? " active" : "") + '" id="trans-toggle">译</button>';
    html += '</div>';

    // Body
    html += '<div class="player-body">';
    html += '<div class="sentence-counter">' + (idx + 1) + ' / ' + total + '</div>';

    // Phrases with inline translations
    var phrases = splitPhrases(paragraph);
    html += '<div class="phrases-container">';
    for (var p = 0; p < phrases.length; p++) {
      html += '<div class="phrase-block">';

      // English phrase with clickable words
      if (state.showText) {
        html += '<div class="phrase-text">';
        var words = phrases[p].split(/\s+/);
        for (var w = 0; w < words.length; w++) {
          var clean = words[w].replace(/[^a-zA-Z'-]/g, "").toLowerCase();
          var isAnnotated = clean.length > 1 && !SKIP_WORDS.has(clean);
          html += '<span class="word-clickable' + (isAnnotated ? " word-has-anno" : "") + '" data-word="' +
            escapeHtml(words[w]) + '" data-ctx="' + escapeHtml(phrases[p]) + '">' +
            escapeHtml(words[w]) + '</span> ';
        }
        html += '</div>';
      }

      // Phrase translation (from AI)
      if (state.showTranslation && state.phraseTranslations[p]) {
        html += '<div class="phrase-translation">' + escapeHtml(state.phraseTranslations[p]) + '</div>';
      }

      html += '</div>';
    }
    html += '</div>';

    // Playing indicator
    if (state.playing) {
      html += '<div class="playing-indicator"><span class="pulse-dot"></span> 正在播放</div>';
    }

    html += '</div>';

    // Controls
    html += '<div class="controls">';
    html += '<div class="progress-bar-container"><div class="progress-bar">';
    html += '<div class="progress-bar-fill" style="width:' + ((idx / Math.max(total - 1, 1)) * 100) + '%"></div>';
    html += '</div></div>';
    html += '<div class="main-controls">';
    html += '<button class="ctrl-btn" id="prev-btn"><svg viewBox="0 0 24 24"><path d="M6 6h2v12H6zm3.5 6l8.5 6V6z"/></svg></button>';
    html += '<button class="ctrl-btn replay-btn" id="replay-btn"><svg viewBox="0 0 24 24"><path d="M12 5V1L7 6l5 5V7c3.31 0 6 2.69 6 6s-2.69 6-6 6-6-2.69-6-6H4c0 4.42 3.58 8 8 8s8-3.58 8-8-3.58-8-8-8z"/></svg></button>';
    if (state.playing) {
      html += '<button class="play-btn" id="play-btn"><svg viewBox="0 0 24 24"><path d="M6 19h4V5H6v14zm8-14v14h4V5h-4z"/></svg></button>';
    } else {
      html += '<button class="play-btn" id="play-btn"><svg viewBox="0 0 24 24"><path d="M8 5v14l11-7z"/></svg></button>';
    }
    html += '<button class="ctrl-btn" id="next-btn"><svg viewBox="0 0 24 24"><path d="M6 18l8.5-6L6 6v12zM16 6v12h2V6h-2z"/></svg></button>';
    html += '</div></div></div>';

    app.innerHTML = html;
    bindPlayerEvents(book, paragraphs);
  }

  function bindPlayerEvents(book, paragraphs) {
    document.getElementById("back-btn").addEventListener("click", function () {
      stopAudio();
      state.playing = false;
      state.view = "list";
      state.currentBook = null;
      state.wordPopup = null;
      render();
    });

    document.getElementById("text-toggle").addEventListener("click", function () {
      state.showText = !state.showText;
      render();
    });

    document.getElementById("trans-toggle").addEventListener("click", function () {
      state.showTranslation = !state.showTranslation;
      render();
    });

    document.getElementById("replay-btn").addEventListener("click", function () {
      replayCurrentSentence();
    });

    document.getElementById("play-btn").addEventListener("click", function () {
      if (state.playing) {
        stopAudio();
        state.playing = false;
        render();
      } else {
        state.playing = true;
        playCurrentSentence();
      }
    });

    document.getElementById("prev-btn").addEventListener("click", function () {
      if (state.sentenceIndex > 0) {
        stopAudio();
        state.playing = false;
        state.sentenceIndex--;
        saveProgress(book.id, state.sentenceIndex);
        loadPhraseTranslations().then(render);
      }
    });

    document.getElementById("next-btn").addEventListener("click", function () {
      if (state.sentenceIndex < paragraphs.length - 1) {
        stopAudio();
        state.playing = false;
        state.sentenceIndex++;
        saveProgress(book.id, state.sentenceIndex);
        loadPhraseTranslations().then(render);
      }
    });

    // Word click events
    var wordEls = document.querySelectorAll(".word-clickable");
    for (var i = 0; i < wordEls.length; i++) {
      wordEls[i].addEventListener("click", function (e) {
        var word = this.getAttribute("data-word");
        var ctx = this.getAttribute("data-ctx");
        var rect = this.getBoundingClientRect();
        onWordClick(word, ctx, rect.left + rect.width / 2, rect.top);
      });
    }

    // Close popup on body click
    document.addEventListener("click", function (e) {
      if (!e.target.closest(".word-clickable") && !e.target.closest(".word-popup")) {
        state.wordPopup = null;
        var el = document.getElementById("word-popup");
        if (el) el.remove();
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
      if (!book) return;
      state.currentBook = book;
      state.sentenceIndex = loadProgress(id);
      state.view = "player";
      state.showText = true;
      state.showTranslation = true;
      state.playing = false;
      loadPhraseTranslations().then(function () {
        render();
      });
    });
  }

  // ---- Init ----

  function init() {
    var savedToken = loadLocal("token");
    if (savedToken) {
      state.token = savedToken;
      fetchMe().then(function () {
        state.view = "list";
        return fetchBooks();
      }).then(render).catch(function () {
        state.view = "login";
        render();
      });
    } else {
      render();
    }
  }

  init();
})();
