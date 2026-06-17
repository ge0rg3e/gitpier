<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { onMount } from 'svelte';
	import { search, type Repository, type User, type Organization, type CodeMatch, type FileMatch } from '$lib/api/client';
	import CodeViewer from '$lib/components/CodeViewer.svelte';
	import { timeAgo, mediaUrl } from '$lib/utils';
	import { BookOpen, Users, Building2, Lock, Star, ChevronLeft, ChevronRight, ChevronDown, Code, File, Folder, X } from '@lucide/svelte';

	const query = $derived(page.url.searchParams.get('q') ?? '');
	const typeParam = $derived(page.url.searchParams.get('type') ?? '');
	const repoParam = $derived(page.url.searchParams.get('repo') ?? ''); // "owner/name"

	const repoOwner = $derived(repoParam ? repoParam.split('/')[0] : '');
	const repoSlug = $derived(repoParam ? repoParam.split('/').slice(1).join('/') : '');
	const isRepo = $derived(!!repoParam && !!repoOwner && !!repoSlug);

	const activeType = $derived(isRepo ? (typeParam === 'files' ? 'files' : 'code') : ['repos', 'users', 'orgs'].includes(typeParam) ? typeParam : 'repos');

	let sortBy = $state('best_match');
	let loading = $state(false);
	let searchMs = $state<number | null>(null);
	let offset = $state(0);

	let repos = $state<Repository[]>([]);
	let reposTotal = $state(0);
	let users = $state<User[]>([]);
	let usersTotal = $state(0);
	let orgsList = $state<Organization[]>([]);
	let orgsTotal = $state(0);

	let codeResults = $state<CodeMatch[]>([]);
	let codeTotal = $state(0);
	let fileResults = $state<FileMatch[]>([]);
	let fileTotal = $state(0);
	let expandedFiles = $state(new Set<string>());
	let langFilter = $state<string | null>(null);
	let pathFilter = $state<string | null>(null);

	const LIMIT = 10;
	const MATCHES_PER_FILE = 3;

	const langColors: Record<string, string> = {
		TypeScript: '#3178c6',
		JavaScript: '#f1e05a',
		Svelte: '#ff3e00',
		Go: '#00add8',
		Python: '#3572a5',
		Rust: '#dea584',
		Java: '#b07219',
		'C++': '#f34b7d',
		C: '#555555',
		Ruby: '#701516',
		PHP: '#4f5d95',
		HTML: '#e34c26',
		CSS: '#563d7c',
		SCSS: '#c6538c',
		Vue: '#41b883',
		Markdown: '#083fa1',
		JSON: '#292929',
		YAML: '#cb171e',
		Shell: '#89e051'
	};

	function detectLanguage(path: string): string {
		const ext = path.split('.').pop()?.toLowerCase() ?? '';
		const m: Record<string, string> = {
			ts: 'TypeScript',
			tsx: 'TypeScript',
			js: 'JavaScript',
			jsx: 'JavaScript',
			svelte: 'Svelte',
			go: 'Go',
			py: 'Python',
			rs: 'Rust',
			java: 'Java',
			c: 'C',
			cpp: 'C++',
			rb: 'Ruby',
			php: 'PHP',
			md: 'Markdown',
			json: 'JSON',
			yaml: 'YAML',
			yml: 'YAML',
			toml: 'TOML',
			sql: 'SQL',
			sh: 'Shell',
			bash: 'Shell',
			html: 'HTML',
			css: 'CSS',
			scss: 'SCSS',
			vue: 'Vue'
		};
		return m[ext] ?? (ext ? ext.toUpperCase() : 'Text');
	}

	function encodeRepoPath(path: string): string {
		return path
			.split('/')
			.filter(Boolean)
			.map((segment) => encodeURIComponent(segment))
			.join('/');
	}

	function treeHrefForPath(path: string): string {
		const normalized = path.replace(/^\/+/, '');
		if (!normalized) return `/${repoOwner}/${repoSlug}`;
		const parts = normalized.split('/').filter(Boolean);
		const dirPath = parts.length > 1 ? parts.slice(0, -1).join('/') : '';
		if (!dirPath) return `/${repoOwner}/${repoSlug}`;
		return `/${repoOwner}/${repoSlug}/tree/${encodeRepoPath(dirPath)}`;
	}

	function blobHrefForPath(path: string): string {
		const normalized = path.replace(/^\/+/, '');
		return `/${repoOwner}/${repoSlug}/blob/${encodeRepoPath(normalized)}`;
	}

	const fileGroups = $derived.by(() => {
		const map = new Map<string, CodeMatch[]>();
		for (const m of codeResults) {
			if (!map.has(m.path)) map.set(m.path, []);
			map.get(m.path)!.push(m);
		}
		return [...map.entries()].map(([path, matches]) => ({ path, matches }));
	});

	const filteredGroups = $derived.by(() => {
		let g = fileGroups;
		if (langFilter) g = g.filter((x) => detectLanguage(x.path) === langFilter);
		if (pathFilter) g = g.filter((x) => x.path.startsWith(pathFilter!.replace(/\/$/, '') + '/') || x.path === pathFilter);
		return g;
	});

	const langBreakdown = $derived.by(() => {
		const counts = new Map<string, number>();
		for (const { path } of fileGroups) {
			const lang = detectLanguage(path);
			counts.set(lang, (counts.get(lang) ?? 0) + 1);
		}
		return [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 7);
	});

	const pathBreakdown = $derived.by(() => {
		const paths = activeType === 'code' ? codeResults.map((m) => m.path) : fileResults.map((m) => m.path);
		const counts = new Map<string, number>();
		for (const p of paths) {
			const parts = p.split('/');
			if (parts.length > 1) {
				const dir = parts[0] + '/';
				counts.set(dir, (counts.get(dir) ?? 0) + 1);
			}
		}
		return [...counts.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5);
	});

	const activeTotal = $derived(activeType === 'repos' ? reposTotal : activeType === 'users' ? usersTotal : activeType === 'orgs' ? orgsTotal : activeType === 'code' ? codeTotal : fileTotal);

	const sortedRepos = $derived.by(() => {
		if (sortBy === 'stars') return [...repos].sort((a, b) => (b.star_count ?? 0) - (a.star_count ?? 0));
		if (sortBy === 'newest') return [...repos].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
		return repos;
	});

	function countFor(t: string) {
		if (t === 'repos') return reposTotal;
		if (t === 'users') return usersTotal;
		if (t === 'orgs') return orgsTotal;
		if (t === 'code') return codeTotal;
		return fileTotal;
	}
	function formatCount(n: number): string {
		return n >= 1000 ? (n / 1000).toFixed(1) + 'k+' : String(n);
	}

	async function fetchGlobal(q: string, type: string, off: number) {
		loading = true;
		const start = Date.now();
		try {
			const [rr, ur, or] = await Promise.all([
				search.repos(q, type === 'repos' ? LIMIT : 1, type === 'repos' ? off : 0),
				search.users(q, type === 'users' ? LIMIT : 1, type === 'users' ? off : 0),
				search.orgs(q, type === 'orgs' ? LIMIT : 1, type === 'orgs' ? off : 0)
			]);
			reposTotal = rr.total;
			usersTotal = ur.total;
			orgsTotal = or.total;
			if (type === 'repos') repos = rr.items;
			else if (type === 'users') users = ur.items;
			else orgsList = or.items;
			searchMs = Date.now() - start;
		} catch {
			/* ignore */
		} finally {
			loading = false;
		}
	}

	async function fetchRepo(q: string, owner: string, repo: string, type: string) {
		loading = true;
		langFilter = null;
		pathFilter = null;
		expandedFiles = new Set();
		const start = Date.now();
		try {
			if (type === 'code') {
				const r = await search.code(owner, repo, q);
				codeResults = r.items;
				codeTotal = r.total;
			} else {
				const r = await search.files(owner, repo, q);
				fileResults = r.items;
				fileTotal = r.total;
			}
			searchMs = Date.now() - start;
		} catch {
			/* ignore */
		} finally {
			loading = false;
		}
	}

	async function switchType(type: string) {
		const params: Record<string, string> = { type };
		if (query) params.q = query;
		if (repoParam) params.repo = repoParam;
		goto(`/search?${new URLSearchParams(params)}`, { replaceState: true, noScroll: true });
	}

	async function paginate(newOffset: number) {
		offset = newOffset;
		loading = true;
		const start = Date.now();
		try {
			if (activeType === 'repos') {
				const r = await search.repos(query, LIMIT, newOffset);
				repos = r.items;
			} else if (activeType === 'users') {
				const r = await search.users(query, LIMIT, newOffset);
				users = r.items;
			} else if (activeType === 'orgs') {
				const r = await search.orgs(query, LIMIT, newOffset);
				orgsList = r.items;
			}
			searchMs = Date.now() - start;
		} finally {
			loading = false;
			if (typeof window !== 'undefined') window.scrollTo({ top: 0, behavior: 'smooth' });
		}
	}

	function removeRepoScope() {
		const params: Record<string, string> = { type: 'repos' };
		if (query) params.q = query;
		goto(`/search?${new URLSearchParams(params)}`, { replaceState: true, noScroll: true });
	}

	function toggleExpand(path: string) {
		const next = new Set(expandedFiles);
		if (next.has(path)) next.delete(path);
		else next.add(path);
		expandedFiles = next;
	}

	function snippetForMatch(content: string, line: number): string {
		return `// line ${line}\n${content ?? ''}`;
	}

	$effect(() => {
		const q = query;
		const type = activeType;
		const repo = repoParam;
		const own = repoOwner;
		const slug = repoSlug;
		const off = 0;
		if (q.trim().length < 2) return;
		if (repo && own && slug) {
			fetchRepo(q, own, slug, type);
		} else if (!repo) {
			fetchGlobal(q, type, off);
		}
	});

	// Global type tabs
	const globalTabs = [
		{ key: 'repos', label: 'Repositories', Icon: BookOpen },
		{ key: 'users', label: 'Users', Icon: Users },
		{ key: 'orgs', label: 'Organizations', Icon: Building2 }
	];
</script>

<svelte:head>
	<title>{query ? `${query} · Search` : 'Search'} · GitPier</title>
</svelte:head>

<div class="mx-auto max-w-screen-xl px-4 py-6">
	{#if query.trim().length >= 2}
		<div class="flex gap-8">
			<!-- Left sidebar -->
			<aside class="w-56 shrink-0 hidden md:block">
				{#if isRepo}
					<!-- Repo scope chip -->
					<div class="mb-4 flex items-center gap-1.5 rounded-md border border-brand/40 bg-brand/10 px-2.5 py-1.5">
						<span class="text-xs text-brand font-mono truncate flex-1">{repoParam}</span>
						<button onclick={removeRepoScope} class="text-brand/70 hover:text-brand transition-colors" aria-label="Remove repo filter">
							<X class="h-3.5 w-3.5" />
						</button>
					</div>
					<!-- Code / Files switcher -->
					<div class="border-t border-border pt-1 mb-5">
						{#each [{ key: 'code', label: 'Code', Icon: Code }, { key: 'files', label: 'Files', Icon: File }] as { key, label, Icon }}
							<button
								onclick={() => switchType(key)}
								class="w-full flex items-center gap-2 py-2 text-sm transition-colors
									{activeType === key ? 'border-l-[3px] border-brand pl-[9px] pr-3 text-foreground font-semibold' : 'border-l-[3px] border-transparent pl-[9px] pr-3 text-muted-foreground hover:text-foreground'}"
							>
								<Icon class="h-4 w-4 shrink-0" />
								<span class="flex-1 text-left">{label}</span>
								{#if countFor(key) > 0}
									<span class="rounded-full bg-[#30363d] px-[6px] py-[1px] text-xs text-[#8b949e]">
										{formatCount(countFor(key))}
									</span>
								{/if}
							</button>
						{/each}
					</div>
					<!-- Language breakdown (code mode) -->
					{#if activeType === 'code' && langBreakdown.length > 0}
						<div class="mb-5">
							<div class="flex items-center justify-between px-3 mb-1.5">
								<h4 class="text-xs font-semibold text-foreground">Languages</h4>
								{#if langFilter}
									<button onclick={() => (langFilter = null)} class="text-xs text-brand hover:underline">Clear</button>
								{/if}
							</div>
							<div class="border-t border-border pt-1">
								{#each langBreakdown as [lang, count]}
									<button
										onclick={() => (langFilter = langFilter === lang ? null : lang)}
										class="w-full flex items-center gap-2 py-1.5 text-sm transition-colors
											{langFilter === lang ? 'border-l-[3px] border-brand pl-[9px] pr-3 text-foreground font-semibold' : 'border-l-[3px] border-transparent pl-[9px] pr-3 text-muted-foreground hover:text-foreground'}"
									>
										<span class="h-2.5 w-2.5 rounded-full shrink-0" style="background-color: {langColors[lang] ?? '#888'}"></span>
										<span class="flex-1 text-left truncate">{lang}</span>
										<span class="text-xs text-[#8b949e]">{count}</span>
									</button>
								{/each}
							</div>
						</div>
					{/if}
					<!-- Path breakdown -->
					{#if pathBreakdown.length > 0}
						<div>
							<div class="flex items-center justify-between px-3 mb-1.5">
								<h4 class="text-xs font-semibold text-foreground">Paths</h4>
								{#if pathFilter}
									<button onclick={() => (pathFilter = null)} class="text-xs text-brand hover:underline">Clear</button>
								{/if}
							</div>
							<div class="border-t border-border pt-1">
								{#each pathBreakdown as [dir, count]}
									<button
										onclick={() => (pathFilter = pathFilter === dir ? null : dir)}
										class="w-full flex items-center gap-2 py-1.5 text-sm transition-colors
											{pathFilter === dir ? 'border-l-[3px] border-brand pl-[9px] pr-3 text-foreground font-semibold' : 'border-l-[3px] border-transparent pl-[9px] pr-3 text-muted-foreground hover:text-foreground'}"
									>
										<Folder class="h-3.5 w-3.5 text-muted-foreground shrink-0" />
										<span class="flex-1 text-left font-mono text-xs truncate">{dir}</span>
										<span class="text-xs text-[#8b949e]">{count}</span>
									</button>
								{/each}
							</div>
						</div>
					{/if}
				{:else}
					<!-- Global filter by -->
					<h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2 px-3">Filter by</h3>
					<div class="border-t border-border pt-1">
						{#each globalTabs as { key, label, Icon }}
							<button
								onclick={() => switchType(key)}
								class="w-full flex items-center gap-2 py-2 text-sm transition-colors
									{activeType === key ? 'border-l-[3px] border-brand pl-[9px] pr-3 text-foreground font-semibold' : 'border-l-[3px] border-transparent pl-[9px] pr-3 text-muted-foreground hover:text-foreground'}"
							>
								<Icon class="h-4 w-4 shrink-0" />
								<span class="flex-1 text-left">{label}</span>
								{#if reposTotal > 0 || usersTotal > 0 || orgsTotal > 0}
									<span class="rounded-full bg-[#30363d] px-[6px] py-[1px] text-xs text-[#8b949e]">
										{formatCount(countFor(key))}
									</span>
								{/if}
							</button>
						{/each}
					</div>
				{/if}
			</aside>

			<!-- Main -->
			<div class="flex-1 min-w-0">
				<!-- Mobile tabs -->
				<div class="md:hidden flex gap-1 mb-4 overflow-x-auto pb-1">
					{#if isRepo}
						{#each [{ key: 'code', label: 'Code', Icon: Code }, { key: 'files', label: 'Files', Icon: File }] as { key, label, Icon }}
							<button
								onclick={() => switchType(key)}
								class="flex items-center gap-1.5 whitespace-nowrap rounded-full px-3 py-1 text-sm border transition-colors
									{activeType === key ? 'bg-secondary border-border text-foreground font-semibold' : 'border-transparent text-muted-foreground hover:bg-secondary/50'}"
							>
								<Icon class="h-3.5 w-3.5" />{label}
							</button>
						{/each}
					{:else}
						{#each globalTabs as { key, label, Icon }}
							<button
								onclick={() => switchType(key)}
								class="flex items-center gap-1.5 whitespace-nowrap rounded-full px-3 py-1 text-sm border transition-colors
									{activeType === key ? 'bg-secondary border-border text-foreground font-semibold' : 'border-transparent text-muted-foreground hover:bg-secondary/50'}"
							>
								<Icon class="h-3.5 w-3.5" />{label}
							</button>
						{/each}
					{/if}
				</div>

				{#if loading}
					<div class="space-y-3">
						{#each Array(4) as _}
							<div class="rounded-lg border border-border bg-card p-5 animate-pulse">
								<div class="h-4 bg-secondary rounded w-52 mb-2.5"></div>
								<div class="h-3 bg-secondary rounded w-80 mb-1.5"></div>
								<div class="h-3 bg-secondary rounded w-44"></div>
							</div>
						{/each}
					</div>
				{:else if isRepo}
					<!-- ── Repo-scoped results ─────────────────────────────────────── -->
					{#if activeType === 'code'}
						<!-- Results info row -->
						{#if codeResults.length > 0}
							<div class="flex items-center gap-2 mb-4 flex-wrap text-sm">
								<p class="text-foreground">
									<strong>{filteredGroups.length}</strong> file{filteredGroups.length !== 1 ? 's' : ''}
									{#if searchMs !== null}<span class="text-muted-foreground ml-1">({searchMs} ms)</span>{/if}
								</p>
								<span class="text-muted-foreground">in</span>
								<span class="inline-flex items-center gap-1 bg-brand/10 border border-brand/30 text-brand text-xs rounded-md px-2 py-0.5 font-mono">
									{repoParam}
									<button onclick={removeRepoScope} class="ml-0.5 hover:opacity-70 text-base leading-none" aria-label="Remove repo filter">×</button>
								</span>
								{#if langFilter}
									<span class="inline-flex items-center gap-1.5 bg-secondary border border-border rounded-md px-2 py-0.5 text-xs">
										<span class="h-2 w-2 rounded-full" style="background-color: {langColors[langFilter] ?? '#888'}"></span>
										{langFilter}
										<button onclick={() => (langFilter = null)} class="ml-0.5 text-muted-foreground hover:text-foreground leading-none">×</button>
									</span>
								{/if}
								{#if pathFilter}
									<span class="inline-flex items-center gap-1 bg-secondary border border-border rounded-md px-2 py-0.5 font-mono text-xs">
										{pathFilter}
										<button onclick={() => (pathFilter = null)} class="ml-0.5 text-muted-foreground hover:text-foreground leading-none">×</button>
									</span>
								{/if}
							</div>
						{/if}

						{#if filteredGroups.length === 0}
							<div class="rounded-lg border border-border bg-card p-12 text-center">
								<Code class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
								<p class="text-foreground font-semibold mb-1">No code matches</p>
								<p class="text-sm text-muted-foreground">
									{codeResults.length === 0 ? `We couldn't find any code matching "${query}".` : 'No results match the current filters.'}
								</p>
							</div>
						{:else}
							<div class="space-y-3">
								{#each filteredGroups as { path, matches }}
									{@const lang = detectLanguage(path)}
									{@const isExpanded = expandedFiles.has(path)}
									{@const visible = isExpanded ? matches : matches.slice(0, MATCHES_PER_FILE)}
									{@const hiddenCount = matches.length - MATCHES_PER_FILE}
									<div class="rounded-lg border border-border overflow-hidden">
										<div class="flex items-center justify-between px-4 py-2.5 bg-[#161b22] border-b border-border">
											<a href={blobHrefForPath(path)} class="flex items-center gap-2 text-sm font-mono text-brand hover:underline min-w-0">
												<File class="h-4 w-4 text-muted-foreground shrink-0" />
												<span class="truncate">{path}</span>
											</a>
											<div class="flex items-center gap-3 shrink-0 ml-3">
												<span class="flex items-center gap-1.5 text-xs text-muted-foreground">
													<span class="h-2 w-2 rounded-full" style="background-color: {langColors[lang] ?? '#888'}"></span>
													{lang}
												</span>
												<span class="text-xs text-muted-foreground">{matches.length} match{matches.length !== 1 ? 'es' : ''}</span>
											</div>
										</div>
										<div class="divide-y divide-border/40 bg-card font-mono text-xs">
											{#each visible as match}
												<a href={blobHrefForPath(path)} class="block group hover:bg-secondary/30 transition-colors">
													<div class="px-4 pt-2 text-[11px] text-muted-foreground">Line {match.line}</div>
													<div class="p-1.5">
														<CodeViewer code={snippetForMatch(match.content, match.line)} filePath={path} containerClass="border-0 rounded-none bg-transparent" />
													</div>
												</a>
											{/each}
										</div>
										{#if hiddenCount > 0}
											<button
												onclick={() => toggleExpand(path)}
												class="w-full flex items-center gap-1.5 px-4 py-2 text-xs text-brand hover:bg-secondary/30 border-t border-border/40 bg-card transition-colors"
											>
												<ChevronRight class="h-3.5 w-3.5 transition-transform {isExpanded ? 'rotate-90' : ''}" />
												{isExpanded ? 'Show fewer matches' : `Show ${hiddenCount} more match${hiddenCount !== 1 ? 'es' : ''}`}
											</button>
										{/if}
									</div>
								{/each}
							</div>
							{#if codeTotal >= 200}
								<p class="text-xs text-muted-foreground mt-5 text-center">Showing first 200 results. Refine your query for more precise results.</p>
							{/if}
						{/if}
					{:else}
						<!-- File results -->
						{#if fileResults.length > 0}
							<p class="text-sm text-foreground mb-4">
								<strong>{fileTotal}</strong> file{fileTotal !== 1 ? 's' : ''}
								{#if searchMs !== null}<span class="text-muted-foreground ml-1">({searchMs} ms)</span>{/if}
							</p>
						{/if}
						{#if fileResults.length === 0}
							<div class="rounded-lg border border-border bg-card p-12 text-center">
								<File class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
								<p class="text-foreground font-semibold mb-1">No files found</p>
								<p class="text-sm text-muted-foreground">We couldn't find any files matching <strong>{query}</strong>.</p>
							</div>
						{:else}
							<div class="rounded-lg border border-border overflow-hidden divide-y divide-border">
								{#each fileResults as f}
									{@const parts = f.path.split('/')}
									{@const fileName = parts[parts.length - 1]}
									{@const dirPath = parts.length > 1 ? parts.slice(0, -1).join('/') + '/' : ''}
									<a href={blobHrefForPath(f.path)} class="flex items-center gap-2 px-4 py-2.5 bg-card hover:bg-secondary/30 transition-colors group">
										<File class="h-4 w-4 text-muted-foreground shrink-0" />
										<span class="flex-1 font-mono text-sm truncate">
											<span class="text-muted-foreground">{dirPath}</span>{@html (() => {
												const esc = (s: string) => s.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;');
												if (!query) return esc(fileName);
												const pts = fileName.split(new RegExp(`(${query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&')})`, 'gi'));
												return pts
													.map((p) =>
														p.toLowerCase() === query.toLowerCase() ? `<mark class="bg-[#bb8009]/30 text-[#e3b341] rounded-sm not-italic">${esc(p)}</mark>` : esc(p)
													)
													.join('');
											})()}
										</span>
										<ChevronRight class="h-4 w-4 text-muted-foreground opacity-0 group-hover:opacity-100 shrink-0 transition-opacity" />
									</a>
								{/each}
							</div>
							{#if fileTotal >= 500}
								<p class="text-xs text-muted-foreground mt-5 text-center">Showing first 500 results.</p>
							{/if}
						{/if}
					{/if}
				{:else}
					<!-- ── Global results ──────────────────────────────────────────── -->
					<div class="flex items-center justify-between mb-4 flex-wrap gap-2">
						<p class="text-sm text-foreground">
							<strong>{activeTotal.toLocaleString()}</strong>
							{#if activeType === 'repos'}{activeTotal === 1 ? 'repository' : 'repositories'}
							{:else if activeType === 'users'}{activeTotal === 1 ? 'user' : 'users'}
							{:else}{activeTotal === 1 ? 'organization' : 'organizations'}{/if}
							{#if searchMs !== null}<span class="text-muted-foreground font-normal ml-1">({searchMs} ms)</span>{/if}
						</p>
						{#if activeType === 'repos'}
							<div class="flex items-center gap-1.5 text-sm">
								<span class="text-muted-foreground">Sort by:</span>
								<div class="relative">
									<select
										bind:value={sortBy}
										class="appearance-none bg-card border border-border rounded-md pl-3 pr-7 py-1 text-sm text-foreground cursor-pointer focus:outline-none focus:ring-1 focus:ring-primary"
									>
										<option value="best_match">Best match</option>
										<option value="newest">Recently updated</option>
										<option value="stars">Most stars</option>
									</select>
									<ChevronDown class="pointer-events-none absolute right-1.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground" />
								</div>
							</div>
						{/if}
					</div>

					{#if activeType === 'repos'}
						{#if sortedRepos.length === 0}
							<div class="rounded-lg border border-border bg-card p-12 text-center">
								<BookOpen class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
								<p class="text-foreground font-semibold mb-1">No repositories found</p>
								<p class="text-sm text-muted-foreground">We couldn't find any repositories matching <strong>{query}</strong>.</p>
							</div>
						{:else}
							<div class="divide-y divide-border rounded-lg border border-border overflow-hidden">
								{#each sortedRepos as repo}
									<div class="bg-card px-5 py-4 hover:bg-secondary/20 transition-colors">
										<div class="flex items-start gap-2 mb-1.5 flex-wrap">
											{#if repo.is_private}
												<span class="flex items-center gap-1 rounded-full border border-[#30363d] px-1.5 py-0.5 text-[10px] text-muted-foreground">
													<Lock class="h-2.5 w-2.5" />Private
												</span>
											{/if}
											<a href="/{repo.owner.username}/{repo.name}" class="text-brand font-semibold hover:underline text-sm">
												{repo.owner.username}/<strong>{repo.name}</strong>
											</a>
										</div>
										{#if repo.description}<p class="text-sm text-muted-foreground line-clamp-2 mb-2">{repo.description}</p>{/if}
										<div class="flex items-center gap-4 text-xs text-muted-foreground flex-wrap">
											{#if repo.language}
												<span class="flex items-center gap-1.5">
													<span class="h-2.5 w-2.5 rounded-full shrink-0" style="background-color:{langColors[repo.language] ?? '#8b949e'}"></span>
													{repo.language}
												</span>
											{/if}
											<span class="flex items-center gap-1"><Star class="h-3 w-3" />{repo.star_count ?? 0}</span>
											<span>Updated {timeAgo(repo.updated_at)}</span>
										</div>
									</div>
								{/each}
							</div>
						{/if}
					{:else if activeType === 'users'}
						{#if users.length === 0}
							<div class="rounded-lg border border-border bg-card p-12 text-center">
								<Users class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
								<p class="text-foreground font-semibold mb-1">No users found</p>
								<p class="text-sm text-muted-foreground">We couldn't find any users matching <strong>{query}</strong>.</p>
							</div>
						{:else}
							<div class="divide-y divide-border rounded-lg border border-border overflow-hidden">
								{#each users as user}
									<a href="/{user.username}" class="flex items-center gap-4 bg-card px-5 py-4 hover:bg-secondary/20 transition-colors">
										<div class="h-10 w-10 rounded-full border border-border bg-secondary flex items-center justify-center overflow-hidden shrink-0">
											{#if user.avatar_url}
												<img src={mediaUrl(user.avatar_url)} alt={user.username} class="h-full w-full object-cover" />
											{:else}
												<span class="text-sm font-semibold text-muted-foreground">{user.username[0].toUpperCase()}</span>
											{/if}
										</div>
										<div class="min-w-0">
											<p class="text-sm font-semibold text-foreground">{user.username}</p>
											{#if user.display_name}<p class="text-xs text-muted-foreground">{user.display_name}</p>{/if}
											{#if user.bio}<p class="text-sm text-muted-foreground truncate mt-0.5">{user.bio}</p>{/if}
										</div>
									</a>
								{/each}
							</div>
						{/if}
					{:else if orgsList.length === 0}
						<div class="rounded-lg border border-border bg-card p-12 text-center">
							<Building2 class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
							<p class="text-foreground font-semibold mb-1">No organizations found</p>
							<p class="text-sm text-muted-foreground">We couldn't find any organizations matching <strong>{query}</strong>.</p>
						</div>
					{:else}
						<div class="divide-y divide-border rounded-lg border border-border overflow-hidden">
							{#each orgsList as org}
								<a href="/{org.login}" class="flex items-center gap-4 bg-card px-5 py-4 hover:bg-secondary/20 transition-colors">
									<div class="h-10 w-10 rounded-full border border-border bg-secondary flex items-center justify-center overflow-hidden shrink-0">
										{#if org.avatar_url}
											<img src={mediaUrl(org.avatar_url)} alt={org.login} class="h-full w-full object-cover" />
										{:else}
											<span class="text-sm font-semibold text-muted-foreground">{org.login[0].toUpperCase()}</span>
										{/if}
									</div>
									<div class="min-w-0">
										<p class="text-sm font-semibold text-foreground">{org.login}</p>
										{#if org.display_name}<p class="text-xs text-muted-foreground">{org.display_name}</p>{/if}
										{#if org.description}<p class="text-sm text-muted-foreground truncate mt-0.5">{org.description}</p>{/if}
									</div>
								</a>
							{/each}
						</div>
					{/if}

					{#if activeTotal > LIMIT}
						<div class="flex items-center justify-between mt-5">
							<button
								onclick={() => paginate(Math.max(0, offset - LIMIT))}
								disabled={offset === 0}
								class="flex items-center gap-1 text-sm px-3 py-1.5 rounded-md border border-border hover:bg-secondary transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
							>
								<ChevronLeft class="h-4 w-4" />Previous
							</button>
							<span class="text-xs text-muted-foreground">
								{offset + 1}–{Math.min(offset + LIMIT, activeTotal)} of {activeTotal.toLocaleString()}
							</span>
							<button
								onclick={() => paginate(offset + LIMIT)}
								disabled={offset + LIMIT >= activeTotal}
								class="flex items-center gap-1 text-sm px-3 py-1.5 rounded-md border border-border hover:bg-secondary transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
							>
								Next<ChevronRight class="h-4 w-4" />
							</button>
						</div>
					{/if}
				{/if}
			</div>
		</div>
	{:else}
		<div class="flex flex-col items-center justify-center py-24 text-center">
			<BookOpen class="h-12 w-12 text-muted-foreground mb-5" />
			<h2 class="text-xl font-semibold text-foreground mb-2">Search GitPier</h2>
			<p class="text-muted-foreground max-w-sm">Find repositories, users, organizations and code across GitPier.</p>
		</div>
	{/if}
</div>
