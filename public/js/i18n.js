(function() {
  var saved = localStorage.getItem('ironfuel-lang') || 'en';
  window.__i18n = window.__i18n || {};

  function applyDir(lang) {
    document.documentElement.dir = lang === 'ar' ? 'rtl' : 'ltr';
    document.documentElement.lang = lang;
  }

  function applyFont(lang) {
    if (lang === 'ar') {
      document.documentElement.style.setProperty('--ff', "'Changa', sans-serif");
    } else {
      document.documentElement.style.setProperty('--ff', "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif");
    }
  }

  applyDir(saved);
  applyFont(saved);

  document.addEventListener('alpine:init', function() {
    Alpine.store('lang', {
      current: saved,
      t: function(key) {
        var dict = window.__i18n[this.current] || {};
        var val = dict[key];
        return val !== undefined ? val : key;
      },
      set: function(lang) {
        if (lang === this.current) return;
        var self = this;
        if (!window.__i18n[lang]) {
          var s = document.createElement('script');
          s.src = '/locales/' + lang + '.js';
          s.onload = function() {
            self.current = lang;
            localStorage.setItem('ironfuel-lang', lang);
            applyDir(lang);
            applyFont(lang);
          };
          s.onerror = function() {
            self.current = lang;
            localStorage.setItem('ironfuel-lang', lang);
            applyDir(lang);
            applyFont(lang);
          };
          document.head.appendChild(s);
        } else {
          self.current = lang;
          localStorage.setItem('ironfuel-lang', lang);
          applyDir(lang);
          applyFont(lang);
        }
      },
      get isRTL() {
        return this.current === 'ar';
      }
    });
  });
})();
