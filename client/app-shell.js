/**
 * AppShell JS Bridge
 * Unified API for communicating with native iOS/Android shells.
 * Falls back gracefully in regular browsers (storage uses IndexedDB).
 */
(function () {
  "use strict";

  var _callbackId = 0;
  var _callbacks = {};
  var _eventListeners = {};

  // Detect platform.
  var platform = "web";
  if (window.webkit && window.webkit.messageHandlers && window.webkit.messageHandlers.appShell) {
    platform = "ios";
  } else if (window.AppShellAndroid) {
    platform = "android";
  }

  // Send message to native.
  function callNative(module, action, params) {
    return new Promise(function (resolve) {
      var id = "cb_" + (++_callbackId);
      _callbacks[id] = resolve;

      var message = {
        module: module,
        action: action,
        params: params || {},
        callbackId: id,
      };

      if (platform === "ios") {
        window.webkit.messageHandlers.appShell.postMessage(message);
      } else if (platform === "android") {
        window.AppShellAndroid.postMessage(JSON.stringify(message));
      } else {
        delete _callbacks[id];
        resolve({ supported: false });
      }

      setTimeout(function () {
        if (_callbacks[id]) {
          delete _callbacks[id];
          resolve({ error: "timeout" });
        }
      }, 30000);
    });
  }

  function _callback(callbackId, result) {
    var cb = _callbacks[callbackId];
    if (cb) {
      delete _callbacks[callbackId];
      cb(result);
    }
  }

  function _onEvent(name, data) {
    var listeners = _eventListeners[name] || [];
    for (var i = 0; i < listeners.length; i++) {
      try { listeners[i](data); } catch (e) { console.error("AppShell event error:", e); }
    }
  }

  function on(eventName, callback) {
    if (!_eventListeners[eventName]) _eventListeners[eventName] = [];
    _eventListeners[eventName].push(callback);
  }

  function off(eventName, callback) {
    var list = _eventListeners[eventName];
    if (!list) return;
    _eventListeners[eventName] = list.filter(function (cb) { return cb !== callback; });
  }

  // --- IndexedDB fallback for web storage ---

  var DB_NAME = "appshell_storage";
  var STORE_NAME = "kv";
  var _dbPromise = null;

  function getDB() {
    if (_dbPromise) return _dbPromise;
    _dbPromise = new Promise(function (resolve, reject) {
      var req = indexedDB.open(DB_NAME, 1);
      req.onupgradeneeded = function () {
        req.result.createObjectStore(STORE_NAME);
      };
      req.onsuccess = function () { resolve(req.result); };
      req.onerror = function () { reject(req.error); };
    });
    return _dbPromise;
  }

  function idbGet(key) {
    return getDB().then(function (db) {
      return new Promise(function (resolve) {
        var tx = db.transaction(STORE_NAME, "readonly");
        var req = tx.objectStore(STORE_NAME).get(key);
        req.onsuccess = function () { resolve(req.result !== undefined ? req.result : null); };
        req.onerror = function () { resolve(null); };
      });
    });
  }

  function idbSet(key, value) {
    return getDB().then(function (db) {
      return new Promise(function (resolve) {
        var tx = db.transaction(STORE_NAME, "readwrite");
        tx.objectStore(STORE_NAME).put(value, key);
        tx.oncomplete = function () { resolve(); };
        tx.onerror = function () { resolve(); };
      });
    });
  }

  function idbRemove(key) {
    return getDB().then(function (db) {
      return new Promise(function (resolve) {
        var tx = db.transaction(STORE_NAME, "readwrite");
        tx.objectStore(STORE_NAME).delete(key);
        tx.oncomplete = function () { resolve(); };
        tx.onerror = function () { resolve(); };
      });
    });
  }

  function idbKeys() {
    return getDB().then(function (db) {
      return new Promise(function (resolve) {
        var tx = db.transaction(STORE_NAME, "readonly");
        var req = tx.objectStore(STORE_NAME).getAllKeys();
        req.onsuccess = function () { resolve(req.result || []); };
        req.onerror = function () { resolve([]); };
      });
    });
  }

  function idbGetAll() {
    return getDB().then(function (db) {
      return new Promise(function (resolve) {
        var tx = db.transaction(STORE_NAME, "readonly");
        var store = tx.objectStore(STORE_NAME);
        var keys = store.getAllKeys();
        var vals = store.getAll();
        tx.oncomplete = function () {
          var data = {};
          var ks = keys.result || [];
          var vs = vals.result || [];
          for (var i = 0; i < ks.length; i++) data[ks[i]] = vs[i];
          resolve(data);
        };
        tx.onerror = function () { resolve({}); };
      });
    });
  }

  function idbClear() {
    return getDB().then(function (db) {
      return new Promise(function (resolve) {
        var tx = db.transaction(STORE_NAME, "readwrite");
        tx.objectStore(STORE_NAME).clear();
        tx.oncomplete = function () { resolve(); };
        tx.onerror = function () { resolve(); };
      });
    });
  }

  // --- Public API ---

  window.AppShell = {
    platform: platform,
    isNative: platform !== "web",

    _callback: _callback,
    _onEvent: _onEvent,

    // Auth
    auth: {
      login: function (provider) {
        return callNative("auth", "login", { provider: provider });
      },
      getProviders: function () {
        return callNative("auth", "getProviders");
      },
    },

    // Lifecycle
    lifecycle: {
      getState: function () {
        return callNative("lifecycle", "getState");
      },
      on: function (event, callback) {
        on("lifecycle." + event, callback);
      },
      off: function (event, callback) {
        off("lifecycle." + event, callback);
      },
    },

    // Haptic
    haptic: {
      impact: function (style) {
        return callNative("haptic", "impact", { style: style || "medium" });
      },
      notification: function (type) {
        return callNative("haptic", "notification", { type: type || "success" });
      },
    },

    // Storage (native persistent — survives cache clearing)
    // Native: UserDefaults (iOS) / SharedPreferences (Android)
    // Web fallback: IndexedDB
    // All methods return Promises. Values are strings; use JSON.stringify/parse for objects.
    storage: {
      get: function (key) {
        if (platform === "web") return idbGet(key).then(function (v) { return { value: v }; });
        return callNative("storage", "get", { key: key });
      },
      set: function (key, value) {
        if (platform === "web") return idbSet(key, value).then(function () { return { ok: true }; });
        return callNative("storage", "set", { key: key, value: value });
      },
      remove: function (key) {
        if (platform === "web") return idbRemove(key).then(function () { return { ok: true }; });
        return callNative("storage", "remove", { key: key });
      },
      has: function (key) {
        if (platform === "web") return idbGet(key).then(function (v) { return { exists: v !== null }; });
        return callNative("storage", "has", { key: key });
      },
      keys: function () {
        if (platform === "web") return idbKeys().then(function (k) { return { keys: k }; });
        return callNative("storage", "keys");
      },
      getAll: function () {
        if (platform === "web") return idbGetAll().then(function (d) { return { data: d }; });
        return callNative("storage", "getAll");
      },
      clear: function () {
        if (platform === "web") return idbClear().then(function () { return { ok: true }; });
        return callNative("storage", "clear");
      },
    },

    // Speech-to-text (on-device)
    // Native: SFSpeechRecognizer (iOS) / android.speech.SpeechRecognizer (Android)
    // Web fallback: Web Speech API (SpeechRecognition)
    speech: {
      // Check if speech recognition is available on this device.
      isAvailable: function (language) {
        if (platform === "web") {
          var SR = window.SpeechRecognition || window.webkitSpeechRecognition;
          return Promise.resolve({
            available: !!SR,
            onDevice: false, // Web Speech API is typically cloud-based
            language: language || "zh-CN",
          });
        }
        return callNative("speech", "isAvailable", { language: language || "zh-CN" });
      },

      // Start listening. Returns { ok: true } when started.
      // Results come via events: speech.partial, speech.result, speech.error
      start: function (language) {
        if (platform === "web") {
          return _webSpeechStart(language || "zh-CN");
        }
        return callNative("speech", "start", { language: language || "zh-CN" });
      },

      // Stop listening.
      stop: function () {
        if (platform === "web") {
          return _webSpeechStop();
        }
        return callNative("speech", "stop");
      },

      // Convenience: listen for events.
      on: function (event, callback) {
        on("speech." + event, callback);
      },
      off: function (event, callback) {
        off("speech." + event, callback);
      },
    },

    // Device info
    // Returns device metadata and capability flags.
    // Data structure:
    //   getInfo() → {
    //     platform: "ios" | "android" | "web",
    //     osVersion: "18.3",          model: "iPhone",
    //     modelName: "iPhone16,1",    screenWidth: 393,
    //     screenHeight: 852,          screenScale: 3,
    //     language: "zh",             region: "CN",
    //     timezone: "Asia/Shanghai",  isTablet: false,
    //     batteryLevel: 0.85,         batteryState: "charging" | "unplugged" | "full" | "unknown",
    //     appVersion: "1.0.0",        buildNumber: "1",
    //     bundleId: "com.example.app"
    //   }
    //   getCapabilities() → {
    //     hasCamera: true,            hasMicrophone: true,
    //     hasHaptic: true,            hasSpeechRecognition: true,
    //     hasBiometrics: true,        biometricType: "faceId" | "touchId" | "none"
    //   }
    // Device info, safe area, status bar, dynamic island
    device: {
      // Full device info including safe area insets.
      getInfo: function () {
        if (platform === "web") {
          var ua = navigator.userAgent;
          return Promise.resolve({
            platform: "web",
            osVersion: navigator.platform || "",
            model: ua.indexOf("Mobile") > -1 ? "Mobile" : "Desktop",
            modelName: navigator.platform || "",
            screenWidth: window.screen.width,
            screenHeight: window.screen.height,
            screenScale: window.devicePixelRatio || 1,
            language: navigator.language ? navigator.language.split("-")[0] : "en",
            region: navigator.language ? (navigator.language.split("-")[1] || "") : "",
            timezone: Intl.DateTimeFormat().resolvedOptions().timeZone || "",
            isTablet: false,
            batteryLevel: -1,
            batteryState: "unknown",
            appVersion: "",
            buildNumber: "",
            bundleId: "",
            safeAreaTop: 0,
            safeAreaBottom: 0,
            safeAreaLeft: 0,
            safeAreaRight: 0,
            hasDynamicIsland: false,
          });
        }
        return callNative("device", "getInfo");
      },

      // Safe area insets (px) — use for padding content away from notch/home indicator.
      // { top: 59, bottom: 34, left: 0, right: 0 }
      getSafeArea: function () {
        if (platform === "web") {
          return Promise.resolve({ top: 0, bottom: 0, left: 0, right: 0 });
        }
        return callNative("device", "getSafeArea");
      },

      // Capability flags.
      getCapabilities: function () {
        if (platform === "web") {
          return Promise.resolve({
            hasCamera: !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia),
            hasMicrophone: !!(navigator.mediaDevices && navigator.mediaDevices.getUserMedia),
            hasHaptic: !!navigator.vibrate,
            hasSpeechRecognition: !!(window.SpeechRecognition || window.webkitSpeechRecognition),
            hasBiometrics: false,
            biometricType: "none",
            hasDynamicIsland: false,
          });
        }
        return callNative("device", "getCapabilities");
      },

      // Set status bar text color: "light" (white text) or "dark" (black text).
      setStatusBarStyle: function (style) {
        if (platform === "web") return Promise.resolve({ ok: true });
        return callNative("device", "setStatusBarStyle", { style: style || "light" });
      },

      // Check if device has Dynamic Island.
      hasDynamicIsland: function () {
        if (platform === "web") return Promise.resolve({ hasDynamicIsland: false });
        return callNative("device", "hasDynamicIsland");
      },
    },

    // Background keep-alive (prevents app suspension)
    background: {
      startKeepAlive: function () {
        if (platform === "web") return Promise.resolve({ ok: true });
        return callNative("background", "startKeepAlive");
      },
      stopKeepAlive: function () {
        if (platform === "web") return Promise.resolve({ ok: true });
        return callNative("background", "stopKeepAlive");
      },
      isKeepAlive: function () {
        if (platform === "web") return Promise.resolve({ active: false });
        return callNative("background", "isKeepAlive");
      },
    },

    // QR code scanning
    // { text: "https://..." } or { error: "cancelled" }
    qrcode: {
      scan: function () {
        if (platform === "web") return Promise.resolve({ supported: false });
        return callNative("qrcode", "scan");
      },
    },

    // Photo picker (camera + library)
    // Returns { dataURL: "data:image/jpeg;base64,..." }
    photo: {
      pick: function (source) {
        if (platform === "web") return Promise.resolve({ supported: false });
        return callNative("photo", "pick", { source: source || "library" });
      },
      pickMultiple: function (limit) {
        if (platform === "web") return Promise.resolve({ supported: false });
        return callNative("photo", "pickMultiple", { limit: limit || 9 });
      },
    },

    // File picker
    // Returns { name, size, mimeType, dataURL }
    file: {
      pick: function (types) {
        if (platform === "web") return Promise.resolve({ supported: false });
        return callNative("file", "pick", { types: types });
      },
      pickMultiple: function (types) {
        if (platform === "web") return Promise.resolve({ supported: false });
        return callNative("file", "pickMultiple", { types: types });
      },
    },

    // Generic call for custom adapters.
    call: function (module, action, params) {
      return callNative(module, action, params);
    },

    // Event system.
    on: on,
    off: off,
  };

  // --- Web Speech API fallback ---
  var _webRecognition = null;

  function _webSpeechStart(language) {
    var SR = window.SpeechRecognition || window.webkitSpeechRecognition;
    if (!SR) return Promise.resolve({ error: "Web Speech API not supported" });

    if (_webRecognition) {
      _webRecognition.abort();
      _webRecognition = null;
    }

    var recognition = new SR();
    recognition.lang = language;
    recognition.interimResults = true;
    recognition.continuous = true;

    recognition.onresult = function (e) {
      var text = "";
      var isFinal = false;
      for (var i = e.resultIndex; i < e.results.length; i++) {
        text += e.results[i][0].transcript;
        if (e.results[i].isFinal) isFinal = true;
      }
      if (isFinal) {
        _onEvent("speech.result", { text: text, isFinal: true });
      } else {
        _onEvent("speech.partial", { text: text });
      }
    };

    recognition.onerror = function (e) {
      if (e.error === "aborted") return;
      _onEvent("speech.error", { error: e.error });
    };

    recognition.onend = function () {
      _webRecognition = null;
    };

    recognition.start();
    _webRecognition = recognition;
    return Promise.resolve({ ok: true });
  }

  function _webSpeechStop() {
    if (_webRecognition) {
      _webRecognition.stop();
      _webRecognition = null;
    }
    return Promise.resolve({ ok: true });
  }
})();
