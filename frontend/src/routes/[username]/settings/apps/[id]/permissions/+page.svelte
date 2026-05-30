<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getContext } from 'svelte';
	import { gitpierApps, type GitPierApp, type Organization } from '$lib/api/client';
	import { ArrowLeft, Loader, AlertTriangle, Check } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	const handle = page.params.username as string;
	const appId = Number(page.params.id);
	const ctx = getContext<{ org: Organization | null; isOwner: boolean; loading: boolean }>('org');

	let app = $state<GitPierApp | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let saveError = $state('');
	let saved = $state(false);
	let installationCount = $state(0);

	const REPO_PERMS = ['contents', 'issues', 'pull_requests', 'webhooks', 'releases', 'workflows', 'metadata', 'collaborators'] as const;
	const ORG_PERMS = ['members'] as const;
	const ACCT_PERMS = ['profile', 'email', 'ssh_keys'] as const;

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

	const PERM_DESCRIPTIONS: Record<string, string> = {
		contents: 'Read and write access to repository code, commits, and branches',
		issues: 'Read and write access to issues and issue comments',
		pull_requests: 'Read and write access to pull requests, reviews, and comments',
		webhooks: 'Manage repository webhooks',
		releases: 'Read and write access to releases and release assets',
		workflows: 'Read access to workflow runs and job logs',
		metadata: 'Read-only access to repository metadata (always recommended)',
		collaborators: 'Manage repository collaborators',
		members: 'Read and write access to organization members and teams',
		profile: 'Read and write access to user profile information',
		email: 'Read access to user email addresses',
		ssh_keys: 'Read and write access to SSH keys'
	};

	let repoPerms = $state<Record<string, string>>(Object.fromEntries(REPO_PERMS.map((p) => [p, 'none'])));
	let orgPerms = $state<Record<string, string>>(Object.fromEntries(ORG_PERMS.map((p) => [p, 'none'])));
	let acctPerms = $state<Record<string, string>>(Object.fromEntries(ACCT_PERMS.map((p) => [p, 'none'])));

	let originalRepoPerms = $state<Record<string, string>>({});
	let originalOrgPerms = $state<Record<string, string>>({});
	let originalAcctPerms = $state<Record<string, string>>({});

	const hasChanges = $derived(
		REPO_PERMS.some((p) => repoPerms[p] !== (originalRepoPerms[p] ?? 'none')) ||
			ORG_PERMS.some((p) => orgPerms[p] !== (originalOrgPerms[p] ?? 'none')) ||
			ACCT_PERMS.some((p) => acctPerms[p] !== (originalAcctPerms[p] ?? 'none'))
	);

	const hasElevatedPerms = $derived(
		REPO_PERMS.some((p) => isElevated(repoPerms[p], originalRepoPerms[p] ?? 'none')) ||
			ORG_PERMS.some((p) => isElevated(orgPerms[p], originalOrgPerms[p] ?? 'none')) ||
			ACCT_PERMS.some((p) => isElevated(acctPerms[p], originalAcctPerms[p] ?? 'none'))
	);

	function levelRank(level: string): number {
		return level === 'write' ? 2 : level === 'read' ? 1 : 0;
	}
	function isElevated(next: string, prev: string): boolean {
		return levelRank(next) > levelRank(prev);
	}

	function parseJSON(s: string): Record<string, string> {
		try {
			return JSON.parse(s) ?? {};
		} catch {
			return {};
		}
	}

	function populateForm(a: GitPierApp) {
		const rp = parseJSON(a.repo_permissions);
		REPO_PERMS.forEach((p) => {
			repoPerms[p] = rp[p] ?? 'none';
		});
		const op = parseJSON(a.org_permissions);
		ORG_PERMS.forEach((p) => {
			orgPerms[p] = op[p] ?? 'none';
		});
		const ap = parseJSON(a.account_permissions);
		ACCT_PERMS.forEach((p) => {
			acctPerms[p] = ap[p] ?? 'none';
		});
		originalRepoPerms = { ...repoPerms };
		originalOrgPerms = { ...orgPerms };
		originalAcctPerms = { ...acctPerms };
	}

	$effect(() => {
		if (!ctx.loading && !ctx.isOwner) {
			goto(`/${handle}/settings/apps`);
		}
	});

	onMount(async () => {
		try {
			const [appData, installData] = await Promise.all([gitpierApps.get(appId), gitpierApps.listInstallations(appId)]);
			app = appData.app;
			installationCount = installData.installations?.length ?? 0;
			populateForm(app);
		} catch (e: unknown) {
			error = (e as { message?: string }).message ?? 'Failed to load app';
		} finally {
			loading = false;
		}
	});

	async function handleSave(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		saveError = '';
		saved = false;
		try {
			const res = await gitpierApps.update(appId, {
				repo_permissions: Object.fromEntries(Object.entries(repoPerms).filter(([, v]) => v !== 'none')),
				org_permissions: Object.fromEntries(Object.entries(orgPerms).filter(([, v]) => v !== 'none')),
				account_permissions: Object.fromEntries(Object.entries(acctPerms).filter(([, v]) => v !== 'none'))
			});
			app = res.app;
			populateForm(res.app);
			saved = true;
			setTimeout(() => (saved = false), 3000);
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? 'Failed to save permissions';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{app ? `Permissions — ${app.name}` : 'App Permissions'}</title>
</svelte:head>

<div class="max-w-2xl">
	<div class="flex items-center gap-3 mb-6">
		<a href="/{handle}/settings/apps/{appId}" class="text-muted-foreground hover:text-foreground transition-colors">
			<ArrowLeft class="h-4 w-4" />
		</a>
		<div>
			{#if app}
				<div class="flex items-center gap-2">
					<h1 class="text-xl font-semibold text-foreground">{app.name}</h1>
					<span class="text-muted-foreground">/</span>
					<span class="text-lg text-muted-foreground">Permissions</span>
				</div>
			{:else}
				<h1 class="text-xl font-semibold text-foreground">App Permissions</h1>
			{/if}
		</div>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-20">
			<Loader class="h-5 w-5 animate-spin text-muted-foreground" />
		</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if app}
		{#if installationCount > 0}
			<div class="mb-5 rounded-md border border-amber-800/40 bg-amber-900/20 p-4 flex gap-3">
				<AlertTriangle class="h-4 w-4 text-amber-400 shrink-0 mt-0.5" />
				<div class="text-sm text-amber-300">
					<p class="font-semibold mb-0.5">Permission changes require re-approval</p>
					<p class="text-amber-400/80 text-xs">
						This app has <strong>{installationCount}</strong>
						{installationCount === 1 ? 'installation' : 'installations'}. If you add or elevate permissions, existing installations will need to re-approve the new permissions before the
						app can use them. Reducing or removing permissions takes effect immediately.
					</p>
				</div>
			</div>
		{/if}

		{#if saveError}
			<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{saveError}</div>
		{/if}

		{#if saved}
			<div class="mb-4 rounded-md border border-emerald-800/40 bg-emerald-900/20 px-4 py-3 flex items-center gap-2 text-sm text-emerald-400">
				<Check class="h-4 w-4 shrink-0" />
				Permissions saved successfully.
			</div>
		{/if}

		<form onsubmit={handleSave}>
			<!-- Repository permissions -->
			<div class="rounded-md border border-border bg-card mb-4 overflow-hidden">
				<div class="px-5 py-3 border-b border-border bg-secondary/30">
					<h2 class="text-sm font-semibold text-foreground">Repository permissions</h2>
					<p class="text-xs text-muted-foreground mt-0.5">Access to repository data and operations</p>
				</div>
				<div class="divide-y divide-border">
					{#each REPO_PERMS as perm}
						<div class="flex items-center gap-4 px-5 py-3">
							<div class="flex-1 min-w-0">
								<p class="text-sm font-medium text-foreground">{PERM_LABELS[perm]}</p>
								<p class="text-xs text-muted-foreground mt-0.5">{PERM_DESCRIPTIONS[perm]}</p>
							</div>
							<div class="flex items-center gap-1 shrink-0">
								{#if isElevated(repoPerms[perm], originalRepoPerms[perm] ?? 'none')}
									<Badge variant="outline" class="text-[10px] text-amber-400 border-amber-700/50 mr-1">elevated</Badge>
								{/if}
								<select
									bind:value={repoPerms[perm]}
									class="rounded border border-border bg-background text-foreground text-xs px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-ring"
								>
									<option value="none">No access</option>
									<option value="read">Read</option>
									<option value="write">Read & write</option>
								</select>
							</div>
						</div>
					{/each}
				</div>
			</div>

			<!-- Organization permissions -->
			<div class="rounded-md border border-border bg-card mb-4 overflow-hidden">
				<div class="px-5 py-3 border-b border-border bg-secondary/30">
					<h2 class="text-sm font-semibold text-foreground">Organization permissions</h2>
					<p class="text-xs text-muted-foreground mt-0.5">Access to organization data and members</p>
				</div>
				<div class="divide-y divide-border">
					{#each ORG_PERMS as perm}
						<div class="flex items-center gap-4 px-5 py-3">
							<div class="flex-1 min-w-0">
								<p class="text-sm font-medium text-foreground">{PERM_LABELS[perm]}</p>
								<p class="text-xs text-muted-foreground mt-0.5">{PERM_DESCRIPTIONS[perm]}</p>
							</div>
							<div class="flex items-center gap-1 shrink-0">
								{#if isElevated(orgPerms[perm], originalOrgPerms[perm] ?? 'none')}
									<Badge variant="outline" class="text-[10px] text-amber-400 border-amber-700/50 mr-1">elevated</Badge>
								{/if}
								<select
									bind:value={orgPerms[perm]}
									class="rounded border border-border bg-background text-foreground text-xs px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-ring"
								>
									<option value="none">No access</option>
									<option value="read">Read</option>
									<option value="write">Read & write</option>
								</select>
							</div>
						</div>
					{/each}
				</div>
			</div>

			<!-- Account permissions -->
			<div class="rounded-md border border-border bg-card mb-6 overflow-hidden">
				<div class="px-5 py-3 border-b border-border bg-secondary/30">
					<h2 class="text-sm font-semibold text-foreground">Account permissions</h2>
					<p class="text-xs text-muted-foreground mt-0.5">Access to user account data</p>
				</div>
				<div class="divide-y divide-border">
					{#each ACCT_PERMS as perm}
						<div class="flex items-center gap-4 px-5 py-3">
							<div class="flex-1 min-w-0">
								<p class="text-sm font-medium text-foreground">{PERM_LABELS[perm]}</p>
								<p class="text-xs text-muted-foreground mt-0.5">{PERM_DESCRIPTIONS[perm]}</p>
							</div>
							<div class="flex items-center gap-1 shrink-0">
								{#if isElevated(acctPerms[perm], originalAcctPerms[perm] ?? 'none')}
									<Badge variant="outline" class="text-[10px] text-amber-400 border-amber-700/50 mr-1">elevated</Badge>
								{/if}
								<select
									bind:value={acctPerms[perm]}
									class="rounded border border-border bg-background text-foreground text-xs px-2 py-1.5 focus:outline-none focus:ring-1 focus:ring-ring"
								>
									<option value="none">No access</option>
									<option value="read">Read</option>
									{#if perm === 'profile' || perm === 'ssh_keys'}
										<option value="write">Read & write</option>
									{/if}
								</select>
							</div>
						</div>
					{/each}
				</div>
			</div>

			<div class="flex justify-end">
				<Button type="submit" variant="brand" disabled={saving || !hasChanges}>
					{#if saving}<Loader class="h-4 w-4 animate-spin" />{/if}
					Save permissions
				</Button>
			</div>
		</form>
	{/if}
</div>
