<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { gitpierApps, orgs as orgApi, users, type AppInstallation, type GitPierApp, type Organization, type Repository } from '$lib/api/client';
	import { Boxes, Loader, Check } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	const slug = page.params.slug;
	const returnTo = $derived(page.url.searchParams.get('return_to')?.trim() ?? '');

	let app = $state<GitPierApp | null>(null);
	let orgs = $state<Organization[]>([]);
	let userInstallations = $state<AppInstallation[]>([]);
	let orgInstallationsByLogin = $state<Record<string, AppInstallation[]>>({});
	let loading = $state(true);
	let error = $state('');

	// Install form state
	let target = $state<string>('user'); // 'user' or 'org:<orgname>'
	let repoSelection = $state<'all' | 'selected'>('all');
	let installing = $state(false);
	let installError = $state('');
	let availableRepos = $state<Repository[]>([]);
	let selectedRepoIDs = $state<number[]>([]);
	let reposLoading = $state(false);
	let reposError = $state('');
	let reposLoadedForTarget = $state('');

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

	const PERM_LEVEL: Record<string, string> = { read: 'Read-only', write: 'Read & write' };

	function parseJSON(s: string): Record<string, string> {
		try {
			return JSON.parse(s) ?? {};
		} catch {
			return {};
		}
	}

	function equalPermissionMaps(a: Record<string, string>, b: Record<string, string>): boolean {
		const aKeys = Object.keys(a).sort();
		const bKeys = Object.keys(b).sort();
		if (aKeys.length !== bKeys.length) return false;
		for (let i = 0; i < aKeys.length; i++) {
			if (aKeys[i] !== bKeys[i]) return false;
			if ((a[aKeys[i]] ?? 'none') !== (b[bKeys[i]] ?? 'none')) return false;
		}
		return true;
	}

	let repoPerms = $derived(app ? Object.entries(parseJSON(app.repo_permissions)).filter(([, v]) => v !== 'none') : []);
	let orgPerms = $derived(app ? Object.entries(parseJSON(app.org_permissions)).filter(([, v]) => v !== 'none') : []);
	let acctPerms = $derived(app ? Object.entries(parseJSON(app.account_permissions)).filter(([, v]) => v !== 'none') : []);
	let activeInstallation = $derived.by(() => {
		if (!app) return null;
		if (target === 'user') {
			return userInstallations.find((inst) => inst.app_id === app.id) ?? null;
		}
		const orgLogin = target.startsWith('org:') ? target.slice(4) : '';
		const installs = orgInstallationsByLogin[orgLogin] ?? [];
		return installs.find((inst) => inst.app_id === app.id) ?? null;
	});
	let permissionsNeedApproval = $derived.by(() => {
		if (!app || !activeInstallation) return false;
		return (
			!equalPermissionMaps(parseJSON(app.repo_permissions), parseJSON(activeInstallation.repo_permissions)) ||
			!equalPermissionMaps(parseJSON(app.org_permissions), parseJSON(activeInstallation.org_permissions)) ||
			!equalPermissionMaps(parseJSON(app.account_permissions), parseJSON(activeInstallation.account_permissions))
		);
	});

	$effect(() => {
		if (repoSelection !== 'selected') {
			selectedRepoIDs = [];
		}
	});

	$effect(() => {
		if (loading || !app || !authStore.user?.username) return;

		if (reposLoadedForTarget === target) return;
		reposLoadedForTarget = target;

		reposLoading = true;
		reposError = '';

		const load = async () => {
			try {
				if (target === 'user') {
					const profile = await users.getProfile(authStore.user!.username);
					availableRepos = profile.repos ?? [];
				} else {
					const orgLogin = target.slice(4);
					availableRepos = await orgApi.repos.list(orgLogin);
				}

				if (activeInstallation?.repository_selection === 'selected') {
					selectedRepoIDs = (activeInstallation.repositories ?? []).map((r) => r.repo?.id).filter((id): id is number => typeof id === 'number');
				} else {
					selectedRepoIDs = [];
				}
			} catch (e: any) {
				reposError = e?.message ?? 'Failed to load repositories.';
				availableRepos = [];
				selectedRepoIDs = [];
			} finally {
				reposLoading = false;
			}
		};

		void load();
	});

	function toggleRepo(repoID: number, checked: boolean) {
		if (checked) {
			if (!selectedRepoIDs.includes(repoID)) {
				selectedRepoIDs = [...selectedRepoIDs, repoID];
			}
			return;
		}
		selectedRepoIDs = selectedRepoIDs.filter((id) => id !== repoID);
	}

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			const fullPath = `/apps/${slug}/install${page.url.search}`;
			goto(`/login?return_to=${encodeURIComponent(fullPath)}`);
			return;
		}
		try {
			const [appData, orgsData, userInstallsRes] = await Promise.all([gitpierApps.getBySlug(slug!), orgApi.listMyOrgs(), gitpierApps.listUserInstallations()]);
			app = appData;
			orgs = orgsData ?? [];
			userInstallations = userInstallsRes.installations ?? [];

			const orgInstallPairs = await Promise.all(
				(orgsData ?? []).map(async (org) => {
					const res = await gitpierApps.listOrgInstallations(org.login);
					return [org.login, res.installations ?? []] as const;
				})
			);
			orgInstallationsByLogin = Object.fromEntries(orgInstallPairs);
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function handleInstall(e: SubmitEvent) {
		e.preventDefault();
		installing = true;
		installError = '';
		try {
			if (repoSelection === 'selected' && selectedRepoIDs.length === 0) {
				installError = 'Select at least one repository to continue.';
				return;
			}

			let setupAction = 'install';
			let installationId = 0;

			if (activeInstallation) {
				if (permissionsNeedApproval) {
					await gitpierApps.syncInstallationPermissions(activeInstallation.id);
					setupAction = 'request_permissions';
				} else {
					setupAction = 'update';
				}

				const updated = await gitpierApps.updateInstallationRepos(activeInstallation.id, {
					repository_selection: repoSelection,
					repo_ids: repoSelection === 'selected' ? selectedRepoIDs : []
				});
				installationId = updated.installation.id;
			} else {
				const targetValue = target === 'user' ? undefined : target.replace('org:', '');
				const res = await gitpierApps.install(slug!, {
					target: targetValue,
					repository_selection: repoSelection,
					repo_ids: repoSelection === 'selected' ? selectedRepoIDs : []
				});
				installationId = res.installation.id;
			}

			if (returnTo) {
				try {
					const targetURL = new URL(returnTo);
					targetURL.searchParams.set('installation_id', String(installationId));
					targetURL.searchParams.set('setup_action', setupAction);
					window.location.href = targetURL.toString();
					return;
				} catch {
					// Ignore invalid return_to values and continue with default redirects.
				}
			}

			// If setup_url is configured and installation succeeded, redirect there
			if (app?.setup_url && app.redirect_on_update) {
				const setupURL = new URL(app.setup_url);
				setupURL.searchParams.set('installation_id', String(installationId));
				setupURL.searchParams.set('setup_action', setupAction);
				window.location.href = setupURL.toString();
			} else if (app?.setup_url) {
				window.location.href = app.setup_url + `?installation_id=${installationId}&setup_action=${setupAction}`;
			} else {
				goto('/settings/applications');
			}
		} catch (e: any) {
			installError = e.message;
		} finally {
			installing = false;
		}
	}
</script>

<svelte:head>
	<title>{app ? `Install ${app.name}` : 'Install app'}</title>
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
				<div class="h-16 w-16 rounded-2xl border border-border bg-card flex items-center justify-center mx-auto mb-3 overflow-hidden">
					{#if app.logo_url}
						<img src={app.logo_url} alt={app.name} class="h-full w-full object-cover" />
					{:else}
						<Boxes class="h-8 w-8 text-muted-foreground" />
					{/if}
				</div>
				<h1 class="text-xl font-bold text-foreground">{activeInstallation ? `Configure ${app.name}` : `Install ${app.name}`}</h1>
				<p class="text-sm text-muted-foreground mt-1">
					{#if activeInstallation && permissionsNeedApproval}
						Review and approve newly requested permissions, then confirm repository access.
					{:else if activeInstallation}
						Update repository access for this existing installation.
					{:else}
						Select where you'd like to install this app and what access to grant.
					{/if}
				</p>
			</div>

			{#if installError}
				<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{installError}</div>
			{/if}

			{#if activeInstallation}
				<div class="mb-4 rounded-md border border-amber-800/30 bg-amber-900/10 px-4 py-3 text-sm text-amber-200">
					<p class="font-semibold">This app is already installed for the selected account.</p>
					{#if permissionsNeedApproval}
						<p class="mt-1 text-amber-300">This app is requesting additional permissions. You must review and approve them to continue.</p>
					{:else}
						<p class="mt-1 text-amber-300">You can update repository access settings from here.</p>
					{/if}
				</div>
			{/if}

			<form onsubmit={handleInstall} class="space-y-5">
				<!-- Install target -->
				<div class="rounded-md border border-border bg-card p-5">
					<h2 class="text-sm font-semibold text-foreground mb-3">Install on account</h2>
					<div class="space-y-2">
						<!-- User account -->
						<label
							class="flex items-center gap-3 p-3 rounded-md border border-border cursor-pointer hover:bg-secondary/50 transition-colors {target === 'user'
								? 'border-brand bg-brand/5'
								: ''}"
						>
							<input type="radio" name="target" value="user" bind:group={target} class="hidden" />
							<div class="h-8 w-8 rounded-full bg-secondary flex items-center justify-center shrink-0 overflow-hidden">
								{#if authStore.user?.avatar_url}
									<img src={authStore.user.avatar_url} alt={authStore.user.username} class="h-full w-full object-cover" />
								{:else}
									<span class="text-xs font-semibold">{authStore.user?.username?.[0]?.toUpperCase()}</span>
								{/if}
							</div>
							<div class="flex-1 min-w-0">
								<p class="text-sm font-semibold text-foreground">{authStore.user?.username}</p>
								<p class="text-xs text-muted-foreground">Your personal account</p>
							</div>
							{#if target === 'user'}
								<Check class="h-4 w-4 text-brand shrink-0" />
							{/if}
						</label>

						<!-- Organization accounts -->
						{#each orgs as org}
							<label
								class="flex items-center gap-3 p-3 rounded-md border border-border cursor-pointer hover:bg-secondary/50 transition-colors {target === `org:${org.login}`
									? 'border-brand bg-brand/5'
									: ''}"
							>
								<input type="radio" name="target" value="org:{org.login}" bind:group={target} class="hidden" />
								<div class="h-8 w-8 rounded-full bg-secondary flex items-center justify-center shrink-0 overflow-hidden">
									{#if org.avatar_url}
										<img src={org.avatar_url} alt={org.login} class="h-full w-full object-cover" />
									{:else}
										<span class="text-xs font-semibold">{org.login?.[0]?.toUpperCase()}</span>
									{/if}
								</div>
								<div class="flex-1 min-w-0">
									<p class="text-sm font-semibold text-foreground">{org.display_name || org.login}</p>
									<p class="text-xs text-muted-foreground">Organization</p>
								</div>
								{#if target === `org:${org.login}`}
									<Check class="h-4 w-4 text-brand shrink-0" />
								{/if}
							</label>
						{/each}
					</div>
				</div>

				<!-- Repository access -->
				<div class="rounded-md border border-border bg-card p-5">
					<h2 class="text-sm font-semibold text-foreground mb-1">Repository access</h2>
					<p class="text-xs text-muted-foreground mb-3">Choose which repositories this app can access.</p>
					<div class="space-y-2">
						<label
							class="flex items-start gap-3 p-3 rounded-md border border-border cursor-pointer hover:bg-secondary/50 transition-colors {repoSelection === 'all'
								? 'border-brand bg-brand/5'
								: ''}"
						>
							<input type="radio" name="repo_selection" value="all" bind:group={repoSelection} class="mt-0.5" />
							<div>
								<p class="text-sm font-semibold text-foreground">All repositories</p>
								<p class="text-xs text-muted-foreground">Grant access to all current and future repositories on this account.</p>
							</div>
						</label>
						<label
							class="flex items-start gap-3 p-3 rounded-md border border-border cursor-pointer hover:bg-secondary/50 transition-colors {repoSelection === 'selected'
								? 'border-brand bg-brand/5'
								: ''}"
						>
							<input type="radio" name="repo_selection" value="selected" bind:group={repoSelection} class="mt-0.5" />
							<div>
								<p class="text-sm font-semibold text-foreground">Only select repositories</p>
								<p class="text-xs text-muted-foreground">Select specific repositories to grant access now. You can change this later in installation settings.</p>
							</div>
						</label>
					</div>

					{#if repoSelection === 'selected'}
						<div class="mt-3 rounded-md border border-border bg-background p-3">
							<p class="text-xs font-semibold text-foreground mb-2">Select repositories</p>
							{#if reposLoading}
								<div class="flex items-center gap-2 text-xs text-muted-foreground">
									<Loader class="h-3.5 w-3.5 animate-spin" />
									Loading repositories...
								</div>
							{:else if reposError}
								<p class="text-xs text-red-400">{reposError}</p>
							{:else if availableRepos.length === 0}
								<p class="text-xs text-muted-foreground">No repositories available for the selected account.</p>
							{:else}
								<div class="max-h-48 overflow-auto space-y-1 pr-1">
									{#each availableRepos as repo}
										<label class="flex items-center gap-2 text-xs text-foreground py-1">
											<input type="checkbox" checked={selectedRepoIDs.includes(repo.id)} onchange={(e) => toggleRepo(repo.id, (e.currentTarget as HTMLInputElement).checked)} />
											<span>{repo.name}</span>
										</label>
									{/each}
								</div>
								<p class="text-xs text-muted-foreground mt-2">{selectedRepoIDs.length} selected</p>
							{/if}
						</div>
					{/if}
				</div>

				<!-- Permissions summary -->
				{#if repoPerms.length > 0 || orgPerms.length > 0 || acctPerms.length > 0}
					<div class="rounded-md border border-border bg-card p-5">
						<h2 class="text-sm font-semibold text-foreground mb-3">Permissions</h2>
						<div class="space-y-1.5">
							{#each repoPerms as [perm, level]}
								<div class="flex items-center justify-between text-xs">
									<span class="text-muted-foreground">{PERM_LABELS[perm] ?? perm} (repository)</span>
									<Badge variant="secondary" class="text-xs">{PERM_LEVEL[level] ?? level}</Badge>
								</div>
							{/each}
							{#each orgPerms as [perm, level]}
								<div class="flex items-center justify-between text-xs">
									<span class="text-muted-foreground">{PERM_LABELS[perm] ?? perm} (organization)</span>
									<Badge variant="secondary" class="text-xs">{PERM_LEVEL[level] ?? level}</Badge>
								</div>
							{/each}
							{#each acctPerms as [perm, level]}
								<div class="flex items-center justify-between text-xs">
									<span class="text-muted-foreground">{PERM_LABELS[perm] ?? perm} (account)</span>
									<Badge variant="secondary" class="text-xs">{PERM_LEVEL[level] ?? level}</Badge>
								</div>
							{/each}
						</div>
					</div>
				{/if}

				<Button type="submit" variant="brand" class="w-full" disabled={installing}>
					{#if installing}<Loader class="h-4 w-4 animate-spin" />{/if}
					{#if activeInstallation && permissionsNeedApproval}
						Review and approve updated permissions
					{:else if activeInstallation}
						Save installation settings
					{:else}
						Install {app.name}
					{/if}
				</Button>

				<p class="text-center text-xs text-muted-foreground">
					By installing, you grant the permissions listed above. You can revoke access at any time from
					<a href="/settings/applications" class="underline hover:text-foreground">Settings → Applications</a>.
				</p>
			</form>
		{/if}
	</div>
</div>
