import { auth as authApi, type User } from '$lib/api/client';

class AuthStore {
	user = $state<User | null>(null);
	token = $state<string | null>(null);
	loading = $state(true);

	get isAuthenticated() {
		return this.user !== null;
	}

	init() {
		// Restore session by calling /auth/me — the HttpOnly cookie is sent
		// automatically by the browser (credentials: 'include'). No token is
		// read from localStorage, keeping it out of reach of injected scripts.
		authApi
			.me()
			.then((u) => {
				this.user = u;
			})
			.catch(() => {
				this.user = null;
			})
			.finally(() => {
				this.loading = false;
			});
	}

	setAuth(t: string, u: User) {
		// Keep token in memory only for the Authorization header on non-cookie
		// API calls (e.g. native clients). The browser session is maintained via
		// the HttpOnly cookie set by the server on login/register.
		this.token = t;
		this.user = u;
	}

	logout() {
		this.token = null;
		this.user = null;
		// Tell the server to clear the HttpOnly cookie.
		authApi.logout().catch(() => {});
	}
}

export const authStore = new AuthStore();
export const user = authStore.user;
export const token = authStore.token;
