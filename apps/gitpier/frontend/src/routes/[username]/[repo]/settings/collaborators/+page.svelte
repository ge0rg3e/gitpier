<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { repos, users, type Collaborator } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { UserPlus, Loader, Trash2, Users } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import SearchSelect from '$lib/components/SearchSelect.svelte';

	let collaborators = $state<Collaborator[]>([]);
	let loading = $state(true);
	let error = $state('');

	let collabUsername = $state('');
	let collabPermission = $state<'read' | 'write' | 'admin'>('write');
	let addingCollab = $state(false);
	let collabError = $state('');

	const username = $derived(page.params.username as string);
	const repoName = $derived(page.params.repo as string);

	onMount(async () => {
		while (authStore.loading) await new Promise((r) => setTimeout(r, 10));
		if (!authStore.isAuthenticated) {
			goto('/login');
			return;
		}
		try {
			const [repoData, collabData] = await Promise.all([repos.get(username, repoName), repos.collaborators.list(username, repoName).catch(() => ({ collaborators: [] }))]);
			if (repoData.repo.owner_id !== authStore.user?.id) {
				goto(`/${username}/${repoName}`);
				return;
			}
			collaborators = collabData.collaborators ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function addCollaborator(e: Event) {
		e.preventDefault();
		if (!collabUsername.trim()) return;
		addingCollab = true;
		collabError = '';
		try {
			const profile = await users.getProfile(collabUsername.trim());
			const collab = await repos.collaborators.add(username, repoName, profile.user.id, collabPermission);
			collaborators = [...collaborators, collab];
			collabUsername = '';
		} catch (e: any) {
			collabError = e.message;
		} finally {
			addingCollab = false;
		}
	}

	async function removeCollaborator(userID: number) {
		try {
			await repos.collaborators.remove(username, repoName, userID);
			collaborators = collaborators.filter((c) => c.user_id !== userID);
		} catch (e: any) {
			alert(e.message);
		}
	}
</script>

<svelte:head>
	<title>Collaborators</title>
</svelte:head>

<div class="max-w-2xl">
	<div class="mb-6">
		<h1 class="text-xl font-semibold text-foreground mb-1 flex items-center gap-2">
			<Users class="h-5 w-5 text-muted-foreground" />
			Collaborators
		</h1>
		<p class="text-sm text-muted-foreground">Manage who can access and contribute to this repository.</p>
	</div>

	{#if loading}
		<div class="text-center py-12 text-muted-foreground">Loading…</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else}
		<div class="rounded-md border border-border overflow-hidden">
			<div class="p-4 border-b border-border bg-card">
				<form onsubmit={addCollaborator} class="flex items-end gap-2 flex-wrap">
					<div class="flex-1 min-w-36">
						<p class="block text-xs font-semibold text-muted-foreground mb-1">Username</p>
						<input
							type="text"
							bind:value={collabUsername}
							placeholder="Enter a username"
							class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
						/>
					</div>
					<div>
						<p class="block text-xs font-semibold text-muted-foreground mb-1">Role</p>
						<SearchSelect
							bind:value={collabPermission}
							options={[
								{ value: 'read', label: 'Read' },
								{ value: 'write', label: 'Write' },
								{ value: 'admin', label: 'Admin' }
							]}
						/>
					</div>
					<Button variant="brand" size="sm" type="submit" disabled={addingCollab || !collabUsername.trim()}>
						{#if addingCollab}
							<Loader class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<UserPlus class="h-3.5 w-3.5" />
						{/if}
						Add collaborator
					</Button>
				</form>
				{#if collabError}
					<p class="mt-2 text-xs text-red-400">{collabError}</p>
				{/if}
			</div>

			{#if collaborators.length === 0}
				<div class="p-6 text-center text-sm text-muted-foreground bg-background">No collaborators yet.</div>
			{:else}
				<div class="divide-y divide-secondary bg-background">
					{#each collaborators as collab}
						<div class="flex items-center gap-3 px-4 py-3">
							<div class="h-8 w-8 rounded-full bg-secondary flex items-center justify-center text-xs font-bold text-primary">
								{collab.user.username[0]?.toUpperCase()}
							</div>
							<a href="/{collab.user.username}" class="flex-1 text-sm font-semibold text-primary hover:underline">{collab.user.username}</a>
							<span class="text-xs capitalize text-muted-foreground border border-border rounded-full px-2 py-0.5">{collab.permission}</span>
							<button type="button" onclick={() => removeCollaborator(collab.user_id)} class="text-muted-foreground hover:text-red-400 transition-colors" aria-label="Remove">
								<Trash2 class="h-4 w-4" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</div>
	{/if}
</div>
