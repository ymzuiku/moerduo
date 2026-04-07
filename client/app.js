(function () {
  "use strict";

  var app = document.getElementById("app");
  var API = (typeof appshell_api_base !== "undefined" && appshell_api_base) || localStorage.getItem("appshell_api_base") || location.origin;

  var state = {
    view: "hello-world", // hello-world | login | list | player | my-books | edit-book | public-books
    listTab: "library", // library | my-books | public
    token: null,
    user: null,
    books: [],          // system books
    myBooks: [],        // user-created books
    publicBooks: [],    // community public books
    currentBook: null,  // { id, title, cover, paragraphs[], isUserBook }
    sentenceIndex: 0,
    playing: false,
    showText: true,
    showTranslation: true,
    currentAudio: null,
    phraseTranslations: [],
    wordPopup: null,    // { word, translation, x, y }
    editBook: null,     // book being created/edited
    currentPhraseIndex: 0,
    phrases: [],
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

  function fetchMyBooks() {
    return apiJSON("GET", "/api/user-books").then(function (books) {
      state.myBooks = books || [];
    });
  }

  function fetchPublicBooks() {
    return apiJSON("GET", "/api/public-books").then(function (books) {
      state.publicBooks = books || [];
    });
  }

  function fetchBook(id) {
    return apiJSON("GET", "/api/books/" + id);
  }

  function fetchUserBook(id) {
    return apiJSON("GET", "/api/user-books/" + id).then(function (b) {
      if (!b) return null;
      b.paragraphs = parseParagraphs(b.paragraphs);
      b.isUserBook = true;
      return b;
    });
  }

  function parseParagraphs(val) {
    if (Array.isArray(val)) return val;
    try { return JSON.parse(val) || []; } catch (e) { return []; }
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

  function setupPhraseSync(audio, phrases) {
    if (!audio || phrases.length <= 1) return;
    var wordCounts = phrases.map(function (p) { return p.split(/\s+/).filter(Boolean).length; });
    var totalWords = wordCounts.reduce(function (a, b) { return a + b; }, 0);
    if (totalWords === 0) return;
    var startFractions = [];
    var cum = 0;
    for (var i = 0; i < phrases.length; i++) {
      startFractions.push(cum);
      cum += wordCounts[i] / totalWords;
    }
    var lastIdx = 0;
    audio.addEventListener('timeupdate', function () {
      var dur = audio.duration;
      if (!dur || isNaN(dur) || dur === 0) return;
      var progress = audio.currentTime / dur;
      var idx = 0;
      for (var i = phrases.length - 1; i >= 0; i--) {
        if (progress >= startFractions[i]) { idx = i; break; }
      }
      if (idx !== lastIdx) {
        lastIdx = idx;
        state.currentPhraseIndex = idx;
        updateKaraokeDisplay();
      }
    });
  }

  function buildPhraseWordHtml(phraseText) {
    var html = '';
    var words = (phraseText || '').split(/\s+/).filter(Boolean);
    for (var w = 0; w < words.length; w++) {
      var clean = words[w].replace(/[^a-zA-Z'-]/g, "").toLowerCase();
      var isAnnotated = clean.length > 1 && !SKIP_WORDS.has(clean);
      html += '<span class="word-clickable' + (isAnnotated ? ' word-has-anno' : '') + '" data-word="' +
        escapeHtml(words[w]) + '" data-ctx="' + escapeHtml(phraseText) + '">' +
        escapeHtml(words[w]) + '</span> ';
    }
    return html;
  }

  function updateKaraokeDisplay() {
    var phrases = state.phrases;
    if (!phrases || phrases.length === 0) return;
    var idx = Math.min(state.currentPhraseIndex, phrases.length - 1);
    var prevEl = document.getElementById('karaoke-prev');
    var currEnEl = document.getElementById('karaoke-current-en');
    var currZhEl = document.getElementById('karaoke-current-zh');
    var nextEl = document.getElementById('karaoke-next');
    if (!currEnEl) return;
    if (prevEl) prevEl.textContent = idx > 0 ? phrases[idx - 1] : '';
    currEnEl.innerHTML = state.showText ? buildPhraseWordHtml(phrases[idx]) : '';
    var wordEls = currEnEl.querySelectorAll('.word-clickable');
    for (var i = 0; i < wordEls.length; i++) {
      wordEls[i].addEventListener('click', function () {
        var word = this.getAttribute('data-word');
        var ctx = this.getAttribute('data-ctx');
        var rect = this.getBoundingClientRect();
        onWordClick(word, ctx, rect.left + rect.width / 2, rect.top);
      });
    }
    currEnEl.classList.remove('karaoke-animate');
    void currEnEl.offsetWidth;
    currEnEl.classList.add('karaoke-animate');
    if (currZhEl) currZhEl.textContent = state.showTranslation ? (state.phraseTranslations[idx] || '') : '';
    if (nextEl) nextEl.textContent = idx < phrases.length - 1 ? phrases[idx + 1] : '';
  }

  function playCurrentSentence() {
    if (!state.currentBook || !state.playing) return;
    var paragraphs = state.currentBook.paragraphs;
    if (state.sentenceIndex >= paragraphs.length) {
      state.playing = false;
      render();
      return;
    }
    var paragraph = paragraphs[state.sentenceIndex];
    state.phrases = splitPhrases(paragraph);
    state.currentPhraseIndex = 0;
    render();
    playSentence(state.currentBook.id, state.sentenceIndex, paragraph, function () {
      if (!state.playing) return;
      setTimeout(function () {
        if (!state.playing) return;
        state.sentenceIndex++;
        saveProgress(state.currentBook.id, state.sentenceIndex);
        loadPhraseTranslations().then(playCurrentSentence);
      }, 1000);
    });
    if (state.currentAudio) setupPhraseSync(state.currentAudio, state.phrases);
  }

  function replayCurrentSentence() {
    if (!state.currentBook) return;
    var paragraphs = state.currentBook.paragraphs;
    var text = paragraphs[state.sentenceIndex] || "";
    stopAudio();
    var wasPlaying = state.playing;
    state.currentPhraseIndex = 0;
    updateKaraokeDisplay();
    playSentence(state.currentBook.id, state.sentenceIndex, text, function () {
      if (wasPlaying) {
        state.playing = true;
        playCurrentSentence();
      }
    });
    if (state.currentAudio) setupPhraseSync(state.currentAudio, state.phrases);
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
    if (state.view === "hello-world") renderHelloWorld();
    else if (state.view === "login") renderLogin();
    else if (state.view === "list") renderBookList();
    else if (state.view === "edit-book") renderEditBook();
    else renderPlayer();
  }

  function renderHelloWorld() {
    app.innerHTML =
      '<div style="display:flex;flex-direction:column;align-items:center;justify-content:center;height:100vh;gap:24px;padding:24px;text-align:center;">' +
      '<div style="font-size:48px;">👂</div>' +
      '<h1 style="font-size:28px;font-weight:700;margin:0;">先搞定听力</h1>' +
      '<p style="color:#888;margin:0;">Hello World!</p>' +
      '<button id="hw-start" style="margin-top:16px;padding:14px 32px;background:#4f46e5;color:#fff;border:none;border-radius:12px;font-size:16px;cursor:pointer;">开始使用</button>' +
      '</div>';
    document.getElementById("hw-start").addEventListener("click", function () {
      state.view = "login";
      render();
    });
  }

  function renderUserBar(showBack) {
    var html = '<div class="user-bar">';
    if (showBack) {
      html += '<button class="back-btn user-bar-back" id="back-btn">返回</button>';
    } else if (state.user) {
      html += '<span class="user-name">' + escapeHtml(state.user.name || state.user.email || "用户") + '</span>';
    }
    if (state.user) {
      var coins = state.user.balance_coins || 0;
      var coinClass = coins < 20 ? "user-balance low" : "user-balance";
      html += '<span class="' + coinClass + '" id="balance-btn">' + coins + ' 阅读币</span>';
    }
    html += '</div>';
    return html;
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
    if (typeof AppShell === "undefined" || !AppShell.auth) {
      showToast("请在 App 内登录");
      return;
    }

    AppShell.auth.login(provider).then(function (result) {
      if (result.error) {
        if (result.error === "cancelled") return;
        throw new Error(result.error);
      }
      var idToken = result.token;
      var name = "";
      if (result.fullName) {
        name = [result.fullName.given, result.fullName.family].filter(Boolean).join(" ");
      }
      if (!name && result.email) name = result.email.split("@")[0];
      return apiJSON("POST", "/api/login", {
        provider: provider,
        id_token: idToken,
        name: name,
      });
    }).then(function (data) {
      if (data) handleLoginResponse(data);
    }).catch(function (e) {
      if (!e || (e.message && e.message.indexOf("cancel") >= 0)) return;
      showToast("登录失败: " + (e.message || e));
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
    var tab = state.listTab;
    var html = '<div class="view">';
    html += renderUserBar(false);

    // Tabs
    html += '<div class="tabs">';
    html += '<button class="tab-btn' + (tab === "library" ? " active" : "") + '" data-tab="library">精选</button>';
    html += '<button class="tab-btn' + (tab === "my-books" ? " active" : "") + '" data-tab="my-books">我的书</button>';
    html += '<button class="tab-btn' + (tab === "public" ? " active" : "") + '" data-tab="public">广场</button>';
    html += '</div>';

    if (tab === "library") {
      html += renderBookGrid(state.books, false);
    } else if (tab === "my-books") {
      html += '<div class="my-books-actions">';
      html += '<button class="create-book-btn" id="create-book-btn">+ 创建新书</button>';
      html += '</div>';
      html += renderMyBookList(state.myBooks);
    } else {
      html += renderBookGrid(state.publicBooks, true);
    }

    html += '</div>';
    app.innerHTML = html;

    // Low balance warning
    if (state.user && state.user.balance_coins < 20) {
      showToast("阅读币不足 20，请及时充值");
    }

    // Tab switching
    var tabBtns = app.querySelectorAll(".tab-btn");
    for (var i = 0; i < tabBtns.length; i++) {
      tabBtns[i].addEventListener("click", function () {
        var t = this.getAttribute("data-tab");
        if (state.listTab === t) return;
        state.listTab = t;
        if (t === "public" && state.publicBooks.length === 0) {
          fetchPublicBooks().then(render);
        } else if (t === "my-books" && state.myBooks.length === 0) {
          fetchMyBooks().then(render);
        } else {
          render();
        }
      });
    }

    // Create book
    var createBtn = document.getElementById("create-book-btn");
    if (createBtn) {
      createBtn.addEventListener("click", function () {
        state.editBook = { id: null, title: "", paragraphs: "" };
        state.view = "edit-book";
        render();
      });
    }

    // Book cards
    var bookCards = app.querySelectorAll("[data-book-id]");
    for (var j = 0; j < bookCards.length; j++) {
      bookCards[j].addEventListener("click", function () {
        var id = this.getAttribute("data-book-id");
        var isUser = this.getAttribute("data-user-book") === "1";
        if (isUser) openUserBook(id);
        else openBook(id);
      });
    }

    // My-book action buttons
    app.querySelectorAll("[data-edit-book]").forEach(function (el) {
      el.addEventListener("click", function (e) {
        e.stopPropagation();
        var id = this.getAttribute("data-edit-book");
        var book = state.myBooks.find(function (b) { return b.id === id; });
        if (book) {
          var paras = parseParagraphs(book.paragraphs);
          state.editBook = { id: book.id, title: book.title, paragraphs: paras.join("\n") };
          state.view = "edit-book";
          render();
        }
      });
    });

    app.querySelectorAll("[data-publish-book]").forEach(function (el) {
      el.addEventListener("click", function (e) {
        e.stopPropagation();
        var id = this.getAttribute("data-publish-book");
        if (!confirm("公开后无法取消，确定公开此书吗？")) return;
        apiJSON("POST", "/api/user-books/" + id + "/publish", {}).then(function () {
          showToast("已公开！其他用户可以阅读了");
          fetchMyBooks().then(render);
        }).catch(function (e) {
          showToast("公开失败: " + (e.message || "请重试"));
        });
      });
    });

    app.querySelectorAll("[data-cover-book]").forEach(function (el) {
      el.addEventListener("click", function (e) {
        e.stopPropagation();
        var id = this.getAttribute("data-cover-book");
        var title = this.getAttribute("data-cover-title");
        apiJSON("POST", "/api/user-books/" + id + "/cover", { prompt: title }).then(function (res) {
          showToast("封面生成成功！");
          fetchMyBooks().then(function () { fetchMe().then(render); });
        }).catch(function (e) {
          showToast("封面生成失败: " + (e.message || "请重试"));
        });
      });
    });
  }

  function renderBookGrid(books, isPublic) {
    if (!books || books.length === 0) {
      return '<div class="empty-tip">' + (isPublic ? "暂无公开书本" : "暂无书籍") + '</div>';
    }
    var html = '<div class="bookshelf">';
    for (var i = 0; i < books.length; i++) {
      var b = books[i];
      var coverSrc = b.cover ? API + "/data/" + b.cover : "";
      html += '<div class="book-3d" data-book-id="' + b.id + '" data-user-book="' + (isPublic ? "1" : "0") + '">';
      html += '<div class="book-3d-inner">';
      html += '<div class="book-3d-spine"></div>';
      html += '<div class="book-3d-front">';
      if (coverSrc) html += '<img src="' + coverSrc + '" alt="">';
      else html += '<div class="book-3d-placeholder">' + escapeHtml(b.title) + '</div>';
      html += '</div>';
      html += '<div class="book-3d-top"></div>';
      html += '<div class="book-3d-right"></div>';
      html += '</div>';
      html += '<div class="book-3d-label">' + escapeHtml(b.title) + '</div>';
      if (!isPublic) {
        var idx = loadProgress(b.id);
        if (idx > 0) html += '<div class="book-3d-progress">已读第 ' + (idx + 1) + ' 句</div>';
      }
      html += '</div>';
    }
    html += '</div>';
    return html;
  }

  function renderMyBookList(books) {
    if (!books || books.length === 0) {
      return '<div class="empty-tip">还没有创建书本</div>';
    }
    var html = '<div class="my-book-list">';
    for (var i = 0; i < books.length; i++) {
      var b = books[i];
      var paras = parseParagraphs(b.paragraphs);
      html += '<div class="my-book-item" data-book-id="' + b.id + '" data-user-book="1">';
      html += '<div class="my-book-cover">';
      if (b.cover) html += '<img src="' + API + '/data/' + b.cover + '" alt="">';
      else html += '<div class="my-book-cover-placeholder">' + escapeHtml(b.title.slice(0, 2)) + '</div>';
      html += '</div>';
      html += '<div class="my-book-info">';
      html += '<div class="my-book-title">' + escapeHtml(b.title) + '</div>';
      html += '<div class="my-book-meta">' + paras.length + ' 段' +
        (b.is_public ? ' · <span class="public-badge">已公开</span>' : '') + '</div>';
      html += '</div>';
      html += '<div class="my-book-actions">';
      html += '<button class="my-book-btn" data-edit-book="' + b.id + '">编辑</button>';
      if (!b.cover) {
        html += '<button class="my-book-btn" data-cover-book="' + b.id + '" data-cover-title="' + escapeHtml(b.title) + '">封面 <small>(4币)</small></button>';
      }
      if (!b.is_public) {
        html += '<button class="my-book-btn my-book-btn-publish" data-publish-book="' + b.id + '">公开</button>';
      }
      html += '</div>';
      html += '</div>';
    }
    html += '</div>';
    return html;
  }

  function renderEditBook() {
    var isNew = !state.editBook.id;
    var html = '<div class="view">';
    html += renderUserBar(false);
    html += '<div class="edit-book-header">';
    html += '<button class="back-btn" id="back-btn">取消</button>';
    html += '<span class="header-title">' + (isNew ? "创建新书" : "编辑书本") + '</span>';
    html += '<button class="save-btn" id="save-book-btn">保存</button>';
    html += '</div>';
    html += '<div class="edit-book-body">';
    html += '<label class="edit-label">书名</label>';
    html += '<input id="edit-title" class="edit-input" placeholder="请输入书名" value="' + escapeHtml(state.editBook.title) + '">';
    html += '<label class="edit-label">段落内容 <small>（每行一个段落）</small></label>';
    html += '<textarea id="edit-paragraphs" class="edit-textarea" placeholder="每行一个英文段落，例如：\nThe sun rose slowly over the mountains.\nShe walked along the quiet path...">' + escapeHtml(state.editBook.paragraphs) + '</textarea>';
    html += '</div>';
    html += '</div>';
    app.innerHTML = html;

    document.getElementById("back-btn").addEventListener("click", function () {
      state.view = "list";
      state.listTab = "my-books";
      render();
    });

    document.getElementById("save-book-btn").addEventListener("click", function () {
      var title = document.getElementById("edit-title").value.trim();
      var raw = document.getElementById("edit-paragraphs").value.trim();
      if (!title) { showToast("请输入书名"); return; }
      var paras = raw.split(/\n+/).map(function (l) { return l.trim(); }).filter(Boolean);
      if (paras.length === 0) { showToast("请输入至少一段内容"); return; }
      saveBook(state.editBook.id, title, paras);
    });
  }

  function saveBook(id, title, paragraphs) {
    var paragraphsJSON = JSON.stringify(paragraphs);
    if (!id) {
      // Create
      apiJSON("POST", "/api/user-books", { title: title }).then(function (book) {
        if (!book || !book.id) throw new Error("创建失败");
        return apiJSON("PUT", "/api/user-books/" + book.id, {
          title: title, cover: "", paragraphs: paragraphsJSON
        }).then(function () { return book.id; });
      }).then(function () {
        showToast("书本已创建");
        state.view = "list";
        state.listTab = "my-books";
        fetchMyBooks().then(render);
      }).catch(function (e) { showToast("创建失败: " + (e.message || "请重试")); });
    } else {
      // Update
      apiJSON("PUT", "/api/user-books/" + id, {
        title: title, cover: "", paragraphs: paragraphsJSON
      }).then(function () {
        showToast("已保存");
        state.view = "list";
        state.listTab = "my-books";
        fetchMyBooks().then(render);
      }).catch(function (e) { showToast("保存失败: " + (e.message || "请重试")); });
    }
  }

  function renderPlayer() {
    var book = state.currentBook;
    if (!book) return;
    var paragraphs = book.paragraphs;
    var idx = state.sentenceIndex;
    var paragraph = paragraphs[idx] || "";
    var total = paragraphs.length;

    if (!state.phrases || state.phrases.length === 0) {
      state.phrases = splitPhrases(paragraph);
    }
    var phrases = state.phrases;
    var pi = Math.min(state.currentPhraseIndex, phrases.length - 1);

    var html = '<div class="view">';

    // Header
    html += '<div class="header-with-back">';
    html += '<button class="back-btn" id="back-btn">返回</button>';
    html += '<span class="header-title">' + escapeHtml(book.title) + '</span>';
    html += '<button class="text-toggle-btn' + (state.showText ? " active" : "") + '" id="text-toggle">文</button>';
    html += '<button class="text-toggle-btn' + (state.showTranslation ? " active" : "") + '" id="trans-toggle">译</button>';
    html += '</div>';

    // Karaoke Stage
    html += '<div class="karaoke-stage">';

    var prevText = pi > 0 ? phrases[pi - 1] : '';
    html += '<div class="karaoke-prev" id="karaoke-prev">' + escapeHtml(prevText) + '</div>';

    html += '<div class="karaoke-current">';
    if (state.showText) {
      html += '<div class="karaoke-english karaoke-animate" id="karaoke-current-en">';
      html += buildPhraseWordHtml(phrases[pi] || '');
      html += '</div>';
    } else {
      html += '<div class="karaoke-english" id="karaoke-current-en"></div>';
    }
    var curTrans = state.showTranslation ? (state.phraseTranslations[pi] || '') : '';
    html += '<div class="karaoke-translation" id="karaoke-current-zh">' + escapeHtml(curTrans) + '</div>';
    html += '</div>';

    var nextText = pi < phrases.length - 1 ? phrases[pi + 1] : '';
    html += '<div class="karaoke-next" id="karaoke-next">' + escapeHtml(nextText) + '</div>';

    html += '</div>';

    // Controls
    html += '<div class="controls">';
    html += '<div class="sentence-counter">' + (idx + 1) + ' / ' + total + '</div>';
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
      if (book.isUserBook) state.listTab = "my-books";
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
        state.currentPhraseIndex = 0;
        state.phrases = [];
        saveProgress(book.id, state.sentenceIndex);
        loadPhraseTranslations().then(render);
      }
    });

    document.getElementById("next-btn").addEventListener("click", function () {
      if (state.sentenceIndex < paragraphs.length - 1) {
        stopAudio();
        state.playing = false;
        state.sentenceIndex++;
        state.currentPhraseIndex = 0;
        state.phrases = [];
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

  function openUserBook(id) {
    // Try local state first
    var book = state.myBooks.find(function (b) { return b.id === id; }) ||
               state.publicBooks.find(function (b) { return b.id === id; });
    var promise = book ? Promise.resolve(book) : fetchUserBook(id);
    promise.then(function (b) {
      if (!b) return;
      var paras = parseParagraphs(b.paragraphs);
      state.currentBook = {
        id: b.id,
        title: b.title,
        cover: b.cover,
        paragraphs: paras,
        isUserBook: true,
      };
      state.sentenceIndex = loadProgress(b.id);
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
        return Promise.all([fetchBooks(), fetchMyBooks()]);
      }).then(render).catch(function () {
        state.token = null;
        state.view = "login";
        render();
      });
    } else {
      render();
    }
  }

  init();
})();
