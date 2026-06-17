	<script lang="ts">
	import { page } from '$app/state';
	import { onMount, setContext } from 'svelte';
	import { repos, orgs, type Repository, type CommitInfo } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { getPublicRuntimeConfig } from '$lib/runtime-config';
	import { Code, Lock, Globe, ChevronDown, Check, Star, GitBranch, GitFork, Copy, CheckCheck, BookOpen, Tag, Download, Settings } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	let { children } = $props();

	let repo = $state<Repository | null>(null);
	let branches = $state<string[]>([]);
	let headCommit = $state<CommitInfo | null>(null);
	let stats = $state<{ commits: number; branches: number; tags: number; branch: string } | null>(null);
	let loading = $state(true);
	let error = $state('');
	let currentBranch = $state('');
	let starred = $state(false);
	let starCount = $state(0);
	let starring = $state(false);
	let showClone = $state(false);
	let copied = $state(false);
	let cloneProtocol = $state<'https' | 'ssh'>('https');
	let downloadingZip = $state(false);
	let showBranchDropdown = $state(false);
	let forking = $state(false);
	let syncingFork = $state(false);
	let showForkDialog = $state(false);
	let forkOwner = $state('');
	let forkRepoName = $state('');
	let forkDescription = $state('');
	let copyMainBranchOnly = $state(true);
	let forkOwners = $state<Array<{ login: string; label: string }>>([]);

	async function reloadRepoMetadata(ref?: string) {
		const data = await repos.get(username!, repoName!, ref, { includeSize: false, includeStats: false });
		repo = data.repo;
		branches = data.branches ?? [];
		headCommit = data.head_commit;
		stats = data.stats;
		currentBranch = ref ?? data.repo.default_branch;
		void refreshStats(ref);
	}

	// Expose layout data to child pages via context
	const repoLayoutCtx = {
		get repo() {
			return repo;
		},
		reloadMetadata: reloadRepoMetadata,
		get cloneProtocol() {
			return cloneProtocol;
		},
		setCloneProtocol(protocol: 'https' | 'ssh') {
			cloneProtocol = protocol;
		},
		get starCount() {
			return starCount;
		},
		get watching() {
			return 0;
		},
		get stats() {
			return stats;
		},
		get branches() {
			return branches;
		},
		get currentBranch() {
			return currentBranch;
		}
	};
	setContext('repoLayout', repoLayoutCtx);

	const sortedBranches = $derived([...branches].sort());
	const isEmptyRepo = $derived(branches.length === 0 && !headCommit);
	const { username, repo: repoName } = $derived(page.params);
	const currentPath = $derived(page.url.pathname);
	const seoOwner = $derived(repo?.org?.login ?? repo?.owner?.username ?? username ?? '');
	const seoRepo = $derived(repo?.name ?? repoName ?? '');
	const seoDescription = $derived(repo?.description?.trim() || 'Source code, issues, commits, pull requests, and releases on GitPier.');
	const seoTitle = $derived(`${seoOwner}/${seoRepo}`);
	const canonicalUrl = $derived(`${page.url.origin}${page.url.pathname}`);
	const ogImageUrl = $derived(`${page.url.origin}/images/logo.png`);

	async function refreshStats(ref?: string, attempt = 0) {
		try {
			const statsData = await repos.get(username!, repoName!, ref, {
				includeBranches: false,
				includeHead: false,
				includeSize: false
			});
			if (statsData.stats) {
				stats = statsData.stats;
				currentBranch = statsData.stats.branch || currentBranch;
				if ((statsData.stats.commits ?? 0) === 0 && attempt < 3) {
					setTimeout(
						() => {
							void refreshStats(ref, attempt + 1);
						},
						1500 * (attempt + 1)
					);
				}
			}
		} catch {}
	}

	onMount(async () => {
		try {
			const ref = page.url.searchParams.get('ref') ?? undefined;
			await reloadRepoMetadata(ref);
		} catch (e: any) {
			if (e.status === 404) error = 'Repository not found.';
			else if (e.status === 401 || e.status === 403) error = 'This repository is private.';
			else error = e.message;
		} finally {
			loading = false;
		}
		void loadStarStatus();
		if (authStore.isAuthenticated) {
			await loadForkOwners();
		}
	});

	async function loadForkOwners() {
		if (!authStore.user) return;
		const owners: Array<{ login: string; label: string }> = [{ login: authStore.user.username, label: authStore.user.username }];
		try {
			const myOrgs = await orgs.listMyOrgs();
			for (const org of myOrgs) {
				owners.push({ login: org.login, label: org.login });
			}
		} catch {}
		forkOwners = owners;
		if (!forkOwner && owners.length > 0) forkOwner = owners[0].login;
	}

	async function loadStarStatus() {
		try {
			const data = await repos.star.getStatus(username!, repoName!);
			starred = data.starred;
			starCount = data.count;
		} catch {}
	}

	async function toggleStar() {
		if (!authStore.isAuthenticated) {
			goto('/login');
			return;
		}
		starring = true;
		try {
			if (starred) {
				await repos.star.unstar(username!, repoName!);
				starred = false;
				starCount--;
			} else {
				await repos.star.star(username!, repoName!);
				starred = true;
				starCount++;
			}
		} catch {
		} finally {
			starring = false;
		}
	}

	function isActive(tabPath: string): boolean {
		const basePath = `/${username}/${repoName}`;
		if (!tabPath) return currentPath === basePath || currentPath.startsWith(`${basePath}/tree`) || currentPath.startsWith(`${basePath}/blob`);
		return currentPath.startsWith(`${basePath}/${tabPath}`);
	}

	function resolveSshCloneHost(raw?: string): string {
		const trimmed = (raw ?? '').trim();
		if (!trimmed) return 'localhost:2424';
		return trimmed
			.replace(/^ssh:\/\//i, '')
			.replace(/^[^@]+@/, '')
			.replace(/\/+$/, '');
	}

	function resolveHTTPCloneBaseURL(raw?: string): string {
		const trimmed = (raw ?? '').trim();
		if (trimmed) return trimmed.replace(/\/+$/, '');
		if (typeof window !== 'undefined') return window.location.origin;
		return 'http://localhost:8828';
	}

	const runtimeConfig = getPublicRuntimeConfig();
	const sshCloneHost = resolveSshCloneHost(runtimeConfig.sshCloneHost);
	const httpCloneBaseURL = resolveHTTPCloneBaseURL(runtimeConfig.httpCloneBaseURL);
	const cloneUrlSSH = $derived(repo ? `ssh://git@${sshCloneHost}/${username}/${repo.name}.git` : '');
	const cloneUrlHTTPS = $derived(repo ? `${httpCloneBaseURL}/${username}/${repo.name}.git` : '');
	const cloneUrl = $derived(cloneProtocol === 'ssh' ? cloneUrlSSH : cloneUrlHTTPS);

	async function copyCloneUrl() {
		await navigator.clipboard.writeText(cloneUrl);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	async function downloadZip() {
		if (!repo || !username || downloadingZip) return;

		downloadingZip = true;
		try {
			const url = repos.downloadZipUrl(username, repo.name, currentBranch || repo.default_branch);
			const token = (window as unknown as { __gitpier_token?: string }).__gitpier_token;
			const res = await fetch(url, {
				credentials: 'include',
				headers: token ? { Authorization: `Bearer ${token}` } : undefined
			});
			if (!res.ok) throw new Error(`Download failed (${res.status})`);

			const blob = await res.blob();
			const objectUrl = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = objectUrl;
			a.download = `${repo.name}-${(currentBranch || repo.default_branch).replaceAll('/', '-')}.zip`;
			document.body.appendChild(a);
			a.click();
			a.remove();
			URL.revokeObjectURL(objectUrl);
			showClone = false;
		} catch (e: any) {
			alert(e?.message ?? 'Failed to download ZIP');
		} finally {
			downloadingZip = false;
		}
	}

	const canAdmin = $derived(repo && authStore.user && repo.owner_id === authStore.user.id);
	const isFork = $derived(!!repo?.forked_from_repo);
	const canFork = $derived(repo && !isFork && !repo.is_private);
	const canSyncFork = $derived(authStore.isAuthenticated && repo && isFork && authStore.user?.id === repo.owner_id);
	const upstreamNamespace = $derived(repo?.forked_from_repo?.org?.login ?? repo?.forked_from_repo?.owner?.username ?? '');

	function openForkDialog() {
		if (!repo) return;
		if (!authStore.isAuthenticated || !authStore.user) {
			goto('/login');
			return;
		}
		showForkDialog = true;
		forkRepoName = repo.name;
		forkDescription = repo.description ?? '';
		copyMainBranchOnly = true;
		if (forkOwners.length > 0) {
			forkOwner = forkOwners[0].login;
		} else {
			forkOwner = authStore.user.username;
		}
	}

	async function forkRepo() {
		if (!repo) return;
		if (!authStore.isAuthenticated) {
			goto('/login');
			return;
		}

		forking = true;
		try {
			const created = await repos.fork.create(username!, repoName!, {
				owner: forkOwner,
				name: forkRepoName,
				description: forkDescription,
				copy_main_branch_only: copyMainBranchOnly
			});
			showForkDialog = false;
			await goto(`/${forkOwner}/${created.name}`);
		} catch (e: any) {
			alert(e?.message ?? 'Failed to fork repository');
		} finally {
			forking = false;
		}
	}

	async function syncFork() {
		if (!repo || !isFork) return;
		syncingFork = true;
		try {
			const result = await repos.fork.sync(username!, repoName!);
			alert(result.message);
			const ref = page.url.searchParams.get('ref') ?? undefined;
			const data = await repos.get(username!, repoName!, ref, { includeSize: false, includeStats: false });
			repo = data.repo;
			branches = data.branches ?? [];
			headCommit = data.head_commit;
			stats = data.stats;
			currentBranch = ref ?? repo.default_branch;
			void refreshStats(ref);
		} catch (e: any) {
			alert(e?.message ?? 'Failed to sync fork');
		} finally {
			syncingFork = false;
		}
	}
</script>

<svelte:head>
	<title>{seoTitle}{seoDescription ? ': ' + seoDescription : ''}</title>
	<meta name="description" content={seoDescription} />
	<link rel="canonical" href={canonicalUrl} />

	<meta property="og:site_name" content="GitPier" />
	<meta property="og:type" content="object" />
	<meta property="og:title" content={seoTitle} />
	<meta property="og:description" content={seoDescription} />
	<meta property="og:url" content={canonicalUrl} />
	<meta property="og:image" content={ogImageUrl} />
	<meta property="og:image:width" content="1200" />
	<meta property="og:image:height" content="630" />
	<meta property="og:image:alt" content={`Repository preview card for ${seoTitle}`} />

	<meta name="twitter:card" content="summary_large_image" />
	<meta name="twitter:title" content={seoTitle} />
	<meta name="twitter:description" content={seoDescription} />
	<meta name="twitter:image" content={ogImageUrl} />
</svelte:head>

<svelte:window
	onclick={(e) => {
		if (!(e.target as HTMLElement).closest('.clone-dropdown')) showClone = false;
		if (!(e.target as HTMLElement).closest('.branch-dropdown')) showBranchDropdown = false;
	}}
/>

{#if loading}
	<div class="bg-background min-h-screen">
		<div class="border-b border-secondary px-4 py-4">
			<div class="mx-auto max-w-screen-xl">
				<div class="h-5 w-64 bg-secondary rounded animate-pulse mb-3"></div>
				<div class="h-8 bg-card rounded animate-pulse"></div>
			</div>
		</div>
	</div>
{:else if error}
	<div class="bg-background min-h-screen flex items-center justify-center">
		<div class="text-center">
			<Lock class="mx-auto h-10 w-10 text-muted-foreground mb-4" />
			<h2 class="text-lg font-semibold text-foreground mb-2">Repository not accessible</h2>
			<p class="text-muted-foreground text-sm mb-6">{error}</p>
			{#if !authStore.isAuthenticated}
				<Button variant="brand" href="/login">Sign in</Button>
			{/if}
		</div>
	</div>
{:else if repo}
	<!-- Repo header -->
	<div class="bg-background border-b border-secondary">
		<div class="mx-auto max-w-screen-xl px-4 pt-4">
			<!-- Top row: breadcrumb + action buttons -->
			<div class="flex items-start justify-between gap-4 mb-2">
				<div class="flex items-center gap-2 flex-wrap min-w-0">
					<BookOpen class="h-4 w-4 text-muted-foreground shrink-0" />
					<div class="flex items-center gap-1 text-sm">
						<a href="/{username}" class="font-semibold text-primary hover:underline">{username}</a>
						<span class="text-muted-foreground text-lg leading-none">/</span>
						<a href="/{username}/{repoName}" class="font-bold text-primary hover:underline">{repoName}</a>
					</div>
					<span
						class={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs font-medium ${
							repo.is_archived ? 'border-amber-700/40 bg-amber-900/20 text-amber-300' : 'border-border text-muted-foreground'
						}`}
					>
						{#if repo.is_private}<Lock class="h-2.5 w-2.5" />{:else}<Globe class="h-2.5 w-2.5" />{/if}
						{repo.is_private ? 'Private' : 'Public'}{repo.is_archived ? ' archive' : ''}
					</span>
					{#if repo.is_archived && repo.archived_at}
						<span class="text-xs text-muted-foreground">archived on {new Date(repo.archived_at).toLocaleDateString()}</span>
					{/if}
				</div>
				<!-- Watch / Star / Fork buttons -->
				<div class="flex items-center gap-2 shrink-0">
					{#if canFork}
						<div class="flex items-center">
							<button
								onclick={openForkDialog}
								disabled={forking}
								class="flex items-center gap-1 h-7 rounded-l-md border border-border bg-secondary px-2.5 text-xs font-semibold text-foreground hover:bg-border transition-colors disabled:opacity-60"
							>
								<GitFork class="h-3.5 w-3.5" />
								Fork
							</button>
							<button class="flex items-center h-7 gap-1 rounded-r-md border-y border-r border-border bg-secondary px-2 text-xs text-muted-foreground hover:bg-border transition-colors">
								<span>{repo.fork_count ?? 0}</span>
							</button>
						</div>
					{/if}
					{#if canSyncFork}
						<button
							onclick={syncFork}
							disabled={syncingFork}
							class="flex items-center gap-1 h-7 rounded-md border border-border bg-secondary px-2.5 text-xs font-semibold text-foreground hover:bg-border transition-colors disabled:opacity-60"
						>
							<GitFork class="h-3.5 w-3.5" />
							{syncingFork ? 'Syncing...' : 'Sync fork'}
						</button>
					{/if}
					<div class="flex items-center">
						<button
							onclick={toggleStar}
							disabled={starring}
							class="flex items-center gap-1 h-7 rounded-l-md border border-border bg-secondary px-2.5 text-xs font-semibold transition-colors disabled:opacity-60"
							class:text-[#e3b341]={starred}
							class:hover:bg-border={!starred}
							class:text-foreground={!starred}
						>
							<Star class="h-3.5 w-3.5" fill={starred ? 'currentColor' : 'none'} />
							{starred ? 'Starred' : 'Star'}
						</button>
						<button class="flex items-center h-7 gap-1 rounded-r-md border-y border-r border-border bg-secondary px-2 text-xs text-muted-foreground hover:bg-border transition-colors">
							<span>{starCount}</span>
						</button>
					</div>
					{#if canAdmin}
						<a
							class="flex items-center gap-1 h-7 rounded-md border border-border bg-secondary px-1.5 text-foreground hover:bg-border transition-colors disabled:opacity-60"
							href="/{username}/{repoName}/settings"
							aria-label="Settings"
						>
							<Settings class="size-4" />
						</a>
					{/if}
				</div>
			</div>

			{#if repo.description}
				<p class="text-sm text-muted-foreground mb-3 ml-6">{repo.description}</p>
			{/if}

			{#if isFork && repo.forked_from_repo}
				<p class="text-xs text-muted-foreground mb-3 ml-6">
					forked from
					<a href="/{upstreamNamespace}/{repo.forked_from_repo.name}" class="text-primary hover:underline">
						{upstreamNamespace}/{repo.forked_from_repo.name}
					</a>
				</p>
			{/if}
		</div>
	</div>

	<!-- Content -->
	<div class="bg-background min-h-screen">
		<div class="mx-auto max-w-screen-xl px-4 py-5">
			<!-- Code tab toolbar -->
			{#if isActive('')}
				<div class="flex items-center gap-2 mb-4 flex-wrap relative">
					{#if !isEmptyRepo}
						<!-- Branch selector -->
						<div class="branch-dropdown relative">
							<button
								onclick={() => (showBranchDropdown = !showBranchDropdown)}
								class="flex items-center gap-1.5 h-8 rounded-md border border-border bg-secondary px-3 text-sm text-foreground hover:bg-border transition-colors font-semibold max-w-[200px]"
							>
								<GitBranch class="h-4 w-4 text-muted-foreground shrink-0" />
								<span class="truncate">{currentBranch || repo.default_branch}</span>
								<ChevronDown class="h-3.5 w-3.5 text-muted-foreground shrink-0" />
							</button>
							{#if showBranchDropdown}
								<div class="absolute left-0 top-full mt-1 w-64 rounded-md border border-border bg-card shadow-xl z-30 overflow-hidden">
									<div class="px-3 py-2 border-b border-secondary">
										<p class="text-xs font-semibold text-foreground">Switch branches/tags</p>
									</div>
									<div class="py-1 max-h-72 overflow-y-auto">
										<p class="px-3 pt-1 pb-0.5 text-xs text-muted-foreground font-semibold uppercase tracking-wide">Branches</p>
										{#each sortedBranches as branch}
											<button
												onclick={() => {
													currentBranch = branch;
													showBranchDropdown = false;
													goto(`/${username}/${repoName}?ref=${branch}`);
												}}
												class="w-full flex items-center gap-2 px-4 py-1.5 text-sm text-left hover:bg-brand transition-colors"
											>
												{#if branch === currentBranch}
													<Check class="h-3.5 w-3.5 text-primary shrink-0" />
												{:else}
													<span class="w-3.5 shrink-0"></span>
												{/if}
												<span class="text-foreground">{branch}</span>
											</button>
										{/each}
									</div>
									<div class="border-t border-secondary py-1">
										<a href="/{username}/{repoName}/branches" class="flex items-center px-4 py-2 text-xs text-primary hover:bg-brand transition-colors">View all branches</a>
										<a href="/{username}/{repoName}/tags" class="flex items-center px-4 py-2 text-xs text-primary hover:bg-brand transition-colors">View all tags</a>
									</div>
								</div>
							{/if}
						</div>
					{/if}

					<!-- Branch / tag counts -->
					<a href="/{username}/{repoName}/branches" class="flex items-center gap-1.5 h-8 px-2 text-xs font-semibold text-muted-foreground hover:text-primary transition-colors">
						<svg class="h-4 w-4" viewBox="0 0 16 16" fill="currentColor"
							><path
								d="M9.5 3.25a2.25 2.25 0 1 1 3 2.122V6A2.5 2.5 0 0 1 10 8.5H6a1 1 0 0 0-1 1v1.128a2.251 2.251 0 1 1-1.5 0V5.372a2.25 2.25 0 1 1 1.5 0v1.836A2.493 2.493 0 0 1 6 7h4a1 1 0 0 0 1-1v-.628A2.25 2.25 0 0 1 9.5 3.25Z"
							/></svg
						>
						{stats?.branches.toLocaleString() ?? 0}
						{(stats?.branches ?? 0) === 1 ? 'Branch' : 'Branches'}
					</a>

					<a href="/{username}/{repoName}/tags" class="flex items-center gap-1.5 h-8 px-2 text-xs font-semibold text-muted-foreground hover:text-primary transition-colors">
						<Tag class="h-4 w-4" />
						{stats?.tags.toLocaleString() ?? 0}
						{(stats?.tags ?? 0) === 1 ? 'Tag' : 'Tags'}
					</a>

					<div class="ml-auto flex items-center gap-2">
						<!-- Code button with dropdown -->
						<div class="clone-dropdown relative">
							<Button variant="brand" size="sm" onclick={() => (showClone = !showClone)}>
								<Code class="h-3.5 w-3.5" />
								Code
								<ChevronDown class="h-3.5 w-3.5" />
							</Button>

							{#if showClone}
								<div class="absolute right-0 top-full mt-1 w-[340px] rounded-md border border-border bg-card shadow-xl z-30 overflow-hidden">
									<div class="p-4">
										<p class="text-xs font-semibold text-foreground mb-3">Clone</p>
										<div class="mb-2 inline-flex rounded-md border border-border bg-background p-0.5">
											<button
												type="button"
												onclick={() => (cloneProtocol = 'https')}
												class={`rounded px-2.5 py-1 text-xs font-semibold transition-colors ${cloneProtocol === 'https' ? 'bg-brand text-white' : 'text-muted-foreground hover:text-foreground'}`}
											>
												HTTPS
											</button>
											<button
												type="button"
												onclick={() => (cloneProtocol = 'ssh')}
												class={`rounded px-2.5 py-1 text-xs font-semibold transition-colors ${cloneProtocol === 'ssh' ? 'bg-brand text-white' : 'text-muted-foreground hover:text-foreground'}`}
											>
												SSH
											</button>
										</div>
										<div class="flex items-center gap-2 rounded-md border border-border bg-background px-3 py-2">
											<code class="flex-1 text-xs font-mono text-muted-foreground truncate">
												{cloneUrl}
											</code>
											<button onclick={copyCloneUrl} class="text-muted-foreground hover:text-foreground transition-colors shrink-0" aria-label="Copy">
												{#if copied}<CheckCheck class="h-4 w-4 text-[#3fb950]" />{:else}<Copy class="h-4 w-4" />{/if}
											</button>
										</div>
										<p class="mt-2 text-xs text-muted-foreground">
											{#if cloneProtocol === 'https'}
												Use a personal access token as your Git password.
												<a href="/settings/tokens" class="text-primary hover:underline">Create token</a>
											{:else}
												Use a password-protected SSH key.
												<a href="/settings/keys" class="text-primary hover:underline">Add SSH key</a>
											{/if}
										</p>
									</div>
									<div class="border-t border-secondary p-2">
										<button
											onclick={downloadZip}
											disabled={downloadingZip}
											class="w-full flex items-center gap-2 rounded-md px-2 py-1.5 text-sm text-foreground hover:bg-brand transition-colors text-left"
										>
											<Download class="h-4 w-4 text-muted-foreground" />
											{downloadingZip ? 'Downloading ZIP...' : 'Download ZIP'}
										</button>
									</div>
								</div>
							{/if}
						</div>
					</div>
				</div>
			{/if}

			{@render children()}
		</div>
	</div>
{/if}

{#if showForkDialog && repo}
	<div class="fixed inset-0 z-50 flex items-center justify-center p-4">
		<button class="absolute inset-0 bg-black/60" onclick={() => (showForkDialog = false)} aria-label="Close"></button>
		<div class="relative w-full max-w-xl rounded-lg border border-border bg-card p-5 shadow-2xl">
			<div class="mb-4">
				<h3 class="text-lg font-semibold text-foreground">Create a new fork</h3>
				<p class="text-sm text-muted-foreground mt-1">A fork is a copy of a repository. You can customize owner, name, description, and copied branch scope.</p>
			</div>
			<div class="space-y-4">
				<div class="grid grid-cols-1 sm:grid-cols-[1fr_auto_1fr] gap-2 items-end">
					<div>
						<label class="block text-xs font-semibold text-foreground mb-1">Owner</label>
						<select bind:value={forkOwner} class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary">
							{#each forkOwners as owner}
								<option value={owner.login}>{owner.label}</option>
							{/each}
						</select>
					</div>
					<div class="text-muted-foreground text-lg pb-2">/</div>
					<div>
						<label class="block text-xs font-semibold text-foreground mb-1">Repository name</label>
						<input
							bind:value={forkRepoName}
							maxlength={100}
							class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
						/>
					</div>
				</div>
				<div>
					<label class="block text-xs font-semibold text-foreground mb-1">Description</label>
					<textarea
						bind:value={forkDescription}
						maxlength={350}
						rows={3}
						class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
					></textarea>
					<p class="mt-1 text-xs text-muted-foreground">{forkDescription.length} / 350 characters</p>
				</div>
				<label class="flex items-center gap-2 text-sm text-foreground">
					<input type="checkbox" bind:checked={copyMainBranchOnly} class="h-4 w-4 rounded border-border" />
					Copy the {repo.default_branch} branch only
				</label>
			</div>
			<div class="mt-5 flex items-center justify-end gap-2">
				<Button variant="outline" onclick={() => (showForkDialog = false)} disabled={forking}>Cancel</Button>
				<Button variant="brand" onclick={forkRepo} disabled={forking || !forkOwner || !forkRepoName.trim()}>{forking ? 'Creating fork...' : 'Create fork'}</Button>
			</div>
		</div>
	</div>
{/if}
