<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { gitpierApps, type GitPierApp, type Organization } from '$lib/api/client';
	import { Boxes, Plus } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	const handle = page.params.username as string;
	const ctx = getContext<{ org: Organization | null; isOwner: boolean; loading: boolean }>('org');

	let apps = $state<GitPierApp[]>([]);
	let loading = $state(true);
	let error = $state('');

	onMount(async () => {
		try {
			const data = await gitpierApps.listOrgApps(handle);
			apps = data.apps ?? [];
		} catch (e: unknown) {
			error = (e as { message?: string }).message ?? 'Failed to load apps';
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>GitPier Apps · {handle}</title>
</svelte:head>

<div class="max-w-3xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-xl font-semibold text-foreground">GitPier Apps</h1>
			<p class="text-sm text-muted-foreground mt-1">
				Apps owned by <strong>{handle}</strong> that can act on their own behalf using fine-grained permissions and short-lived installation tokens.
			</p>
		</div>
		{#if ctx.isOwner}
			<Button variant="brand" size="sm" onclick={() => goto(`/${handle}/settings/apps/new`)}>
				<Plus class="h-4 w-4" />
				New GitPier App
			</Button>
		{/if}
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
			<Boxes class="h-10 w-10 text-muted-foreground mx-auto mb-3" />
			<p class="text-sm font-semibold text-foreground mb-1">No GitPier Apps</p>
			<p class="text-xs text-muted-foreground mb-4">Register a GitPier App owned by this organization to integrate with and extend GitPier.</p>
			{#if ctx.isOwner}
				<Button variant="brand" size="sm" onclick={() => goto(`/${handle}/settings/apps/new`)}>New GitPier App</Button>
			{/if}
		</div>
	{:else}
		<div class="divide-y divide-secondary rounded-md border border-border bg-card overflow-hidden">
			{#each apps as app}
				<a
					href={ctx.isOwner ? `/${handle}/settings/apps/${app.id}` : undefined}
					class="flex items-center gap-4 px-4 py-4 {ctx.isOwner ? 'hover:bg-secondary/50 cursor-pointer' : ''} transition-colors"
				>
					<div class="h-10 w-10 rounded-md border border-border bg-secondary flex items-center justify-center shrink-0 overflow-hidden">
						{#if app.logo_url}
							<img src={app.logo_url} alt={app.name} class="h-full w-full object-cover" />
						{:else}
							<Boxes class="h-5 w-5 text-muted-foreground" />
						{/if}
					</div>
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<p class="text-sm font-semibold text-foreground truncate">{app.name}</p>
							{#if app.is_public}
								<Badge variant="secondary" class="text-xs">Public</Badge>
							{:else}
								<Badge variant="outline" class="text-xs">Private</Badge>
							{/if}
							{#if app.installation_count > 0}
								<Badge variant="secondary" class="text-xs">
									{app.installation_count} installation{app.installation_count !== 1 ? 's' : ''}
								</Badge>
							{/if}
						</div>
						{#if app.description}
							<p class="text-xs text-muted-foreground mt-0.5 truncate">{app.description}</p>
						{/if}
						<p class="text-xs text-muted-foreground mt-0.5 font-mono">{app.slug}</p>
					</div>
					<div class="text-xs text-muted-foreground shrink-0">{app.key_count} key{app.key_count !== 1 ? 's' : ''}</div>
				</a>
			{/each}
		</div>
	{/if}
</div>
