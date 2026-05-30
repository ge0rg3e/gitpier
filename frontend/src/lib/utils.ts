import { API_BASE } from '$lib/api/client';
import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';
import dayjs from 'dayjs';
import relativeTime from 'dayjs/plugin/relativeTime';
import Prism from 'prismjs';
import 'prismjs/components/prism-typescript';
import 'prismjs/components/prism-javascript';
import 'prismjs/components/prism-go';
import 'prismjs/components/prism-python';
import 'prismjs/components/prism-rust';
import 'prismjs/components/prism-java';
import 'prismjs/components/prism-c';
import 'prismjs/components/prism-cpp';
import 'prismjs/components/prism-css';
import 'prismjs/components/prism-json';
import 'prismjs/components/prism-yaml';
import 'prismjs/components/prism-bash';
import 'prismjs/components/prism-sql';
import 'prismjs/components/prism-toml';
import 'prismjs/components/prism-markdown';

dayjs.extend(relativeTime);

const MIN_REASONABLE_GIT_YEAR = 2005;

export function cn(...inputs: ClassValue[]) {
	return twMerge(clsx(inputs));
}

/** Prepends the backend base URL to relative media paths (e.g. /avatars/...) */
export function mediaUrl(path: string | null | undefined): string {
	if (!path) return '';
	if (path.startsWith('http://') || path.startsWith('https://')) return path;
	const normalizedPath = path.startsWith('/') ? path : `/${path}`;
	return `${API_BASE}${normalizedPath}`;
}

type CommitAuthorLike = {
	name?: string | null;
	username?: string | null;
	avatar_url?: string | null;
};

export function commitAuthorName(author: CommitAuthorLike | null | undefined): string {
	return author?.username?.trim() || author?.name?.trim() || 'Unknown';
}

export function commitAuthorHref(author: CommitAuthorLike | null | undefined): string | null {
	const username = author?.username?.trim();
	return username ? `/${username}` : null;
}

export function commitAuthorAvatarUrl(author: CommitAuthorLike | null | undefined): string {
	return mediaUrl(author?.avatar_url);
}

export function commitAuthorInitial(author: CommitAuthorLike | null | undefined): string {
	return commitAuthorName(author).slice(0, 1).toUpperCase() || '?';
}

export function timeAgo(date: string | undefined | null): string {
	if (!isValidGitDate(date)) return '';
	return dayjs(date).fromNow();
}

export function isValidGitDate(date: string | undefined | null): boolean {
	if (!date) return false;
	const parsed = dayjs(date);
	if (!parsed.isValid()) return false;
	if (parsed.year() < MIN_REASONABLE_GIT_YEAR) return false;
	if (parsed.isAfter(dayjs().add(5, 'minute'))) return false;
	return true;
}

export function formatDate(date: string): string {
	return dayjs(date).format('MMM D, YYYY');
}

export function highlightCode(code: string, language: string): string {
	const langMap: Record<string, string> = {
		ts: 'typescript',
		js: 'javascript',
		go: 'go',
		py: 'python',
		rs: 'rust',
		java: 'java',
		cs: 'csharp',
		cpp: 'cpp',
		c: 'c',
		html: 'html',
		css: 'css',
		json: 'json',
		yaml: 'yaml',
		yml: 'yaml',
		md: 'markdown',
		sh: 'bash',
		shell: 'bash',
		sql: 'sql',
		toml: 'toml',
		xml: 'xml',
		dockerfile: 'bash'
	};
	const lang = langMap[language.toLowerCase()] || language.toLowerCase();
	if (Prism.languages[lang]) {
		return Prism.highlight(code, Prism.languages[lang], lang);
	}
	return code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
}

// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChild<T> = T extends { child?: any } ? Omit<T, 'child'> : T;
// eslint-disable-next-line @typescript-eslint/no-explicit-any
export type WithoutChildren<T> = T extends { children?: any } ? Omit<T, 'children'> : T;
export type WithoutChildrenOrChild<T> = WithoutChildren<WithoutChild<T>>;
export type WithElementRef<T, U extends HTMLElement = HTMLElement> = T & { ref?: U | null };
