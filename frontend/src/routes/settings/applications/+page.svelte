<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { oauthApps, gitpierApps, type OAuthAuthorization, type AppInstallation } from '$lib/api/client';
	import { formatDate } from '$lib/utils';
	import { AppWindow, ExternalLink, Loader, Boxes, Settings } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	let authorizations = $state<OAuthAuthorization[]>([]);
	let installations = $state<AppInstallation[]>([]);
	let loading = $state(true);
	let error = $state('');
	let revoking = $state<number | null>(null);
	let uninstalling = $state<number | null>(null);

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		try {
			const [authData, instData] = await Promise.all([oauthApps.listAuthorizedApps(), gitpierApps.listUserInstallations()]);
			authorizations = authData.authorizations ?? [];
			installations = instData.installations ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function handleRevoke(auth: OAuthAuthorization) {
		revoking = auth.id;
		try {
			await oauthApps.revokeAuthorization(auth.id);
			authorizations = authorizations.filter((a) => a.id !== auth.id);
		} catch (e: any) {
			error = e.message;
		} finally {
			revoking = null;
		}
	}

	async function handleUninstall(inst: AppInstallation) {
		uninstalling = inst.id;
		try {
			await gitpierApps.uninstall(inst.id);
			installations = installations.filter((i) => i.id !== inst.id);
		} catch (e: any) {
			error = e.message;
		} finally {
			uninstalling = null;
		}
	}
</script>

<svelte:head>
	<title>Applications</title>
</svelte:head>

<div class="max-w-3xl space-y-10">
	<!-- GitPier App Installations -->
	<div>
		<div class="mb-6">
			<h1 class="text-2xl font-semibold text-foreground">Installed GitPier Apps</h1>
			<p class="text-sm text-muted-foreground mt-1">Apps installed on your account can access repositories and perform actions on your behalf.</p>
		</div>

		{#if loading}
			<div class="space-y-3">
				{#each Array(2) as _}
					<div class="h-20 rounded-md border border-border bg-card animate-pulse"></div>
				{/each}
			</div>
		{:else if installations.length === 0}
			<div class="rounded-md border border-border bg-card p-8 text-center">
				<Boxes class="h-8 w-8 text-muted-foreground mx-auto mb-3" />
				<p class="text-sm font-semibold text-foreground mb-1">No installed apps</p>
				<p class="text-xs text-muted-foreground">You haven't installed any GitPier Apps yet.</p>
			</div>
		{:else}
			<div class="divide-y divide-secondary rounded-md border border-border bg-card overflow-hidden">
				{#each installations as inst}
					<div class="flex items-center gap-4 px-4 py-4">
						<div class="h-10 w-10 rounded-md border border-border bg-secondary flex items-center justify-center shrink-0 overflow-hidden">
							{#if inst.app?.logo_url}
								<img src={inst.app.logo_url} alt={inst.app.name} class="h-full w-full object-cover" />
							{:else}
								<Boxes class="h-5 w-5 text-muted-foreground" />
							{/if}
						</div>
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
						<div class="flex gap-2 shrink-0">
							{#if inst.app?.homepage_url}
								<a
									href={inst.app.homepage_url}
									target="_blank"
									rel="noopener noreferrer"
									class="inline-flex items-center justify-center h-8 w-8 text-muted-foreground hover:text-foreground transition-colors"
								>
									<ExternalLink class="h-3.5 w-3.5" />
								</a>
							{/if}
							<Button variant="outline" size="sm" class="gap-1.5" href="/settings/applications/{inst.id}">
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

	<!-- OAuth App Authorizations -->
	<div>
		<div class="mb-6">
			<h2 class="text-xl font-semibold text-foreground">Authorized OAuth Apps</h2>
			<p class="text-sm text-muted-foreground mt-1">
				You have granted {authorizations.length} application{authorizations.length !== 1 ? 's' : ''} access to your account.
			</p>
		</div>

		{#if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400 mb-4">{error}</div>
		{/if}

		{#if !loading && authorizations.length === 0}
			<div class="rounded-md border border-border bg-card p-8 text-center">
				<AppWindow class="h-8 w-8 text-muted-foreground mx-auto mb-3" />
				<p class="text-sm font-semibold text-foreground mb-1">No authorized OAuth apps</p>
				<p class="text-xs text-muted-foreground">You haven't authorized any OAuth applications yet.</p>
			</div>
		{:else if !loading}
			<div class="divide-y divide-secondary rounded-md border border-border bg-card overflow-hidden">
				{#each authorizations as auth}
					<div class="flex items-center gap-4 px-4 py-4">
						<div class="h-10 w-10 rounded-md border border-border bg-secondary flex items-center justify-center shrink-0 overflow-hidden">
							{#if auth.app?.logo_url}
								<img src={auth.app.logo_url} alt={auth.app.name} class="h-full w-full object-cover" />
							{:else}
								<AppWindow class="h-5 w-5 text-muted-foreground" />
							{/if}
						</div>
						<div class="flex-1 min-w-0">
							<p class="text-sm font-semibold text-foreground">{auth.app?.name ?? 'Unknown app'}</p>
							{#if auth.app?.description}
								<p class="text-xs text-muted-foreground mt-0.5 truncate">{auth.app.description}</p>
							{/if}
							<div class="flex items-center gap-3 mt-1">
								{#if auth.app?.homepage_url}
									<a
										href={auth.app.homepage_url}
										target="_blank"
										rel="noopener noreferrer"
										class="inline-flex items-center gap-1 text-xs text-blue-400 hover:text-blue-300 transition-colors"
										onclick={(e) => e.stopPropagation()}
									>
										<ExternalLink class="h-3 w-3" />
										{auth.app.homepage_url}
									</a>
								{/if}
								<span class="text-xs text-muted-foreground">Authorized {formatDate(auth.created_at)}</span>
							</div>
						</div>
						<Button
							variant="outline"
							size="sm"
							onclick={() => handleRevoke(auth)}
							disabled={revoking === auth.id}
							class="shrink-0 text-red-400 hover:text-red-300 hover:border-red-800/50 hover:bg-red-900/20"
						>
							{#if revoking === auth.id}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
							Revoke
						</Button>
					</div>
				{/each}
			</div>
		{/if}
	</div>
</div>
