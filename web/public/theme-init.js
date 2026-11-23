;(function () {
  var stored = ''
  try {
    stored = localStorage.getItem('weft-theme') || 'auto'
  } catch (_) {
    stored = 'auto'
  }
  var dark =
    stored === 'dark' ||
    (stored !== 'light' &&
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-color-scheme: dark)').matches)
  var theme = dark ? 'dark' : 'light'
  var color = dark ? '#141416' : '#faf9f7'
  var root = document.documentElement
  root.dataset.theme = theme
  var standalone =
    (typeof window.matchMedia === 'function' &&
      window.matchMedia('(display-mode: standalone)').matches) ||
    navigator.standalone === true
  if (standalone) root.dataset.standalone = 'true'
  var metas = document.querySelectorAll('meta[name="theme-color"]')
  for (var i = 0; i < metas.length; i++) {
    metas[i].removeAttribute('media')
    metas[i].setAttribute('content', color)
  }
})()
