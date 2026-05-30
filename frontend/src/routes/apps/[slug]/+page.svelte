<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { gitpierApps, type GitPierApp } from '$lib/api/client';
	import { Boxes, Loader, Globe, ExternalLink } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	const slug = page.params.slug;

	let app = $state<GitPierApp | null>(null);
	let loading = $state(true);
	let error = $state('');

	const PERM_LABELS: Record<string, string> = {
		contents: 'Repository contents',
		issues: 'Issues',
		pull_requests: 'Pull requests',
		webhooks: 'Webhooks',
		releases: 'Releases',
		workflows: 'Workflow runs',
		metadata: 'Repository metadata',
		collaborators: 'Collaborators',
		members: 'Organization members',
		profile: 'User profile',
		email: 'Email addresses',
		ssh_keys: 'SSH keys'
	};

	const PERM_LEVEL: Record<string, string> = {
		read: 'Read-only',
		write: 'Read & write',
		none: 'No access'
	};

	function parseJSON(s: string): Record<string, string> {
		try {
			return JSON.parse(s) ?? {};
		} catch {
			return {};
		}
	}

	let repoPerms = $derived(app ? Object.entries(parseJSON(app.repo_permissions)).filter(([, v]) => v !== 'none') : []);
	let orgPerms = $derived(app ? Object.entries(parseJSON(app.org_permissions)).filter(([, v]) => v !== 'none') : []);
	let acctPerms = $derived(app ? Object.entries(parseJSON(app.account_permissions)).filter(([, v]) => v !== 'none') : []);
	let hasPerms = $derived(repoPerms.length + orgPerms.length + acctPerms.length > 0);

	onMount(async () => {
		try {
			app = await gitpierApps.getBySlug(slug!);
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});
</script>

<svelte:head>
	<title>{app ? `${app.name} · GitPier App` : 'GitPier App'}</title>
</svelte:head>

<div class="min-h-screen bg-background py-12 px-4">
	<div class="mx-auto max-w-lg">
		{#if loading}
			<div class="flex items-center justify-center py-20">
				<Loader class="h-5 w-5 animate-spin text-muted-foreground" />
			</div>
		{:else if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
		{:else if app}
			<!-- App header -->
			<div class="text-center mb-8">
				<div class="h-20 w-20 rounded-2xl border border-border bg-card flex items-center justify-center mx-auto mb-4 overflow-hidden">
					{#if app.logo_url}
						<img src={app.logo_url} alt={app.name} class="h-full w-full object-cover" />
					{:else}
						<Boxes class="h-10 w-10 text-muted-foreground" />
					{/if}
				</div>
				<h1 class="text-2xl font-bold text-foreground">{app.name}</h1>
				{#if app.description}
					<p class="text-sm text-muted-foreground mt-2 max-w-sm mx-auto">{app.description}</p>
				{/if}
				<div class="flex items-center justify-center gap-3 mt-3">
					{#if app.is_public}
						<Badge variant="secondary" class="text-xs">
							<Globe class="h-3 w-3 mr-1" />Public
						</Badge>
					{/if}
					{#if app.installation_count > 0}
						<span class="text-xs text-muted-foreground">{app.installation_count} installation{app.installation_count !== 1 ? 's' : ''}</span>
					{/if}
					{#if app.homepage_url}
						<a href={app.homepage_url} target="_blank" rel="noopener noreferrer" class="text-xs text-muted-foreground hover:text-foreground flex items-center gap-1 transition-colors">
							<ExternalLink class="h-3 w-3" />Website
						</a>
					{/if}
				</div>
			</div>

			<!-- Permissions -->
			<div class="rounded-md border border-border bg-card p-5 mb-6">
				<h2 class="text-sm font-semibold text-foreground mb-3">Permissions requested</h2>
				{#if !hasPerms}
					<p class="text-xs text-muted-foreground">This app does not request any permissions.</p>
				{:else}
					<div class="space-y-4">
						{#if repoPerms.length > 0}
							<div>
								<p class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">Repository</p>
								<div class="space-y-1.5">
									{#each repoPerms as [perm, level]}
										<div class="flex items-center justify-between text-sm">
											<span class="text-foreground">{PERM_LABELS[perm] ?? perm}</span>
											<Badge variant="secondary" class="text-xs">{PERM_LEVEL[level] ?? level}</Badge>
										</div>
									{/each}
								</div>
							</div>
						{/if}
						{#if orgPerms.length > 0}
							<div>
								<p class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">Organization</p>
								<div class="space-y-1.5">
									{#each orgPerms as [perm, level]}
										<div class="flex items-center justify-between text-sm">
											<span class="text-foreground">{PERM_LABELS[perm] ?? perm}</span>
											<Badge variant="secondary" class="text-xs">{PERM_LEVEL[level] ?? level}</Badge>
										</div>
									{/each}
								</div>
							</div>
						{/if}
						{#if acctPerms.length > 0}
							<div>
								<p class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">Account</p>
								<div class="space-y-1.5">
									{#each acctPerms as [perm, level]}
										<div class="flex items-center justify-between text-sm">
											<span class="text-foreground">{PERM_LABELS[perm] ?? perm}</span>
											<Badge variant="secondary" class="text-xs">{PERM_LEVEL[level] ?? level}</Badge>
										</div>
									{/each}
								</div>
							</div>
						{/if}
					</div>
				{/if}
			</div>

			<!-- Install button -->
			{#if authStore.isAuthenticated}
				<Button variant="brand" class="w-full" onclick={() => goto(`/apps/${slug}/installations/new`)}>
					Install {app.name}
				</Button>
			{:else}
				<Button variant="brand" class="w-full" onclick={() => goto(`/login?return_to=/apps/${slug}/installations/new`)}>Sign in to install</Button>
			{/if}

			<p class="text-center text-xs text-muted-foreground mt-4">By installing this app you agree to allow it to access your data as listed above.</p>
		{/if}
	</div>
</div>
