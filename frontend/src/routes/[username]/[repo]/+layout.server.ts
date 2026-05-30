import { env } from '$env/dynamic/public';
import type { LayoutServerLoad } from './$types';

type RepoSeoPayload = {
	repo?: {
		name?: string;
		description?: string | null;
		is_private?: boolean;
		owner?: {
			username?: string;
		};
		org?: {
			login?: string;
		};
	};
};

const API_BASE = env.PUBLIC_API_URL ?? 'http://localhost:8080';

export const load: LayoutServerLoad = async ({ params, fetch }) => {
	const username = params.username;
	const repo = params.repo;

	try {
		const qs = new URLSearchParams({
			include_branches: 'false',
			include_head: 'false',
			include_stats: 'false',
			include_size: 'false'
		});
		const response = await fetch(`${API_BASE}/api/v1/repos/${encodeURIComponent(username)}/${encodeURIComponent(repo)}?${qs}`);
		if (!response.ok) {
			return {
				seo: {
					owner: username,
					repo,
					description: null,
					isPrivate: false
				}
			};
		}

		const payload = (await response.json()) as RepoSeoPayload;
		const owner = payload.repo?.org?.login ?? payload.repo?.owner?.username ?? username;

		return {
			seo: {
				owner,
				repo: payload.repo?.name ?? repo,
				description: payload.repo?.description ?? null,
				isPrivate: payload.repo?.is_private ?? false
			}
		};
	} catch {
		return {
			seo: {
				owner: username,
				repo,
				description: null,
				isPrivate: false
			}
		};
	}
};
