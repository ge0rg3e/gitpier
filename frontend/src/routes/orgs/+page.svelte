<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { orgs, type Organization } from '$lib/api/client';
	import { Building2, Plus, Settings } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { mediaUrl } from '$lib/utils';

	let myOrgs = $state<Organization[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(() => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		(async () => {
			try {
				myOrgs = await orgs.listMyOrgs();
			} catch (e: any) {
				error = e.message ?? 'Failed to load organizations';
			} finally {
				loading = false;
			}
		})();
	});
</script>

<svelte:head>
	<title>Your organizations · GitPier</title>
</svelte:head>

<div class="bg-background min-h-screen py-8 px-4">
	<div class="mx-auto max-w-screen-lg">
		<div class="flex items-center justify-between mb-6">
			<h1 class="text-2xl font-semibold text-foreground">Your organizations</h1>
			<Button variant="brand" size="sm" href="/orgs/new">
				<Plus class="h-3.5 w-3.5" />New organization
			</Button>
		</div>

		{#if loading}
			<div class="divide-y divide-secondary rounded-md border border-border">
				{#each Array(3) as _}
					<div class="px-4 py-4 animate-pulse bg-card">
						<div class="h-5 bg-secondary rounded w-48 mb-2"></div>
						<div class="h-3 bg-secondary rounded w-72"></div>
					</div>
				{/each}
			</div>
		{:else if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
		{:else if myOrgs.length === 0}
			<div class="rounded-md border border-border bg-card p-16 text-center">
				<Building2 class="mx-auto h-10 w-10 text-muted-foreground mb-4" />
				<p class="text-foreground font-semibold text-lg mb-1">No organizations yet</p>
				<p class="text-muted-foreground text-sm mb-6">Organizations let you collaborate with many people across many projects.</p>
				<Button variant="brand" href="/orgs/new">
					<Plus class="h-4 w-4" />Create your first organization
				</Button>
			</div>
		{:else}
			<div class="divide-y divide-secondary rounded-md border border-border overflow-hidden">
				{#each myOrgs as org}
					<div class="flex items-center gap-4 px-4 py-4 bg-card hover:bg-card/80 transition-colors">
						<a href="/{org.login}">
							<div class="h-12 w-12 rounded-md border border-border bg-secondary flex items-center justify-center overflow-hidden shrink-0">
								{#if org.avatar_url}
									<img src={mediaUrl(org.avatar_url)} alt={org.login} class="h-full w-full object-cover" />
								{:else}
									<span class="text-lg font-bold text-primary">{org.login[0].toUpperCase()}</span>
								{/if}
							</div>
						</a>
						<div class="flex-1 min-w-0">
							<a href="/{org.login}" class="text-sm font-semibold text-primary hover:underline">{org.display_name || org.login}</a>
							{#if org.display_name}
								<p class="text-xs text-muted-foreground">@{org.login}</p>
							{/if}
							{#if org.description}
								<p class="text-xs text-muted-foreground mt-0.5 truncate">{org.description}</p>
							{/if}
						</div>
						<div class="flex items-center gap-2 shrink-0">
							<a href="/{org.login}" class="text-xs text-primary hover:underline border border-border rounded-md px-3 py-1">View</a>
							<a
								href="/{org.login}/settings"
								class="h-8 w-8 flex items-center justify-center rounded-md border border-border text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors"
								title="Settings"
							>
								<Settings class="h-3.5 w-3.5" />
							</a>
						</div>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
