<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { dashboard, users, type DashboardActivityRepo, type DashboardPullRequest, type Repository } from '$lib/api/client';
	import { timeAgo } from '$lib/utils';
	import { GitPullRequest, CircleDot, ListChecks } from '@lucide/svelte';
	import ContributionGraph from '$lib/components/ContributionGraph.svelte';

	let loading = $state(false);
	let error = $state('');
	let openPullsCount = $state(0);
	let openIssuesCount = $state(0);
	let reviewRequestsCount = $state(0);
	let recentPulls = $state<DashboardPullRequest[]>([]);
	let recentActivityRepos = $state<DashboardActivityRepo[]>([]);
	let profileRepos = $state<Repository[]>([]);
	let contributions = $state<Record<string, number>>({});

	async function loadOverview() {
		if (!authStore.user?.id) return;

		loading = true;
		error = '';
		openPullsCount = 0;
		openIssuesCount = 0;
		reviewRequestsCount = 0;
		recentPulls = [];
		recentActivityRepos = [];
		profileRepos = [];
		contributions = {};

		try {
			const data = await dashboard.overview(16);
			openPullsCount = data.open_pull_requests ?? 0;
			openIssuesCount = data.open_issues ?? 0;
			reviewRequestsCount = data.review_requests ?? 0;
			recentPulls = data.recent_pull_requests ?? [];
			recentActivityRepos = data.recent_activity_repos ?? [];

			const username = authStore.user?.username;
			if (username) {
				const [profile, contributionData] = await Promise.all([
					users.getProfile(username, { limit: 24, offset: 0 }),
					users.getContributions(username).catch(() => ({ contributions: {} as Record<string, number> }))
				]);
				profileRepos = profile.repos ?? [];
				contributions = contributionData.contributions ?? {};
			}
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
				return;
			}
			goto('/login?redirect=/', { replaceState: true });
		}, 80);
	});

	const welcomeName = $derived.by(() => {
		const display = authStore.user?.display_name?.trim();
		if (display) return display.split(' ')[0];
		return authStore.user?.username ?? 'Local';
	});

	type ContributionActivity = { date: string; count: number };

	function buildFallbackContributions(repos: Repository[]): Record<string, number> {
		const fallback: Record<string, number> = {};
		for (const repo of repos) {
			const date = new Date(repo.updated_at);
			if (Number.isNaN(date.getTime())) continue;
			const day = date.toISOString().slice(0, 10);
			fallback[day] = (fallback[day] ?? 0) + 1;
		}
		return fallback;
	}

	function buildYearActivity(contribs: Record<string, number>): ContributionActivity[] {
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		const start = new Date(today);
		start.setDate(start.getDate() - 52 * 7);
		start.setDate(start.getDate() - start.getDay());
		const out: ContributionActivity[] = [];
		for (const cur = new Date(start); cur <= today; cur.setDate(cur.getDate() + 1)) {
			const y = cur.getFullYear();
			const m = String(cur.getMonth() + 1).padStart(2, '0');
			const d = String(cur.getDate()).padStart(2, '0');
			const key = `${y}-${m}-${d}`;
			out.push({ date: key, count: contribs[key] ?? 0 });
		}
		return out;
	}

	const displayContributions = $derived.by(() => {
		if (Object.keys(contributions).length > 0) return contributions;
		return buildFallbackContributions(profileRepos);
	});
	const contributionActivity = $derived(buildYearActivity(displayContributions));
	const totalContributions = $derived.by(() => contributionActivity.reduce((sum, day) => sum + day.count, 0));
	const contributionTitleTemplate = '{{count}} contributions in the last year';

	const recentInteractionRepos = $derived.by(() => {
		const seen = new Set<string>();
		const repos: Array<{ owner: string; name: string; updated_at: string; prs: number }> = [];

		for (const pr of [...recentPulls].sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())) {
			const key = `${pr.repo_owner}/${pr.repo_name}`;
			if (seen.has(key)) continue;
			seen.add(key);

			repos.push({
				owner: pr.repo_owner,
				name: pr.repo_name,
				updated_at: pr.updated_at,
				prs: recentPulls.filter((item) => item.repo_owner === pr.repo_owner && item.repo_name === pr.repo_name).length
			});
		}

		for (const repo of recentActivityRepos) {
			if (repos.length >= 8) break;
			const key = `${repo.owner}/${repo.name}`;
			if (seen.has(key)) continue;
			seen.add(key);
			repos.push({
				owner: repo.owner,
				name: repo.name,
				updated_at: repo.updated_at,
				prs: 0
			});
		}

		if (repos.length > 0) return repos;

		return [...profileRepos]
			.sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime())
			.slice(0, 8)
			.map((repo) => ({
				owner: repo.org?.login || repo.owner?.username || authStore.user?.username || 'unknown',
				name: repo.name,
				updated_at: repo.updated_at,
				prs: 0
			}));
	});
</script>

<svelte:head>
	<title>Overview · GitPier</title>
</svelte:head>

{#if authStore.isAuthenticated}
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

		<section class="mb-8">
			<h2 class="mb-3 text-xl font-medium text-muted-foreground">Activity</h2>
			<ContributionGraph
				data={contributionActivity}
				totalCount={totalContributions}
				labels={{
					totalCount: contributionTitleTemplate,
					legend: { less: 'Less', more: 'More' }
				}}
			/>
		</section>

		<section>
			<h2 class="mb-3 text-xl font-medium text-muted-foreground">Recent Activity Repos</h2>
			<div class="overflow-hidden rounded-2xl border border-border bg-card">
				{#if loading}
					<div class="h-20 bg-secondary/20 animate-pulse"></div>
				{:else if recentInteractionRepos.length === 0}
					<div class="px-5 py-8 text-center text-sm text-muted-foreground">No repository interactions yet.</div>
				{:else}
					{#each recentInteractionRepos as repo}
						<a href="/{repo.owner}/{repo.name}" class="flex items-center gap-4 border-b border-border/70 px-4 py-3 transition-colors hover:bg-secondary/30 last:border-b-0">
							<div class="text-brand">
								<GitPullRequest class="h-4 w-4" />
							</div>
							<div class="min-w-0 flex-1">
								<p class="truncate text-sm font-medium text-foreground">{repo.owner}/{repo.name}</p>
								<p class="text-xs text-muted-foreground">
									{repo.prs > 0 ? `${repo.prs} recent pull request${repo.prs === 1 ? '' : 's'}` : 'Recently updated repository'}
								</p>
							</div>
							<div class="shrink-0 text-xs text-muted-foreground">{timeAgo(repo.updated_at)}</div>
						</a>
					{/each}
				{/if}
			</div>
		</section>
	</div>
{/if}
