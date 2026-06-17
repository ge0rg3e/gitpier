const DEFAULT_SSH_PORT = '2424';

export type PublicRuntimeConfig = {
	sshCloneHost: string;
	httpCloneBaseURL: string;
	turnstileSiteKey: string;
};

function defaultSshCloneHost(): string {
	if (typeof window === 'undefined') return `localhost:${DEFAULT_SSH_PORT}`;

	const hostname = window.location.hostname.trim();
	return hostname ? `${hostname}:${DEFAULT_SSH_PORT}` : `localhost:${DEFAULT_SSH_PORT}`;
}

export function getPublicRuntimeConfig(): PublicRuntimeConfig {
	const cfg = typeof window === 'undefined' ? undefined : window.__gitpier_config;
	const fallbackHTTPBaseURL = typeof window === 'undefined' ? 'http://localhost:8828' : window.location.origin;

	return {
		sshCloneHost: cfg?.sshCloneHost?.trim() || defaultSshCloneHost(),
		httpCloneBaseURL: cfg?.httpCloneBaseURL?.trim().replace(/\/+$/, '') || fallbackHTTPBaseURL,
		turnstileSiteKey: cfg?.turnstileSiteKey?.trim() || ''
	};
}
