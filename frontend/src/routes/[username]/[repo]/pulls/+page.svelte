<script lang="ts">
	import { page } from '$app/state';
	import { pullRequests, type PullRequest } from '$lib/api/client';
	import { mediaUrl, timeAgo } from '$lib/utils';
	import { authStore } from '$lib/stores/auth.svelte';
	import { getContext } from 'svelte';
	import { GitPullRequest, GitMerge, XCircle, Plus, CircleDot, Search } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	let pull_requests = $state<PullRequest[]>([]);
	let loading = $state(true);
	let error = $state('');
	let filter = $state<'open' | 'closed'>('open');

	const { username, repo } = $derived(page.params);
	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const isLoggedIn = $derived(authStore.user != null);

	async function loadPRs() {
		loading = true;
		error = '';
		try {
			const data = await pullRequests.list(username!, repo!);
			pull_requests = data.pull_requests ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadPRs();
	});

	const openPRs = $derived(pull_requests.filter((p) => p.status === 'open'));
	const closedPRs = $derived(pull_requests.filter((p) => p.status === 'closed' || p.status === 'merged'));
	const filtered = $derived(filter === 'open' ? openPRs : closedPRs);

	function getStatusColor(status: string) {
		if (status === 'merged') return 'text-[#a371f7]';
		if (status === 'closed') return 'text-red-500';
		return 'text-[#3fb950]';
	}

	function StatusIcon(status: string) {
		if (status === 'merged') return GitMerge;
		if (status === 'closed') return XCircle;
		return CircleDot;
	}

	function avatarLetter(u: string | undefined) {
		return (u ?? '?')[0].toUpperCase();
	}
</script>

<svelte:head>
	<title>Pull requests · {username}/{repo} · GitPier</title>
</svelte:head>

{#if loading}
	<div class="space-y-2">
		<div class="h-10 rounded-md border border-border bg-card animate-pulse"></div>
		{#each Array(3) as _}
			<div class="h-16 rounded-md border border-secondary bg-card animate-pulse"></div>
		{/each}
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else}
	<!-- Filter bar -->
	<div class="flex items-center justify-between gap-3 mb-3">
		<div class="flex items-center gap-0.5 flex-1">
			<button
				onclick={() => (filter = 'open')}
				class="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors"
				class:bg-secondary={filter === 'open'}
				class:text-foreground={filter === 'open'}
				class:font-semibold={filter === 'open'}
				class:text-muted-foreground={filter !== 'open'}
			>
				<CircleDot class="h-4 w-4 text-[#3fb950]" />
				{openPRs.length} Open
			</button>
			<button
				onclick={() => (filter = 'closed')}
				class="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors"
				class:bg-secondary={filter === 'closed'}
				class:text-foreground={filter === 'closed'}
				class:font-semibold={filter === 'closed'}
				class:text-muted-foreground={filter !== 'closed'}
			>
				<XCircle class="h-4 w-4" />
				{closedPRs.length} Closed
			</button>
		</div>
		{#if isLoggedIn && !isRepoArchived}
			<Button variant="brand" size="sm" href="/{username}/{repo}/pulls/new">
				<Plus class="h-3.5 w-3.5" />
				New pull request
			</Button>
		{/if}
	</div>

	{#if filtered.length === 0}
		<div class="rounded-md border border-border bg-card py-16 text-center">
			<GitPullRequest class="mx-auto h-12 w-12 text-muted-foreground mb-4" />
			<h3 class="text-base font-semibold text-foreground mb-2">There aren't any {filter} pull requests.</h3>
			<p class="text-sm text-muted-foreground">
				{#if filter === 'open'}Pull requests help you collaborate on code with other people.{:else}No closed pull requests to show.{/if}
			</p>
		</div>
	{:else}
		<div class="rounded-md border border-border overflow-hidden divide-y divide-secondary">
			{#each filtered as pr}
				<a href="/{username}/{repo}/pulls/{pr.number || pr.id}" class="flex items-start gap-3 px-4 py-3 bg-card hover:bg-accent transition-colors">
					<span class={getStatusColor(pr.status)} style="margin-top: 1px">
						<svelte:component this={StatusIcon(pr.status)} class="h-4 w-4 shrink-0" />
					</span>
					<div class="flex-1 min-w-0">
						<p class="text-sm font-semibold text-foreground hover:text-primary truncate">{pr.title}</p>
						<div class="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
							<div class="h-4 w-4 rounded-full bg-secondary border border-border overflow-hidden flex items-center justify-center text-[9px] font-semibold text-foreground shrink-0">
								{#if pr.author?.avatar_url}
									<img src={mediaUrl(pr.author.avatar_url)} alt={pr.author?.username ?? 'author'} class="h-full w-full object-cover" />
								{:else}
									{avatarLetter(pr.author?.username)}
								{/if}
							</div>
							<span>#{pr.number || pr.id} opened {timeAgo(pr.created_at)} by {pr.author?.username ?? ''} · {pr.head_ref} → {pr.base_ref}</span>
						</div>
					</div>
				</a>
			{/each}
		</div>
	{/if}
{/if}
