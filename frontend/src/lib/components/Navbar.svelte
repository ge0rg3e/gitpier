<script lang="ts">
	import { browser } from '$app/environment';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { tick } from 'svelte';
	import { fade, fly } from 'svelte/transition';
	import { authStore } from '$lib/stores/auth.svelte';
	import {
		Plus,
		Search,
		Settings,
		LogOut,
		User,
		ChevronDown,
		BookOpen,
		Building2,
		Code,
		File,
		MessageSquare,
		Home,
		Minus,
		Square,
		X,
		GitPullRequest,
		CircleDot,
		Archive,
		Zap,
		Tag,
		Users
	} from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Command from '$lib/components/ui/command/index.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import { mediaUrl } from '$lib/utils';
	import { feedback, search, orgs, type Repository, type User as ApiUser, type Organization } from '$lib/api/client';
	import { addOrUpdateBrowserTab, browserTabs, closeBrowserTab, reorderBrowserTabs, type BrowserTab, type BrowserTabKind } from '$lib/stores/browser-tabs';
	import { setBrowserTabsEnabled, uiPreferences } from '$lib/stores/ui-preferences';

	let searchQuery = $state('');
	let showPlusMenu = $state(false);
	let showUserMenu = $state(false);
	let searchDialogOpen = $state(false);
	let searchInputEl = $state<HTMLInputElement | null>(null);
	let isElectronApp = $state(false);
	let isElectronWindowMaximized = $state(false);

	// Live search state
	let liveRepos = $state<Repository[]>([]);
	let liveUsers = $state<ApiUser[]>([]);
	let liveOrgs = $state<Organization[]>([]);
	let searchLoading = $state(false);
	let searchDebounce: ReturnType<typeof setTimeout> | null = null;

	// Detect if we're inside a repo context
	const repoContext = $derived.by(() => {
		const p = page.params;
		if (p.username && p.repo) return { owner: p.username, repo: p.repo };
		return null;
	});
	const profileContext = $derived.by(() => {
		const p = page.params;
		if (p.username && !p.repo) return { username: p.username };
		return null;
	});
	const orgProfileCache = new Map<string, boolean>();
	let profileIsOrg = $state<boolean | null>(null);

	$effect(() => {
		if (!profileContext) {
			profileIsOrg = null;
			return;
		}

		const handle = profileContext.username;
		const cached = orgProfileCache.get(handle);
		if (cached !== undefined) {
			profileIsOrg = cached;
			return;
		}

		profileIsOrg = null;
		let cancelled = false;
		orgs
			.get(handle)
			.then(() => {
				if (cancelled) return;
				orgProfileCache.set(handle, true);
				profileIsOrg = true;
			})
			.catch(() => {
				if (cancelled) return;
				orgProfileCache.set(handle, false);
				profileIsOrg = false;
			});

		return () => {
			cancelled = true;
		};
	});

	type TopNavItem = {
		label: string;
		href: string;
		icon: typeof Home;
		active: (url: URL) => boolean;
	};

	const topNavItems = $derived.by<TopNavItem[]>(() => {
		const path = page.url.pathname;
		const hiddenTopNavPaths = ['/', '/no-tabs', '/new', '/orgs', '/orgs/new', '/login', '/signup'];
		const isSettingsRoute = path === '/settings' || path.startsWith('/settings/') || /\/settings(?:\/|$)/.test(path);
		const isAdminRoute = path === '/admin' || path.startsWith('/admin/');
		const isOrgProfileRoute = !!profileContext && profileIsOrg === true;
		if (hiddenTopNavPaths.includes(path) || isAdminRoute || (isSettingsRoute && !repoContext && !isOrgProfileRoute)) {
			return [];
		}
		if (repoContext) {
			const base = `/${repoContext.owner}/${repoContext.repo}`;
			return [
				{
					label: 'Code',
					href: base,
					icon: Archive,
					active: (url) => url.pathname === base || url.pathname.startsWith(`${base}/tree`) || url.pathname.startsWith(`${base}/blob`)
				},
				{ label: 'Issues', href: `${base}/issues`, icon: CircleDot, active: (url) => url.pathname.startsWith(`${base}/issues`) },
				{ label: 'Pull Requests', href: `${base}/pulls`, icon: GitPullRequest, active: (url) => url.pathname.startsWith(`${base}/pulls`) },
				{ label: 'Actions', href: `${base}/actions`, icon: Zap, active: (url) => url.pathname.startsWith(`${base}/actions`) },
				{ label: 'Releases', href: `${base}/releases`, icon: Tag, active: (url) => url.pathname.startsWith(`${base}/releases`) }
			];
		}
		if (profileContext) {
			if (!profileIsOrg) {
				return [];
			}
			const base = `/${profileContext.username}`;
			return [
				{ label: 'Overview', href: base, icon: Home, active: (url) => url.pathname === base },
				{ label: 'People', href: `${base}/people`, icon: Users, active: (url) => url.pathname.startsWith(`${base}/people`) },
				{ label: 'Settings', href: `${base}/settings`, icon: Settings, active: (url) => url.pathname.startsWith(`${base}/settings`) }
			];
		}

		const user = authStore.user?.username;
		return [
			{ label: 'Overview', href: '/', icon: Home, active: (url) => url.pathname === '/' },
			{
				label: 'Repositories',
				href: user ? `/${user}` : '/search?type=repos',
				icon: Archive,
				active: (url) => (user ? url.pathname === `/${user}` : url.pathname === '/search' && (url.searchParams.get('type') ?? '') === 'repos')
			}
		];
	});

	let tabsScrollEl = $state<HTMLDivElement | null>(null);
	let canScrollTabsLeft = $state(false);
	let canScrollTabsRight = $state(false);
	let tabsViewportWidth = $state(0);
	let draggingTabId = $state<string | null>(null);
	let dragOverTabId = $state<string | null>(null);

	function updateTabScrollState() {
		if (!tabsScrollEl) {
			canScrollTabsLeft = false;
			canScrollTabsRight = false;
			tabsViewportWidth = 0;
			return;
		}
		tabsViewportWidth = tabsScrollEl.clientWidth;
		canScrollTabsLeft = tabsScrollEl.scrollLeft > 0;
		canScrollTabsRight = tabsScrollEl.scrollLeft + tabsScrollEl.clientWidth < tabsScrollEl.scrollWidth - 1;
	}

	function pathToTab(urlPath: string, query: URLSearchParams): Omit<BrowserTab, 'id'> {
		const parts = urlPath.split('/').filter(Boolean);
		const second = parts[1];
		const third = parts[2];
		const ref = query.get('ref')?.trim() ?? '';

		const isSystem = ['auth', 'legal', 'settings', 'search', 'new', 'orgs', 'apps', 'login'].includes(parts[0] ?? '');
		const isUserSection = ['repos', 'starred', 'packages', 'people', 'settings', 'projects'].includes(second ?? '');

		let title = 'Page';
		let kind: BrowserTabKind = 'generic';
		let number: number | undefined;

		if (parts.length === 0) {
			title = 'Overview';
			kind = 'overview';
		} else if (parts[0] === 'search') {
			title = query.get('q')?.trim() ? `Search: ${query.get('q')}` : 'Search';
			kind = 'search';
		} else if (parts[0] === 'settings') {
			title = `Settings${parts[1] ? `/${parts[1]}` : ''}`;
			kind = 'settings';
			const url = `${urlPath}${query.toString() ? `?${query.toString()}` : ''}`;
			return { key: 'account-settings', url, title, kind, number };
		} else if (parts[0] === 'orgs') {
			title = parts[1] ? `Org: ${parts[1]}` : 'Organizations';
			kind = 'org';
		} else if (parts[0] === 'login') {
			title = 'Sign in';
			kind = 'generic';
		} else if (parts[0] === 'signup') {
			title = 'Sign up';
			kind = 'generic';
		} else if (!isSystem && parts.length >= 2 && !isUserSection) {
			const owner = parts[0];
			const repo = parts[1];
			const isRepoCodeRoute = !third || third === 'tree' || third === 'blob';
			title = `${owner}/${repo}${ref ? ` (${ref})` : ''}`;
			kind = 'repo';
			if (third === 'issues') {
				kind = 'issue';
				if (parts[3] && /^\d+$/.test(parts[3])) {
					number = Number(parts[3]);
					title = `${owner}/${repo} #${parts[3]}`;
				} else {
					title = `${owner}/${repo} issues`;
				}
			} else if (third === 'pulls') {
				kind = 'pull';
				if (parts[3] && /^\d+$/.test(parts[3])) {
					number = Number(parts[3]);
					title = `${owner}/${repo} #${parts[3]}`;
				} else {
					title = `${owner}/${repo} pulls`;
				}
			}
			if (isRepoCodeRoute) {
				const branchKey = ref || '__default__';
				const key = `repo-code:${owner}/${repo}:${branchKey}`;
				const url = `${urlPath}${query.toString() ? `?${query.toString()}` : ''}`;
				return { key, url, title, kind, number };
			}
		} else {
			if (parts[1] === 'repos') title = `${parts[0]} repos`;
			else if (parts[1] === 'projects') title = `${parts[0]} projects`;
			else if (parts[1] === 'starred') title = `${parts[0]} stars`;
			else if (parts[1] === 'packages') title = `${parts[0]} packages`;
			else title = `@${parts[0]}`;
			kind = 'profile';
		}

		const url = `${urlPath}${query.toString() ? `?${query.toString()}` : ''}`;
		return { url, title, kind, number };
	}

	const currentTabUrl = $derived(`${page.url.pathname}${page.url.search || ''}`);
	const openBrowserTabs = $derived($browserTabs);
	const browserTabsEnabled = $derived($uiPreferences.browserTabsEnabled);
	const electronTabsEnabled = $derived(isElectronApp && browserTabsEnabled);
	const browserTabWidthPx = $derived.by(() => {
		const count = Math.max(1, openBrowserTabs.length);
		const available = Math.max(0, tabsViewportWidth - 12);
		const ideal = available > 0 ? Math.floor(available / count) - 2 : 176;
		return Math.max(64, Math.min(210, ideal));
	});
	const compactTabs = $derived(browserTabWidthPx < 118);
	const veryCompactTabs = $derived(browserTabWidthPx < 90);

	$effect(() => {
		if (!electronTabsEnabled) return;
		if (page.url.pathname === '/no-tabs') return;
		const tab = pathToTab(page.url.pathname, page.url.searchParams);
		addOrUpdateBrowserTab({
			id: crypto.randomUUID(),
			...tab
		});
	});

	$effect(() => {
		if (!electronTabsEnabled) {
			canScrollTabsLeft = false;
			canScrollTabsRight = false;
			tabsViewportWidth = 0;
			return;
		}
		openBrowserTabs.length;
		tick().then(updateTabScrollState);
	});

	$effect(() => {
		if (!electronTabsEnabled) return;
		if (!tabsScrollEl) return;
		const observer = new ResizeObserver(() => updateTabScrollState());
		observer.observe(tabsScrollEl);
		updateTabScrollState();
		return () => observer.disconnect();
	});

	function isTabActive(tab: BrowserTab): boolean {
		return tab.url === currentTabUrl;
	}

	function tabIconForKind(kind: BrowserTabKind) {
		if (kind === 'overview') return Home;
		if (kind === 'repo') return Archive;
		if (kind === 'pull') return GitPullRequest;
		if (kind === 'issue') return CircleDot;
		if (kind === 'settings') return Settings;
		if (kind === 'search') return Search;
		if (kind === 'profile') return User;
		if (kind === 'org') return Building2;
		return File;
	}

	function closeTabById(id: string) {
		const tab = openBrowserTabs.find((entry) => entry.id === id);
		if (!tab) return;
		const next = closeBrowserTab(id);
		if (isTabActive(tab) && next) {
			goto(next.url);
		}
		if (isTabActive(tab) && !next) {
			goto('/no-tabs');
		}
	}

	function closeTab(id: string, event: MouseEvent) {
		event.preventDefault();
		event.stopPropagation();
		closeTabById(id);
	}

	function handleTabAuxClick(id: string, event: MouseEvent) {
		if (event.button !== 1) return;
		event.preventDefault();
		event.stopPropagation();
		closeTabById(id);
	}

	function handleTabDragStart(id: string, event: DragEvent) {
		draggingTabId = id;
		dragOverTabId = null;
		if (event.dataTransfer) {
			event.dataTransfer.effectAllowed = 'move';
			event.dataTransfer.setData('text/plain', id);
		}
	}

	function handleTabDragOver(id: string, event: DragEvent) {
		if (!draggingTabId || draggingTabId === id) return;
		event.preventDefault();
		if (event.dataTransfer) {
			event.dataTransfer.dropEffect = 'move';
		}
		dragOverTabId = id;
	}

	function handleTabDrop(id: string, event: DragEvent) {
		event.preventDefault();
		const fromId = draggingTabId ?? event.dataTransfer?.getData('text/plain') ?? null;
		if (!fromId || fromId === id) {
			draggingTabId = null;
			dragOverTabId = null;
			return;
		}
		reorderBrowserTabs(fromId, id);
		draggingTabId = null;
		dragOverTabId = null;
	}

	function handleTabDragEnd() {
		draggingTabId = null;
		dragOverTabId = null;
	}

	function extractRepoScope(input: string): string {
		const match = input.match(/(?:^|\s)repo:([^\s]+)/i);
		const candidate = match?.[1]?.trim() ?? '';
		return candidate.includes('/') ? candidate : '';
	}

	function extractSearchTerms(input: string): string {
		return input.replace(/(?:^|\s)repo:[^\s]+/gi, ' ').trim();
	}

	function handleSearch() {
		if (!searchQuery.trim()) return;
		const repoScope = extractRepoScope(searchQuery);
		const terms = extractSearchTerms(searchQuery);
		if (!terms) return;

		const params: Record<string, string> = { q: terms };
		if (repoScope) {
			params.repo = repoScope;
			params.type = 'code';
		}
		searchDialogOpen = false;
		goto(`/search?${new URLSearchParams(params)}`);
	}

	function handleLogout() {
		authStore.logout();
		showUserMenu = false;
		goto('/');
	}

	async function runLiveSearch(q: string) {
		if (q.trim().length < 2) {
			liveRepos = [];
			liveUsers = [];
			liveOrgs = [];
			return;
		}
		searchLoading = true;
		try {
			const [repoRes, userRes, orgRes] = await Promise.all([
				search.repos(q, 5, 0).catch(() => ({ items: [], total: 0 })),
				search.users(q, 4, 0).catch(() => ({ items: [], total: 0 })),
				search.orgs(q, 3, 0).catch(() => ({ items: [], total: 0 }))
			]);
			liveRepos = repoRes.items;
			liveUsers = userRes.items;
			liveOrgs = orgRes.items;
		} finally {
			searchLoading = false;
		}
	}

	$effect(() => {
		if (!searchDialogOpen) return;
		void tick().then(() => searchInputEl?.focus());
	});

	$effect(() => {
		if (searchDialogOpen) return;
		searchQuery = '';
		liveRepos = [];
		liveUsers = [];
		liveOrgs = [];
	});

	$effect(() => {
		if (!searchDialogOpen) return;
		if (searchDebounce) clearTimeout(searchDebounce);
		searchDebounce = setTimeout(() => runLiveSearch(extractSearchTerms(searchQuery)), 200);
		return () => {
			if (searchDebounce) clearTimeout(searchDebounce);
		};
	});

	function handleKeydown(e: KeyboardEvent) {
		// Press / to focus search (when not in an input)
		if (e.key === '/' && document.activeElement?.tagName !== 'INPUT' && document.activeElement?.tagName !== 'TEXTAREA') {
			e.preventDefault();
			searchDialogOpen = true;
		}
		if (e.key === 'Escape' && searchDialogOpen) {
			searchDialogOpen = false;
		}
	}

	function minimizeElectronWindow() {
		void window.electronWindowControls?.minimize().catch(() => undefined);
	}

	function toggleElectronWindowMaximize() {
		void window.electronWindowControls
			?.toggleMaximize()
			.then((maximized) => {
				isElectronWindowMaximized = maximized;
			})
			.catch(() => undefined);
	}

	function closeElectronWindow() {
		void window.electronWindowControls?.close().catch(() => undefined);
	}

	const hasLiveResults = $derived(liveRepos.length > 0 || liveUsers.length > 0 || liveOrgs.length > 0);
	const liveSearchTerms = $derived(extractSearchTerms(searchQuery));

	// Feedback dialog
	let feedbackOpen = $state(false);
	let feedbackText = $state('');
	let feedbackCategory = $state<'bug' | 'feature' | 'other'>('bug');
	let feedbackSent = $state(false);
	let feedbackLoading = $state(false);
	let feedbackError = $state('');

	function openFeedback() {
		feedbackOpen = true;
		feedbackSent = false;
		feedbackError = '';
		feedbackText = '';
		feedbackCategory = 'bug';
	}

	function closeFeedback() {
		feedbackOpen = false;
	}

	async function submitFeedback(e: Event) {
		e.preventDefault();
		if (!feedbackText.trim()) return;
		feedbackLoading = true;
		feedbackError = '';
		try {
			await feedback.submit(feedbackCategory, feedbackText.trim());
			feedbackSent = true;
		} catch (err: any) {
			feedbackError = err?.message ?? 'Failed to send feedback.';
		} finally {
			feedbackLoading = false;
		}
	}

	// Listen for open-feedback event from the alpha banner
	$effect(() => {
		const handler = () => openFeedback();
		document.addEventListener('open-feedback', handler);
		return () => document.removeEventListener('open-feedback', handler);
	});

	$effect(() => {
		if (!browser) return;
		const controls = window.electronWindowControls;
		if (!controls) return;
		isElectronApp = true;
		if (!browserTabsEnabled) {
			setBrowserTabsEnabled(true);
		}

		void controls
			.isMaximized()
			.then((maximized) => {
				isElectronWindowMaximized = maximized;
			})
			.catch(() => undefined);

		const unsubscribe = controls.onWindowStateChange((state) => {
			isElectronWindowMaximized = state.maximized;
		});

		return () => unsubscribe();
	});
</script>

<svelte:window
	onkeydown={handleKeydown}
	onclick={(e) => {
		if (!(e.target as HTMLElement).closest('.user-menu-container')) showUserMenu = false;
		if (!(e.target as HTMLElement).closest('.plus-menu-container')) showPlusMenu = false;
	}}
/>

<header class="sticky top-0 z-50 border-b border-border/80 bg-card/95 backdrop-blur-sm" class:electron-app={isElectronApp}>
	<div class="flex h-14 items-center gap-2 sm:gap-3 {isElectronApp ? 'w-full px-4' : 'mx-auto max-w-screen-2xl px-3 sm:px-4'}">
		<a href="/" class="mr-0.5 flex shrink-0 items-center sm:mr-1" aria-label="GitPier home">
			<img src="/images/logo.png" alt="GitPier" class="h-7 w-7 object-contain sm:h-8 sm:w-8" />
		</a>

		{#if topNavItems.length > 0}
			<div class="min-w-0 flex-1 items-center overflow-hidden md:flex-none" in:fly={{ x: -10, duration: 220, opacity: 0.2 }} out:fade={{ duration: 150 }}>
				<nav class="no-scrollbar flex min-w-0 flex-1 items-center gap-0.5 overflow-x-auto whitespace-nowrap md:min-w-max md:flex-none md:overflow-visible">
					{#each topNavItems as item}
						{@const NavIcon = item.icon}
						<a
							href={item.href}
							class="flex h-8 shrink-0 items-center gap-1.5 rounded-md px-3 text-[13px] font-medium text-muted-foreground transition-colors hover:bg-secondary/80 hover:text-foreground"
							class:bg-secondary={item.active(page.url)}
							class:text-foreground={item.active(page.url)}
						>
							<NavIcon class="h-3.5 w-3.5" />
							<span>{item.label}</span>
						</a>
					{/each}
				</nav>

				{#if electronTabsEnabled && openBrowserTabs.length > 0}
					<div class="mx-2 h-4 shrink-0 border-l border-border/60"></div>
				{/if}
			</div>
		{/if}

		{#if electronTabsEnabled}
			<div class="relative hidden min-w-0 flex-1 overflow-hidden md:block">
				<div
					class="pointer-events-none absolute inset-y-0 left-0 z-10 w-6 bg-gradient-to-r from-card to-transparent transition-opacity"
					class:opacity-100={canScrollTabsLeft}
					class:opacity-0={!canScrollTabsLeft}
				></div>
				<div
					class="pointer-events-none absolute inset-y-0 right-0 z-10 w-6 bg-gradient-to-l from-card to-transparent transition-opacity"
					class:opacity-100={canScrollTabsRight}
					class:opacity-0={!canScrollTabsRight}
				></div>
				<div
					bind:this={tabsScrollEl}
					onwheel={(event) => {
						if (!tabsScrollEl || tabsScrollEl.scrollWidth <= tabsScrollEl.clientWidth || event.deltaY === 0) return;
						event.preventDefault();
						tabsScrollEl.scrollLeft += event.deltaY;
						updateTabScrollState();
					}}
					onscroll={updateTabScrollState}
					class="no-scrollbar flex min-w-full items-center gap-0.5 overflow-x-auto"
				>
					{#each openBrowserTabs as tab (tab.id)}
						{@const TabIcon = tabIconForKind(tab.kind)}
						<a
							href={tab.url}
							title={tab.title}
							class="group relative flex h-8 shrink-0 cursor-grab items-center rounded-md text-[13px] font-medium text-muted-foreground transition-[colors,opacity] hover:bg-secondary hover:text-foreground active:cursor-grabbing"
							class:gap-1={compactTabs}
							class:gap-2={!compactTabs}
							class:px-2={compactTabs}
							class:px-3={!compactTabs}
							class:bg-secondary={isTabActive(tab)}
							class:text-foreground={isTabActive(tab)}
							class:opacity-60={draggingTabId === tab.id}
							class:ring-1={dragOverTabId === tab.id}
							class:ring-primary={dragOverTabId === tab.id}
							style={`width: ${browserTabWidthPx}px; min-width: 64px; max-width: 210px;`}
							draggable={true}
							ondragstart={(event) => handleTabDragStart(tab.id, event)}
							ondragover={(event) => handleTabDragOver(tab.id, event)}
							ondrop={(event) => handleTabDrop(tab.id, event)}
							ondragleave={() => {
								if (dragOverTabId === tab.id) dragOverTabId = null;
							}}
							ondragend={handleTabDragEnd}
							onauxclick={(event) => handleTabAuxClick(tab.id, event)}
						>
							<TabIcon class="h-3.5 w-3.5 shrink-0" />
							{#if !veryCompactTabs}
								<span class="min-w-0 flex-1 truncate">{tab.title}</span>
							{/if}
							{#if tab.number !== undefined && !veryCompactTabs}
								<span class="text-[11px] tabular-nums text-muted-foreground">#{tab.number}</span>
							{/if}
							<button
								type="button"
								class="-mr-1 flex h-6 w-6 shrink-0 items-center justify-center rounded-md text-muted-foreground transition-colors hover:bg-background/70 hover:text-foreground md:opacity-0 md:group-hover:opacity-100"
								aria-label={`Close ${tab.title}`}
								onclick={(event) => closeTab(tab.id, event)}
							>
								<X class="h-3 w-3" />
							</button>
						</a>
					{/each}
				</div>
			</div>
		{/if}

		<div class="search-container relative ml-auto shrink-0">
			<Button variant="ghost" size="icon" onclick={() => (searchDialogOpen = true)} aria-label="Open search">
				<Search />
			</Button>

			<Command.Dialog bind:open={searchDialogOpen} shouldFilter={false}>
				<Command.Input bind:ref={searchInputEl} bind:value={searchQuery} placeholder="Type to search..." />
				<Command.List>
					<Command.Group heading="Actions">
						{#if repoContext}
							<Command.LinkItem
								href={`/search?${new URLSearchParams({
									q: liveSearchTerms,
									repo: `${repoContext.owner}/${repoContext.repo}`,
									type: 'code'
								})}`}
								onclick={() => (searchDialogOpen = false)}
								value={`code:${liveSearchTerms}`}
							>
								<Code class="h-4 w-4" />
								Search code in {repoContext.owner}/{repoContext.repo} for <strong>{liveSearchTerms}</strong>
							</Command.LinkItem>
							<Command.LinkItem
								href={`/search?${new URLSearchParams({
									q: liveSearchTerms,
									repo: `${repoContext.owner}/${repoContext.repo}`,
									type: 'files'
								})}`}
								onclick={() => (searchDialogOpen = false)}
								value={`files:${liveSearchTerms}`}
							>
								<File class="h-4 w-4" />
								Search files in {repoContext.owner}/{repoContext.repo} for <strong>{liveSearchTerms}</strong>
							</Command.LinkItem>
						{/if}
						<Command.Item onclick={handleSearch} value={`search:${liveSearchTerms}`}>
							<Search class="h-4 w-4" />
							Search for <strong>{liveSearchTerms}</strong>
						</Command.Item>
						<Command.LinkItem href={`/search?${new URLSearchParams({ q: liveSearchTerms, type: 'users' })}`} onclick={() => (searchDialogOpen = false)} value={`users:${liveSearchTerms}`}>
							<User class="h-4 w-4" />
							Search users for <strong>{liveSearchTerms}</strong>
						</Command.LinkItem>
						<Command.LinkItem href={`/search?${new URLSearchParams({ q: liveSearchTerms, type: 'orgs' })}`} onclick={() => (searchDialogOpen = false)} value={`orgs:${liveSearchTerms}`}>
							<Building2 class="h-4 w-4" />
							Search orgs for <strong>{liveSearchTerms}</strong>
						</Command.LinkItem>
					</Command.Group>

					{#if liveRepos.length > 0}
						<Command.Group heading="Repositories">
							{#each liveRepos as repo}
								<Command.LinkItem href="/{repo.owner.username}/{repo.name}" onclick={() => (searchDialogOpen = false)} value={`repo:${repo.owner.username}/${repo.name}`}>
									<BookOpen class="h-4 w-4 text-muted-foreground" />
									<span class="truncate"><span class="text-muted-foreground">{repo.owner.username}/</span>{repo.name}</span>
									{#if repo.is_private}<span class="ml-auto text-xs text-muted-foreground">Private</span>{/if}
								</Command.LinkItem>
							{/each}
						</Command.Group>
					{/if}

					{#if liveUsers.length > 0}
						<Command.Group heading="Users">
							{#each liveUsers as u}
								<Command.LinkItem href="/{u.username}" onclick={() => (searchDialogOpen = false)} value={`user:${u.username}`}>
									<div class="h-5 w-5 rounded-full bg-secondary border border-border flex items-center justify-center overflow-hidden shrink-0">
										{#if u.avatar_url}
											<img src={mediaUrl(u.avatar_url)} alt={u.username} class="h-full w-full object-cover" />
										{:else}
											<span class="text-[9px] font-semibold">{u.username[0].toUpperCase()}</span>
										{/if}
									</div>
									<span class="truncate">{u.username}</span>
									{#if u.display_name}<span class="text-xs text-muted-foreground truncate">{u.display_name}</span>{/if}
								</Command.LinkItem>
							{/each}
						</Command.Group>
					{/if}

					{#if liveOrgs.length > 0}
						<Command.Group heading="Organizations">
							{#each liveOrgs as org}
								<Command.LinkItem href="/{org.login}" onclick={() => (searchDialogOpen = false)} value={`org:${org.login}`}>
									<div class="h-5 w-5 rounded-full bg-secondary border border-border flex items-center justify-center overflow-hidden shrink-0">
										{#if org.avatar_url}
											<img src={mediaUrl(org.avatar_url)} alt={org.login} class="h-full w-full object-cover" />
										{:else}
											<span class="text-[9px] font-semibold">{org.login[0].toUpperCase()}</span>
										{/if}
									</div>
									<span class="truncate">{org.login}</span>
								</Command.LinkItem>
							{/each}
						</Command.Group>
					{/if}
				</Command.List>
			</Command.Dialog>
		</div>

		<div class="flex items-center gap-0.5 sm:gap-1">
			{#if authStore.isAuthenticated && authStore.user}
				<div class="plus-menu-container relative">
					<button
						onclick={() => {
							showPlusMenu = !showPlusMenu;
							showUserMenu = false;
						}}
						class="flex h-8 items-center gap-0.5 rounded-md px-1.5 text-foreground transition-colors hover:bg-secondary sm:px-2"
						aria-label="Create new"
					>
						<Plus class="h-4 w-4" />
						<ChevronDown class="hidden h-3 w-3 text-muted-foreground sm:block" />
					</button>

					{#if showPlusMenu}
						<div class="absolute right-0 top-full mt-1 w-52 rounded-md border border-border bg-card shadow-xl py-1 z-50">
							<a href="/new" onclick={() => (showPlusMenu = false)} class="flex items-center gap-2 px-4 py-1.5 text-sm text-foreground hover:bg-brand transition-colors">
								<BookOpen class="h-4 w-4" />
								New repository
							</a>
							<a href="/orgs/new" onclick={() => (showPlusMenu = false)} class="flex items-center gap-2 px-4 py-1.5 text-sm text-foreground hover:bg-brand transition-colors">
								<Building2 class="h-4 w-4" />
								New organization
							</a>
						</div>
					{/if}
				</div>

				<div class="user-menu-container relative">
					<button
						onclick={() => {
							showUserMenu = !showUserMenu;
							showPlusMenu = false;
						}}
						class="flex h-8 items-center gap-1 rounded-full px-1 hover:opacity-80 transition-opacity"
						aria-label="User menu"
					>
						<div class="flex h-6.5 w-6.5 items-center justify-center overflow-hidden rounded-full border border-border bg-secondary">
							{#if authStore.user.avatar_url}
								<img src={mediaUrl(authStore.user.avatar_url)} alt={authStore.user.username} class="h-full w-full object-cover" />
							{:else}
								<span class="text-xs font-semibold text-primary">{authStore.user.username[0].toUpperCase()}</span>
							{/if}
						</div>
						<ChevronDown class="h-3 w-3 text-muted-foreground" />
					</button>

					{#if showUserMenu}
						<div class="absolute right-0 top-full mt-1 w-56 rounded-md border border-border bg-card shadow-xl py-1 z-50">
							<div class="px-4 py-2 border-b border-secondary">
								<p class="text-xs text-muted-foreground">Signed in as</p>
								<p class="text-sm font-bold text-foreground truncate">{authStore.user.username}</p>
							</div>

							<a
								href="/{authStore.user.username}"
								onclick={() => (showUserMenu = false)}
								class="flex items-center gap-2 px-4 py-1.5 text-sm text-foreground hover:bg-brand transition-colors"
							>
								<User class="h-3.5 w-3.5" />
								Your profile
							</a>
							<a href="/orgs" onclick={() => (showUserMenu = false)} class="flex items-center gap-2 px-4 py-1.5 text-sm text-foreground hover:bg-brand transition-colors">
								<Building2 class="h-3.5 w-3.5" />
								Your organizations
							</a>

							<div class="border-t border-secondary my-1"></div>

							<a href="/settings/profile" onclick={() => (showUserMenu = false)} class="flex items-center gap-2 px-4 py-1.5 text-sm text-foreground hover:bg-brand transition-colors">
								<Settings class="h-3.5 w-3.5" />
								Settings
							</a>
							<button onclick={openFeedback} class="flex w-full items-center gap-2 px-4 py-1.5 text-sm text-foreground hover:bg-brand transition-colors">
								<MessageSquare class="h-3.5 w-3.5" />
								Feedback
							</button>

							<div class="border-t border-secondary my-1"></div>

							<button onclick={handleLogout} class="flex w-full items-center gap-2 px-4 py-1.5 text-sm text-foreground hover:bg-brand transition-colors">
								<LogOut class="h-3.5 w-3.5" />
								Sign out
							</button>
						</div>
					{/if}
				</div>
			{:else if !authStore.loading}
				<a href="/login" class="flex h-7.5 items-center rounded-md border border-border px-3 text-sm text-foreground transition-colors hover:bg-secondary">Sign in</a>
				<div class="w-1"></div>
				<Button variant="brand" size="sm" href="/signup">Sign up</Button>
			{/if}
		</div>
		{#if isElectronApp}
			<div class="electron-window-controls">
				<button type="button" class="electron-window-button" aria-label="Minimize window" onclick={minimizeElectronWindow}>
					<Minus class="h-3.5 w-3.5" />
				</button>
				<button
					type="button"
					class="electron-window-button"
					aria-label={isElectronWindowMaximized ? 'Restore window' : 'Maximize window'}
					onclick={toggleElectronWindowMaximize}
				>
					{#if isElectronWindowMaximized}
						<span class="electron-restore-icon" aria-hidden="true"></span>
					{:else}
						<Square class="h-3.5 w-3.5" />
					{/if}
				</button>
				<button type="button" class="electron-window-button electron-window-button-close" aria-label="Close window" onclick={closeElectronWindow}>
					<X class="h-3.5 w-3.5" />
				</button>
			</div>
		{/if}
	</div>
</header>

<!-- Feedback dialog -->
<Dialog.Root bind:open={feedbackOpen}>
	<Dialog.Content class="sm:max-w-md">
		<Dialog.Header>
			<Dialog.Title>Send feedback</Dialog.Title>
			<Dialog.Description>GitPier is in alpha. Help us improve by reporting bugs or suggesting features.</Dialog.Description>
		</Dialog.Header>

		{#if feedbackSent}
			<div class="flex flex-col items-center gap-3 py-6 text-center">
				<div class="flex h-12 w-12 items-center justify-center rounded-full bg-green-500/15">
					<MessageSquare class="h-6 w-6 text-green-600 dark:text-green-400" />
				</div>
				<p class="font-medium text-foreground">Thanks for your feedback!</p>
				<p class="text-sm text-muted-foreground">We'll review it and use it to improve GitPier.</p>
				<Button variant="outline" size="sm" onclick={closeFeedback}>Close</Button>
			</div>
		{:else}
			<form onsubmit={submitFeedback} class="flex flex-col gap-4 py-2">
				{#if feedbackError}
					<p class="rounded-md border border-red-800/40 bg-red-900/20 px-3 py-2 text-xs text-red-400">{feedbackError}</p>
				{/if}

				<!-- Category -->
				<div class="flex gap-2">
					{#each [['bug', '🐛 Bug report'], ['feature', '✨ Feature request'], ['other', '💬 Other']] as [val, label]}
						<button
							type="button"
							onclick={() => (feedbackCategory = val as typeof feedbackCategory)}
							class="flex-1 rounded-md border px-2 py-1.5 text-xs transition-colors {feedbackCategory === val
								? 'border-primary bg-primary/10 text-primary font-medium'
								: 'border-border text-muted-foreground hover:border-primary/50 hover:text-foreground'}"
						>
							{label}
						</button>
					{/each}
				</div>

				<!-- Message -->
				<Textarea
					bind:value={feedbackText}
					placeholder={feedbackCategory === 'bug'
						? 'Describe what happened and how to reproduce it…'
						: feedbackCategory === 'feature'
							? "Describe the feature you'd like to see…"
							: 'Share your thoughts…'}
					rows={5}
					required
					class="resize-none"
				/>

				<Dialog.Footer class="gap-2">
					<Button type="button" variant="outline" onclick={closeFeedback}>Cancel</Button>
					<Button type="submit" variant="brand" disabled={feedbackLoading || !feedbackText.trim()}>
						{feedbackLoading ? 'Sending…' : 'Send feedback'}
					</Button>
				</Dialog.Footer>
			</form>
		{/if}
	</Dialog.Content>
</Dialog.Root>

<style>
	.electron-app {
		-webkit-app-region: drag;
	}

	.electron-app :is(a, button) {
		-webkit-app-region: no-drag;
	}

	.electron-window-controls {
		display: flex;
		margin-left: 0.375rem;
		overflow: hidden;
		border-radius: 0.625rem;
		height: 2rem;
		border: 1px solid var(--border);
		background: color-mix(in oklch, var(--secondary) 82%, transparent);
		backdrop-filter: blur(6px);
		box-shadow: 0 1px 0 color-mix(in oklch, var(--border) 72%, transparent) inset;
		-webkit-app-region: no-drag;
	}

	.electron-window-button {
		height: 100%;
		width: 2.45rem;
		display: flex;
		align-items: center;
		justify-content: center;
		color: var(--muted-foreground);
		background: transparent;
		border-left: 1px solid color-mix(in oklch, var(--border) 90%, transparent);
		transition:
			background-color 140ms ease,
			color 140ms ease;
	}

	.electron-window-button:first-child {
		border-left: 0;
	}

	.electron-window-button:hover {
		background: color-mix(in oklch, var(--accent) 82%, transparent);
		color: var(--foreground);
	}

	.electron-window-button-close:hover {
		background: color-mix(in oklch, var(--destructive) 22%, transparent);
		color: var(--destructive);
	}

	.electron-restore-icon {
		position: relative;
		display: block;
		width: 0.72rem;
		height: 0.72rem;
	}

	.electron-restore-icon::before,
	.electron-restore-icon::after {
		content: '';
		position: absolute;
		box-sizing: border-box;
		width: 0.52rem;
		height: 0.52rem;
		border: 1.5px solid currentColor;
		border-radius: 1px;
		background: transparent;
	}

	.electron-restore-icon::before {
		top: 0;
		right: 0;
	}

	.electron-restore-icon::after {
		left: 0;
		bottom: 0;
	}
</style>
