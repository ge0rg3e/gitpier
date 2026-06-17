<script lang="ts">
	import { page } from '$app/state';
	import { releases, type Release } from '$lib/api/client';
	import { timeAgo } from '$lib/utils';
	import { authStore } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { getContext } from 'svelte';
	import { renderMarkdownHtml } from '$lib/markdown';
	import { Tag, Download, Package, Pencil, Trash2, FileArchive, ChevronLeft, AlertCircle } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	const username = $derived(page.params.username);
	const repoName = $derived(page.params.repo);
	const releaseId = $derived(page.params.id ?? '');
	const repoCtx: any = getContext('repoLayout');
	const canAdmin = $derived(repoCtx?.repo && authStore.user && repoCtx.repo.owner_id === authStore.user.id);

	let release = $state<Release | null>(null);
	let loading = $state(true);
	let error = $state('');
	let deleting = $state(false);
	let deleteError = $state('');

	async function load() {
		loading = true;
		error = '';
		try {
			if (!releaseId.trim()) {
				throw new Error('Invalid release id.');
			}
			const data = await releases.get(username!, repoName!, releaseId);
			release = data.release;
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	const renderedBody = $derived(renderMarkdownHtml(release?.body));

	async function deleteRelease() {
		if (!release) return;
		if (!confirm(`Delete release "${release.name || release.tag_name}"? This cannot be undone.`)) return;
		deleting = true;
		deleteError = '';
		try {
			await releases.delete(username!, repoName!, release.id);
			goto(`/${username}/${repoName}/releases`);
		} catch (e: any) {
			deleteError = e.message;
			deleting = false;
		}
	}

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}
</script>

<div>
	<a href="/{username}/{repoName}/releases" class="flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground mb-6 transition-colors">
		<ChevronLeft class="h-4 w-4" /> Back to releases
	</a>

	{#if loading}
		<div class="animate-pulse space-y-4">
			<div class="h-7 w-64 bg-secondary rounded"></div>
			<div class="h-4 w-48 bg-secondary rounded"></div>
			<div class="h-40 w-full bg-secondary rounded"></div>
		</div>
	{:else if error}
		<div class="flex items-center gap-2 text-destructive text-sm p-4 border border-destructive/30 rounded-lg">
			<AlertCircle class="h-4 w-4 shrink-0" />{error}
		</div>
	{:else if release}
		<div class="flex items-start justify-between gap-4 mb-4 flex-wrap">
			<div class="flex items-center flex-wrap gap-2">
				<h1 class="text-2xl font-bold text-foreground">{release.name || release.tag_name}</h1>
				<span class="flex items-center gap-1 text-sm border border-border rounded-full px-2.5 py-0.5 text-muted-foreground font-mono">
					<Tag class="h-3.5 w-3.5" />{release.tag_name}
				</span>
				{#if release.is_prerelease}
					<span class="text-sm rounded-full bg-yellow-500/10 border border-yellow-500/30 text-yellow-600 dark:text-yellow-400 px-2.5 py-0.5 font-medium">Pre-release</span>
				{/if}
				{#if release.is_draft}
					<span class="text-sm rounded-full bg-secondary border border-border text-muted-foreground px-2.5 py-0.5 font-medium">Draft</span>
				{/if}
			</div>
			{#if canAdmin}
				<div class="flex items-center gap-2 shrink-0">
					<Button variant="outline" href="/{username}/{repoName}/releases/new?edit={release.id}" class="flex items-center gap-1.5 h-8 text-sm">
						<Pencil class="h-3.5 w-3.5" /> Edit
					</Button>
					<Button variant="destructive" onclick={deleteRelease} disabled={deleting} class="flex items-center gap-1.5 h-8 text-sm">
						<Trash2 class="h-3.5 w-3.5" />
						{deleting ? 'Deleting…' : 'Delete'}
					</Button>
				</div>
			{/if}
		</div>

		<p class="text-sm text-muted-foreground mb-6">
			{#if release.is_draft}
				Draft — created by <a href="/{release.created_by?.username}" class="text-foreground hover:underline">{release.created_by?.username ?? 'unknown'}</a> {timeAgo(release.created_at)}
			{:else}
				Released by <a href="/{release.created_by?.username}" class="text-foreground hover:underline">{release.created_by?.username ?? 'unknown'}</a> on {new Date(
					release.published_at ?? release.created_at
				).toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' })}
			{/if}
		</p>

		{#if deleteError}
			<div class="mb-4 p-3 border border-destructive/30 rounded-lg text-sm text-destructive">{deleteError}</div>
		{/if}

		<div class="grid grid-cols-1 lg:grid-cols-3 gap-6">
			<!-- Release notes (left/main) -->
			<div class="lg:col-span-2">
				<div class="border border-border rounded-lg p-6">
					{#if renderedBody}
						<div class="prose prose-invert prose-sm max-w-none" style="font-size:0.9rem">
							{@html renderedBody}
						</div>
					{:else}
						<p class="text-sm text-muted-foreground italic">No release notes provided.</p>
					{/if}
				</div>
			</div>

			<!-- Assets sidebar (right) -->
			<div class="space-y-4">
				<!-- Binary assets -->
				<div class="border border-border rounded-lg overflow-hidden">
					<div class="px-4 py-3 border-b border-border bg-muted/30">
						<h3 class="text-sm font-semibold text-foreground flex items-center gap-1.5">
							<Package class="h-4 w-4 text-muted-foreground" /> Assets
							{#if release.assets && release.assets.length > 0}
								<span class="ml-auto text-xs text-muted-foreground font-normal">{release.assets.length} file{release.assets.length !== 1 ? 's' : ''}</span>
							{/if}
						</h3>
					</div>
					{#if release.assets && release.assets.length > 0}
						<ul class="divide-y divide-border">
							{#each release.assets as asset}
								<li class="px-4 py-3 flex items-center gap-3">
									<a href={releases.downloadAssetUrl(username!, repoName!, asset.id)} class="flex-1 min-w-0 group">
										<p class="text-sm text-foreground group-hover:text-brand truncate transition-colors font-medium">{asset.name}</p>
										<p class="text-xs text-muted-foreground">{formatBytes(asset.size)} · {asset.download_count} download{asset.download_count !== 1 ? 's' : ''}</p>
									</a>
									<a href={releases.downloadAssetUrl(username!, repoName!, asset.id)} class="shrink-0 text-muted-foreground hover:text-foreground transition-colors" title="Download">
										<Download class="h-4 w-4" />
									</a>
								</li>
							{/each}
						</ul>
					{:else}
						<p class="px-4 py-3 text-sm text-muted-foreground">No binary assets.</p>
					{/if}
				</div>

				<!-- Source archives -->
				<div class="border border-border rounded-lg overflow-hidden">
					<div class="px-4 py-3 border-b border-border bg-muted/30">
						<h3 class="text-sm font-semibold text-foreground flex items-center gap-1.5">
							<FileArchive class="h-4 w-4 text-muted-foreground" /> Source code
						</h3>
					</div>
					<ul class="divide-y divide-border">
						<li class="px-4 py-3">
							<a href={releases.sourceZipUrl(username!, repoName!, release.id)} class="flex items-center gap-2 text-sm text-foreground hover:text-brand transition-colors">
								<Download class="h-4 w-4 text-muted-foreground" />
								Source code (zip)
							</a>
						</li>
						<li class="px-4 py-3">
							<a href={releases.sourceTarUrl(username!, repoName!, release.id)} class="flex items-center gap-2 text-sm text-foreground hover:text-brand transition-colors">
								<Download class="h-4 w-4 text-muted-foreground" />
								Source code (tar.gz)
							</a>
						</li>
					</ul>
				</div>
			</div>
		</div>
	{/if}
</div>
