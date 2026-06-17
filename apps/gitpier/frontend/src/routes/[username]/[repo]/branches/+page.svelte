<script lang="ts">
	import { page } from '$app/state';
	import { repos } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { GitBranch, Plus, Trash2, X } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import SearchSelect from '$lib/components/SearchSelect.svelte';

	let branches = $state<string[]>([]);
	let loading = $state(true);
	let error = $state('');
	let showCreateModal = $state(false);
	let newBranchName = $state('');
	let newBranchFrom = $state('');
	let creating = $state(false);
	let createError = $state('');

	const { username, repo } = $derived(page.params);
	const isLoggedIn = $derived(authStore.user != null);
	const sortedBranches = $derived([...branches].sort());

	async function loadBranches() {
		loading = true;
		error = '';
		try {
			const data = await repos.branches.list(username!, repo!);
			branches = data.branches ?? [];
			if (!newBranchFrom && branches.length > 0) newBranchFrom = branches[0];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		loadBranches();
	});

	async function handleCreate(e: Event) {
		e.preventDefault();
		creating = true;
		createError = '';
		try {
			await repos.branches.create(username!, repo!, newBranchName, newBranchFrom);
			showCreateModal = false;
			newBranchName = '';
			await loadBranches();
		} catch (e: any) {
			createError = e.message;
		} finally {
			creating = false;
		}
	}

	async function handleDelete(branch: string) {
		if (!confirm(`Delete branch '${branch}'?`)) return;
		try {
			await repos.branches.delete(username!, repo!, branch);
			branches = branches.filter((b) => b !== branch);
		} catch (e: any) {
			alert(e.message);
		}
	}
</script>

<svelte:head>
	<title>Branches · {username}/{repo} · GitPier</title>
</svelte:head>

{#if showCreateModal}
	<div class="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
		<div class="w-full max-w-md rounded-md border border-border bg-card shadow-xl">
			<div class="flex items-center justify-between px-4 py-3 border-b border-secondary">
				<h3 class="text-sm font-semibold text-foreground">Create a new branch</h3>
				<button onclick={() => (showCreateModal = false)} class="text-muted-foreground hover:text-foreground"><X class="h-4 w-4" /></button>
			</div>
			<form onsubmit={handleCreate} class="p-4 space-y-4">
				{#if createError}
					<p class="text-sm text-red-400 bg-red-900/20 border border-red-800/40 rounded-md px-3 py-2">{createError}</p>
				{/if}
				<div>
					<label class="block text-xs font-semibold text-foreground mb-1.5">Branch name</label>
					<input
						bind:value={newBranchName}
						required
						placeholder="feature/my-new-branch"
						class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<div>
					<label class="block text-xs font-semibold text-foreground mb-1.5">From branch</label>
					<SearchSelect bind:value={newBranchFrom} options={sortedBranches.map((b) => ({ value: b }))} class="w-full" />
				</div>
				<div class="flex gap-2 justify-end pt-1">
					<Button variant="outline" type="button" onclick={() => (showCreateModal = false)}>Cancel</Button>
					<Button variant="brand" type="submit" disabled={creating}>
						{creating ? 'Creating…' : 'Create branch'}
					</Button>
				</div>
			</form>
		</div>
	</div>
{/if}

<div class="flex items-center justify-between mb-4">
	<h2 class="text-sm font-semibold text-foreground">{branches.length} branch{branches.length !== 1 ? 'es' : ''}</h2>
	{#if isLoggedIn}
		<Button variant="brand" size="sm" onclick={() => (showCreateModal = true)}>
			<Plus class="h-3.5 w-3.5" />
			New branch
		</Button>
	{/if}
</div>

{#if loading}
	<div class="space-y-1">
		{#each Array(4) as _}
			<div class="h-12 rounded-md border border-secondary bg-card animate-pulse"></div>
		{/each}
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else if branches.length === 0}
	<div class="rounded-md border border-border bg-card p-10 text-center">
		<GitBranch class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
		<p class="text-muted-foreground text-sm">No branches found.</p>
	</div>
{:else}
	<div class="rounded-md border border-border overflow-hidden divide-y divide-secondary">
		{#each sortedBranches as branch}
			<div class="flex items-center gap-3 px-4 py-3 bg-card hover:bg-accent transition-colors">
				<GitBranch class="h-4 w-4 text-muted-foreground shrink-0" />
				<a href="/{username}/{repo}?ref={branch}" class="flex-1 text-sm font-semibold text-primary hover:underline">
					{branch}
				</a>
				{#if isLoggedIn}
					<button onclick={() => handleDelete(branch)} class="text-muted-foreground hover:text-red-400 transition-colors p-1 rounded-md hover:bg-red-900/20" aria-label="Delete branch">
						<Trash2 class="h-3.5 w-3.5" />
					</button>
				{/if}
			</div>
		{/each}
	</div>
{/if}
