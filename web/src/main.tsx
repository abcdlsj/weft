import { createRoot } from 'react-dom/client'
import App from './App'
import './styles.css'

createRoot(document.getElementById('root')!).render(<App />)

if ('serviceWorker' in navigator) {
  const hadController = !!navigator.serviceWorker.controller
  window.addEventListener('load', () => {
    navigator.serviceWorker
      .register('/sw.js')
      .then((reg) => {
        const announce = () => {
          // The first install has no controller; only a replacement needs a prompt.
          if (navigator.serviceWorker.controller) {
            window.dispatchEvent(new CustomEvent('weft:update'))
          }
        }
        if (reg.waiting) announce()
        reg.addEventListener('updatefound', () => {
          const installing = reg.installing
          installing?.addEventListener('statechange', () => {
            if (installing.state === 'installed') announce()
          })
        })
        window.setInterval(() => void reg.update(), 30 * 60 * 1000)
      })
      .catch(() => {})
  })

  let reloading = false
  navigator.serviceWorker.addEventListener('controllerchange', () => {
    // clients.claim() on the very first install must not reload the page.
    if (reloading || !hadController) return
    reloading = true
    window.location.reload()
  })
  window.addEventListener('weft:apply-update', () => {
    void navigator.serviceWorker.getRegistration().then((reg) => {
      if (reg?.waiting) reg.waiting.postMessage('SKIP_WAITING')
      else window.location.reload()
    })
  })
}
