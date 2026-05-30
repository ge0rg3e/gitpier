type MentionUser = {
	username: string;
	avatar_url?: string | null;
};

type MentionOptions = {
	users: MentionUser[];
};

type MentionMatch = {
	start: number;
	end: number;
	query: string;
};

const MENU_ID = 'mention-autocomplete-menu';

function escapeHtml(value: string): string {
	return value.replace(/[&<>\"']/g, (ch) => {
		switch (ch) {
			case '&':
				return '&amp;';
			case '<':
				return '&lt;';
			case '>':
				return '&gt;';
			case '"':
				return '&quot;';
			default:
				return '&#39;';
		}
	});
}

function findMentionMatch(value: string, cursor: number): MentionMatch | null {
	if (cursor < 0 || cursor > value.length) return null;
	let at = -1;
	for (let i = cursor - 1; i >= 0; i -= 1) {
		const ch = value[i];
		if (ch === '@') {
			at = i;
			break;
		}
		if (/\s/.test(ch)) break;
	}
	if (at < 0) return null;
	if (at > 0 && /[A-Za-z0-9_-]/.test(value[at - 1])) return null;
	const query = value.slice(at + 1, cursor);
	if (!/^[A-Za-z0-9_-]*$/.test(query)) return null;
	return { start: at, end: cursor, query };
}

function getCaretCoordinates(textarea: HTMLTextAreaElement, position: number): { left: number; top: number } {
	const div = document.createElement('div');
	const style = window.getComputedStyle(textarea);
	const props = [
		'boxSizing',
		'width',
		'height',
		'overflowX',
		'overflowY',
		'borderTopWidth',
		'borderRightWidth',
		'borderBottomWidth',
		'borderLeftWidth',
		'paddingTop',
		'paddingRight',
		'paddingBottom',
		'paddingLeft',
		'fontStyle',
		'fontVariant',
		'fontWeight',
		'fontStretch',
		'fontSize',
		'lineHeight',
		'fontFamily',
		'textAlign',
		'textTransform',
		'textIndent',
		'textDecoration',
		'letterSpacing',
		'wordSpacing'
	] as const;

	for (const prop of props) {
		div.style[prop] = style[prop];
	}

	div.style.position = 'absolute';
	div.style.visibility = 'hidden';
	div.style.whiteSpace = 'pre-wrap';
	div.style.wordWrap = 'break-word';
	div.style.overflow = 'hidden';

	div.textContent = textarea.value.slice(0, position);
	const span = document.createElement('span');
	span.textContent = textarea.value.slice(position) || '.';
	div.appendChild(span);
	document.body.appendChild(div);

	const rect = textarea.getBoundingClientRect();
	const left = rect.left + span.offsetLeft - textarea.scrollLeft;
	const top = rect.top + span.offsetTop - textarea.scrollTop + Number.parseFloat(style.lineHeight || '16');

	document.body.removeChild(div);
	return { left, top };
}

export function mentionAutocomplete(textarea: HTMLTextAreaElement, options: MentionOptions) {
	let users = options.users ?? [];
	let match: MentionMatch | null = null;
	let filtered: MentionUser[] = [];
	let selectedIndex = 0;

	const menu = document.createElement('div');
	menu.id = MENU_ID;
	menu.style.position = 'fixed';
	menu.style.zIndex = '1200';
	menu.style.minWidth = '200px';
	menu.style.maxWidth = '280px';
	menu.style.maxHeight = '220px';
	menu.style.overflowY = 'auto';
	menu.style.display = 'none';
	menu.style.border = '1px solid var(--border)';
	menu.style.borderRadius = '8px';
	menu.style.background = 'var(--popover, #0d1117)';
	menu.style.color = 'var(--foreground, #e6edf3)';
	menu.style.boxShadow = '0 12px 28px rgba(0,0,0,0.35)';
	document.body.appendChild(menu);

	function closeMenu() {
		menu.style.display = 'none';
		menu.innerHTML = '';
		match = null;
		filtered = [];
		selectedIndex = 0;
	}

	function applySelection(user: MentionUser) {
		if (!match) return;
		const before = textarea.value.slice(0, match.start);
		const after = textarea.value.slice(match.end);
		const inserted = `@${user.username} `;
		const next = before + inserted + after;
		const cursor = before.length + inserted.length;
		textarea.value = next;
		textarea.setSelectionRange(cursor, cursor);
		textarea.dispatchEvent(new Event('input', { bubbles: true }));
		closeMenu();
	}

	function renderMenu() {
		if (!match || filtered.length === 0) {
			closeMenu();
			return;
		}
		const coords = getCaretCoordinates(textarea, match.end);
		menu.style.left = `${Math.max(8, Math.round(coords.left))}px`;
		menu.style.top = `${Math.round(coords.top + 6)}px`;
		menu.style.display = 'block';
		menu.innerHTML = filtered
			.map((user, i) => {
				const active = i === selectedIndex;
				return `<button type="button" data-mention-index="${i}" style="display:flex;align-items:center;gap:8px;width:100%;padding:9px 10px;border:0;text-align:left;background:${active ? 'var(--accent)' : 'var(--accent)'};color:${active ? 'var(--accent-foreground)' : 'var(--foreground, #e6edf3)'};cursor:pointer;">
					<span style="display:inline-flex;align-items:center;justify-content:center;width:20px;height:20px;border-radius:9999px;background:var(--secondary);font-size:11px;font-weight:700;overflow:hidden;">${user.avatar_url ? `<img src="${escapeHtml(user.avatar_url)}" alt="${escapeHtml(user.username)}" style="width:100%;height:100%;object-fit:cover;"/>` : escapeHtml(user.username.slice(0, 1).toUpperCase())}</span>
					<span style="font-size:13px;font-weight:600;">${escapeHtml(user.username)}</span>
				</button>`;
			})
			.join('');
	}

	function updateMenu() {
		const cursor = textarea.selectionStart ?? 0;
		const nextMatch = findMentionMatch(textarea.value, cursor);
		if (!nextMatch) {
			closeMenu();
			return;
		}
		const query = nextMatch.query.toLowerCase();
		const deduped = new Map<string, MentionUser>();
		for (const user of users) {
			if (!user?.username) continue;
			if (!deduped.has(user.username)) deduped.set(user.username, user);
		}
		filtered = Array.from(deduped.values())
			.filter((u) => u.username.toLowerCase().includes(query))
			.slice(0, 8);
		if (filtered.length === 0) {
			closeMenu();
			return;
		}
		match = nextMatch;
		selectedIndex = Math.min(selectedIndex, filtered.length - 1);
		renderMenu();
	}

	function onInput() {
		updateMenu();
	}

	function onClick() {
		updateMenu();
	}

	function onScroll() {
		if (menu.style.display === 'block') renderMenu();
	}

	function onKeyDown(e: KeyboardEvent) {
		if (menu.style.display !== 'block' || filtered.length === 0) return;
		if (e.key === 'ArrowDown') {
			e.preventDefault();
			selectedIndex = (selectedIndex + 1) % filtered.length;
			renderMenu();
			return;
		}
		if (e.key === 'ArrowUp') {
			e.preventDefault();
			selectedIndex = (selectedIndex - 1 + filtered.length) % filtered.length;
			renderMenu();
			return;
		}
		if (e.key === 'Enter' || e.key === 'Tab') {
			e.preventDefault();
			applySelection(filtered[selectedIndex]);
			return;
		}
		if (e.key === 'Escape') {
			e.preventDefault();
			closeMenu();
		}
	}

	function onMenuMouseDown(e: MouseEvent) {
		e.preventDefault();
	}

	function onMenuClick(e: MouseEvent) {
		const target = e.target as HTMLElement | null;
		const item = target?.closest('[data-mention-index]') as HTMLElement | null;
		if (!item) return;
		const index = Number(item.getAttribute('data-mention-index') ?? '-1');
		if (Number.isNaN(index) || !filtered[index]) return;
		applySelection(filtered[index]);
	}

	function onDocumentClick(e: MouseEvent) {
		const target = e.target as HTMLElement | null;
		if (target && (target === textarea || textarea.contains(target) || menu.contains(target))) return;
		closeMenu();
	}

	textarea.addEventListener('input', onInput);
	textarea.addEventListener('click', onClick);
	textarea.addEventListener('keydown', onKeyDown);
	textarea.addEventListener('scroll', onScroll);
	menu.addEventListener('mousedown', onMenuMouseDown);
	menu.addEventListener('click', onMenuClick);
	document.addEventListener('click', onDocumentClick);

	return {
		update(next: MentionOptions) {
			users = next.users ?? [];
			updateMenu();
		},
		destroy() {
			textarea.removeEventListener('input', onInput);
			textarea.removeEventListener('click', onClick);
			textarea.removeEventListener('keydown', onKeyDown);
			textarea.removeEventListener('scroll', onScroll);
			menu.removeEventListener('mousedown', onMenuMouseDown);
			menu.removeEventListener('click', onMenuClick);
			document.removeEventListener('click', onDocumentClick);
			menu.remove();
		}
	};
}
