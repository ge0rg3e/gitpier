<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { repos, type CommitInfo } from '$lib/api/client';
	import { timeAgo, isValidGitDate, commitAuthorAvatarUrl, commitAuthorHref, commitAuthorInitial, commitAuthorName } from '$lib/utils';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Command from '$lib/components/ui/command/index.js';
	import * as Popover from '$lib/components/ui/popover/index.js';
	import { RangeCalendar } from '$lib/components/ui/range-calendar/index.js';
	import { CalendarDays, ChevronDown, GitBranch, GitCommit, Search, Users } from '@lucide/svelte';
	import { parseDate, type DateValue } from '@internationalized/date';
	import { getContext } from 'svelte';

	const PAGE_SIZE = 30;
	type RepoLayoutCtx = {
		branches: string[];
		currentBranch: string;
	};

	const repoLayout = getContext<RepoLayoutCtx>('repoLayout');

	let commits = $state<CommitInfo[]>([]);
	let hasMore = $state(false);
	let totalPages = $state(0);
	let totalCommits = $state(0);
	let loading = $state(true);
	let loadingInvalidData = $state(false);
	let error = $state('');

	const { username, repo } = $derived(page.params);
	const ref = $derived(page.url.searchParams.get('ref') ?? undefined);
	const query = $derived(page.url.searchParams.get('q') ?? '');
	const author = $derived(page.url.searchParams.get('author') ?? '');
	const since = $derived(page.url.searchParams.get('since') ?? '');
	const until = $derived(page.url.searchParams.get('until') ?? '');
	type TimeDraft = { start: DateValue | undefined; end: DateValue | undefined } | undefined;
	const currentPage = $derived.by(() => {
		const raw = Number(page.url.searchParams.get('page') ?? '1');
		return Number.isFinite(raw) && raw > 0 ? Math.floor(raw) : 1;
	});
	const inlineFilters = $derived.by(() => {
		const items: Array<{ key: 'ref' | 'q'; label: string; icon: typeof GitBranch | typeof Search }> = [];
		if (ref && ref !== repoLayout?.currentBranch) items.push({ key: 'ref', label: ref, icon: GitBranch });
		if (query) items.push({ key: 'q', label: query, icon: Search });
		return items;
	});
	const authorOptions = $derived.by(() => {
		const authors = new Map<string, { name: string; email: string }>();
		if (author.trim()) {
			authors.set(author.toLowerCase(), { name: author.trim(), email: '' });
		}
		for (const commit of commits) {
			const name = commit.author?.name?.trim();
			if (!name) continue;
			const email = commit.author?.email?.trim() ?? '';
			const key = `${name.toLowerCase()}|${email.toLowerCase()}`;
			if (!authors.has(key)) authors.set(key, { name, email });
		}
		return [...authors.values()].sort((left, right) => left.name.localeCompare(right.name));
	});

	let branchPickerOpen = $state(false);
	let authorPickerOpen = $state(false);
	let timePickerOpen = $state(false);
	let branchSearch = $state('');
	let authorSearch = $state('');
	let timeDraft = $state<TimeDraft>(undefined);

	const branchLabel = $derived(ref ?? repoLayout?.currentBranch ?? 'main');
	const filteredBranches = $derived.by(() => {
		const search = branchSearch.trim().toLowerCase();
		const branches = repoLayout?.branches ?? [];
		if (!search) return branches;
		return branches.filter((b) => b.toLowerCase().includes(search));
	});

	const filteredAuthorOptions = $derived.by(() => {
		const search = authorSearch.trim().toLowerCase();
		if (!search) return authorOptions;
		return authorOptions.filter((option) => `${option.name} ${option.email}`.toLowerCase().includes(search));
	});
	const usersLabel = $derived(author.trim() || 'All users');
	const timeLabel = $derived.by(() => {
		if (!since && !until) return 'All time';
		if (since && until) {
			if (since === until) return formatShortDate(since);
			return `${formatShortDate(since)} - ${formatShortDate(until)}`;
		}
		if (since) return `Since ${formatShortDate(since)}`;
		return `Until ${formatShortDate(until)}`;
	});

	$effect(() => {
		authorSearch = '';
		branchSearch = '';
	});

	$effect(() => {
		if (since || until) {
			timeDraft = {
				start: since ? parseDate(since) : undefined,
				end: until ? parseDate(until) : undefined
			};
		} else {
			timeDraft = undefined;
		}
	});

	async function loadCommits(retryCount = 0) {
		loading = true;
		loadingInvalidData = false;
		error = '';
		try {
			const offset = (currentPage - 1) * PAGE_SIZE;
			const data = await repos.commits(username!, repo!, {
				ref,
				limit: PAGE_SIZE,
				offset,
				author,
				q: query,
				since,
				until
			});
			const nextCommits = (data.commits ?? []).filter(isRenderableCommit);
			const hasRawButInvalid = (data.commits?.length ?? 0) > 0 && nextCommits.length === 0;

			if (hasRawButInvalid && retryCount === 0) {
				loading = false;
				loadingInvalidData = true;
				setTimeout(() => {
					void loadCommits(1);
				}, 180);
				return;
			}

			commits = nextCommits;
			hasMore = data.has_more ?? false;
			totalPages = data.total_pages ?? 0;
			totalCommits = data.total ?? 0;
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	function isRenderableCommit(commit: CommitInfo): boolean {
		if (!commit?.sha || !/^[0-9a-f]{7,40}$/i.test(commit.sha)) return false;
		if (!commit.message?.trim()) return false;
		if (!commit.author?.name?.trim()) return false;
		if (!isValidGitDate(commit.author?.date)) return false;
		return true;
	}

	$effect(() => {
		loadCommits();
	});

	function subjectLine(msg: string) {
		return msg.split('\n')[0].trim();
	}

	function formatShortDate(raw: string) {
		const date = new Date(raw);
		if (Number.isNaN(date.getTime())) return raw;
		return date.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
	}

	function pageHref(targetPage: number) {
		const params = new URLSearchParams(page.url.searchParams);
		if (targetPage > 1) params.set('page', String(targetPage));
		else params.delete('page');
		const query = params.toString();
		return `/${username}/${repo}/commits${query ? `?${query}` : ''}`;
	}

	function buildCommitsHref(overrides: Partial<Record<'ref' | 'q' | 'author' | 'since' | 'until' | 'page', string | undefined>> = {}, resetPage = false) {
		const params = new URLSearchParams(page.url.searchParams);
		for (const [key, value] of Object.entries(overrides)) {
			const nextValue = value?.trim();
			if (nextValue) params.set(key, nextValue);
			else params.delete(key);
		}
		if (resetPage) params.delete('page');
		const queryString = params.toString();
		return `/${username}/${repo}/commits${queryString ? `?${queryString}` : ''}`;
	}

	function formatDateInput(date: Date) {
		const offsetMs = date.getTimezoneOffset() * 60_000;
		return new Date(date.getTime() - offsetMs).toISOString().slice(0, 10);
	}

	async function applyTimeFilter(nextSince?: string, nextUntil?: string) {
		if (nextSince && nextUntil && nextSince > nextUntil) {
			error = 'Start date must be before end date.';
			return;
		}
		error = '';
		await goto(
			buildCommitsHref(
				{
					since: nextSince,
					until: nextUntil
				},
				true
			),
			{ noScroll: true, keepFocus: true }
		);
		timePickerOpen = false;
	}

	async function applyBranchFilter(branch?: string) {
		error = '';
		await goto(buildCommitsHref({ ref: branch ?? undefined }, true), { noScroll: true, keepFocus: true });
		branchPickerOpen = false;
		branchSearch = '';
	}

	async function applyAuthorFilter(nextAuthor?: string) {
		error = '';
		await goto(buildCommitsHref({ author: nextAuthor?.trim() || undefined }, true), { noScroll: true, keepFocus: true });
		authorPickerOpen = false;
		authorSearch = '';
	}

	function removeFilter(key: 'ref' | 'q') {
		void goto(buildCommitsHref({ [key]: undefined }, true), { noScroll: true });
	}

	function applyQuickRange(days: number) {
		const today = new Date();
		const start = new Date(today);
		start.setDate(today.getDate() - (days - 1));
		void applyTimeFilter(formatDateInput(start), formatDateInput(today));
	}

	function applyToday() {
		const today = formatDateInput(new Date());
		void applyTimeFilter(today, today);
	}

	$effect(() => {
		const draft = timeDraft;
		if (!timePickerOpen) return;
		if (!draft?.start || !draft?.end) return;
		const nextSince = draft.start.toString();
		const nextUntil = draft.end.toString();
		if (nextSince === since && nextUntil === until) return;
		void applyTimeFilter(nextSince, nextUntil);
	});

	// Group commits by date
	function groupByDate(commits: CommitInfo[]) {
		const groups: Record<string, CommitInfo[]> = {};
		for (const c of commits) {
			const d = new Date(c.author.date);
			const key = d.toLocaleDateString('en-US', { year: 'numeric', month: 'long', day: 'numeric' });
			if (!groups[key]) groups[key] = [];
			groups[key].push(c);
		}
		return Object.entries(groups);
	}

	const grouped = $derived(groupByDate(commits));
</script>

<svelte:head>
	<title>Commits · {username}/{repo} · GitPier</title>
</svelte:head>

{#if loading || loadingInvalidData}
	<div class="space-y-4">
		{#each Array(3) as _}
			<div class="space-y-1">
				<div class="h-4 w-40 bg-secondary rounded animate-pulse mb-2"></div>
				{#each Array(3) as __}
					<div class="h-14 rounded-md border border-secondary bg-card animate-pulse"></div>
				{/each}
			</div>
		{/each}
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else}
	<div class="space-y-6">
		<div class="border-b border-border/70 pb-4">
			<div class="flex flex-wrap items-center gap-2">
				<Popover.Root bind:open={branchPickerOpen}>
					<Popover.Trigger
						class="inline-flex items-center gap-2 rounded-md border border-border bg-secondary/35 px-2.5 py-1.5 text-sm text-foreground hover:bg-secondary/55 focus:outline-none focus:ring-1 focus:ring-primary"
					>
						<GitBranch class="h-3.5 w-3.5 text-muted-foreground" />
						<span class="max-w-40 truncate">{branchLabel}</span>
						<ChevronDown class="h-3.5 w-3.5 text-muted-foreground" />
					</Popover.Trigger>
					<Popover.Content class="w-70 overflow-hidden p-0" align="start" sideOffset={8}>
						<Command.Root class="rounded-none border-0 bg-card p-0">
							<Command.Input bind:value={branchSearch} placeholder="Find a branch..." />
							<Command.List class="max-h-64 border-t border-border/60">
								{#each filteredBranches as branch}
									<Command.Item value={branch} onclick={() => applyBranchFilter(branch)}>
										<div class="flex min-w-0 items-center gap-2">
											<GitBranch class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
											<span class="truncate text-sm text-foreground">{branch}</span>
											{#if branch === branchLabel}
												<span class="ml-auto shrink-0 text-xs text-muted-foreground">current</span>
											{/if}
										</div>
									</Command.Item>
								{/each}
								{#if filteredBranches.length === 0}
									<div class="px-3 py-6 text-sm text-muted-foreground">No branches found.</div>
								{/if}
							</Command.List>
						</Command.Root>
					</Popover.Content>
				</Popover.Root>

				<Popover.Root bind:open={authorPickerOpen}>
					<Popover.Trigger
						class="inline-flex items-center gap-2 rounded-md border border-border bg-secondary/35 px-2.5 py-1.5 text-sm text-foreground hover:bg-secondary/55 focus:outline-none focus:ring-1 focus:ring-primary"
					>
						<Users class="h-3.5 w-3.5 text-muted-foreground" />
						<span class="max-w-40 truncate">{usersLabel}</span>
						<ChevronDown class="h-3.5 w-3.5 text-muted-foreground" />
					</Popover.Trigger>
					<Popover.Content class="w-[320px] overflow-hidden p-0" align="start" sideOffset={8}>
						<Command.Root class="rounded-none border-0 bg-card p-0">
							<Command.Input bind:value={authorSearch} placeholder="Find a user..." />
							<Command.List class="max-h-64 border-t border-border/60">
								{#if authorSearch.trim() && !filteredAuthorOptions.some((option) => option.name.toLowerCase() === authorSearch
													.trim()
													.toLowerCase() || option.email.toLowerCase() === authorSearch.trim().toLowerCase())}
									<Command.Item value={authorSearch.trim()} onclick={() => applyAuthorFilter(authorSearch.trim())}>
										<div class="flex min-w-0 items-center gap-2">
											<div class="flex h-5 w-5 items-center justify-center rounded-full border border-border bg-secondary text-[11px] font-semibold text-foreground">
												{authorSearch.trim().slice(0, 1).toUpperCase()}
											</div>
											<div class="min-w-0">
												<p class="truncate text-sm text-foreground">Use “{authorSearch.trim()}”</p>
											</div>
										</div>
									</Command.Item>
								{/if}
								{#each filteredAuthorOptions as option}
									<Command.Item value={`${option.name} ${option.email}`} onclick={() => applyAuthorFilter(option.name)}>
										<div class="flex min-w-0 items-center gap-2">
											<div class="flex h-5 w-5 items-center justify-center rounded-full border border-border bg-secondary text-[11px] font-semibold text-foreground">
												{option.name.slice(0, 1).toUpperCase()}
											</div>
											<div class="min-w-0">
												<p class="truncate text-sm text-foreground">{option.name}</p>
												{#if option.email}
													<p class="truncate text-xs text-muted-foreground">{option.email}</p>
												{/if}
											</div>
										</div>
									</Command.Item>
								{/each}
								{#if filteredAuthorOptions.length === 0 && !authorSearch.trim()}
									<div class="px-3 py-6 text-sm text-muted-foreground">No users in this commit page yet.</div>
								{/if}
							</Command.List>
						</Command.Root>
						<div class="border-t border-border/60 px-3 py-2 text-sm">
							<button type="button" class="text-primary hover:underline" onclick={() => applyAuthorFilter(undefined)}>View commits for all users</button>
						</div>
					</Popover.Content>
				</Popover.Root>

				<Popover.Root bind:open={timePickerOpen}>
					<Popover.Trigger
						class="inline-flex items-center gap-2 rounded-md border border-border bg-secondary/35 px-2.5 py-1.5 text-sm text-foreground hover:bg-secondary/55 focus:outline-none focus:ring-1 focus:ring-primary"
					>
						<CalendarDays class="h-3.5 w-3.5 text-muted-foreground" />
						<span class="max-w-48 truncate">{timeLabel}</span>
						<ChevronDown class="h-3.5 w-3.5 text-muted-foreground" />
					</Popover.Trigger>
					<Popover.Content class="w-auto overflow-hidden p-0" align="start" sideOffset={8}>
						<div class="flex items-center gap-1 border-b border-border/60 px-3 py-2">
							<Button type="button" variant="ghost" size="xs" class="px-1.5 text-muted-foreground hover:text-foreground" onclick={() => applyQuickRange(7)}>7d</Button>
							<Button type="button" variant="ghost" size="xs" class="px-1.5 text-muted-foreground hover:text-foreground" onclick={() => applyQuickRange(30)}>30d</Button>
							<Button type="button" variant="ghost" size="xs" class="px-1.5 text-muted-foreground hover:text-foreground" onclick={() => applyQuickRange(90)}>90d</Button>
						</div>
						<RangeCalendar bind:value={timeDraft} captionLayout="dropdown" class="bg-card" aria-label="Commit time range" />
						<div class="flex items-center justify-between border-t border-border/60 px-4 py-3 text-sm">
							<button type="button" class="font-medium text-foreground hover:underline" onclick={() => applyTimeFilter(undefined, undefined)}>Clear</button>
							<button type="button" class="text-muted-foreground hover:text-foreground" onclick={applyToday}>Today</button>
						</div>
					</Popover.Content>
				</Popover.Root>

				{#if inlineFilters.length > 0}
					<div class="mx-1 h-4 w-px bg-border/70"></div>
					<div class="flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
						{#each inlineFilters as filter}
							<button type="button" class="inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 hover:bg-secondary/35 hover:text-foreground" onclick={() => removeFilter(filter.key)}>
								<svelte:component this={filter.icon} class="h-3 w-3" />
								<span class="max-w-40 truncate">{filter.label}</span>
								<span aria-hidden="true">×</span>
							</button>
						{/each}
					</div>
				{/if}
			</div>

			<p class="mt-3 text-xs text-muted-foreground/90">
				Showing {commits.length} commit{commits.length === 1 ? '' : 's'} on this page.
			</p>
		</div>

		{#if commits.length === 0}
			<div class="rounded-md border border-border bg-card p-10 text-center">
				<GitCommit class="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
				<p class="text-sm text-muted-foreground">{author || since || until || query || ref ? 'No commits match the current filters.' : 'No commits yet.'}</p>
			</div>
		{:else}
			{#each grouped as [date, dayCommits]}
				<div>
					<h3 class="mb-2 text-sm font-semibold text-muted-foreground">Commits on {date}</h3>
					<div class="overflow-hidden rounded-md border border-border divide-y divide-secondary">
						{#each dayCommits as commit}
							<div class="flex items-center gap-4 bg-card px-4 py-3 transition-colors hover:bg-accent">
								<div class="min-w-0 flex-1">
									<div class="flex items-center gap-3">
										<div class="flex h-8 w-8 shrink-0 items-center justify-center overflow-hidden rounded-full border border-border bg-secondary text-xs font-bold text-primary">
											{#if commitAuthorAvatarUrl(commit.author)}
												<img src={commitAuthorAvatarUrl(commit.author)} alt={commitAuthorName(commit.author)} class="h-full w-full object-cover" />
											{:else}
												{commitAuthorInitial(commit.author)}
											{/if}
										</div>
										<div class="min-w-0 flex-1">
											<div class="flex items-center gap-2">
												<a href="/{username}/{repo}/commit/{commit.sha}" class="block truncate text-sm font-semibold text-foreground hover:text-primary">
													{subjectLine(commit.message)}
												</a>
												{#if commit.web_commit}
													<span
														class="shrink-0 rounded border border-green-600/40 bg-green-600/10 px-1.5 py-0.5 text-xs font-medium text-green-600 dark:border-green-500/40 dark:bg-green-500/10 dark:text-green-400"
														>Web</span
													>
												{/if}
											</div>
											<p class="mt-0.5 text-xs text-muted-foreground">
												{#if commitAuthorHref(commit.author)}
													<a href={commitAuthorHref(commit.author) ?? undefined} class="font-medium text-foreground hover:text-primary hover:underline"
														>{commitAuthorName(commit.author)}</a
													>
												{:else}
													<span class="font-medium text-foreground" title="Not registered on GitPier">{commitAuthorName(commit.author)}</span>
												{/if}
												committed {timeAgo(commit.author.date)}
											</p>
										</div>
									</div>
								</div>
								<div class="flex shrink-0 items-center gap-2">
									<a
										href="/{username}/{repo}/commit/{commit.sha}"
										class="flex items-center gap-1 rounded-md border border-border bg-secondary px-2.5 py-1 text-xs font-mono text-muted-foreground transition-colors hover:border-primary hover:text-primary"
									>
										<GitCommit class="h-3 w-3" />
										{commit.sha.slice(0, 7)}
									</a>
									<a
										href="/{username}/{repo}?ref={commit.sha}"
										class="rounded-md border border-border bg-secondary px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:border-primary hover:text-primary"
									>
										Browse files
									</a>
								</div>
							</div>
						{/each}
					</div>
				</div>
			{/each}

			<div class="flex items-center justify-between gap-3 border-t border-border pt-4">
				<p class="text-xs text-muted-foreground">Page {currentPage} of {Math.max(1, totalPages)} • {totalCommits} commits</p>
				<div class="flex items-center gap-2">
					<a
						href={pageHref(currentPage - 1)}
						aria-disabled={currentPage <= 1}
						class={`rounded-md border px-3 py-1.5 text-xs transition-colors ${
							currentPage <= 1
								? 'pointer-events-none border-border/60 bg-secondary/40 text-muted-foreground/60'
								: 'border-border bg-secondary text-muted-foreground hover:text-primary hover:border-primary'
						}`}
					>
						Previous
					</a>
					<a
						href={pageHref(currentPage + 1)}
						aria-disabled={!hasMore}
						class={`rounded-md border px-3 py-1.5 text-xs transition-colors ${
							!hasMore
								? 'pointer-events-none border-border/60 bg-secondary/40 text-muted-foreground/60'
								: 'border-border bg-secondary text-muted-foreground hover:text-primary hover:border-primary'
						}`}
					>
						Next
					</a>
				</div>
			</div>
		{/if}
	</div>
{/if}
