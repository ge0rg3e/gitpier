import { env } from '$env/dynamic/public';
import { error } from '@sveltejs/kit';
import { Resvg } from '@resvg/resvg-js';
import type { RequestHandler } from './$types';

type RepoPayload = {
	repo?: {
		name?: string;
		description?: string | null;
		star_count?: number;
		fork_count?: number;
		owner?: {
			username?: string;
			display_name?: string;
			avatar_url?: string | null;
		};
		org?: {
			login?: string;
			display_name?: string;
			avatar_url?: string | null;
		};
	};
	stats?: {
		commits?: number;
	};
};

const API_BASE = env.PUBLIC_API_URL ?? 'http://localhost:8080';
const CARD_WIDTH = 1200;
const CARD_HEIGHT = 630;

function escapeXml(value: string): string {
	return value.replaceAll('&', '&amp;').replaceAll('<', '&lt;').replaceAll('>', '&gt;').replaceAll('"', '&quot;').replaceAll("'", '&apos;');
}

function clampText(value: string, maxLength: number): string {
	if (value.length <= maxLength) return value;
	return `${value.slice(0, maxLength - 1).trim()}...`;
}

function wrapText(value: string, maxCharsPerLine: number, maxLines: number): string[] {
	const words = value.trim().split(/\s+/).filter(Boolean);
	if (words.length === 0) return [];

	const lines: string[] = [];
	let currentLine = '';

	for (const word of words) {
		const next = currentLine ? `${currentLine} ${word}` : word;
		if (next.length <= maxCharsPerLine) {
			currentLine = next;
			continue;
		}

		if (currentLine) {
			lines.push(currentLine);
			if (lines.length === maxLines) {
				return [...lines.slice(0, maxLines - 1), clampText(lines[maxLines - 1], maxCharsPerLine - 1)];
			}
		}

		if (word.length > maxCharsPerLine) {
			lines.push(clampText(word, maxCharsPerLine));
			currentLine = '';
			if (lines.length === maxLines) {
				return [...lines.slice(0, maxLines - 1), clampText(lines[maxLines - 1], maxCharsPerLine - 1)];
			}
		} else {
			currentLine = word;
		}
	}

	if (currentLine && lines.length < maxLines) {
		lines.push(currentLine);
	}

	if (lines.length > maxLines) {
		return lines.slice(0, maxLines - 1).concat(clampText(lines[maxLines - 1], maxCharsPerLine - 1));
	}

	return lines;
}

function formatNumber(value: number | undefined): string {
	if (!value || value <= 0) return '0';
	if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1).replace(/\.0$/, '')}M`;
	if (value >= 1_000) return `${(value / 1_000).toFixed(1).replace(/\.0$/, '')}k`;
	return String(value);
}

async function loadRepoData(username: string, repo: string): Promise<RepoPayload | null> {
	const response = await fetch(`${API_BASE}/api/v1/repos/${encodeURIComponent(username)}/${encodeURIComponent(repo)}`);
	if (!response.ok) {
		if (response.status === 404) return null;
		throw error(response.status, 'Failed to load repository metadata');
	}

	return (await response.json()) as RepoPayload;
}

function buildSvg(params: { ownerName: string; repoName: string; description: string; stars: number; forks: number; commits: number; logoHref: string | null }) {
	const owner = escapeXml(clampText(params.ownerName, 24));
	const repo = escapeXml(clampText(params.repoName, 32));
	const descriptionLines = wrapText(params.description, 56, 2);
	const stats = [
		{ label: 'Stars', value: formatNumber(params.stars) },
		{ label: 'Forks', value: formatNumber(params.forks) },
		{ label: 'Commits', value: formatNumber(params.commits) }
	];

	const descriptionMarkup = descriptionLines.map((line, index) => `<tspan x="124" dy="${index === 0 ? 0 : 42}">${escapeXml(line)}</tspan>`).join('');

	const logoMarkup = params.logoHref
		? `<image href="${escapeXml(params.logoHref)}" x="954" y="72" width="168" height="168" clip-path="url(#logoClip)" preserveAspectRatio="xMidYMid contain" opacity="0.96" />`
		: `<circle cx="1038" cy="156" r="84" fill="#0F172A"/><text x="1038" y="165" text-anchor="middle" fill="#F8FAFC" font-size="46" font-family="Arial, sans-serif" font-weight="700">G</text>`;

	return `<?xml version="1.0" encoding="UTF-8"?>
<svg width="${CARD_WIDTH}" height="${CARD_HEIGHT}" viewBox="0 0 ${CARD_WIDTH} ${CARD_HEIGHT}" fill="none" xmlns="http://www.w3.org/2000/svg">
	<defs>
		<linearGradient id="bgA" x1="120" y1="24" x2="1120" y2="624" gradientUnits="userSpaceOnUse">
			<stop stop-color="#F8FAFC"/>
			<stop offset="1" stop-color="#E2E8F0"/>
		</linearGradient>
		<linearGradient id="shapeA" x1="66" y1="44" x2="420" y2="320" gradientUnits="userSpaceOnUse">
			<stop stop-color="#C7D2FE"/>
			<stop offset="1" stop-color="#BFDBFE"/>
		</linearGradient>
		<linearGradient id="lineA" x1="72" y1="604" x2="1128" y2="604" gradientUnits="userSpaceOnUse">
			<stop stop-color="#2563EB"/>
			<stop offset="1" stop-color="#0EA5E9"/>
		</linearGradient>
		<linearGradient id="logoBg" x1="954" y1="72" x2="1122" y2="240" gradientUnits="userSpaceOnUse">
			<stop stop-color="#0F172A"/>
			<stop offset="1" stop-color="#1E293B"/>
		</linearGradient>
		<clipPath id="logoClip">
			<circle cx="1038" cy="156" r="84"/>
		</clipPath>
		<filter id="shadow" x="52" y="32" width="1096" height="566" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
			<feDropShadow dx="0" dy="14" stdDeviation="14" flood-color="#0F172A" flood-opacity="0.12"/>
		</filter>
	</defs>

	<rect width="${CARD_WIDTH}" height="${CARD_HEIGHT}" fill="url(#bgA)"/>
	<circle cx="210" cy="100" r="170" fill="url(#shapeA)" opacity="0.48"/>
	<circle cx="1102" cy="566" r="230" fill="#DBEAFE" opacity="0.46"/>

	<g filter="url(#shadow)">
		<rect x="72" y="48" width="1056" height="534" rx="34" fill="white"/>
	</g>

	<text x="124" y="166" fill="#0F172A" font-size="60" font-family="Geist, Inter, Segoe UI, Arial, sans-serif" font-weight="500">${owner}<tspan fill="#64748B" font-weight="400">/</tspan><tspan fill="#0F172A" font-weight="500">${repo}</tspan></text>
	<text x="124" y="238" fill="#334155" font-size="26" font-family="Geist, Inter, Segoe UI, Arial, sans-serif">${descriptionMarkup}</text>

	<g>
		${stats
			.map(
				(stat, index) => `
		<g transform="translate(${124 + index * 248} 442)">
			<text x="0" y="0" fill="#0F172A" font-size="56" font-family="Geist, Inter, Segoe UI, Arial, sans-serif" font-weight="750">${escapeXml(stat.value)}</text>
			<text x="0" y="42" fill="#64748B" font-size="24" font-family="Geist, Inter, Segoe UI, Arial, sans-serif" font-weight="560">${escapeXml(stat.label)}</text>
		</g>`
			)
			.join('')}
	</g>

	<circle cx="1038" cy="156" r="84" fill="url(#logoBg)"/>
	<circle cx="1038" cy="156" r="84" fill="none" stroke="#BFDBFE" stroke-width="3" opacity="0.9"/>
	${logoMarkup}
	<rect x="72" y="598" width="1056" height="12" rx="6" fill="url(#lineA)"/>
	<text x="1110" y="560" text-anchor="end" fill="#64748B" font-size="20" font-family="Geist, Inter, Segoe UI, Arial, sans-serif" letter-spacing="0.3">GitPier.dev</text>
</svg>`;
}

export const GET: RequestHandler = async ({ params, url }) => {
	const username = params.username?.trim();
	const repoName = params.repo?.trim();

	if (!username || !repoName) {
		throw error(400, 'Missing repository parameters');
	}

	const repoData = await loadRepoData(username, repoName);
	const fallbackLogoHref = `${url.origin}/images/logo.png`;

	// Prefer org avatar → owner avatar → GitPier logo
	const rawAvatar = repoData?.repo?.org?.avatar_url ?? repoData?.repo?.owner?.avatar_url ?? null;
	const avatarHref = rawAvatar ? (rawAvatar.startsWith('http') ? rawAvatar : `${API_BASE}${rawAvatar}`) : null;
	const logoHref = avatarHref ?? fallbackLogoHref;

	const ownerLabel = repoData?.repo?.org?.login ?? repoData?.repo?.owner?.username ?? username;
	const description = repoData?.repo?.description?.trim() && repoData.repo.description.length > 0 ? repoData.repo.description : 'Source code, issues, commits, and releases on GitPier.';

	const svg = buildSvg({
		ownerName: ownerLabel,
		repoName: repoData?.repo?.name ?? repoName,
		description,
		stars: repoData?.repo?.star_count ?? 0,
		forks: repoData?.repo?.fork_count ?? 0,
		commits: repoData?.stats?.commits ?? 0,
		logoHref
	});

	try {
		const renderer = new Resvg(svg, {
			fitTo: {
				mode: 'width',
				value: CARD_WIDTH
			}
		});
		const png = renderer.render().asPng();
		return new Response(png, {
			headers: {
				'content-type': 'image/png',
				'cache-control': 'public, max-age=300, s-maxage=300'
			}
		});
	} catch {
		// Fallback to SVG so OG cards still have an image if PNG rasterization fails.
		return new Response(svg, {
			headers: {
				'content-type': 'image/svg+xml; charset=utf-8',
				'cache-control': 'public, max-age=300, s-maxage=300'
			}
		});
	}
};
