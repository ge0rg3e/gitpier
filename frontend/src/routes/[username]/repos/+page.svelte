<script lang="ts">
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { type Repository } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { timeAgo } from '$lib/utils';
	import { Globe, Lock, Book, Star as StarIcon } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	const userProfileCtx = getContext<{ profile: any; repos: Repository[]; loading: boolean }>('userProfile');
	const username = $derived(page.params.username);
	const isOwn = $derived(authStore.user?.username === username);

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
		CSS: '#563d7c',
		Dart: '#00B4AB',
		Scala: '#c22d40',
		Haskell: '#5e5086'
	};
	function langColor(lang: string): string {
		return LANG_COLORS[lang] ?? '#8b949e';
	}
	function getRepoOwner(r: Repository, fallback: string) {
		return r.owner?.username ?? fallback;
	}
</script>

<svelte:head>
	<title>{username} · Repositories · GitPier</title>
</svelte:head>

{#if userProfileCtx.loading}
	<div class="space-y-3">
		{#each Array(5) as _}
			<div class="h-20 bg-card rounded-md border border-secondary animate-pulse"></div>
		{/each}
	</div>
{:else if userProfileCtx.repos.length === 0}
	<div class="rounded-md border border-border bg-card p-10 text-center">
		<Book class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
		<p class="text-muted-foreground text-sm">
			{isOwn ? "You don't have any repositories yet." : `${username} doesn't have any public repositories.`}
		</p>
		{#if isOwn}
			<div class="mt-4"><Button variant="brand" size="sm" href="/new">Create your first repo</Button></div>
		{/if}
	</div>
{:else}
	<div class="space-y-3">
		{#each userProfileCtx.repos as repo}
			{@const repoOwner = getRepoOwner(repo, username!)}
			<div class="rounded-md border border-secondary bg-card p-4 hover:border-border transition-colors">
				<div class="flex items-start gap-4">
					<div class="min-w-0 flex-1">
						<div class="flex items-center gap-2 flex-wrap">
							<a href="/{repoOwner}/{repo.name}" class="text-base font-semibold text-primary hover:underline truncate">{repo.name}</a>
							<span
								class={`inline-flex items-center gap-1 rounded-full border px-2 py-0.5 text-xs ${
									repo.is_archived ? 'border-amber-700/40 bg-amber-900/20 text-amber-300' : 'border-border text-muted-foreground'
								}`}
							>
								{#if repo.is_private}<Lock class="h-2.5 w-2.5" />{:else}<Globe class="h-2.5 w-2.5" />{/if}
								{repo.is_private ? 'Private' : 'Public'}{repo.is_archived ? ' archive' : ''}
							</span>
						</div>
						{#if repo.description}
							<p class="text-xs text-muted-foreground mt-1">{repo.description}</p>
						{/if}
					</div>
				</div>
				<div class="mt-3 flex items-center gap-4 text-xs text-muted-foreground">
					{#if repo.language}
						<span class="flex items-center gap-1.5">
							<span class="h-2.5 w-2.5 rounded-full shrink-0" style="background-color:{langColor(repo.language)}"></span>
							{repo.language}
						</span>
					{/if}
					{#if (repo.star_count ?? 0) > 0}
						<span class="flex items-center gap-1"><StarIcon class="h-3 w-3" />{repo.star_count}</span>
					{/if}
					<span>Updated {timeAgo(repo.updated_at)}</span>
				</div>
			</div>
		{/each}
	</div>
{/if}
