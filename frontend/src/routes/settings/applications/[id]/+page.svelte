<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { gitpierApps, type AppInstallation } from '$lib/api/client';
	import { ArrowLeft, Loader, Check, Boxes, ExternalLink } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	const instId = Number(page.params.id);

	let inst = $state<AppInstallation | null>(null);
	let loading = $state(true);
	let error = $state('');
	let saving = $state(false);
	let saveError = $state('');
	let saved = $state(false);
	let confirmUninstall = $state(false);
	let uninstalling = $state(false);
	let syncing = $state(false);
	let syncDone = $state(false);

	let repoSelection = $state<'all' | 'selected'>('all');
	let userRepos = $state<Array<{ id: number; name: string }>>([]);
	let selectedRepoIds = $state<Set<number>>(new Set());
	let loadingRepos = $state(false);

	function parseJSON(s: string): Record<string, string> {
		try {
			return JSON.parse(s) ?? {};
		} catch {
			return {};
		}
	}

	function permLabel(perm: string): string {
		const map: Record<string, string> = {
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
		return map[perm] ?? perm;
	}

	function levelLabel(level: string): string {
		return level === 'write' ? 'Read & write' : level === 'read' ? 'Read-only' : level;
	}

	function allGrantedPerms(installation: AppInstallation): Array<{ label: string; level: string }> {
		const rp = parseJSON(installation.repo_permissions);
		const op = parseJSON(installation.org_permissions);
		const ap = parseJSON(installation.account_permissions);
		return [
			...Object.entries(rp).map(([k, v]) => ({ label: permLabel(k), level: levelLabel(v) })),
			...Object.entries(op).map(([k, v]) => ({ label: permLabel(k), level: levelLabel(v) })),
			...Object.entries(ap).map(([k, v]) => ({ label: permLabel(k), level: levelLabel(v) }))
		];
	}

	async function loadUserRepos() {
		loadingRepos = true;
		try {
			const res = await fetch('/api/users/me/repos').then((r) => r.json());
			userRepos = res.repos ?? [];
		} catch {
			userRepos = [];
		} finally {
			loadingRepos = false;
		}
	}

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		try {
			const data = await gitpierApps.getInstallation(instId);
			inst = data.installation;
			repoSelection = inst.repository_selection;
			if (inst.repositories) {
				selectedRepoIds = new Set(inst.repositories.map((r) => r.id));
			}
			if (repoSelection === 'selected') {
				await loadUserRepos();
			}
		} catch (e: unknown) {
			error = (e as { message?: string }).message ?? 'Failed to load installation';
		} finally {
			loading = false;
		}
	});

	async function handleSelectionChange(val: 'all' | 'selected') {
		repoSelection = val;
		if (val === 'selected' && userRepos.length === 0) {
			await loadUserRepos();
		}
	}

	async function handleSave(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		saveError = '';
		saved = false;
		try {
			const data = await gitpierApps.updateInstallationRepos(instId, {
				repository_selection: repoSelection,
				repo_ids: repoSelection === 'selected' ? Array.from(selectedRepoIds) : undefined
			});
			inst = data.installation;
			saved = true;
			setTimeout(() => (saved = false), 3000);
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? 'Save failed';
		} finally {
			saving = false;
		}
	}

	async function handleSyncPermissions() {
		syncing = true;
		syncDone = false;
		try {
			const data = await gitpierApps.syncInstallationPermissions(instId);
			inst = data.installation;
			syncDone = true;
			setTimeout(() => (syncDone = false), 3000);
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? 'Failed to sync permissions';
		} finally {
			syncing = false;
		}
	}

	async function handleUninstall() {
		uninstalling = true;
		try {
			await gitpierApps.uninstall(instId);
			goto('/settings/applications');
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? 'Uninstall failed';
			uninstalling = false;
		}
	}

	function toggleRepo(id: number) {
		const next = new Set(selectedRepoIds);
		if (next.has(id)) next.delete(id);
		else next.add(id);
		selectedRepoIds = next;
	}
</script>

<svelte:head>
	<title>{inst?.app?.name ?? 'App'} — Installation</title>
</svelte:head>

<div class="max-w-2xl">
	<div class="flex items-center gap-3 mb-6">
		<a href="/settings/applications" class="text-muted-foreground hover:text-foreground transition-colors">
			<ArrowLeft class="h-4 w-4" />
		</a>
		<h1 class="text-xl font-semibold text-foreground">
			{inst?.app?.name ?? 'App installation'}
		</h1>
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-20">
			<Loader class="h-5 w-5 animate-spin text-muted-foreground" />
		</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if inst}
		<!-- App header card -->
		<div class="rounded-md border border-border bg-card p-5 mb-6 flex items-center gap-4">
			<div class="h-12 w-12 rounded-md border border-border bg-secondary flex items-center justify-center shrink-0 overflow-hidden">
				{#if inst.app?.logo_url}
					<img src={inst.app.logo_url} alt={inst.app.name} class="h-full w-full object-cover" />
				{:else}
					<Boxes class="h-6 w-6 text-muted-foreground" />
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
					<p class="text-xs text-muted-foreground mt-0.5">{inst.app.description}</p>
				{/if}
			</div>
			{#if inst.app?.homepage_url}
				<a href={inst.app.homepage_url} target="_blank" rel="noopener noreferrer" class="inline-flex items-center gap-1.5 text-xs text-blue-400 hover:text-blue-300 transition-colors shrink-0">
					<ExternalLink class="h-3.5 w-3.5" />
					Website
				</a>
			{/if}
		</div>

		{#if saveError}
			<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{saveError}</div>
		{/if}

		{#if saved}
			<div class="mb-4 rounded-md border border-emerald-800/40 bg-emerald-900/20 px-4 py-3 flex items-center gap-2 text-sm text-emerald-400">
				<Check class="h-4 w-4 shrink-0" />
				Repository access updated.
			</div>
		{/if}

		{#if syncDone}
			<div class="mb-4 rounded-md border border-emerald-800/40 bg-emerald-900/20 px-4 py-3 flex items-center gap-2 text-sm text-emerald-400">
				<Check class="h-4 w-4 shrink-0" />
				Permissions synced and approved.
			</div>
		{/if}

		<!-- Permissions -->
		<div class="rounded-md border border-border bg-card p-5 mb-6">
			<div class="flex items-start justify-between gap-4 mb-4">
				<div>
					<h2 class="text-sm font-semibold text-foreground">Permissions granted</h2>
					<p class="text-xs text-muted-foreground mt-0.5">What this app can access on your account</p>
				</div>
				<Button variant="outline" size="sm" onclick={handleSyncPermissions} disabled={syncing} class="shrink-0">
					{#if syncing}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
					Approve new permissions
				</Button>
			</div>

			{#if allGrantedPerms(inst).length === 0}
				<p class="text-xs text-muted-foreground">This app has no granted permissions.</p>
			{:else}
				<div class="divide-y divide-border rounded border border-border overflow-hidden">
					{#each allGrantedPerms(inst) as p}
						<div class="flex items-center justify-between px-4 py-2">
							<span class="text-xs text-foreground">{p.label}</span>
							<Badge variant="secondary" class="text-[10px]">{p.level}</Badge>
						</div>
					{/each}
				</div>
			{/if}

			<p class="text-xs text-muted-foreground mt-3">If the app developer updated permissions, click "Approve new permissions" to accept them.</p>
		</div>

		<!-- Repository access -->
		<form onsubmit={handleSave} class="mb-6">
			<div class="rounded-md border border-border bg-card p-5 space-y-4">
				<div>
					<h2 class="text-sm font-semibold text-foreground">Repository access</h2>
					<p class="text-xs text-muted-foreground mt-0.5">Choose which repositories this app can access</p>
				</div>

				<div class="space-y-2">
					<label class="flex items-start gap-3 cursor-pointer">
						<input type="radio" name="repo-selection" value="all" checked={repoSelection === 'all'} onchange={() => handleSelectionChange('all')} class="mt-0.5" />
						<div>
							<span class="text-sm font-semibold text-foreground block">All repositories</span>
							<span class="text-xs text-muted-foreground">This applies to all current and future repositories on your account.</span>
						</div>
					</label>
					<label class="flex items-start gap-3 cursor-pointer">
						<input type="radio" name="repo-selection" value="selected" checked={repoSelection === 'selected'} onchange={() => handleSelectionChange('selected')} class="mt-0.5" />
						<div>
							<span class="text-sm font-semibold text-foreground block">Only select repositories</span>
							<span class="text-xs text-muted-foreground">Select at least one repository.</span>
						</div>
					</label>
				</div>

				{#if repoSelection === 'selected'}
					<div class="pt-2 border-t border-border">
						{#if loadingRepos}
							<div class="flex items-center gap-2 py-2 text-xs text-muted-foreground">
								<Loader class="h-3.5 w-3.5 animate-spin" />
								Loading repositories…
							</div>
						{:else if userRepos.length === 0}
							<p class="text-xs text-muted-foreground">No repositories found on your account.</p>
						{:else}
							<div class="max-h-60 overflow-y-auto space-y-1 rounded border border-border p-2">
								{#each userRepos as repo}
									<label class="flex items-center gap-2 px-2 py-1.5 rounded hover:bg-secondary/50 cursor-pointer">
										<input type="checkbox" checked={selectedRepoIds.has(repo.id)} onchange={() => toggleRepo(repo.id)} class="rounded border-border" />
										<span class="text-xs text-foreground">{repo.name}</span>
									</label>
								{/each}
							</div>
							<p class="text-xs text-muted-foreground mt-1">{selectedRepoIds.size} repositor{selectedRepoIds.size !== 1 ? 'ies' : 'y'} selected</p>
						{/if}
					</div>
				{/if}

				<div class="flex justify-end pt-2 border-t border-border">
					<Button type="submit" variant="brand" disabled={saving}>
						{#if saving}<Loader class="h-4 w-4 animate-spin" />{/if}
						Save
					</Button>
				</div>
			</div>
		</form>

		<!-- Danger zone -->
		<div class="rounded-md border border-red-800/40 bg-card p-5">
			<h2 class="text-sm font-semibold text-red-400 mb-1">Danger zone</h2>
			{#if confirmUninstall}
				<p class="text-sm text-foreground mb-3">
					Are you sure you want to uninstall <strong>{inst.app?.name}</strong>? The app will immediately lose access to all your repositories and account data.
				</p>
				<div class="flex gap-2">
					<Button variant="destructive" size="sm" onclick={handleUninstall} disabled={uninstalling}>
						{#if uninstalling}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
						Yes, uninstall
					</Button>
					<Button variant="outline" size="sm" onclick={() => (confirmUninstall = false)}>Cancel</Button>
				</div>
			{:else}
				<p class="text-xs text-muted-foreground mb-3">Uninstalling this app will immediately revoke its access to your repositories and account data.</p>
				<Button variant="outline" size="sm" class="border-red-800/40 text-red-400 hover:bg-red-900/20" onclick={() => (confirmUninstall = true)}>
					Uninstall {inst.app?.name ?? 'app'}
				</Button>
			{/if}
		</div>
	{/if}
</div>
