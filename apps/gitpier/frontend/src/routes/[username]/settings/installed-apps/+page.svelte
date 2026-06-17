<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getContext } from 'svelte';
	import { gitpierApps, type AppInstallation, type Organization } from '$lib/api/client';
	import { formatDate } from '$lib/utils';
	import { Boxes, ExternalLink, Loader, Settings } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	const handle = page.params.username as string;
	const ctx = getContext<{ org: Organization | null; isOwner: boolean; loading: boolean }>('org');

	let installations = $state<AppInstallation[]>([]);
	let loading = $state(true);
	let error = $state('');
	let uninstalling = $state<number | null>(null);

	$effect(() => {
		if (!ctx.loading && !ctx.isOwner) {
			goto(`/${handle}`);
		}
	});

	onMount(async () => {
		try {
			const data = await gitpierApps.listOrgInstallations(handle);
			installations = data.installations ?? [];
		} catch (e: unknown) {
			error = (e as { message?: string }).message ?? 'Failed to load installations';
		} finally {
			loading = false;
		}
	});

	async function handleUninstall(inst: AppInstallation) {
		uninstalling = inst.id;
		try {
			await gitpierApps.uninstall(inst.id);
			installations = installations.filter((i) => i.id !== inst.id);
		} catch (e: unknown) {
			error = (e as { message?: string }).message ?? 'Failed to uninstall';
		} finally {
			uninstalling = null;
		}
	}
</script>

<svelte:head>
	<title>Installed GitPier Apps · {handle}</title>
</svelte:head>

<div class="max-w-3xl">
	<div class="mb-6">
		<h1 class="text-2xl font-semibold text-foreground">Installed GitPier Apps</h1>
		<p class="text-sm text-muted-foreground mt-1">
			Apps installed on <strong>{handle}</strong> can access repositories and perform actions on behalf of the organization.
		</p>
	</div>

	{#if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400 mb-4">{error}</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			{#each Array(3) as _}
				<div class="h-20 rounded-md border border-border bg-card animate-pulse"></div>
			{/each}
		</div>
	{:else if installations.length === 0}
		<div class="rounded-md border border-border bg-card p-10 text-center">
			<Boxes class="h-8 w-8 text-muted-foreground mx-auto mb-3" />
			<p class="text-sm font-semibold text-foreground mb-1">No installed apps</p>
			<p class="text-xs text-muted-foreground">No GitPier Apps have been installed on this organization yet.</p>
		</div>
	{:else}
		<div class="divide-y divide-border rounded-md border border-border bg-card overflow-hidden">
			{#each installations as inst}
				<div class="flex items-center gap-4 px-4 py-4">
					<!-- App logo -->
					<div class="h-10 w-10 rounded-md border border-border bg-secondary flex items-center justify-center shrink-0 overflow-hidden">
						{#if inst.app?.logo_url}
							<img src={inst.app.logo_url} alt={inst.app.name} class="h-full w-full object-cover" />
						{:else}
							<Boxes class="h-5 w-5 text-muted-foreground" />
						{/if}
					</div>

					<!-- App info -->
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2">
							<p class="text-sm font-semibold text-foreground">{inst.app?.name ?? 'Unknown app'}</p>
							{#if inst.suspended_at}
								<Badge variant="destructive" class="text-xs">Suspended</Badge>
							{/if}
						</div>
						{#if inst.app?.description}
							<p class="text-xs text-muted-foreground mt-0.5 truncate">{inst.app.description}</p>
						{/if}
						<p class="text-xs text-muted-foreground mt-0.5">
							{inst.repository_selection === 'all' ? 'All repositories' : 'Selected repositories'} · Installed {formatDate(inst.created_at)}
						</p>
					</div>

					<!-- Actions -->
					<div class="flex items-center gap-2 shrink-0">
						{#if inst.app?.homepage_url}
							<a
								href={inst.app.homepage_url}
								target="_blank"
								rel="noopener noreferrer"
								class="inline-flex items-center justify-center h-8 w-8 text-muted-foreground hover:text-foreground transition-colors"
								title="Visit homepage"
							>
								<ExternalLink class="h-3.5 w-3.5" />
							</a>
						{/if}
						<Button variant="outline" size="sm" class="gap-1.5" onclick={() => goto(`/${handle}/settings/installed-apps/${inst.id}`)}>
							<Settings class="h-3.5 w-3.5" />
							Configure
						</Button>
						<Button
							variant="outline"
							size="sm"
							onclick={() => handleUninstall(inst)}
							disabled={uninstalling === inst.id}
							class="text-red-400 hover:text-red-300 hover:border-red-800/50 hover:bg-red-900/20"
						>
							{#if uninstalling === inst.id}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
							Uninstall
						</Button>
					</div>
				</div>
			{/each}
		</div>
	{/if}
</div>
