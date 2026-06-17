<script lang="ts">
	import { getContext } from 'svelte';
	import { page } from '$app/state';
	import { repos, type Repository, type TagInfo } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { Plus, Tag, Trash2, X } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import SearchSelect from '$lib/components/SearchSelect.svelte';

	type RepoLayoutContext = {
		repo: Repository | null;
		branches: string[];
		currentBranch: string;
	};

	const repoLayout = getContext<RepoLayoutContext | null>('repoLayout');

	let tags = $state<TagInfo[]>([]);
	let loading = $state(true);
	let error = $state('');
	let showCreateModal = $state(false);
	let newTagName = $state('');
	let newTagMessage = $state('');
	let newTagFrom = $state('');
	let creating = $state(false);
	let createError = $state('');

	const { username, repo } = $derived(page.params);
	const isLoggedIn = $derived(authStore.user != null);
	const repoInfo = $derived(repoLayout?.repo ?? null);
	const branchOptions = $derived([...(repoLayout?.branches ?? [])].sort());
	const sortedTags = $derived([...tags].sort((left, right) => left.name.localeCompare(right.name)));

	async function loadTags() {
		loading = true;
		error = '';
		try {
			const data = await repos.tags.list(username!, repo!);
			tags = data.tags ?? [];
			if (!newTagFrom) {
				newTagFrom = repoLayout?.currentBranch || repoInfo?.default_branch || branchOptions[0] || '';
			}
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadTags();
	});

	async function handleCreate(e: Event) {
		e.preventDefault();
		creating = true;
		createError = '';
		try {
			await repos.tags.create(username!, repo!, newTagName, newTagFrom || undefined, newTagMessage || undefined);
			showCreateModal = false;
			newTagName = '';
			newTagMessage = '';
			await loadTags();
		} catch (e: any) {
			createError = e.message;
		} finally {
			creating = false;
		}
	}

	async function handleDelete(tagName: string) {
		if (!confirm(`Delete tag '${tagName}'?`)) return;
		try {
			await repos.tags.delete(username!, repo!, tagName);
			tags = tags.filter((tag) => tag.name !== tagName);
		} catch (e: any) {
			alert(e.message);
		}
	}

	function formatDate(value: string) {
		if (!value) return 'Unknown date';
		return new Intl.DateTimeFormat(undefined, {
			dateStyle: 'medium',
			timeStyle: 'short'
		}).format(new Date(value));
	}

	function shortSha(sha: string) {
		return sha ? sha.slice(0, 7) : 'unknown';
	}
</script>

<svelte:head>
	<title>Tags · {username}/{repo} · GitPier</title>
</svelte:head>

{#if showCreateModal}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
		<div class="w-full max-w-md rounded-md border border-border bg-card shadow-xl">
			<div class="flex items-center justify-between border-b border-secondary px-4 py-3">
				<h3 class="text-sm font-semibold text-foreground">Create a new tag</h3>
				<button onclick={() => (showCreateModal = false)} class="text-muted-foreground hover:text-foreground"><X class="h-4 w-4" /></button>
			</div>
			<form onsubmit={handleCreate} class="space-y-4 p-4">
				{#if createError}
					<p class="rounded-md border border-red-800/40 bg-red-900/20 px-3 py-2 text-sm text-red-400">{createError}</p>
				{/if}
				<div>
					<label class="mb-1.5 block text-xs font-semibold text-foreground">Tag name</label>
					<input
						bind:value={newTagName}
						required
						placeholder="v1.0.0"
						class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
					/>
				</div>
				<div>
					<label class="mb-1.5 block text-xs font-semibold text-foreground">From branch</label>
					{#if branchOptions.length > 0}
						<SearchSelect bind:value={newTagFrom} options={branchOptions.map((branch) => ({ value: branch }))} class="w-full" />
					{:else}
						<input
							bind:value={newTagFrom}
							placeholder={repoInfo?.default_branch || 'main'}
							class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
						/>
					{/if}
				</div>
				<div>
					<label class="mb-1.5 block text-xs font-semibold text-foreground">Message</label>
					<input
						bind:value={newTagMessage}
						placeholder="Release v1.0.0"
						class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
					/>
				</div>
				<div class="flex justify-end gap-2 pt-1">
					<Button variant="outline" type="button" onclick={() => (showCreateModal = false)}>Cancel</Button>
					<Button variant="brand" type="submit" disabled={creating || !newTagFrom}>
						{creating ? 'Creating…' : 'Create tag'}
					</Button>
				</div>
			</form>
		</div>
	</div>
{/if}

<div class="mb-4 flex items-center justify-between">
	<h2 class="text-sm font-semibold text-foreground">{tags.length} tag{tags.length !== 1 ? 's' : ''}</h2>
	{#if isLoggedIn}
		<Button variant="brand" size="sm" onclick={() => (showCreateModal = true)}>
			<Plus class="h-3.5 w-3.5" />
			New tag
		</Button>
	{/if}
</div>

{#if loading}
	<div class="space-y-1">
		{#each Array(4) as _}
			<div class="h-16 animate-pulse rounded-md border border-secondary bg-card"></div>
		{/each}
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else if tags.length === 0}
	<div class="rounded-md border border-border bg-card p-10 text-center">
		<Tag class="mx-auto mb-3 h-8 w-8 text-muted-foreground" />
		<p class="text-sm text-muted-foreground">No tags found.</p>
	</div>
{:else}
	<div class="divide-y divide-secondary overflow-hidden rounded-md border border-border">
		{#each sortedTags as tag}
			<div class="flex items-start gap-3 bg-card px-4 py-3 transition-colors hover:bg-accent">
				<Tag class="mt-0.5 h-4 w-4 shrink-0 text-muted-foreground" />
				<div class="min-w-0 flex-1">
					<div class="flex flex-wrap items-center gap-x-3 gap-y-1">
						<a href="/{username}/{repo}?ref={encodeURIComponent(tag.name)}" class="text-sm font-semibold text-primary hover:underline">
							{tag.name}
						</a>
						<a href="/{username}/{repo}/commit/{tag.commit_sha}" class="text-xs text-muted-foreground hover:text-foreground">
							{shortSha(tag.commit_sha)}
						</a>
						<span class="text-xs text-muted-foreground">{formatDate(tag.date)}</span>
					</div>
					{#if tag.message}
						<p class="mt-1 text-sm text-muted-foreground">{tag.message}</p>
					{/if}
				</div>
				{#if isLoggedIn}
					<button onclick={() => handleDelete(tag.name)} class="rounded-md p-1 text-muted-foreground transition-colors hover:bg-red-900/20 hover:text-red-400" aria-label="Delete tag">
						<Trash2 class="h-3.5 w-3.5" />
					</button>
				{/if}
			</div>
		{/each}
	</div>
{/if}
