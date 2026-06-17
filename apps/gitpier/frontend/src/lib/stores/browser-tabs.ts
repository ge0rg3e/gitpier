import { browser } from '$app/environment';
import { get, writable } from 'svelte/store';

export type BrowserTabKind =
	| 'overview'
	| 'profile'
	| 'repo'
	| 'pull'
	| 'issue'
	| 'review'
	| 'settings'
	| 'org'
	| 'search'
	| 'generic';

export type BrowserTab = {
	id: string;
	key?: string;
	url: string;
	title: string;
	kind: BrowserTabKind;
	number?: number;
};

const STORAGE_KEY = 'gitpier.browser-tabs.v1';
const MAX_TABS = 14;

function loadTabs(): BrowserTab[] {
	if (!browser) return [];
	try {
		const raw = window.localStorage.getItem(STORAGE_KEY);
		if (!raw) return [];
		const parsed = JSON.parse(raw) as BrowserTab[];
		if (!Array.isArray(parsed)) return [];
		return parsed.filter((tab) => tab && typeof tab.url === 'string' && typeof tab.title === 'string').slice(-MAX_TABS);
	} catch {
		return [];
	}
}

export const browserTabs = writable<BrowserTab[]>(loadTabs());

browserTabs.subscribe((tabs) => {
	if (!browser) return;
	window.localStorage.setItem(STORAGE_KEY, JSON.stringify(tabs));
});

export function addOrUpdateBrowserTab(tab: BrowserTab) {
	browserTabs.update((tabs) => {
		const idx = tabs.findIndex((item) => {
			if (tab.key && item.key) return item.key === tab.key;
			return item.url === tab.url;
		});
		if (idx >= 0) {
			const next = [...tabs];
			next[idx] = { ...next[idx], ...tab, id: next[idx].id };
			return next;
		}
		return [...tabs, tab].slice(-MAX_TABS);
	});
}

export function closeBrowserTab(id: string): BrowserTab | undefined {
	const tabs = get(browserTabs);
	const index = tabs.findIndex((tab) => tab.id === id);
	if (index === -1) return undefined;
	const neighbor = tabs[index + 1] ?? tabs[index - 1];
	browserTabs.set(tabs.filter((tab) => tab.id !== id));
	return neighbor;
}

export function reorderBrowserTabs(fromId: string, toId: string) {
	if (fromId === toId) return;
	browserTabs.update((tabs) => {
		const fromIndex = tabs.findIndex((tab) => tab.id === fromId);
		const toIndex = tabs.findIndex((tab) => tab.id === toId);
		if (fromIndex < 0 || toIndex < 0 || fromIndex === toIndex) return tabs;

		const next = [...tabs];
		const [moved] = next.splice(fromIndex, 1);
		next.splice(toIndex, 0, moved);
		return next;
	});
}

export function clearBrowserTabs() {
	browserTabs.set([]);
}

export function getBrowserTabByUrl(url: string): BrowserTab | undefined {
	return get(browserTabs).find((tab) => tab.url === url);
}
