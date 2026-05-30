import { env } from '$env/dynamic/public';

export const API_BASE = env.PUBLIC_API_URL ?? 'http://localhost:8080';

export interface User {
	id: number;
	username: string;
	display_name: string;
	email: string;
	bio: string;
	avatar_url: string;
	location: string;
	website: string;
	role?: 'user' | 'admin';
	is_suspended?: boolean;
	suspension_reason?: string;
	created_at: string;
}

export interface RepoCreationStatus {
	can_create_repositories: boolean;
	restricted: boolean;
	self_host_url?: string;
}

export interface LangStat {
	name: string;
	count: number;
}

export interface ProfileStats {
	total_stars: number;
	total_repos: number;
	total_prs: number;
	total_issues: number;
	total_commits: number;
	top_languages: LangStat[];
	current_streak: number;
	longest_streak: number;
}

export interface FollowListItem {
	entity_type?: 'user' | 'org';
	user?: User;
	org?: Organization;
	is_following: boolean;
	follows_you: boolean;
}

export interface Repository {
	id: number;
	name: string;
	description: string;
	is_private: boolean;
	is_archived?: boolean;
	archived_at?: string;
	is_suspended?: boolean;
	suspension_reason?: string;
	default_branch: string;
	size?: number;
	size_limit_bytes?: number;
	owner_id: number;
	owner: User;
	org?: Organization;
	forked_from_repo_id?: number;
	forked_from_repo?: {
		id: number;
		name: string;
		owner: User;
		org?: Organization;
	};
	created_at: string;
	updated_at: string;
	star_count?: number;
	fork_count?: number;
	language?: string;
}

export interface Star {
	id: number;
	user_id: number;
	repo_id: number;
	repo: Repository;
	created_at: string;
}

export interface RepoStarEvent {
	id: number;
	user_id: number;
	repo_id: number;
	created_at: string;
}

export interface Collaborator {
	id: number;
	repo_id: number;
	user_id: number;
	permission: 'read' | 'write' | 'admin';
	user: User;
}

export interface SSHKey {
	id: number;
	user_id: number;
	title: string;
	key: string;
	fingerprint: string;
	created_at: string;
}

export interface FileEntry {
	name: string;
	type: 'blob' | 'tree';
	path: string;
	mode: string;
	sha: string;
	message?: string;
	author?: string;
	date?: string;
	commit_message?: string;
	commit_author?: string;
	commit_date?: string;
	commit_sha?: string;
}

export interface CommitInfo {
	sha: string;
	message: string;
	author: {
		name: string;
		email: string;
		date: string;
		username?: string;
		avatar_url?: string;
	};
	files?: string[];
	additions?: number;
	deletions?: number;
	changed_files?: number;
	web_commit?: boolean;
}

export interface FileDiff {
	path: string;
	old_path?: string;
	type: 'added' | 'modified' | 'deleted' | 'renamed' | string;
	additions: number;
	deletions: number;
	content?: string;
	old_content?: string;
	patch?: string;
}

export interface MarkdownAssetUploadResult {
	asset_url: string;
	content_type: string;
	original_name: string;
	markdown: string;
}

export interface CommitDetail {
	sha: string;
	message: string;
	author: {
		name: string;
		email: string;
		date: string;
		username?: string;
		avatar_url?: string;
	};
	files?: string[];
	diffs: FileDiff[];
	additions?: number;
	deletions?: number;
	changed_files?: number;
}

export interface CommitDiffPage {
	sha: string;
	diffs: FileDiff[];
	limit: number;
	offset: number;
	has_more: boolean;
	total: number;
}

export interface ApiError {
	message: string;
	status: number;
}

export interface LoginResponse {
	token?: string;
	user?: User;
	requires_2fa?: boolean;
	two_factor_challenge_token?: string;
}

export interface RegisterOTPResponse {
	message: string;
	registration_token: string;
	expires_in_seconds: number;
}

export interface TwoFactorStatus {
	enabled: boolean;
	has_pending_setup: boolean;
}

export interface TwoFactorSetupResponse {
	secret: string;
	otpauth_url: string;
}

function getToken(): string | null {
	// Token is kept in-memory only (in authStore.token). It is used as the
	// Authorization header for non-browser API clients. Browser sessions
	// rely on the HttpOnly cookie set by the server (sent via credentials: 'include').
	// We intentionally do NOT read from localStorage to prevent XSS token theft.
	if (typeof window === 'undefined') return null;
	// Access the in-memory store value directly to avoid circular imports.
	// The token is set by authStore.setAuth() on login/register.
	return (window as unknown as { __gitpier_token?: string }).__gitpier_token ?? null;
}

async function request<T>(path: string, options: RequestInit = {}): Promise<T> {
	const token = getToken();
	const headers: Record<string, string> = {
		'Content-Type': 'application/json',
		...(options.headers as Record<string, string>)
	};

	if (token) {
		headers['Authorization'] = `Bearer ${token}`;
	}

	const res = await fetch(`${API_BASE}/api/v1${path}`, {
		...options,
		credentials: 'include', // send HttpOnly session cookie automatically
		headers
	});

	if (!res.ok) {
		let message = `Request failed with status ${res.status}`;
		try {
			const body = await res.json();
			message = body.message ?? message;
		} catch {}
		const err: ApiError = { message, status: res.status };
		throw err;
	}

	if (res.status === 204) return undefined as T;
	return res.json();
}

async function requestWithHeaders<T>(path: string, headers: Record<string, string>, options: RequestInit = {}): Promise<T> {
	return request<T>(path, {
		...options,
		headers: {
			...(options.headers as Record<string, string>),
			...headers
		}
	});
}

// Multipart upload helper — does NOT set Content-Type so the browser sets the boundary automatically.
async function requestUpload<T>(path: string, formData: FormData): Promise<T> {
	const token = getToken();
	const headers: Record<string, string> = {};
	if (token) headers['Authorization'] = `Bearer ${token}`;

	const res = await fetch(`${API_BASE}/api/v1${path}`, {
		method: 'POST',
		credentials: 'include', // send HttpOnly session cookie automatically
		headers,
		body: formData
	});

	if (!res.ok) {
		let message = `Request failed with status ${res.status}`;
		try {
			const body = await res.json();
			message = body.message ?? message;
		} catch {}
		const err: ApiError = { message, status: res.status };
		throw err;
	}

	return res.json();
}

// Auth
export const auth = {
	requestRegisterOTP: (username: string, email: string, password: string, gdprConsent: boolean, turnstileToken: string) =>
		request<RegisterOTPResponse>('/auth/register', {
			method: 'POST',
			body: JSON.stringify({
				username,
				email,
				password,
				gdpr_consent: gdprConsent,
				turnstile_token: turnstileToken
			})
		}),

	verifyRegisterOTP: (registrationToken: string, otpCode: string) =>
		request<{ token: string; user: User }>('/auth/register/verify', {
			method: 'POST',
			body: JSON.stringify({ registration_token: registrationToken, otp_code: otpCode })
		}),

	login: (data: { email?: string; password?: string; challenge_token?: string; two_factor_code?: string; two_factor_recovery_code?: string; turnstile_token?: string }) =>
		request<LoginResponse>('/auth/login', {
			method: 'POST',
			body: JSON.stringify(data)
		}),

	me: () => request<User>('/auth/me'),
	repoCreationStatus: () => request<RepoCreationStatus>('/auth/repo-creation'),

	logout: () => request<void>('/auth/logout', { method: 'POST' }),

	forgotPassword: (email: string) =>
		request<{ message: string }>('/auth/forgot-password', {
			method: 'POST',
			body: JSON.stringify({ email })
		}),

	resetPassword: (token: string, newPassword: string) =>
		request<{ message: string }>('/auth/reset-password', {
			method: 'POST',
			body: JSON.stringify({ token, new_password: newPassword })
		}),

	changePassword: (currentPassword: string, newPassword: string) =>
		request<{ token?: string }>('/auth/change-password', {
			method: 'POST',
			body: JSON.stringify({ current_password: currentPassword, new_password: newPassword })
		}),

	verifyPassword: (password: string) =>
		request<void>('/auth/verify-password', {
			method: 'POST',
			body: JSON.stringify({ password })
		}),

	twoFactor: {
		status: () => request<TwoFactorStatus>('/auth/2fa/status'),
		setup: (password: string) =>
			request<TwoFactorSetupResponse>('/auth/2fa/setup', {
				method: 'POST',
				body: JSON.stringify({ password })
			}),
		enable: (code: string) =>
			request<{ enabled: boolean; recovery_codes: string[] }>('/auth/2fa/enable', {
				method: 'POST',
				body: JSON.stringify({ code })
			}),
		disable: (password: string, code?: string, recoveryCode?: string) =>
			request<{ enabled: boolean }>('/auth/2fa/disable', {
				method: 'POST',
				body: JSON.stringify({ password, ...(code ? { code } : {}), ...(recoveryCode ? { recovery_code: recoveryCode } : {}) })
			}),
		regenerateRecoveryCodes: (password: string, code?: string, recoveryCode?: string) =>
			request<{ recovery_codes: string[] }>('/auth/2fa/recovery-codes/regenerate', {
				method: 'POST',
				body: JSON.stringify({ password, ...(code ? { code } : {}), ...(recoveryCode ? { recovery_code: recoveryCode } : {}) })
			})
	},

	sessions: {
		list: () => request<{ sessions: Session[] }>('/auth/sessions'),
		revoke: (tokenId: string) => request<void>(`/auth/sessions/${tokenId}`, { method: 'DELETE' }),
		revokeOthers: () => request<void>('/auth/sessions', { method: 'DELETE' })
	}
};

export interface Session {
	id: number;
	token_id: string;
	ip_address: string;
	user_agent: string;
	browser: string;
	os: string;
	is_mobile: boolean;
	last_seen_at: string;
	created_at: string;
	is_current: boolean;
}

// Users
export const users = {
	getProfile: (username: string, options?: { limit?: number; offset?: number }) =>
		request<{
			user: User;
			repos: Repository[];
			follower_count?: number;
			following_count?: number;
			is_following?: boolean;
			follows_you?: boolean;
		}>(
			`/users/${username}?${new URLSearchParams({
				...(options?.limit ? { limit: String(options.limit) } : {}),
				...(options?.offset ? { offset: String(options.offset) } : {})
			})}`
		),

	follow: (username: string) => request<void>(`/users/${username}/follow`, { method: 'POST' }),

	unfollow: (username: string) => request<void>(`/users/${username}/follow`, { method: 'DELETE' }),

	listFollowers: (username: string) => request<{ users: FollowListItem[]; count: number }>(`/users/${username}/followers`),

	listFollowing: (username: string) => request<{ users: FollowListItem[]; items?: FollowListItem[]; count: number }>(`/users/${username}/following`),

	updateProfile: (data: { bio?: string; avatar_url?: string; display_name?: string; location?: string; website?: string }) =>
		request<User>('/users/me', { method: 'PATCH', body: JSON.stringify(data) }),

	uploadAvatar: (file: File) => {
		const fd = new FormData();
		fd.append('avatar', file);
		return requestUpload<{ avatar_url: string }>('/users/me/avatar', fd);
	},

	getActionsUsage: () => request<ActionsUsage>('/users/me/actions/usage'),

	getContributions: (username: string) => request<{ contributions: Record<string, number>; total: number }>(`/users/${username}/contributions`),

	exportData: () => request<Record<string, unknown>>('/users/me/export'),

	deleteAccount: (password: string) => request<{ message: string }>('/users/me', { method: 'DELETE', body: JSON.stringify({ password }) }),

	getProfileStats: (username: string) => request<ProfileStats>(`/users/${username}/stats`)
};

// Repositories
export const repos = {
	create: (data: { name: string; description: string; is_private: boolean; initialize_with_readme?: boolean }) =>
		request<Repository>('/repos', { method: 'POST', body: JSON.stringify(data) }),

	fork: {
		create: (username: string, repo: string, data?: { owner?: string; name?: string; description?: string; copy_main_branch_only?: boolean }) =>
			request<Repository>(`/repos/${username}/${repo}/fork`, {
				method: 'POST',
				body: JSON.stringify(data ?? {})
			}),
		sync: (username: string, repo: string) =>
			request<{ status: 'synced' | 'up_to_date'; before_sha?: string; after_sha?: string; message: string }>(`/repos/${username}/${repo}/fork/sync`, {
				method: 'POST'
			})
	},

	forks: {
		list: (username: string, repo: string, limit?: number) =>
			request<{ forks: Repository[] }>(
				`/repos/${username}/${repo}/forks${
					limit ? `?${new URLSearchParams({ limit: String(limit) }).toString()}` : ''
				}`
			)
	},

	explore: (limit?: number, offset?: number) =>
		request<{ repos: Repository[]; total: number; limit: number; offset: number }>(
			`/explore?${new URLSearchParams({ ...(limit ? { limit: String(limit) } : {}), ...(offset ? { offset: String(offset) } : {}) })}`
		),

	get: (
		username: string,
		repo: string,
		ref?: string,
		options?: {
			includeBranches?: boolean;
			includeHead?: boolean;
			includeStats?: boolean;
			includeSize?: boolean;
		}
	) =>
		request<{ repo: Repository; branches: string[]; head_commit: CommitInfo | null; stats: { commits: number; branches: number; tags: number; branch: string } }>(
			`/repos/${username}/${repo}?${new URLSearchParams({
				...(ref ? { ref } : {}),
				...(options?.includeBranches === false ? { include_branches: 'false' } : {}),
				...(options?.includeHead === false ? { include_head: 'false' } : {}),
				...(options?.includeStats === false ? { include_stats: 'false' } : {}),
				...(options?.includeSize === false ? { include_size: 'false' } : {})
			})}`
		),

	update: (username: string, repo: string, data: Partial<{ name?: string; description?: string; is_private?: boolean; default_branch?: string }>) =>
		request<Repository>(`/repos/${username}/${repo}`, { method: 'PATCH', body: JSON.stringify(data) }),

	setVisibility: (username: string, repo: string, isPrivate: boolean, confirmPassword: string) =>
		request<Repository>(`/repos/${username}/${repo}/visibility`, { method: 'POST', body: JSON.stringify({ private: isPrivate }), headers: { 'X-Confirm-Password': confirmPassword } }),

	archive: (username: string, repo: string, confirmPassword: string) =>
		request<Repository>(`/repos/${username}/${repo}/archive`, { method: 'POST', headers: { 'X-Confirm-Password': confirmPassword } }),

	unarchive: (username: string, repo: string, confirmPassword: string) =>
		request<Repository>(`/repos/${username}/${repo}/unarchive`, { method: 'POST', headers: { 'X-Confirm-Password': confirmPassword } }),

	delete: (username: string, repo: string, confirmPassword: string) => request<void>(`/repos/${username}/${repo}`, { method: 'DELETE', headers: { 'X-Confirm-Password': confirmPassword } }),

	tree: (username: string, repo: string, ref?: string, path?: string, options?: { includeMeta?: boolean; includeHead?: boolean }) =>
		request<{ files: FileEntry[]; head_commit: CommitInfo | null; empty?: boolean }>(
			`/repos/${username}/${repo}/tree?${new URLSearchParams({
				...(ref ? { ref } : {}),
				...(path ? { path } : {}),
				...(options?.includeMeta === false ? { include_meta: 'false' } : {}),
				...(options?.includeHead === false ? { include_head: 'false' } : {})
			})}`
		),

	blob: (username: string, repo: string, path: string, ref?: string) =>
		request<{ content: string; path: string; ref: string; size: number }>(`/repos/${username}/${repo}/blob?${new URLSearchParams({ path, ...(ref ? { ref } : {}) })}`),

	downloadZipUrl: (username: string, repo: string, ref?: string) =>
		`${API_BASE}/api/v1/repos/${encodeURIComponent(username)}/${encodeURIComponent(repo)}/zip${
			ref ? `?${new URLSearchParams({ ref }).toString()}` : ''
		}`,

	updateBlob: (username: string, repo: string, data: { path: string; content: string; message: string; branch?: string }) =>
		request<{ sha: string; path: string; branch: string; message: string }>(`/repos/${username}/${repo}/blob`, { method: 'PUT', body: JSON.stringify(data) }),

	commits: (username: string, repo: string, options?: { ref?: string; limit?: number; offset?: number; author?: string; q?: string; since?: string; until?: string }) =>
		request<{ commits: CommitInfo[]; ref: string; limit: number; offset: number; has_more: boolean; total: number; total_pages: number }>(
			`/repos/${username}/${repo}/commits?${new URLSearchParams({
				...(options?.ref ? { ref: options.ref } : {}),
				...(options?.limit ? { limit: String(options.limit) } : {}),
				...(options?.offset ? { offset: String(options.offset) } : {}),
				...(options?.author ? { author: options.author } : {}),
				...(options?.q ? { q: options.q } : {}),
				...(options?.since ? { since: options.since } : {}),
				...(options?.until ? { until: options.until } : {})
			})}`
		),

	commit: (username: string, repo: string, sha: string) => request<CommitDetail>(`/repos/${username}/${repo}/commit/${sha}`),

	commitMeta: (username: string, repo: string, sha: string) => request<CommitInfo>(`/repos/${username}/${repo}/commit/${sha}?meta=true`),

	commitDiffs: (username: string, repo: string, sha: string, limit = 10, offset = 0) =>
		request<CommitDiffPage>(
			`/repos/${username}/${repo}/commit/${sha}/diffs?${new URLSearchParams({
				limit: String(limit),
				offset: String(offset)
			})}`
		),

	languages: (username: string, repo: string) => request<{ languages: { name: string; bytes: number; percent: number }[] }>(`/repos/${username}/${repo}/languages`),

	uploadMarkdownAsset: (username: string, repo: string, file: File) => {
		const fd = new FormData();
		fd.append('file', file);
		return requestUpload<MarkdownAssetUploadResult>(`/repos/${username}/${repo}/markdown-assets`, fd);
	},

	branches: {
		list: (username: string, repo: string) => request<{ branches: string[] }>(`/repos/${username}/${repo}/branches`),
		create: (username: string, repo: string, name: string, fromRef?: string) =>
			request<{ name: string; from_ref: string }>(`/repos/${username}/${repo}/branches`, {
				method: 'POST',
				body: JSON.stringify({ name, from_ref: fromRef })
			}),
		delete: (username: string, repo: string, name: string) =>
			request<void>(`/repos/${username}/${repo}/branches`, {
				method: 'DELETE',
				body: JSON.stringify({ name })
			})
	},

	tags: {
		list: (username: string, repo: string) => request<{ tags: TagInfo[] }>(`/repos/${username}/${repo}/tags`),
		create: (username: string, repo: string, name: string, targetRef?: string, message?: string) =>
			request<{ name: string; target_ref: string }>(`/repos/${username}/${repo}/tags`, {
				method: 'POST',
				body: JSON.stringify({ name, target_ref: targetRef, message })
			}),
		delete: (username: string, repo: string, name: string) =>
			request<void>(`/repos/${username}/${repo}/tags`, {
				method: 'DELETE',
				body: JSON.stringify({ name })
			})
	},

	collaborators: {
		list: (username: string, repo: string) => request<{ collaborators: Collaborator[] }>(`/repos/${username}/${repo}/collaborators`),
		add: (username: string, repo: string, user_id: number, permission: string) =>
			request<Collaborator>(`/repos/${username}/${repo}/collaborators`, {
				method: 'POST',
				body: JSON.stringify({ user_id, permission })
			}),
		remove: (username: string, repo: string, userID: number) => request<void>(`/repos/${username}/${repo}/collaborators/${userID}`, { method: 'DELETE' })
	},

	star: {
		getStatus: (username: string, repo: string) => request<{ starred: boolean; count: number }>(`/repos/${username}/${repo}/star`),

		history: (username: string, repo: string) => request<{ stars: RepoStarEvent[] }>(`/repos/${username}/${repo}/stars/history`),

		star: (username: string, repo: string) => request<Star>(`/repos/${username}/${repo}/star`, { method: 'POST' }),

		unstar: (username: string, repo: string) => request<void>(`/repos/${username}/${repo}/star`, { method: 'DELETE' })
	},

	compare: (username: string, repo: string, base: string, head: string, headRepoID?: string | number) =>
		request<{ commits: CommitInfo[]; files: FileDiff[]; mergeable: boolean; contributors: number }>(
			`/repos/${username}/${repo}/compare?${new URLSearchParams({
				base,
				head,
				...(headRepoID != null ? { head_repo_id: String(headRepoID) } : {})
			})}`
		)
};

export const starred = {
	list: () => request<{ stars: Star[] }>('/users/me/starred'),
	listForUser: (username: string) => request<{ stars: Star[] }>(`/users/${username}/starred`)
};

// SSH Keys
export const sshKeys = {
	list: () => request<{ keys: SSHKey[] }>('/ssh-keys'),
	add: (title: string, key: string) => request<SSHKey>('/ssh-keys', { method: 'POST', body: JSON.stringify({ title, key }) }),
	delete: (id: number, confirmPassword: string) => request<void>(`/ssh-keys/${id}`, { method: 'DELETE', headers: { 'X-Confirm-Password': confirmPassword } })
};

// Pull Requests
export interface PullRequest {
	id: number;
	number: number;
	title: string;
	description: string;
	status: 'open' | 'closed' | 'merged';
	head_ref: string;
	base_ref: string;
	head_sha: string;
	is_draft: boolean;
	repo_id: number;
	head_repo_id?: string | number;
	author_id: number;
	author: User;
	repo: Repository;
	head_repo?: Repository;
	merged_by?: User;
	merged_by_id?: number;
	merged_at?: string;
	closed_at?: string;
	merge_sha?: string;
	merge_method?: 'merge' | 'squash' | 'rebase';
	assignee_id?: number;
	assignee?: User;
	labels: Label[];
	created_at: string;
	updated_at: string;
}

export interface DashboardPullRequest extends PullRequest {
	repo_owner: string;
	repo_name: string;
}

export interface DashboardOverview {
	open_pull_requests: number;
	open_issues: number;
	review_requests: number;
	recent_pull_requests: DashboardPullRequest[];
}

export interface PRComment {
	id: number;
	pr_id: number;
	body: string;
	author_id: number;
	author: User;
	created_at: string;
	updated_at: string;
}

export interface PRReviewComment {
	id: number;
	review_id: number;
	pr_id: number;
	body: string;
	path: string;
	line: number;
	side: 'LEFT' | 'RIGHT';
	commit_sha: string;
	author_id: number;
	author: User;
	created_at: string;
	updated_at: string;
}

export interface PRReview {
	id: number;
	pr_id: number;
	body: string;
	state: 'APPROVED' | 'CHANGES_REQUESTED' | 'COMMENTED' | 'DISMISSED';
	commit_sha: string;
	author_id: number;
	author: User;
	comments: PRReviewComment[];
	created_at: string;
	updated_at: string;
}

export const pullRequests = {
	list: (username: string, repo: string, status?: string) => request<{ pull_requests: PullRequest[] }>(`/repos/${username}/${repo}/pulls${status ? `?status=${status}` : ''}`),

	get: (username: string, repo: string, number: number) =>
		request<{ pull_request: PullRequest; base_commit: CommitInfo | null; head_commit: CommitInfo | null; mergeable: boolean }>(`/repos/${username}/${repo}/pulls/${number}`),

	create: (
		username: string,
		repo: string,
		data: {
			title: string;
			description?: string;
			head_ref: string;
			base_ref: string;
			head_sha?: string;
			is_draft?: boolean;
			head_repo_id?: string | number;
		}
	) => request<PullRequest>(`/repos/${username}/${repo}/pulls`, { method: 'POST', body: JSON.stringify(data) }),

	close: (username: string, repo: string, number: number) => request<void>(`/repos/${username}/${repo}/pulls/${number}/close`, { method: 'POST' }),

	reopen: (username: string, repo: string, number: number) => request<PullRequest>(`/repos/${username}/${repo}/pulls/${number}/reopen`, { method: 'POST' }),

	merge: (username: string, repo: string, number: number, method?: string, commitTitle?: string) =>
		request<PullRequest>(`/repos/${username}/${repo}/pulls/${number}/merge`, {
			method: 'POST',
			body: JSON.stringify({ merge_method: method ?? 'merge', commit_title: commitTitle })
		}),

	getCommits: (username: string, repo: string, number: number) => request<{ commits: CommitInfo[] }>(`/repos/${username}/${repo}/pulls/${number}/commits`),

	getFiles: (username: string, repo: string, number: number) => request<{ files: FileDiff[] }>(`/repos/${username}/${repo}/pulls/${number}/files`),

	listComments: (username: string, repo: string, number: number) => request<{ comments: PRComment[] }>(`/repos/${username}/${repo}/pulls/${number}/comments`),

	createComment: (username: string, repo: string, number: number, body: string) =>
		request<PRComment>(`/repos/${username}/${repo}/pulls/${number}/comments`, {
			method: 'POST',
			body: JSON.stringify({ body })
		}),

	updateComment: (username: string, repo: string, number: number, commentID: number, body: string) =>
		request<PRComment>(`/repos/${username}/${repo}/pulls/${number}/comments/${commentID}`, {
			method: 'PATCH',
			body: JSON.stringify({ body })
		}),

	deleteComment: (username: string, repo: string, number: number, commentID: number) => request<void>(`/repos/${username}/${repo}/pulls/${number}/comments/${commentID}`, { method: 'DELETE' }),

	update: (username: string, repo: string, number: number, data: { assignee_id?: number; clear_assignee?: boolean; label_ids?: number[] }) =>
		request<{ pull_request: PullRequest }>(`/repos/${username}/${repo}/pulls/${number}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		}),

	listReviews: (username: string, repo: string, number: number) => request<{ reviews: PRReview[] }>(`/repos/${username}/${repo}/pulls/${number}/reviews`),

	createReview: (
		username: string,
		repo: string,
		number: number,
		data: {
			state: 'APPROVED' | 'CHANGES_REQUESTED' | 'COMMENTED';
			body?: string;
			commit_sha?: string;
			comments?: Array<{ path: string; line: number; side?: string; body: string; commit_sha?: string }>;
		}
	) =>
		request<PRReview>(`/repos/${username}/${repo}/pulls/${number}/reviews`, {
			method: 'POST',
			body: JSON.stringify(data)
		})
};

export const dashboard = {
	overview: (recentLimit = 16) => request<DashboardOverview>(`/users/me/dashboard?${new URLSearchParams({ recent_limit: String(recentLimit) })}`)
};

// Workflow Actions

export type WorkflowStatus = 'pending' | 'running' | 'success' | 'failure' | 'cancelled';

export interface WorkflowStep {
	id: string;
	job_id: string;
	name: string;
	status: WorkflowStatus;
	exit_code?: number;
	log: string;
	started_at?: string;
	finished_at?: string;
	created_at: string;
}

export interface WorkflowJob {
	id: string;
	run_id: string;
	name: string;
	status: WorkflowStatus;
	started_at?: string;
	finished_at?: string;
	created_at: string;
	steps?: WorkflowStep[];
}

export interface WorkflowRun {
	id: string;
	repo_id: string;
	workflow_name: string;
	workflow_file: string;
	event: string;
	branch: string;
	commit_sha: string;
	status: WorkflowStatus;
	created_at: string;
	updated_at: string;
	jobs?: WorkflowJob[];
}

export interface ActionsUsage {
	used_minutes: number;
	limit_minutes: number;
	remaining_minutes: number;
	month: string;
}

export const workflows = {
	listRuns: (username: string, repo: string, limit = 20, offset = 0) =>
		request<{ runs: WorkflowRun[]; total: number; limit: number; offset: number }>(`/repos/${username}/${repo}/actions?limit=${limit}&offset=${offset}`),

	getRun: (username: string, repo: string, runId: string) => request<{ run: WorkflowRun }>(`/repos/${username}/${repo}/actions/${runId}`),

	cancelRun: (username: string, repo: string, runId: string) => request<void>(`/repos/${username}/${repo}/actions/${runId}/cancel`, { method: 'POST' }),

	rerun: (username: string, repo: string, runId: string) => request<{ run_id: string }>(`/repos/${username}/${repo}/actions/${runId}/rerun`, { method: 'POST' }),

	deleteRun: (username: string, repo: string, runId: string) => request<void>(`/repos/${username}/${repo}/actions/${runId}`, { method: 'DELETE' }),

	usage: (username: string, repo: string) => request<ActionsUsage>(`/repos/${username}/${repo}/actions/usage`),

	listDispatchable: (username: string, repo: string, ref: string) => request<{ workflows: string[] }>(`/repos/${username}/${repo}/actions/dispatchable?ref=${encodeURIComponent(ref)}`),

	dispatch: (username: string, repo: string, ref: string, workflowFile: string) =>
		request<{ runs_created: number }>(`/repos/${username}/${repo}/actions/dispatch`, { method: 'POST', body: JSON.stringify({ ref, workflow_file: workflowFile }) })
};

// Repository Variables & Secrets

export interface RepoVariable {
	id: number;
	repo_id: number;
	name: string;
	value: string;
	created_at: string;
	updated_at: string;
}

export interface RepoSecretInfo {
	name: string;
	created_at: string;
	updated_at: string;
}

export const repoEnv = {
	// Variables — values are readable
	listVariables: (username: string, repo: string) => request<{ variables: RepoVariable[] }>(`/repos/${username}/${repo}/actions/variables`),

	setVariable: (username: string, repo: string, name: string, value: string) =>
		request<void>(`/repos/${username}/${repo}/actions/variables/${encodeURIComponent(name)}`, {
			method: 'PUT',
			body: JSON.stringify({ value })
		}),

	deleteVariable: (username: string, repo: string, name: string) => request<void>(`/repos/${username}/${repo}/actions/variables/${encodeURIComponent(name)}`, { method: 'DELETE' }),

	// Secrets — values are write-only, list only returns metadata
	listSecrets: (username: string, repo: string) => request<{ secrets: RepoSecretInfo[] }>(`/repos/${username}/${repo}/actions/secrets`),

	setSecret: (username: string, repo: string, name: string, value: string) =>
		request<void>(`/repos/${username}/${repo}/actions/secrets/${encodeURIComponent(name)}`, {
			method: 'PUT',
			body: JSON.stringify({ value })
		}),

	deleteSecret: (username: string, repo: string, name: string) => request<void>(`/repos/${username}/${repo}/actions/secrets/${encodeURIComponent(name)}`, { method: 'DELETE' })
};

// Releases

export interface ReleaseAsset {
	id: string;
	release_id: string;
	name: string;
	size: number;
	content_type: string;
	download_count: number;
	created_at: string;
	updated_at: string;
}

export interface Release {
	id: string;
	repo_id: string;
	tag_name: string;
	target_commit: string;
	name: string;
	body: string;
	is_draft: boolean;
	is_prerelease: boolean;
	published_at: string | null;
	created_by_id: string;
	created_by: User;
	assets: ReleaseAsset[];
	created_at: string;
	updated_at: string;
}

export interface TagInfo {
	name: string;
	sha: string;
	commit_sha: string;
	message: string;
	date: string;
}

export const releases = {
	list: (username: string, repo: string) => request<{ releases: Release[] }>(`/repos/${username}/${repo}/releases`),

	get: (username: string, repo: string, id: string) => request<{ release: Release }>(`/repos/${username}/${repo}/releases/${id}`),

	getLatest: (username: string, repo: string) => request<{ release: Release }>(`/repos/${username}/${repo}/releases/latest`),

	getByTag: (username: string, repo: string, tag: string) => request<{ release: Release }>(`/repos/${username}/${repo}/releases/tags/${encodeURIComponent(tag)}`),

	getTags: (username: string, repo: string) => request<{ tags: TagInfo[] }>(`/repos/${username}/${repo}/releases/tags`),

	create: (
		username: string,
		repo: string,
		data: {
			tag_name: string;
			target_commitish?: string;
			name?: string;
			body?: string;
			is_draft?: boolean;
			is_prerelease?: boolean;
		}
	) => request<{ release: Release }>(`/repos/${username}/${repo}/releases`, { method: 'POST', body: JSON.stringify(data) }),

	update: (username: string, repo: string, id: string, data: { name?: string; body?: string; is_draft?: boolean; is_prerelease?: boolean }) =>
		request<{ release: Release }>(`/repos/${username}/${repo}/releases/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),

	delete: (username: string, repo: string, id: string) => request<void>(`/repos/${username}/${repo}/releases/${id}`, { method: 'DELETE' }),

	uploadAsset: (username: string, repo: string, id: string, file: File, name?: string) => {
		const form = new FormData();
		form.append('file', file);
		if (name) form.append('name', name);
		const token = getToken();
		const headers: Record<string, string> = {};
		if (token) headers['Authorization'] = `Bearer ${token}`;
		return fetch(`${API_BASE}/api/v1/repos/${username}/${repo}/releases/${id}/assets`, {
			method: 'POST',
			credentials: 'include',
			headers,
			body: form
		}).then(async (res) => {
			if (!res.ok) {
				const body = await res.json().catch(() => ({}));
				throw { message: body.message ?? `Upload failed with status ${res.status}`, status: res.status };
			}
			return res.json() as Promise<{ asset: ReleaseAsset }>;
		});
	},

	deleteAsset: (username: string, repo: string, releaseId: string, assetId: string) => request<void>(`/repos/${username}/${repo}/releases/${releaseId}/assets/${assetId}`, { method: 'DELETE' }),

	downloadAssetUrl: (username: string, repo: string, assetId: string) => `${API_BASE}/api/v1/repos/${username}/${repo}/releases/assets/${assetId}`,

	sourceZipUrl: (username: string, repo: string, id: string) => `${API_BASE}/api/v1/repos/${username}/${repo}/releases/${id}/source.zip`,

	sourceTarUrl: (username: string, repo: string, id: string) => `${API_BASE}/api/v1/repos/${username}/${repo}/releases/${id}/source.tar.gz`
};
// Issues

export interface Milestone {
	id: number;
	title: string;
	description: string;
	status: 'open' | 'closed';
	due_date?: string;
	repo_id: number;
	created_at: string;
	updated_at: string;
}

export interface Label {
	id: number;
	name: string;
	color: string;
	description: string;
	repo_id: number;
	created_at: string;
}

export interface IssueComment {
	id: number;
	issue_id: number;
	body: string;
	author_id: number;
	author: User;
	created_at: string;
	updated_at: string;
}

export interface Issue {
	id: number;
	number: number;
	title: string;
	body: string;
	status: 'open' | 'closed';
	issue_type: string;
	repo_id: number;
	author_id: number;
	author: User;
	assignee_id?: number;
	assignee?: User;
	milestone_id?: number;
	milestone?: Milestone;
	labels: Label[];
	comments?: IssueComment[];
	created_at: string;
	updated_at: string;
}

export const issues = {
	list: (username: string, repo: string, status?: string) => request<{ issues: Issue[] }>(`/repos/${username}/${repo}/issues${status ? `?status=${status}` : ''}`),

	get: (username: string, repo: string, number: number) => request<{ issue: Issue }>(`/repos/${username}/${repo}/issues/${number}`),

	create: (username: string, repo: string, data: { title: string; body?: string; issue_type?: string; assignee_id?: number; milestone_id?: number; label_ids?: number[] }) =>
		request<{ issue: Issue }>(`/repos/${username}/${repo}/issues`, {
			method: 'POST',
			body: JSON.stringify(data)
		}),

	update: (
		username: string,
		repo: string,
		number: number,
		data: { title?: string; body?: string; issue_type?: string; assignee_id?: number; clear_assignee?: boolean; milestone_id?: number; clear_milestone?: boolean }
	) =>
		request<{ issue: Issue }>(`/repos/${username}/${repo}/issues/${number}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		}),

	close: (username: string, repo: string, number: number) => request<{ issue: Issue }>(`/repos/${username}/${repo}/issues/${number}/close`, { method: 'POST' }),

	reopen: (username: string, repo: string, number: number) => request<{ issue: Issue }>(`/repos/${username}/${repo}/issues/${number}/reopen`, { method: 'POST' }),

	delete: (username: string, repo: string, number: number) => request<void>(`/repos/${username}/${repo}/issues/${number}`, { method: 'DELETE' }),

	setLabels: (username: string, repo: string, number: number, labelIds: number[]) =>
		request<{ issue: Issue }>(`/repos/${username}/${repo}/issues/${number}/labels`, {
			method: 'PUT',
			body: JSON.stringify({ label_ids: labelIds })
		}),

	comments: {
		list: (username: string, repo: string, number: number) => request<{ comments: IssueComment[] }>(`/repos/${username}/${repo}/issues/${number}/comments`),

		create: (username: string, repo: string, number: number, body: string) =>
			request<{ comment: IssueComment }>(`/repos/${username}/${repo}/issues/${number}/comments`, {
				method: 'POST',
				body: JSON.stringify({ body })
			}),

		update: (username: string, repo: string, number: number, commentId: number, body: string) =>
			request<{ comment: IssueComment }>(`/repos/${username}/${repo}/issues/${number}/comments/${commentId}`, {
				method: 'PATCH',
				body: JSON.stringify({ body })
			}),

		delete: (username: string, repo: string, number: number, commentId: number) => request<void>(`/repos/${username}/${repo}/issues/${number}/comments/${commentId}`, { method: 'DELETE' })
	}
};

export const labels = {
	list: (username: string, repo: string) => request<{ labels: Label[] }>(`/repos/${username}/${repo}/labels`),

	create: (username: string, repo: string, data: { name: string; color: string; description?: string }) =>
		request<{ label: Label }>(`/repos/${username}/${repo}/labels`, {
			method: 'POST',
			body: JSON.stringify(data)
		}),

	update: (username: string, repo: string, labelId: number, data: { name?: string; color?: string; description?: string }) =>
		request<{ label: Label }>(`/repos/${username}/${repo}/labels/${labelId}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		}),

	delete: (username: string, repo: string, labelId: number) => request<void>(`/repos/${username}/${repo}/labels/${labelId}`, { method: 'DELETE' })
};

export const milestones = {
	list: (username: string, repo: string, status?: string) => request<{ milestones: Milestone[] }>(`/repos/${username}/${repo}/milestones${status ? `?status=${status}` : ''}`),

	create: (username: string, repo: string, data: { title: string; description?: string }) =>
		request<{ milestone: Milestone }>(`/repos/${username}/${repo}/milestones`, {
			method: 'POST',
			body: JSON.stringify(data)
		}),

	update: (username: string, repo: string, milestoneId: number, data: { title?: string; description?: string; status?: string }) =>
		request<{ milestone: Milestone }>(`/repos/${username}/${repo}/milestones/${milestoneId}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		}),

	delete: (username: string, repo: string, milestoneId: number) => request<void>(`/repos/${username}/${repo}/milestones/${milestoneId}`, { method: 'DELETE' })
};

// Organizations

export interface Organization {
	id: number;
	login: string;
	display_name: string;
	description: string;
	avatar_url: string;
	website: string;
	social_links?: OrgSocialLink[];
	location: string;
	is_public: boolean;
	is_suspended?: boolean;
	suspension_reason?: string;
	created_at: string;
	updated_at: string;
}

export interface OrgSocialLink {
	label: string;
	url: string;
}

export interface OrgMember {
	id: number;
	org_id: number;
	user_id: number;
	role: 'owner' | 'member';
	user: User;
	created_at: string;
}

export interface Team {
	id: number;
	org_id: number;
	name: string;
	description: string;
	permission: 'read' | 'write' | 'admin';
	member_count?: number;
	repo_count?: number;
	created_at: string;
	updated_at: string;
}

export interface TeamMember {
	id: number;
	team_id: number;
	user_id: number;
	user: User;
	created_at: string;
}

export interface TeamRepo {
	id: number;
	team_id: number;
	repo_id: number;
	repo: Repository;
	created_at: string;
}

export interface ProjectItem {
	id: string;
	project_id: string;
	column_id: string;
	title: string;
	body: string;
	position: number;
	assignee_user_id?: string;
	assignee_user?: User;
	created_at: string;
	updated_at: string;
}

export interface ProjectColumn {
	id: string;
	project_id: string;
	name: string;
	description: string;
	color: string;
	position: number;
	items?: ProjectItem[];
	created_at: string;
	updated_at: string;
}

export interface Project {
	id: string;
	title: string;
	description: string;
	is_public: boolean;
	owner_user_id?: string;
	owner_org_id?: string;
	owner_user?: User;
	owner_org?: Organization;
	created_by_id: string;
	created_by: User;
	columns?: ProjectColumn[];
	created_at: string;
	updated_at: string;
}

export const orgs = {
	create: (data: { login: string; display_name?: string; description?: string }) => request<Organization>('/orgs', { method: 'POST', body: JSON.stringify(data) }),

	get: (orgname: string) =>
		request<{
			org: Organization;
			is_member: boolean;
			is_owner: boolean;
			is_following?: boolean;
			member_count: number;
			repo_count: number;
			follower_count?: number;
		}>(`/orgs/${orgname}`),

	follow: (orgname: string) => request<void>(`/orgs/${orgname}/follow`, { method: 'POST' }),

	unfollow: (orgname: string) => request<void>(`/orgs/${orgname}/follow`, { method: 'DELETE' }),

	listFollowers: (orgname: string) => request<{ users: FollowListItem[]; count: number }>(`/orgs/${orgname}/followers`),

	update: (
		orgname: string,
		data: Partial<{
			display_name: string;
			description: string;
			avatar_url: string;
			website: string;
			social_links: OrgSocialLink[];
			location: string;
			is_public: boolean;
		}>
	) =>
		request<Organization>(`/orgs/${orgname}`, { method: 'PATCH', body: JSON.stringify(data) }),

	uploadAvatar: (orgname: string, file: File) => {
		const fd = new FormData();
		fd.append('avatar', file);
		return requestUpload<{ avatar_url: string }>(`/orgs/${orgname}/avatar`, fd);
	},

	delete: (orgname: string, confirmPassword: string) => request<void>(`/orgs/${orgname}`, { method: 'DELETE', headers: { 'X-Confirm-Password': confirmPassword } }),

	getActionsUsage: (orgname: string) => request<ActionsUsage>(`/orgs/${orgname}/actions/usage`),

	members: {
		list: (orgname: string) => request<OrgMember[]>(`/orgs/${orgname}/members`),

		add: (orgname: string, username: string, role?: string) => request<OrgMember>(`/orgs/${orgname}/members`, { method: 'POST', body: JSON.stringify({ username, role: role ?? 'member' }) }),

		updateRole: (orgname: string, username: string, role: string) => request<OrgMember>(`/orgs/${orgname}/members/${username}`, { method: 'PATCH', body: JSON.stringify({ role }) }),

		remove: (orgname: string, username: string) => request<void>(`/orgs/${orgname}/members/${username}`, { method: 'DELETE' })
	},

	teams: {
		list: (orgname: string) => request<Team[]>(`/orgs/${orgname}/teams`),

		create: (orgname: string, data: { name: string; description?: string; permission?: string }) => request<Team>(`/orgs/${orgname}/teams`, { method: 'POST', body: JSON.stringify(data) }),

		get: (orgname: string, teamId: number) => request<Team>(`/orgs/${orgname}/teams/${teamId}`),

		update: (orgname: string, teamId: number, data: Partial<{ name: string; description: string; permission: string }>) =>
			request<Team>(`/orgs/${orgname}/teams/${teamId}`, { method: 'PATCH', body: JSON.stringify(data) }),

		delete: (orgname: string, teamId: number, confirmPassword: string) =>
			request<void>(`/orgs/${orgname}/teams/${teamId}`, { method: 'DELETE', headers: { 'X-Confirm-Password': confirmPassword } }),

		members: {
			list: (orgname: string, teamId: number) => request<TeamMember[]>(`/orgs/${orgname}/teams/${teamId}/members`),
			add: (orgname: string, teamId: number, username: string) => request<TeamMember[]>(`/orgs/${orgname}/teams/${teamId}/members`, { method: 'POST', body: JSON.stringify({ username }) }),
			remove: (orgname: string, teamId: number, username: string) => request<void>(`/orgs/${orgname}/teams/${teamId}/members/${username}`, { method: 'DELETE' })
		},

		repos: {
			list: (orgname: string, teamId: number) => request<TeamRepo[]>(`/orgs/${orgname}/teams/${teamId}/repos`),
			add: (orgname: string, teamId: number, repo_name: string) =>
				request<{ message: string }>(`/orgs/${orgname}/teams/${teamId}/repos`, { method: 'POST', body: JSON.stringify({ repo_name }) }),
			remove: (orgname: string, teamId: number, repoId: number) => request<void>(`/orgs/${orgname}/teams/${teamId}/repos/${repoId}`, { method: 'DELETE' })
		}
	},

	repos: {
		list: (orgname: string) => request<Repository[]>(`/orgs/${orgname}/repos`),
		create: (orgname: string, data: { name: string; description?: string; is_private?: boolean; initialize_with_readme?: boolean }) =>
			request<Repository>(`/orgs/${orgname}/repos`, { method: 'POST', body: JSON.stringify(data) })
	},

	listUserOrgs: (username: string) => request<Organization[]>(`/users/${username}/orgs`),
	listMyOrgs: () => request<Organization[]>('/users/me/orgs')
};

export const projects = {
	listForUser: (username: string) => request<{ projects: Project[] }>(`/users/${username}/projects`),
	createForUser: (data: { title: string; description?: string; is_public?: boolean }) =>
		request<{ project: Project }>('/users/me/projects', { method: 'POST', body: JSON.stringify(data) }),

	listForOrg: (orgname: string) => request<{ projects: Project[] }>(`/orgs/${orgname}/projects`),
	createForOrg: (orgname: string, data: { title: string; description?: string; is_public?: boolean }) =>
		request<{ project: Project }>(`/orgs/${orgname}/projects`, { method: 'POST', body: JSON.stringify(data) }),

	get: (projectId: string) => request<{ project: Project }>(`/projects/${projectId}`),
	update: (projectId: string, data: { title?: string; description?: string; is_public?: boolean }) =>
		request<{ project: Project }>(`/projects/${projectId}`, { method: 'PATCH', body: JSON.stringify(data) }),
	delete: (projectId: string) => request<void>(`/projects/${projectId}`, { method: 'DELETE' }),

	columns: {
		create: (projectId: string, data: { name: string; description?: string; color?: string; position?: number }) =>
			request<{ column: ProjectColumn }>(`/projects/${projectId}/columns`, { method: 'POST', body: JSON.stringify(data) }),
		update: (projectId: string, columnId: string, data: { name?: string; description?: string; color?: string; position?: number }) =>
			request<{ column: ProjectColumn }>(`/projects/${projectId}/columns/${columnId}`, { method: 'PATCH', body: JSON.stringify(data) }),
		delete: (projectId: string, columnId: string) => request<void>(`/projects/${projectId}/columns/${columnId}`, { method: 'DELETE' })
	},

	items: {
		create: (
			projectId: string,
			data: { column_id: string; title: string; body?: string; position?: number; assignee_user_id?: string }
		) => request<{ item: ProjectItem }>(`/projects/${projectId}/items`, { method: 'POST', body: JSON.stringify(data) }),
		update: (projectId: string, itemId: string, data: { title?: string; body?: string; assignee_user_id?: string; clear_assignee?: boolean }) =>
			request<{ item: ProjectItem }>(`/projects/${projectId}/items/${itemId}`, { method: 'PATCH', body: JSON.stringify(data) }),
		move: (projectId: string, itemId: string, data: { column_id?: string; position?: number }) =>
			request<{ item: ProjectItem }>(`/projects/${projectId}/items/${itemId}/move`, { method: 'POST', body: JSON.stringify(data) }),
		delete: (projectId: string, itemId: string) => request<void>(`/projects/${projectId}/items/${itemId}`, { method: 'DELETE' })
	}
};

export interface ModerationBlockedUser {
	id: number;
	policy_id: number;
	user_id: number;
	reason: string;
	created_at: string;
	user: User;
}

export interface ModerationBlockedKeyword {
	id: number;
	policy_id: number;
	keyword: string;
	apply_to: 'all' | 'issues' | 'prs' | 'commits';
	created_at: string;
}

export interface ModerationPolicy {
	id: number;
	created_at: string;
	updated_at: string;
	user_id?: number;
	org_id?: number;
	repo_id?: number;
	inherit_from_owner: boolean;
	block_issues: boolean;
	block_prs: boolean;
	block_pushes: boolean;
	block_comments: boolean;
	max_issues_per_day: number;
	max_prs_per_day: number;
	max_comments_per_day: number;
	min_account_age_days: number;
	require_min_activity: boolean;
	min_commits: number;
	min_contributions: number;
	blocked_users: ModerationBlockedUser[];
	blocked_keywords: ModerationBlockedKeyword[];
}

type PolicyUpdateInput = Partial<{
	inherit_from_owner: boolean;
	block_issues: boolean;
	block_prs: boolean;
	block_pushes: boolean;
	block_comments: boolean;
	max_issues_per_day: number;
	max_prs_per_day: number;
	max_comments_per_day: number;
	min_account_age_days: number;
	require_min_activity: boolean;
	min_commits: number;
	min_contributions: number;
}>;

function modRoutes(base: string) {
	return {
		getPolicy: () => request<{ policy: ModerationPolicy }>(base),
		updatePolicy: (data: PolicyUpdateInput) => request<{ policy: ModerationPolicy }>(base, { method: 'PUT', body: JSON.stringify(data) }),
		blockUser: (username: string, reason?: string) => request<{ blocked_user: ModerationBlockedUser }>(`${base}/blocked-users`, { method: 'POST', body: JSON.stringify({ username, reason }) }),
		unblockUser: (userID: number) => request<void>(`${base}/blocked-users/${userID}`, { method: 'DELETE' }),
		addKeyword: (keyword: string, apply_to: string = 'all') => request<{ keyword: ModerationBlockedKeyword }>(`${base}/keywords`, { method: 'POST', body: JSON.stringify({ keyword, apply_to }) }),
		removeKeyword: (keywordID: number) => request<void>(`${base}/keywords/${keywordID}`, { method: 'DELETE' })
	};
}

export const moderation = {
	user: () => modRoutes('/users/me/moderation'),
	org: (orgname: string) => modRoutes(`/orgs/${orgname}/moderation`),
	repo: (username: string, repo: string) => modRoutes(`/repos/${username}/${repo}/moderation`)
};

export interface Webhook {
	id: number;
	repo_id: number;
	payload_url: string;
	content_type: string;
	has_secret: boolean;
	insecure_ssl: boolean;
	active: boolean;
	events: string[];
	created_at: string;
	updated_at: string;
}

export interface WebhookDelivery {
	id: number;
	webhook_id: number;
	guid: string;
	event: string;
	payload: string;
	response_code: number;
	response_body: string;
	duration_ms: number;
	success: boolean;
	created_at: string;
}

export const webhooks = {
	list: (username: string, repo: string) => request<{ webhooks: Webhook[] }>(`/repos/${username}/${repo}/hooks`),

	get: (username: string, repo: string, id: number) => request<{ webhook: Webhook }>(`/repos/${username}/${repo}/hooks/${id}`),

	create: (username: string, repo: string, data: { payload_url: string; content_type?: string; secret?: string; insecure_ssl?: boolean; active?: boolean; events: string[] }) =>
		request<{ webhook: Webhook }>(`/repos/${username}/${repo}/hooks`, { method: 'POST', body: JSON.stringify(data) }),

	update: (username: string, repo: string, id: number, data: { payload_url?: string; content_type?: string; secret?: string; insecure_ssl?: boolean; active?: boolean; events?: string[] }) =>
		request<{ webhook: Webhook }>(`/repos/${username}/${repo}/hooks/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),

	delete: (username: string, repo: string, id: number) => request<void>(`/repos/${username}/${repo}/hooks/${id}`, { method: 'DELETE' }),

	deliveries: {
		list: (username: string, repo: string, hookId: number) => request<{ deliveries: WebhookDelivery[] }>(`/repos/${username}/${repo}/hooks/${hookId}/deliveries`),

		get: (username: string, repo: string, hookId: number, deliveryId: number) => request<{ delivery: WebhookDelivery }>(`/repos/${username}/${repo}/hooks/${hookId}/deliveries/${deliveryId}`),

		redeliver: (username: string, repo: string, hookId: number, deliveryId: number) =>
			request<void>(`/repos/${username}/${repo}/hooks/${hookId}/deliveries/${deliveryId}/redeliver`, { method: 'POST' })
	}
};

export interface OAuthApp {
	id: number;
	client_id: string;
	name: string;
	description: string;
	homepage_url: string;
	callback_url: string;
	logo_url: string;
	enable_device_flow: boolean;
	owner_id: number;
	owner_type: 'user' | 'org';
	authorization_count: number;
	created_at: string;
	updated_at: string;
}

export interface OAuthAuthorization {
	id: number;
	app_id: number;
	user_id: number;
	scopes: string;
	app: OAuthApp;
	created_at: string;
	updated_at: string;
}

export const oauthApps = {
	// Developer-facing: manage apps you own
	listUserApps: () => request<{ apps: OAuthApp[] }>('/users/me/oauth-apps'),

	createUserApp: (data: { name: string; description?: string; homepage_url: string; callback_url: string; logo_url?: string; enable_device_flow?: boolean }) =>
		request<{ app: OAuthApp; client_secret: string }>('/users/me/oauth-apps', { method: 'POST', body: JSON.stringify(data) }),

	listOrgApps: (orgname: string) => request<{ apps: OAuthApp[] }>(`/orgs/${orgname}/oauth-apps`),

	createOrgApp: (orgname: string, data: { name: string; description?: string; homepage_url: string; callback_url: string; logo_url?: string; enable_device_flow?: boolean }) =>
		request<{ app: OAuthApp; client_secret: string }>(`/orgs/${orgname}/oauth-apps`, { method: 'POST', body: JSON.stringify(data) }),

	get: (id: number) => request<{ app: OAuthApp }>(`/oauth-apps/${id}`),

	update: (id: number, data: Partial<{ name: string; description: string; homepage_url: string; callback_url: string; logo_url: string; enable_device_flow: boolean }>) =>
		request<{ app: OAuthApp }>(`/oauth-apps/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),

	delete: (id: number) => request<void>(`/oauth-apps/${id}`, { method: 'DELETE' }),

	regenerateSecret: (id: number) => request<{ client_secret: string }>(`/oauth-apps/${id}/regenerate-secret`, { method: 'POST' }),

	// User-facing: apps the current user has authorized
	listAuthorizedApps: () => request<{ authorizations: OAuthAuthorization[] }>('/users/me/authorized-apps'),

	revokeAuthorization: (id: number) => request<void>(`/users/me/authorized-apps/${id}`, { method: 'DELETE' })
};

export interface OAuthConsentInfo {
	id: number;
	client_id: string;
	name: string;
	description: string;
	homepage_url: string;
	callback_url: string;
	logo_url: string;
}

export interface OAuthDeviceInfo {
	user_code: string;
	scopes: string;
	expires_at: string;
	app: {
		name: string;
		description: string;
		homepage_url: string;
		logo_url: string;
	};
}

export const oauthFlow = {
	/** Fetch app info for the consent page (no auth required). */
	getAppInfo: (clientId: string) => request<OAuthConsentInfo>(`/oauth/app-info?client_id=${encodeURIComponent(clientId)}`),

	/** Submit the user's authorization decision. Returns the redirect URI with code+state. */
	authorize: (data: { client_id: string; redirect_uri: string; scope: string; state: string; code_challenge?: string; code_challenge_method?: string }) =>
		request<{ redirect_uri: string }>('/oauth/authorize', { method: 'POST', body: JSON.stringify(data) }),

	/** Fetch device code info for the device activation page. */
	getDeviceInfo: (userCode: string) => request<OAuthDeviceInfo>(`/oauth/device-info?user_code=${encodeURIComponent(userCode)}`),

	/** Approve a device code on behalf of the authenticated user. */
	approveDevice: (userCode: string) =>
		request<{ message: string; app_name: string }>('/oauth/device/approve', {
			method: 'POST',
			body: JSON.stringify({ user_code: userCode })
		}),

	/** Deny a device code on behalf of the authenticated user. */
	denyDevice: (userCode: string) =>
		request<{ message: string }>('/oauth/device/deny', {
			method: 'POST',
			body: JSON.stringify({ user_code: userCode })
		})
};

export interface CodeMatch {
	path: string;
	line: number;
	content: string;
}

export interface FileMatch {
	path: string;
	type: string;
}

export const search = {
	repos: (q: string, limit = 20, offset = 0) => request<{ items: Repository[]; total: number }>(`/search?${new URLSearchParams({ q, type: 'repos', limit: String(limit), offset: String(offset) })}`),

	users: (q: string, limit = 20, offset = 0) => request<{ items: User[]; total: number }>(`/search?${new URLSearchParams({ q, type: 'users', limit: String(limit), offset: String(offset) })}`),

	orgs: (q: string, limit = 20, offset = 0) => request<{ items: Organization[]; total: number }>(`/search?${new URLSearchParams({ q, type: 'orgs', limit: String(limit), offset: String(offset) })}`),

	code: (owner: string, repo: string, q: string, ref?: string) =>
		request<{ items: CodeMatch[]; total: number }>(`/search?${new URLSearchParams({ q, type: 'code', owner, repo, ...(ref ? { ref } : {}) })}`),

	files: (owner: string, repo: string, q: string, ref?: string) =>
		request<{ items: FileMatch[]; total: number }>(`/search?${new URLSearchParams({ q, type: 'files', owner, repo, ...(ref ? { ref } : {}) })}`)
};

export type AppPermissionLevel = 'none' | 'read' | 'write';

export interface AppPermissions {
	// Repository permissions
	contents?: AppPermissionLevel;
	issues?: AppPermissionLevel;
	pull_requests?: AppPermissionLevel;
	webhooks?: AppPermissionLevel;
	releases?: AppPermissionLevel;
	workflows?: AppPermissionLevel;
	metadata?: AppPermissionLevel;
	collaborators?: AppPermissionLevel;
	// Organization permissions
	members?: AppPermissionLevel;
	// Account permissions
	profile?: AppPermissionLevel;
	email?: AppPermissionLevel;
	ssh_keys?: AppPermissionLevel;
}

export interface GitPierApp {
	id: number;
	name: string;
	slug: string;
	description: string;
	homepage_url: string;
	logo_url: string;
	setup_url: string;
	redirect_on_update: boolean;
	webhook_url: string;
	webhook_active: boolean;
	is_public: boolean;
	callback_urls: string; // JSON string array
	request_user_auth: boolean;
	expire_user_tokens: boolean;
	enable_device_flow: boolean;
	client_id: string;
	repo_permissions: string; // JSON object string
	org_permissions: string; // JSON object string
	account_permissions: string; // JSON object string
	events: string; // JSON string array
	owner_id: number;
	owner_type: 'user' | 'org';
	installation_count: number;
	key_count: number;
	created_at: string;
	updated_at: string;
}

export interface AppPrivateKey {
	id: number;
	app_id: number;
	fingerprint: string;
	created_at: string;
}

export interface AppInstallation {
	id: number;
	app_id: number;
	app: GitPierApp;
	account_id: number;
	account_type: 'user' | 'org';
	repository_selection: 'all' | 'selected';
	repo_permissions: string;
	org_permissions: string;
	account_permissions: string;
	suspended_at?: string;
	suspended_by?: number;
	repositories?: Array<{ id: number; repo: Repository }>;
	created_at: string;
	updated_at: string;
}

type CreateAppData = {
	name: string;
	description?: string;
	homepage_url: string;
	logo_url?: string;
	setup_url?: string;
	redirect_on_update?: boolean;
	webhook_url?: string;
	webhook_secret?: string;
	webhook_active?: boolean;
	is_public?: boolean;
	callback_urls?: string[];
	request_user_auth?: boolean;
	expire_user_tokens?: boolean;
	enable_device_flow?: boolean;
	repo_permissions?: Record<string, string>;
	org_permissions?: Record<string, string>;
	account_permissions?: Record<string, string>;
	events?: string[];
};

export const gitpierApps = {
	// Developer — user-owned apps
	listUserApps: () => request<{ apps: GitPierApp[] }>('/users/me/apps'),
	createUserApp: (data: CreateAppData) => request<{ app: GitPierApp; client_secret: string }>('/users/me/apps', { method: 'POST', body: JSON.stringify(data) }),

	// Developer — org-owned apps
	listOrgApps: (orgname: string) => request<{ apps: GitPierApp[] }>(`/orgs/${orgname}/apps`),
	createOrgApp: (orgname: string, data: CreateAppData) => request<{ app: GitPierApp; client_secret: string }>(`/orgs/${orgname}/apps`, { method: 'POST', body: JSON.stringify(data) }),

	// Developer — CRUD by ID
	get: (id: number) => request<{ app: GitPierApp }>(`/apps/${id}`),
	update: (id: number, data: Partial<CreateAppData>) => request<{ app: GitPierApp }>(`/apps/${id}`, { method: 'PATCH', body: JSON.stringify(data) }),
	delete: (id: number) => request<void>(`/apps/${id}`, { method: 'DELETE' }),
	regenerateSecret: (id: number) => request<{ client_secret: string }>(`/apps/${id}/regenerate-secret`, { method: 'POST' }),

	// Developer — private keys
	listKeys: (id: number) => request<{ keys: AppPrivateKey[] }>(`/apps/${id}/keys`),
	generateKey: (id: number) => request<{ key: AppPrivateKey; private_key: string }>(`/apps/${id}/keys`, { method: 'POST' }),
	deleteKey: (id: number, keyID: number) => request<void>(`/apps/${id}/keys/${keyID}`, { method: 'DELETE' }),

	// Developer — view installations
	listInstallations: (id: number) => request<{ installations: AppInstallation[] }>(`/apps/${id}/installations`),

	// Public — app info by slug
	getBySlug: (slug: string) => request<GitPierApp>(`/apps/slug/${slug}`),

	// Install / uninstall
	install: (slug: string, data: { target?: string; repository_selection?: string; repo_ids?: number[] }) =>
		request<{ installation: AppInstallation }>(`/apps/slug/${slug}/install`, { method: 'POST', body: JSON.stringify(data) }),

	// Installations
	getInstallation: (id: number) => request<{ installation: AppInstallation }>(`/installations/${id}`),
	uninstall: (id: number) => request<void>(`/installations/${id}`, { method: 'DELETE' }),
	updateInstallationRepos: (id: number, data: { repository_selection: string; repo_ids?: number[] }) =>
		request<{ installation: AppInstallation }>(`/installations/${id}/repositories`, { method: 'PATCH', body: JSON.stringify(data) }),
	syncInstallationPermissions: (id: number) => request<{ installation: AppInstallation }>(`/installations/${id}/permissions`, { method: 'PATCH' }),
	suspendInstallation: (id: number) => request<void>(`/installations/${id}/suspended`, { method: 'PUT' }),
	unsuspendInstallation: (id: number) => request<void>(`/installations/${id}/suspended`, { method: 'DELETE' }),

	// User's installed apps
	listUserInstallations: () => request<{ installations: AppInstallation[] }>('/users/me/installations'),
	listOrgInstallations: (orgname: string) => request<{ installations: AppInstallation[] }>(`/orgs/${orgname}/installations`)
};

export interface ContainerRepository {
	id: number;
	namespace: string;
	name: string;
	is_public: boolean;
	owner_id: number;
	owner_type: 'user' | 'org';
	created_at: string;
	updated_at: string;
}

export interface ContainerTagEntry {
	id: number;
	tag: string;
	digest: string;
	pull_count: number;
	created_at: string;
	updated_at: string;
}

export interface ContainerPackageDetails {
	package: ContainerRepository;
	tags: ContainerTagEntry[];
	tags_count: number;
	pull_command: string;
}

export const packages = {
	list: (namespace: string) => request<ContainerRepository[]>(`/packages/${namespace}`),
	get: (namespace: string, image: string) => request<ContainerPackageDetails>(`/packages/${namespace}/${image}`),
	update: (namespace: string, image: string, data: { is_public: boolean }) =>
		request<ContainerRepository>(`/packages/${namespace}/${image}`, {
			method: 'PATCH',
			body: JSON.stringify(data)
		}),
	delete: (namespace: string, image: string) =>
		request<void>(`/packages/${namespace}/${image}`, {
			method: 'DELETE'
		})
};

export interface AdminLargestRepository {
	id: string;
	namespace: string;
	name: string;
	full_name: string;
	size_bytes: number;
}

export interface AdminSystemStats {
	generated_at: string;
	repositories: {
		total: number;
		public: number;
		private: number;
		archived: number;
		suspended: number;
		total_size_bytes: number;
		total_size_gb: number;
		filesystem_total_size_bytes: number;
		filesystem_total_size_gb: number;
		average_size_bytes: number;
		average_size_mb: number;
		filesystem_scan_errors: number;
	};
	users: {
		total: number;
		suspended: number;
	};
	organizations: {
		total: number;
		suspended: number;
	};
	issues: {
		total: number;
		open: number;
		closed: number;
	};
	pull_requests: {
		total: number;
		open: number;
		closed: number;
		merged: number;
	};
	workflow_runs: {
		total: number;
		running: number;
		success: number;
		failure: number;
	};
	largest_repositories: AdminLargestRepository[];
}

export const adminSystem = {
	getStats: (password: string) =>
		request<AdminSystemStats>('/admin/system', {
			method: 'GET',
			headers: { 'X-System-Admin-Password': password }
		})
};

export interface StorageIncreaseRequest {
	id: number;
	repo_id: number;
	requested_by_user_id: number;
	requested_limit_bytes: number;
	message: string;
	status: 'pending' | 'approved' | 'rejected';
	review_note: string;
	reviewed_by_user_id?: number;
	reviewed_at?: string;
	created_at: string;
	updated_at: string;
	repo: Repository;
	requested_by_user: User;
	reviewed_by_user?: User;
}

export interface FeedbackEntry {
	id: number;
	category: 'bug' | 'feature' | 'other';
	message: string;
	status: 'new' | 'in_review' | 'implemented' | 'dismissed';
	admin_note: string;
	user_id?: number;
	user?: User;
	reviewed_by_user_id?: number;
	reviewed_at?: string;
	created_at: string;
	updated_at: string;
}

export const feedback = {
	submit: (category: 'bug' | 'feature' | 'other', message: string) =>
		request<{ feedback: FeedbackEntry }>('/feedback', {
			method: 'POST',
			body: JSON.stringify({ category, message })
		})
};
