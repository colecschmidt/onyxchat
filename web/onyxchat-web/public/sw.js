// Bumping CACHE_NAME is no longer part of shipping a release. Navigations are
// network-first, so a deploy's fresh index.html — and therefore the new asset
// hashes it points at — is picked up on the next load. Bump it only when the
// strategies below change and old entries have to be discarded.
const CACHE_NAME = 'onyxchat-v3'

const PRECACHE = ['/', '/index.html', '/manifest.json']

self.addEventListener('install', (event) => {
  event.waitUntil(caches.open(CACHE_NAME).then((cache) => cache.addAll(PRECACHE)))
  self.skipWaiting()
})

self.addEventListener('activate', (event) => {
  event.waitUntil(
    caches
      .keys()
      .then((keys) => Promise.all(keys.filter((k) => k !== CACHE_NAME).map((k) => caches.delete(k))))
      .then(() => self.clients.claim())
  )
})

// Only GET responses are cacheable, and an error or opaque response must never
// be stored — caching a 404 or a failed cross-origin fetch pins the failure
// until someone manually clears site data.
async function putInCache(request, response) {
  if (request.method !== 'GET' || !response.ok || response.type === 'opaque') return
  const cache = await caches.open(CACHE_NAME)
  await cache.put(request, response.clone())
}

// Vite emits content-hashed filenames under /assets/ (index-C3ACh07t.js), so a
// given URL's bytes never change. Those are the only things safe to serve from
// cache without checking the network first.
async function cacheFirst(request) {
  const cached = await caches.match(request)
  if (cached) return cached

  const response = await fetch(request)
  await putInCache(request, response)
  return response
}

// Serve from cache immediately and refresh in the background — right for small,
// rarely-changing, unhashed files (icons, manifest) where a network round trip
// on every load buys nothing but the file still has to be updatable.
async function staleWhileRevalidate(request) {
  const cached = await caches.match(request)

  const network = fetch(request)
    .then(async (response) => {
      await putInCache(request, response)
      return response
    })
    .catch(() => cached)

  return cached || network
}

// An SPA navigation to any path resolves to index.html. Offline, fall back to
// the precached copy so the app still boots.
async function handleNavigation(request) {
  try {
    const response = await fetch(request)
    await putInCache(request, response)
    return response
  } catch {
    return (await caches.match(request)) || (await caches.match('/index.html')) || Response.error()
  }
}

self.addEventListener('fetch', (event) => {
  const { request } = event
  const url = new URL(request.url)

  // The API is never cached. An offline read gets a JSON body the client can
  // parse rather than an opaque network failure.
  if (url.pathname.includes('/api/')) {
    event.respondWith(
      fetch(request.clone()).catch(
        () => new Response('{"error":"offline"}', { headers: { 'Content-Type': 'application/json' } })
      )
    )
    return
  }

  // Anything that isn't a GET can't be served from a cache — leave it to the
  // browser rather than intercepting it.
  if (request.method !== 'GET') return

  if (request.mode === 'navigate') {
    event.respondWith(handleNavigation(request))
    return
  }

  if (url.origin === self.location.origin && url.pathname.startsWith('/assets/')) {
    event.respondWith(cacheFirst(request))
    return
  }

  event.respondWith(staleWhileRevalidate(request))
})
