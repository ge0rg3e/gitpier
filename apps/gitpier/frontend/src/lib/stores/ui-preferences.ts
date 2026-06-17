import { browser } from '$app/environment';
import { writable } from 'svelte/store';

const STORAGE_KEY = 'gitpier.ui-preferences.v1';

export type UiPreferences = {
	browserTabsEnabled: boolean;
};

const DEFAULT_PREFERENCES: UiPreferences = {
	browserTabsEnabled: false
};

function loadPreferences(): UiPreferences {
	if (!browser) return DEFAULT_PREFERENCES;
	try {
		const raw = window.localStorage.getItem(STORAGE_KEY);
		if (!raw) return DEFAULT_PREFERENCES;
		const parsed = JSON.parse(raw) as Partial<UiPreferences>;
		return {
			browserTabsEnabled: parsed.browserTabsEnabled === true
		};
	} catch {
		return DEFAULT_PREFERENCES;
	}
}

export const uiPreferences = writable<UiPreferences>(loadPreferences());

uiPreferences.subscribe((value) => {
	if (!browser) return;
	window.localStorage.setItem(STORAGE_KEY, JSON.stringify(value));
});

export function setBrowserTabsEnabled(enabled: boolean) {
	uiPreferences.update((prefs) => ({ ...prefs, browserTabsEnabled: enabled }));
}
