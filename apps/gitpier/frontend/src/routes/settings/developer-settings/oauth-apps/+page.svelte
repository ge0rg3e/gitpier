<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { oauthApps, type OAuthApp } from '$lib/api/client';
	import { formatDate } from '$lib/utils';
	import { AppWindow, Plus, Loader, ExternalLink } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	let apps = $state<OAuthApp[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		try {
			const data = await oauthApps.listUserApps();
			apps = data.apps ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>OAuth Apps</title>
</svelte:head>

<div class="max-w-3xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold text-foreground">OAuth Apps</h1>
			<p class="text-sm text-muted-foreground mt-1">OAuth apps are used to access the API on behalf of users.</p>
		</div>
		<Button variant="brand" size="sm" onclick={() => goto('/settings/developer-settings/oauth-apps/new')}>
			<Plus class="h-4 w-4" />
			New OAuth app
		</Button>
	</div>

	{#if loading}
		<div class="space-y-3">
			{#each Array(2) as _}
				<div class="h-20 rounded-md border border-border bg-card animate-pulse"></div>
			{/each}
		</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if apps.length === 0}
		<div class="rounded-md border border-border bg-card p-10 text-center">
			<AppWindow class="h-10 w-10 text-muted-foreground mx-auto mb-3" />
			<p class="text-sm font-semibold text-foreground mb-1">No OAuth apps</p>
			<p class="text-xs text-muted-foreground mb-4">OAuth apps are used to access the API. Read the docs to find out more.</p>
			<Button variant="brand" size="sm" onclick={() => goto('/settings/developer-settings/oauth-apps/new')}>New OAuth app</Button>
		</div>
	{:else}
		<div class="divide-y divide-secondary rounded-md border border-border bg-card overflow-hidden">
			{#each apps as app}
				<a href="/settings/developer-settings/oauth-apps/{app.id}" class="flex items-center gap-4 px-4 py-4 hover:bg-secondary/50 transition-colors">
					<div class="h-10 w-10 rounded-md border border-border bg-secondary flex items-center justify-center shrink-0 overflow-hidden">
						{#if app.logo_url}
							<img src={app.logo_url} alt={app.name} class="h-full w-full object-cover" />
						{:else}
							<AppWindow class="h-5 w-5 text-muted-foreground" />
						{/if}
					</div>
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<p class="text-sm font-semibold text-foreground truncate">{app.name}</p>
							{#if app.authorization_count > 0}
								<Badge variant="secondary" class="text-xs">{app.authorization_count} user{app.authorization_count !== 1 ? 's' : ''}</Badge>
							{/if}
						</div>
						{#if app.description}
							<p class="text-xs text-muted-foreground mt-0.5 truncate">{app.description}</p>
						{/if}
						<div class="flex items-center gap-1 mt-0.5">
							<ExternalLink class="h-3 w-3 text-muted-foreground" />
							<p class="text-xs text-muted-foreground truncate">{app.homepage_url}</p>
						</div>
					</div>
					<p class="text-xs text-muted-foreground shrink-0">Created {formatDate(app.created_at)}</p>
				</a>
			{/each}
		</div>
	{/if}
</div>
