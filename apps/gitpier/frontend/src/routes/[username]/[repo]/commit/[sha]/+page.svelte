<script lang="ts">
	import { page } from '$app/state';
	import { repos, type CommitDetail } from '$lib/api/client';
	import { commitAuthorAvatarUrl, commitAuthorHref, commitAuthorInitial, commitAuthorName, formatDate } from '$lib/utils';
	import DiffViewer from '$lib/components/DiffViewer.svelte';
	import { ChevronRight, FileCode, Plus, Minus, FileText, ChevronDown } from '@lucide/svelte';

	let detail = $state<CommitDetail | null>(null);
	let loadingMeta = $state(true);
	let loadingDiffs = $state(false);
	let error = $state('');
	let diffError = $state('');
	let expandedFiles = $state<Set<string>>(new Set());
	let hasMoreDiffs = $state(false);
	let diffOffset = $state(0);
	let requestToken = 0;
	let loadMoreSentinel = $state<HTMLDivElement | null>(null);
	let showFullMessage = $state(false);

	const DIFF_PAGE_SIZE = 10;

	const { username, repo, sha } = $derived(page.params);

	async function loadMetadataAndFirstDiffPage() {
		const token = ++requestToken;
		loadingMeta = true;
		loadingDiffs = false;
		diffOffset = 0;
		hasMoreDiffs = false;
		diffError = '';
		error = '';
		detail = null;
		expandedFiles = new Set();
		try {
			const meta = await repos.commitMeta(username!, repo!, sha!);
			if (token !== requestToken) return;
			detail = {
				...meta,
				diffs: []
			};
			hasMoreDiffs = (meta.changed_files ?? 0) > 0;
			loadingMeta = false;
			await loadMoreDiffs(token);
		} catch (e: any) {
			if (token !== requestToken) return;
			error = e.message;
		} finally {
			if (token === requestToken && loadingMeta) {
				loadingMeta = false;
			}
		}
	}

	async function loadMoreDiffs(token = requestToken) {
		if (!detail || loadingMeta || loadingDiffs || !hasMoreDiffs) return;
		loadingDiffs = true;
		diffError = '';
		try {
			const pageData = await repos.commitDiffs(username!, repo!, sha!, DIFF_PAGE_SIZE, diffOffset);
			if (token !== requestToken || !detail) return;
			detail = {
				...detail,
				diffs: [...(detail.diffs ?? []), ...(pageData.diffs ?? [])]
			};
			diffOffset += pageData.diffs?.length ?? 0;
			hasMoreDiffs = pageData.has_more;

			if (expandedFiles.size === 0 && (detail.diffs?.length ?? 0) > 0) {
				expandedFiles = new Set(detail.diffs.slice(0, 3).map((d) => d.path));
			}
		} catch (e: any) {
			if (token !== requestToken) return;
			diffError = e.message;
		} finally {
			if (token === requestToken) {
				loadingDiffs = false;
			}
		}
	}

	$effect(() => {
		loadMetadataAndFirstDiffPage();
	});

	$effect(() => {
		if (!loadMoreSentinel) return;
		const observer = new IntersectionObserver(
			(entries) => {
				if (entries.some((entry) => entry.isIntersecting) && hasMoreDiffs && !loadingDiffs) {
					loadMoreDiffs();
				}
			},
			{ rootMargin: '200px 0px' }
		);
		observer.observe(loadMoreSentinel);
		return () => observer.disconnect();
	});

	const totalAdditions = $derived(detail?.additions ?? 0);
	const totalDeletions = $derived(detail?.deletions ?? 0);
	const totalChangedFiles = $derived(detail?.changed_files ?? detail?.diffs?.length ?? 0);

	function subjectLine(msg: string) {
		return msg.split('\n')[0].trim();
	}
	function bodyLines(msg: string) {
		return msg.split('\n').slice(1).join('\n').trim();
	}

	function toggleFile(path: string) {
		const s = new Set(expandedFiles);
		if (s.has(path)) s.delete(path);
		else s.add(path);
		expandedFiles = s;
	}
</script>

<!-- Breadcrumb -->
<nav class="flex items-center gap-1 mb-4 text-sm flex-wrap">
	<a href="/{username}/{repo}" class="text-primary hover:underline font-semibold">{repo}</a>
	<ChevronRight class="h-3.5 w-3.5 text-muted-foreground" />
	<a href="/{username}/{repo}/commits" class="text-primary hover:underline">commits</a>
	<ChevronRight class="h-3.5 w-3.5 text-muted-foreground" />
	<span class="font-mono text-foreground">{sha?.slice(0, 7)}</span>
</nav>

{#if loadingMeta}
	<div class="space-y-3">
		<div class="h-24 rounded-md border border-border bg-card animate-pulse"></div>
		{#each Array(3) as _}
			<div class="h-12 rounded-md border border-border bg-card animate-pulse"></div>
		{/each}
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else if detail}
	<!-- Commit header -->
	<div class="rounded-md border border-border bg-card p-5 mb-5">
		<h2 class="text-xl font-semibold text-foreground mb-3 leading-snug">{subjectLine(detail.message)}</h2>
		{#if bodyLines(detail.message)}
			<div class="mb-4">
				<button onclick={() => (showFullMessage = !showFullMessage)} class="mb-2 text-xs text-primary hover:underline">
					{showFullMessage ? 'Hide full commit message' : 'Show full commit message'}
				</button>
				{#if showFullMessage}
					<pre class="whitespace-pre-wrap text-sm text-muted-foreground bg-background rounded-md border border-border p-3">{bodyLines(detail.message)}</pre>
				{/if}
			</div>
		{/if}
		<div class="flex items-center gap-4 flex-wrap">
			<div class="flex items-center gap-2">
				<div class="h-6 w-6 rounded-full bg-secondary border border-border flex items-center justify-center shrink-0">
					{#if commitAuthorAvatarUrl(detail.author)}
						<img src={commitAuthorAvatarUrl(detail.author)} alt={commitAuthorName(detail.author)} class="h-full w-full rounded-full object-cover" />
					{:else}
						<span class="text-xs font-bold text-primary">{commitAuthorInitial(detail.author)}</span>
					{/if}
				</div>
				{#if commitAuthorHref(detail.author)}
					<a href={commitAuthorHref(detail.author) ?? undefined} class="text-sm font-semibold text-foreground hover:underline">{commitAuthorName(detail.author)}</a>
				{:else}
					<span class="text-sm font-semibold text-foreground" title="Not registered on GitPier">{commitAuthorName(detail.author)}</span>
				{/if}
			</div>
			<span class="text-sm text-muted-foreground">committed</span>
			<span class="text-sm text-muted-foreground">{formatDate(detail.author.date)}</span>
			<code class="ml-auto rounded-md border border-border bg-background px-2.5 py-1 text-xs font-mono text-muted-foreground">{detail.sha}</code>
		</div>
	</div>

	<!-- Diff stats -->
	{#if totalChangedFiles > 0}
		<div class="flex items-center gap-3 mb-3 text-sm text-muted-foreground flex-wrap">
			<span class="font-semibold text-foreground">Showing {totalChangedFiles} changed file{totalChangedFiles !== 1 ? 's' : ''}</span>
			<span class="font-mono text-[#3fb950]">+{totalAdditions}</span>
			<span class="font-mono text-[#f85149]">-{totalDeletions}</span>
		</div>
	{/if}

	<!-- File diffs -->
	<div class="space-y-3">
		{#each detail.diffs ?? [] as diff}
			{@const isExpanded = expandedFiles.has(diff.path)}
			<div class="rounded-md border border-border overflow-hidden">
				<!-- File header -->
				<button onclick={() => toggleFile(diff.path)} class="flex items-center gap-3 w-full px-4 py-2.5 bg-card hover:bg-accent text-left transition-colors">
					{#if isExpanded}
						<ChevronDown class="h-4 w-4 text-muted-foreground shrink-0" />
					{:else}
						<ChevronRight class="h-4 w-4 text-muted-foreground shrink-0" />
					{/if}
					{#if diff.type === 'added'}
						<Plus class="h-4 w-4 text-[#3fb950] shrink-0" />
					{:else if diff.type === 'deleted'}
						<Minus class="h-4 w-4 text-destructive shrink-0" />
					{:else if diff.type === 'renamed'}
						<FileText class="h-4 w-4 text-[#d29922] shrink-0" />
					{:else}
						<FileCode class="h-4 w-4 text-primary shrink-0" />
					{/if}
					{#if diff.old_path && diff.old_path !== diff.path}
						<span class="text-sm text-muted-foreground line-through font-mono">{diff.old_path}</span>
						<span class="text-muted-foreground">→</span>
					{/if}
					<span class="flex-1 text-sm text-foreground font-mono truncate">{diff.path}</span>
					<span
						class="text-xs shrink-0 capitalize ml-2"
						class:text-[#3fb950]={diff.type === 'added'}
						class:text-destructive={diff.type === 'deleted'}
						class:text-[#d29922]={diff.type === 'renamed'}
						class:text-muted-foreground={diff.type !== 'added' && diff.type !== 'deleted' && diff.type !== 'renamed'}>{diff.type}</span
					>
				</button>

				{#if isExpanded && diff.patch}
					<DiffViewer patch={diff.patch} filePath={diff.path} />
				{/if}
			</div>
		{/each}

		{#if !loadingDiffs && !hasMoreDiffs && (detail.diffs?.length ?? 0) === 0 && totalChangedFiles === 0}
			<div class="rounded-md border border-border bg-card p-4 text-sm text-muted-foreground">No file changes found for this commit.</div>
		{/if}

		{#if diffError}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400 flex items-center justify-between gap-3">
				<span>{diffError}</span>
				<button
					onclick={() => {
						hasMoreDiffs = true;
						loadMoreDiffs();
					}}
					class="rounded border border-border bg-card px-2.5 py-1 text-xs text-foreground hover:bg-accent">Retry</button
				>
			</div>
		{/if}

		{#if loadingDiffs}
			<div class="rounded-md border border-border bg-card px-4 py-3 text-sm text-muted-foreground">Loading more changed files...</div>
		{:else if hasMoreDiffs}
			<div class="rounded-md border border-border bg-card px-4 py-3 text-sm text-muted-foreground">Scroll to load more changes...</div>
		{/if}
	</div>

	<!-- Stable scroll sentinel: always in DOM so the IntersectionObserver $effect only runs once on mount -->
	<div bind:this={loadMoreSentinel} class="h-px"></div>
{/if}
