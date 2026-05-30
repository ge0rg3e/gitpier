<script lang="ts">
	import { page } from '$app/state';
	import { starred, type Star } from '$lib/api/client';
	import { timeAgo } from '$lib/utils';
	import { Lock, Star as StarIcon } from '@lucide/svelte';

	const username = $derived(page.params.username);

	let loading = $state(true);
	let starredRepos = $state<Star[]>([]);

	$effect(() => {
		const u = username;
		loading = true;
		starredRepos = [];

		starred
			.listForUser(u!)
			.then((d) => {
				if (username !== u) return;
				starredRepos = d.stars ?? [];
			})
			.catch(() => {})
			.finally(() => {
				if (username === u) loading = false;
			});
	});

	function timeAgoStr(dateStr: string) {
		return timeAgo(dateStr);
	}

	const LANG_COLORS: Record<string, string> = {
		Go: '#00ADD8',
		JavaScript: '#f1e05a',
		TypeScript: '#3178c6',
		Python: '#3572A5',
		Ruby: '#701516',
		Rust: '#dea584',
		Java: '#b07219',
		Kotlin: '#A97BFF',
		Swift: '#F05138',
		'C#': '#178600',
		'C++': '#f34b7d',
		C: '#555555',
		PHP: '#4F5D95',
		Shell: '#89e051',
		HTML: '#e34c26',
		CSS: '#563d7c'
	};
	function langColor(lang: string): string {
		return LANG_COLORS[lang] ?? '#8b949e';
	}
</script>

<svelte:head>
	<title>{username} · Stars · GitPier</title>
</svelte:head>

{#if loading}
	<div class="space-y-3">
		{#each Array(5) as _}
			<div class="h-20 bg-card rounded-md border border-secondary animate-pulse"></div>
		{/each}
	</div>
{:else if starredRepos.length === 0}
	<div class="rounded-md border border-border bg-card p-10 text-center">
		<StarIcon class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
		<p class="text-muted-foreground text-sm">{username} hasn't starred any repositories yet.</p>
	</div>
{:else}
	<div class="space-y-3">
		{#each starredRepos as star}
			{@const repo = star.repo}
			{@const repoOwner = repo.org?.username ?? repo.owner?.username ?? username}
			<div class="rounded-md border border-secondary bg-card p-4 hover:border-border transition-colors">
				<div class="min-w-0">
					<div class="flex items-center gap-2 flex-wrap">
						<a href="/{repoOwner}/{repo.name}" class="text-base font-semibold text-primary hover:underline truncate">{repoOwner}/{repo.name}</a>
						<span class="inline-flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">
							{#if repo.is_private}<Lock class="h-2.5 w-2.5" />Private{:else}Public{/if}
						</span>
					</div>
					{#if repo.description}
						<p class="text-xs text-muted-foreground mt-1">{repo.description}</p>
					{/if}
				</div>
				<div class="mt-3 flex items-center gap-4 text-xs text-muted-foreground">
					{#if repo.language}
						<span class="flex items-center gap-1.5">
							<span class="h-2.5 w-2.5 rounded-full shrink-0" style="background-color:{langColor(repo.language)}"></span>
							{repo.language}
						</span>
					{/if}
					Updated {timeAgoStr(repo.updated_at)}
				</div>
			</div>
		{/each}
	</div>
{/if}
