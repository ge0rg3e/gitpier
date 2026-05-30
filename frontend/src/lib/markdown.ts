import { marked, type Tokens } from 'marked';
import DOMPurify from 'dompurify';
import { API_BASE } from '$lib/api/client';

const MEDIA_SANITIZE_OPTIONS = {
	ADD_TAGS: ['video', 'source'],
	ADD_ATTR: ['src', 'type', 'controls', 'preload', 'poster', 'playsinline', 'muted', 'loop', 'data-mention']
};

const mentionRegex = /(^|[^\w-])@([a-zA-Z0-9](?:[a-zA-Z0-9-]{0,38}))/g;

function linkMentions(html: string): string {
	return html.replace(mentionRegex, (full, prefix, username) => {
		return `${prefix}<a href="/${username}" class="mention-link" style="color:var(--primary, #58a6ff) !important;text-decoration:underline !important;text-decoration-thickness:2px;text-underline-offset:2px;text-decoration-color:var(--primary, #58a6ff);font-weight:700;" data-mention="true">@${username}</a>`;
	});
}

export function renderMarkdownHtml(markdown: string | null | undefined): string {
	if (!markdown) return '';
	const parsed = linkMentions(marked.parse(markdown) as string);
	return DOMPurify.sanitize(parsed, MEDIA_SANITIZE_OPTIONS);
}

/**
 * Renders markdown for a specific repo, rewriting relative image URLs to use
 * the backend /raw endpoint so images embedded in READMEs like
 * `![image.png](image.png)` are served correctly.
 */
export function renderRepoMarkdownHtml(markdown: string | null | undefined, username: string, repo: string, ref?: string): string {
	if (!markdown) return '';

	const renderer = new marked.Renderer();

	renderer.image = ({ href, title, text }: Tokens.Image): string => {
		let src = href;
		if (src && !src.startsWith('http://') && !src.startsWith('https://') && !src.startsWith('data:')) {
			const params = new URLSearchParams({ path: src });
			if (ref) params.set('ref', ref);
			src = `${API_BASE}/api/v1/repos/${username}/${repo}/raw?${params}`;
		}
		const titleAttr = title ? ` title="${title}"` : '';
		return `<img src="${src}" alt="${text}"${titleAttr}>`;
	};

	const parsed = linkMentions(marked.parse(markdown, { renderer }) as string);
	return DOMPurify.sanitize(parsed, MEDIA_SANITIZE_OPTIONS);
}
