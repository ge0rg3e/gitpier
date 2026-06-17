<script lang="ts">
	import { page } from '$app/state';
	import { releases, type Release } from '$lib/api/client';
	import { timeAgo } from '$lib/utils';
	import { authStore } from '$lib/stores/auth.svelte';
	import { getContext } from 'svelte';
	import { Tag, Download, Package, Plus, FileArchive } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	const { username, repo: repoName } = $derived(page.params);
	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const canAdmin = $derived(repoCtx?.repo && authStore.user && repoCtx.repo.owner_id === authStore.user.id && !isRepoArchived);

	let releaseList = $state<Release[]>([]);
	let loading = $state(true);
	let error = $state('');

	async function load() {
		loading = true;
		error = '';
		try {
			const data = await releases.list(username!, repoName!);
			releaseList = data.releases ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}
</script>

<div>
	<div class="flex items-center justify-between mb-6">
		<h2 class="text-xl font-semibold text-foreground">Releases</h2>
		{#if canAdmin}
			<Button variant="brand" href="/{username}/{repoName}/releases/new" class="flex items-center gap-1.5">
				<Plus class="h-4 w-4" />
				New release
			</Button>
		{/if}
	</div>

	{#if loading}
		<div class="space-y-4">
			{#each [1, 2, 3] as _}
				<div class="border border-border rounded-lg p-6 animate-pulse">
					<div class="h-5 w-48 bg-secondary rounded mb-3"></div>
					<div class="h-4 w-32 bg-secondary rounded mb-2"></div>
					<div class="h-4 w-full bg-secondary rounded"></div>
				</div>
			{/each}
		</div>
	{:else if error}
		<p class="text-sm text-destructive">{error}</p>
	{:else if releaseList.length === 0}
		<div class="border border-dashed border-border rounded-lg p-12 text-center">
			<Tag class="h-10 w-10 text-muted-foreground mx-auto mb-4" />
			<h3 class="text-base font-semibold text-foreground mb-1">No releases yet</h3>
			<p class="text-sm text-muted-foreground mb-4">Create a release to package your project for distribution.</p>
			{#if canAdmin}
				<Button variant="brand" href="/{username}/{repoName}/releases/new">Create your first release</Button>
			{/if}
		</div>
	{:else}
		<div class="space-y-0 divide-y divide-border border border-border rounded-lg overflow-hidden">
			{#each releaseList as release}
				<div class="p-6 hover:bg-muted/30 transition-colors">
					<div class="flex items-start justify-between gap-4">
						<div class="min-w-0 flex-1">
							<!-- Tag + badges -->
							<div class="flex items-center flex-wrap gap-2 mb-1">
								<a href="/{username}/{repoName}/releases/{release.id}" class="text-lg font-semibold text-foreground hover:text-brand transition-colors leading-tight">
									{release.name || release.tag_name}
								</a>
								<span class="flex items-center gap-1 text-xs border border-border rounded-full px-2 py-0.5 text-muted-foreground font-mono">
									<Tag class="h-3 w-3" />{release.tag_name}
								</span>
								{#if release.is_prerelease}
									<span class="text-xs rounded-full bg-yellow-500/10 border border-yellow-500/30 text-yellow-600 dark:text-yellow-400 px-2 py-0.5 font-medium">Pre-release</span>
								{/if}
								{#if release.is_draft}
									<span class="text-xs rounded-full bg-secondary border border-border text-muted-foreground px-2 py-0.5 font-medium">Draft</span>
								{/if}
							</div>

							<!-- Meta: author + date -->
							<p class="text-xs text-muted-foreground mb-3">
								{#if release.is_draft}
									Draft — created by {release.created_by?.username ?? 'unknown'} {timeAgo(release.created_at)}
								{:else}
									Released by {release.created_by?.username ?? 'unknown'} {timeAgo(release.published_at ?? release.created_at)}
								{/if}
							</p>

							<!-- Assets summary -->
							{#if release.assets && release.assets.length > 0}
								<div class="flex items-center gap-4 flex-wrap text-xs text-muted-foreground">
									<span class="flex items-center gap-1">
										<Package class="h-3.5 w-3.5" />
										{release.assets.length} asset{release.assets.length !== 1 ? 's' : ''}
									</span>
									{#each release.assets.slice(0, 3) as asset}
										<a href={releases.downloadAssetUrl(username!, repoName!, asset.id)} class="flex items-center gap-1 hover:text-foreground transition-colors">
											<Download class="h-3.5 w-3.5" />
											{asset.name} ({formatBytes(asset.size)})
										</a>
									{/each}
									{#if release.assets.length > 3}
										<a href="/{username}/{repoName}/releases/{release.id}" class="hover:text-foreground transition-colors">
											+{release.assets.length - 3} more
										</a>
									{/if}
								</div>
							{/if}

							<!-- Source archives -->
							<div class="flex items-center gap-3 mt-2 text-xs text-muted-foreground">
								<span class="flex items-center gap-1 font-medium text-foreground">
									<FileArchive class="h-3.5 w-3.5" />Source code:
								</span>
								<a href={releases.sourceZipUrl(username!, repoName!, release.id)} class="hover:text-foreground transition-colors">zip</a>
								<a href={releases.sourceTarUrl(username!, repoName!, release.id)} class="hover:text-foreground transition-colors">tar.gz</a>
							</div>
						</div>

						<a href="/{username}/{repoName}/releases/{release.id}" class="shrink-0 text-sm text-muted-foreground hover:text-foreground transition-colors"> View → </a>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
