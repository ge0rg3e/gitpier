import { users, type User } from '$lib/api/client';
import { mediaUrl } from '$lib/utils';

type HoverProfile = {
	user: User;
};

const cache = new Map<string, HoverProfile | null>();

let cardEl: HTMLDivElement | null = null;
let hideTimer: ReturnType<typeof setTimeout> | null = null;

function ensureCard(): HTMLDivElement {
	if (cardEl) return cardEl;
	const el = document.createElement('div');
	el.style.position = 'fixed';
	el.style.zIndex = '1400';
	el.style.width = '320px';
	el.style.display = 'none';
	el.style.borderRadius = '10px';
	el.style.border = '1px solid var(--border, #30363d)';
	el.style.background = 'var(--popover, #0d1117)';
	el.style.color = 'var(--foreground, #e6edf3)';
	el.style.boxShadow = '0 14px 36px rgba(0,0,0,0.38)';
	el.style.padding = '14px';
	el.addEventListener('mouseenter', () => {
		if (hideTimer) clearTimeout(hideTimer);
	});
	el.addEventListener('mouseleave', () => hideCardSoon());
	document.body.appendChild(el);
	cardEl = el;
	return el;
}

function hideCardSoon() {
	if (hideTimer) clearTimeout(hideTimer);
	hideTimer = setTimeout(() => {
		if (cardEl) cardEl.style.display = 'none';
	}, 120);
}

function formatJoinDate(iso: string): string {
	const d = new Date(iso);
	return d.toLocaleDateString('en-US', { month: 'long', year: 'numeric' });
}

function renderCard(profile: HoverProfile | null, username: string): string {
	if (!profile?.user) {
		return `
			<div style="font-size:13px;line-height:1.4;">
				<div style="font-weight:700;color:var(--foreground, #e6edf3);">@${username}</div>
				<div style="margin-top:4px;color:var(--muted-foreground, #9aa4b2);">Profile unavailable</div>
			</div>
		`;
	}
	const user = profile.user;
	const avatar = user.avatar_url
		? `<img src="${mediaUrl(user.avatar_url)}" alt="${user.username}" style="width:40px;height:40px;border-radius:9999px;object-fit:cover;" />`
		: `<div style="width:40px;height:40px;border-radius:9999px;background:var(--secondary);display:flex;align-items:center;justify-content:center;font-size:14px;font-weight:700;">${user.username[0].toUpperCase()}</div>`;
	const display = user.display_name?.trim() || user.username;
	const bio = user.bio?.trim() ? `<p style="margin:8px 0 0;font-size:13px;line-height:1.45;color:var(--foreground, #e6edf3);opacity:0.92;">${user.bio}</p>` : '';
	return `
		<div style="display:flex;gap:10px;align-items:flex-start;">
			${avatar}
			<div style="min-width:0;flex:1;">
				<div style="font-size:14px;font-weight:700;line-height:1.2;color:var(--foreground, #e6edf3);">${display}</div>
				<div style="margin-top:2px;font-size:12px;color:var(--primary, #58a6ff);font-weight:700;">@${user.username}</div>
			</div>
		</div>
		${bio}
		<div style="margin-top:10px;font-size:12px;color:var(--muted-foreground, #9aa4b2);">Joined ${formatJoinDate(user.created_at)}</div>
	`;
}

function positionCard(anchor: HTMLElement, card: HTMLDivElement) {
	const rect = anchor.getBoundingClientRect();
	const top = rect.bottom + 8;
	const left = Math.min(window.innerWidth - 340, Math.max(8, rect.left));
	card.style.top = `${Math.round(top)}px`;
	card.style.left = `${Math.round(left)}px`;
}

async function getProfile(username: string): Promise<HoverProfile | null> {
	if (cache.has(username)) return cache.get(username) ?? null;
	try {
		const data = await users.getProfile(username);
		const profile = { user: data.user };
		cache.set(username, profile);
		return profile;
	} catch {
		cache.set(username, null);
		return null;
	}
}

export function mentionHoverCard(node: HTMLElement) {
	const card = ensureCard();

	async function onMouseEnter(event: MouseEvent) {
		const target = event.target as HTMLElement | null;
		const anchor = target?.closest('a[data-mention="true"]') as HTMLAnchorElement | null;
		if (!anchor) return;
		if (hideTimer) clearTimeout(hideTimer);
		const username = (anchor.textContent ?? '').replace('@', '').trim();
		if (!username) return;
		positionCard(anchor, card);
		card.innerHTML = renderCard(null, username);
		card.style.display = 'block';
		const profile = await getProfile(username);
		card.innerHTML = renderCard(profile, username);
		positionCard(anchor, card);
	}

	function onMouseLeave(event: MouseEvent) {
		const target = event.target as HTMLElement | null;
		const anchor = target?.closest('a[data-mention="true"]') as HTMLAnchorElement | null;
		if (!anchor) return;
		hideCardSoon();
	}

	node.addEventListener('mouseover', onMouseEnter);
	node.addEventListener('mouseout', onMouseLeave);

	return {
		destroy() {
			node.removeEventListener('mouseover', onMouseEnter);
			node.removeEventListener('mouseout', onMouseLeave);
		}
	};
}
