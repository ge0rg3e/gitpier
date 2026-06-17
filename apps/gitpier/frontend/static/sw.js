const CACHE_NAME = 'gitpier-pwa-v1';
const APP_SHELL = ['/', '/manifest.webmanifest', '/images/logo.png', '/icons/icon-192.png', '/icons/icon-512.png', '/icons/apple-touch-icon.png'];
const STATIC_PATH_PREFIXES = ['/_app/', '/images/', '/icons/'];

self.addEventListener('install', (event) => {
	event.waitUntil(
		caches
			.open(CACHE_NAME)
			.then((cache) => cache.addAll(APP_SHELL))
			.then(() => self.skipWaiting())
	);
});

self.addEventListener('activate', (event) => {
	event.waitUntil(
		caches
			.keys()
			.then((keys) => Promise.all(keys.filter((key) => key !== CACHE_NAME).map((key) => caches.delete(key))))
			.then(() => self.clients.claim())
	);
});

self.addEventListener('fetch', (event) => {
	const { request } = event;

	if (request.method !== 'GET') {
		return;
	}

	const requestUrl = new URL(request.url);

	if (request.mode === 'navigate') {
		event.respondWith(
			fetch(request)
				.then((response) => response)
				.catch(async () => {
					const cachedPage = await caches.match(request);
					return cachedPage || caches.match('/');
				})
		);
		return;
	}

	const isStaticRequest =
		requestUrl.origin === self.location.origin &&
		['style', 'script', 'worker', 'image', 'font'].includes(request.destination) &&
		STATIC_PATH_PREFIXES.some((prefix) => requestUrl.pathname.startsWith(prefix));

	if (isStaticRequest) {
		event.respondWith(
			caches.match(request).then((cachedResponse) => {
				const networkFetch = fetch(request)
					.then((networkResponse) => {
						caches.open(CACHE_NAME).then((cache) => cache.put(request, networkResponse.clone())).catch(() => {});
						return networkResponse;
					})
					.catch(() => cachedResponse);

				return cachedResponse || networkFetch;
			})
		);
	}
});
