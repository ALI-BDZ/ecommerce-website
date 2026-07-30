# Internationalization (i18n) System — EN/AR

## Architecture

**Stack**: Alpine.js stores + JSON translation files + CSS logical properties

**Approach**: Shared `/js/i18n.js` file loaded on all pages, initializes `Alpine.store('lang')` before Alpine boots. Each page replaces hardcoded strings with `t('key')` calls.

## Files to Create

| File | Purpose |
|------|---------|
| `/locales/en.json` | English translations (~200 keys) |
| `/locales/ar.json` | Arabic translations (~200 keys) |
| `/js/i18n.js` | Language store, loader, RTL toggle, font swap, localStorage |

## Files to Modify (7 pages)

| File | Changes |
|------|---------|
| `index.html` | Add `<script src="/js/i18n.js">`, replace strings, language switcher, CSS logical props |
| `shop.html` | Same pattern |
| `product.html` | Same pattern |
| `cart.html` | Same pattern |
| `deals.html` | Same pattern |
| `about.html` | Same pattern |
| `pack.html` | Same pattern |

## i18n.js — Alpine Store

```js
Alpine.store('lang', {
  current: localStorage.getItem('lang') || 'en',
  translations: {},
  
  async load(lang) {
    if (!this.translations[lang]) {
      const mod = await import(`/locales/${lang}.json`);
      this.translations[lang] = mod.default || mod;
    }
  },
  
  async set(lang) {
    await this.load(lang);
    this.current = lang;
    localStorage.setItem('lang', lang);
    document.documentElement.dir = lang === 'ar' ? 'rtl' : 'ltr';
    document.documentElement.lang = lang;
    // swap fonts
    // update all x-text elements via Alpine reactivity
  },
  
  t(key) {
    return this.translations[this.current]?.[key] || this.translations['en']?.[key] || key;
  },
  
  get isRTL() { return this.current === 'ar'; }
});
```

## Language Switcher Design

### Desktop (pill switch in navbar)
```
┌─────────────────────────────┐
│ Logo  Home Shop Offers  [EN|AR]  🔍 🛒 │
└─────────────────────────────┘
```
- Rounded pill, glass background
- Active: dark bg, white text
- Inactive: transparent, gray text
- Sliding indicator (200ms spring)
- Positioned between nav links and action icons

### Mobile (in hamburger menu)
```
IRONFUEL
──────────
Home
Shop
Offers
About
──────────
🌐 Language
  ○ English
  ● العربية
──────────
```

## CSS Logical Properties (114 occurrences)

Convert across all 7 files:
- `margin-left/right` → `margin-inline-start/end`
- `padding-left/right` → `padding-inline-start/end`
- `left:/right:` (position) → `inset-inline-start/end`
- `border-left/right` → `border-inline-start/end`
- `text-align: left/right` → `text-align: start/end`
- `scroll-padding-left` → `scroll-padding-inline-start`

**Exception**: Keep `left:/right:` for truly directional elements (e.g., whatsapp-fab position fixed right-bottom).

## Font Strategy

- English: Inter (already loaded)
- Arabic: IBM Plex Sans Arabic (Google Fonts)
- Swap via `<link>` tag id swap on language change

## Translation Key Structure (~200 keys)

```
nav.home, nav.shop, nav.offers, nav.about
hero.eyebrow, hero.title1, hero.title2, hero.desc, hero.cta
offers.eyebrow, offers.title, offers.desc, offers.viewAll
offers.badge, offers.limited, offers.endsIn, offers.viewBundle
products.title, products.all, products.sale, products.new
cart.title, cart.empty, cart.browse, cart.continue
checkout.title, checkout.delivery, checkout.placeOrder
... (form labels, errors, badges, etc.)
```

## Implementation Order

1. Create `/locales/en.json` and `/locales/ar.json`
2. Create `/js/i18n.js` (Alpine store + loader)
3. Update `index.html` (add script, switcher, replace strings, CSS props)
4. Update remaining 6 pages (same pattern)
5. Test RTL layout, font swap, language persistence

## Verification

- Switch to Arabic → page flips RTL, fonts change, all text translated
- Switch back → LTR restored, English fonts, English text
- Refresh page → language persists
- Mobile hamburger → language selector works
- All 7 pages display correctly in both languages
