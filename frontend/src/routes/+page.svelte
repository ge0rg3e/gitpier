<script lang="ts">
	import { onMount } from 'svelte';
	import { authStore } from '$lib/stores/auth.svelte';
	import { dashboard, type DashboardPullRequest } from '$lib/api/client';
	import { timeAgo } from '$lib/utils';
	import { GitPullRequest, CircleDot, ListChecks } from '@lucide/svelte';

	let loading = $state(false);
	let error = $state('');
	let openPullsCount = $state(0);
	let openIssuesCount = $state(0);
	let reviewRequestsCount = $state(0);
	let recentPulls = $state<DashboardPullRequest[]>([]);

	async function loadOverview() {
		if (!authStore.user?.id) return;

		loading = true;
		error = '';
		openPullsCount = 0;
		openIssuesCount = 0;
		reviewRequestsCount = 0;
		recentPulls = [];

		try {
			const data = await dashboard.overview(16);
			openPullsCount = data.open_pull_requests ?? 0;
			openIssuesCount = data.open_issues ?? 0;
			reviewRequestsCount = data.review_requests ?? 0;
			recentPulls = data.recent_pull_requests ?? [];
		} catch (e: any) {
			error = e.message ?? 'Failed to load dashboard overview';
		} finally {
			loading = false;
		}
	}

	onMount(() => {
		const interval = setInterval(() => {
			if (authStore.loading) return;
			clearInterval(interval);
			if (authStore.isAuthenticated) {
				void loadOverview();
			}
		}, 80);
	});

	const welcomeName = $derived.by(() => {
		const display = authStore.user?.display_name?.trim();
		if (display) return display.split(' ')[0];
		return authStore.user?.username ?? 'Local';
	});
</script>

<svelte:head>
	<title>Overview · GitPier</title>
</svelte:head>

{#if !authStore.isAuthenticated && !authStore.loading}
	<div class="mx-auto max-w-3xl px-6 py-16 text-sm text-muted-foreground">Sign in to view your dashboard.</div>
{:else if authStore.isAuthenticated}
	<div class="mx-auto max-w-4xl px-6 py-12">
		{#if error}
			<div class="mb-4 rounded-md border border-red-500/40 bg-red-500/10 px-3 py-2 text-sm text-red-300">{error}</div>
		{/if}

		<section class="mb-6">
			<h1 class="text-4xl font-semibold tracking-tight text-foreground">Welcome back, {welcomeName}</h1>
		</section>

		<section class="mb-8 grid grid-cols-1 gap-2 md:grid-cols-3">
			<div class="rounded-xl border border-border bg-card px-4 py-3">
				<div class="flex items-center gap-2 text-xs text-muted-foreground">
					<GitPullRequest class="h-4 w-4" />
					<span class="font-semibold text-foreground">{openPullsCount}</span>
				</div>
				<p class="mt-1 text-sm text-muted-foreground">Open Pull Requests</p>
			</div>
			<div class="rounded-xl border border-border bg-card px-4 py-3">
				<div class="flex items-center gap-2 text-xs text-muted-foreground">
					<CircleDot class="h-4 w-4" />
					<span class="font-semibold text-foreground">{openIssuesCount}</span>
				</div>
				<p class="mt-1 text-sm text-muted-foreground">Open Issues</p>
			</div>
			<div class="rounded-xl border border-border bg-card px-4 py-3">
				<div class="flex items-center gap-2 text-xs text-muted-foreground">
					<ListChecks class="h-4 w-4" />
					<span class="font-semibold text-foreground">{reviewRequestsCount}</span>
				</div>
				<p class="mt-1 text-sm text-muted-foreground">Review Requests</p>
			</div>
		</section>

		<section>
			<h2 class="mb-3 text-xl font-medium text-muted-foreground">Recent Pull Requests</h2>
			<div class="overflow-hidden rounded-2xl border border-border bg-card">
				{#if loading}
					<div class="h-16 bg-secondary/20 animate-pulse"></div>
				{:else if recentPulls.length === 0}
					<div class="px-5 py-8 text-center text-sm text-muted-foreground">No pull requests yet.</div>
				{:else}
					{#each recentPulls as pr}
						<a href="/{pr.repo_owner}/{pr.repo_name}/pulls/{pr.number}" class="flex items-center gap-4 border-b border-border/70 px-4 py-3 transition-colors hover:bg-secondary/30 last:border-b-0">
							<div class="text-brand">
								<GitPullRequest class="h-4 w-4" />
							</div>
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-foreground">{pr.title}</p>
								<p class="truncate text-xs text-muted-foreground">{pr.repo_owner}/{pr.repo_name} #{pr.number}</p>
							</div>
							<div class="hidden items-center gap-1.5 text-xs text-muted-foreground md:flex">
								<span class="h-2 w-2 rounded-full bg-indigo-400/80"></span>
								<span>{pr.author?.username ?? pr.repo_owner}</span>
							</div>
							<div class="shrink-0 text-xs text-muted-foreground">{timeAgo(pr.updated_at)}</div>
						</a>
					{/each}
				{/if}
			</div>
		</section>
	</div>
{/if}
