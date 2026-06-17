<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { getContext } from 'svelte';
	import { gitpierApps, type GitPierApp, type AppPrivateKey, type AppInstallation, type Organization } from '$lib/api/client';
	import { ArrowLeft, Loader, Key, Trash2, Plus, Copy, Check, AlertTriangle, Boxes } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';

	const handle = page.params.username as string;
	const appId = Number(page.params.id);
	const ctx = getContext<{ org: Organization | null; isOwner: boolean; loading: boolean }>('org');

	let app = $state<GitPierApp | null>(null);
	let keys = $state<AppPrivateKey[]>([]);
	let installations = $state<AppInstallation[]>([]);
	let loading = $state(true);
	let error = $state('');
	let saving = $state(false);
	let saveError = $state('');

	let name = $state('');
	let description = $state('');
	let homepageURL = $state('');
	let logoURL = $state('');
	let setupURL = $state('');
	let redirectOnUpdate = $state(false);
	let webhookURL = $state('');
	let webhookActive = $state(true);
	let isPublic = $state(false);
	let callbackURLs = $state<string[]>(['']);
	let requestUserAuth = $state(false);
	let expireUserTokens = $state(true);
	let enableDeviceFlow = $state(false);

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

	let repoPerms = $state<Record<string, string>>(Object.fromEntries(REPO_PERMS.map((p) => [p, 'none'])));
	let orgPerms = $state<Record<string, string>>(Object.fromEntries(ORG_PERMS.map((p) => [p, 'none'])));
	let acctPerms = $state<Record<string, string>>(Object.fromEntries(ACCT_PERMS.map((p) => [p, 'none'])));

	let newSecret = $state('');
	let copiedSecret = $state(false);
	let copiedClientID = $state(false);
	let regenerating = $state(false);
	let generatingKey = $state(false);
	let newPrivateKey = $state('');
	let newKeyFingerprint = $state('');
	let confirmDelete = $state(false);
	let deletingApp = $state(false);

	const secretHash = typeof window !== 'undefined' ? window.location.hash : '';
	if (secretHash.startsWith('#secret=')) {
		newSecret = secretHash.slice('#secret='.length);
		if (typeof window !== 'undefined') history.replaceState({}, '', window.location.pathname);
	}

	function parseJSON(s: string): Record<string, string> {
		try {
			return JSON.parse(s) ?? {};
		} catch {
			return {};
		}
	}
	function parseStringArray(s: string): string[] {
		try {
			return JSON.parse(s) ?? [];
		} catch {
			return [];
		}
	}

	function populateForm(a: GitPierApp) {
		name = a.name;
		description = a.description;
		homepageURL = a.homepage_url;
		logoURL = a.logo_url;
		setupURL = a.setup_url;
		redirectOnUpdate = a.redirect_on_update;
		webhookURL = a.webhook_url;
		webhookActive = a.webhook_active;
		isPublic = a.is_public;
		requestUserAuth = a.request_user_auth;
		expireUserTokens = a.expire_user_tokens;
		enableDeviceFlow = a.enable_device_flow;
		const urls = parseStringArray(a.callback_urls);
		callbackURLs = urls.length ? urls : [''];
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
	}

	$effect(() => {
		if (!ctx.loading && !ctx.isOwner) {
			goto(`/${handle}/settings/apps`);
		}
	});

	onMount(async () => {
		try {
			const [appData, keysData, installData] = await Promise.all([gitpierApps.get(appId), gitpierApps.listKeys(appId), gitpierApps.listInstallations(appId)]);
			app = appData.app;
			keys = keysData.keys ?? [];
			installations = installData.installations ?? [];
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
		try {
			const filteredCallbacks = callbackURLs.filter((u) => u.trim() !== '');
			const res = await gitpierApps.update(appId, {
				name,
				description,
				homepage_url: homepageURL,
				logo_url: logoURL,
				setup_url: setupURL,
				redirect_on_update: redirectOnUpdate,
				webhook_url: webhookURL,
				webhook_active: webhookActive,
				is_public: isPublic,
				callback_urls: filteredCallbacks,
				request_user_auth: requestUserAuth,
				expire_user_tokens: expireUserTokens,
				enable_device_flow: enableDeviceFlow,
				repo_permissions: Object.fromEntries(Object.entries(repoPerms).filter(([, v]) => v !== 'none')),
				org_permissions: Object.fromEntries(Object.entries(orgPerms).filter(([, v]) => v !== 'none')),
				account_permissions: Object.fromEntries(Object.entries(acctPerms).filter(([, v]) => v !== 'none'))
			});
			app = res.app;
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? 'Save failed';
		} finally {
			saving = false;
		}
	}

	async function handleRegenerateSecret() {
		regenerating = true;
		try {
			const res = await gitpierApps.regenerateSecret(appId);
			newSecret = res.client_secret;
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? '';
		} finally {
			regenerating = false;
		}
	}

	async function handleGenerateKey() {
		generatingKey = true;
		try {
			const res = await gitpierApps.generateKey(appId);
			keys = [...keys, res.key];
			newPrivateKey = res.private_key;
			newKeyFingerprint = res.key.fingerprint;
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? '';
		} finally {
			generatingKey = false;
		}
	}

	async function handleDeleteKey(keyId: number) {
		try {
			await gitpierApps.deleteKey(appId, keyId);
			keys = keys.filter((k) => k.id !== keyId);
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? '';
		}
	}

	async function handleDeleteApp() {
		deletingApp = true;
		try {
			await gitpierApps.delete(appId);
			goto(`/${handle}/settings/apps`);
		} catch (e: unknown) {
			saveError = (e as { message?: string }).message ?? '';
			deletingApp = false;
		}
	}

	async function copy(text: string, flag: 'secret' | 'clientid') {
		await navigator.clipboard.writeText(text);
		if (flag === 'secret') {
			copiedSecret = true;
			setTimeout(() => (copiedSecret = false), 2000);
		} else {
			copiedClientID = true;
			setTimeout(() => (copiedClientID = false), 2000);
		}
	}

	async function downloadKey() {
		const blob = new Blob([newPrivateKey], { type: 'application/x-pem-file' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = `${app?.slug ?? 'app'}.${newKeyFingerprint}.private-key.pem`;
		a.click();
		URL.revokeObjectURL(url);
	}
</script>

<svelte:head>
	<title>{app ? `${app.name} — GitPier App` : 'GitPier App'}</title>
</svelte:head>

<div class="max-w-2xl">
	<div class="flex items-center gap-3 mb-6">
		<a href="/{handle}/settings/apps" class="text-muted-foreground hover:text-foreground transition-colors">
			<ArrowLeft class="h-4 w-4" />
		</a>
		{#if app}
			<div class="flex items-center gap-2">
				<h1 class="text-xl font-semibold text-foreground">{app.name}</h1>
				{#if app.is_public}
					<Badge variant="secondary" class="text-xs">Public</Badge>
				{:else}
					<Badge variant="outline" class="text-xs">Private</Badge>
				{/if}
			</div>
		{:else}
			<h1 class="text-xl font-semibold text-foreground">GitPier App</h1>
		{/if}
	</div>

	{#if loading}
		<div class="flex items-center justify-center py-20">
			<Loader class="h-5 w-5 animate-spin text-muted-foreground" />
		</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if app}
		<!-- One-time secret reveal -->
		{#if newSecret}
			<div class="mb-6 rounded-md border border-amber-800/40 bg-amber-900/20 p-4">
				<div class="flex items-start gap-2 mb-2">
					<AlertTriangle class="h-4 w-4 text-amber-400 shrink-0 mt-0.5" />
					<p class="text-sm font-semibold text-amber-300">Save your client secret now</p>
				</div>
				<p class="text-xs text-amber-400/80 mb-3">This secret will never be shown again.</p>
				<div class="flex items-center gap-2">
					<code class="flex-1 rounded border border-amber-800/40 bg-background/50 px-3 py-2 text-xs font-mono text-amber-300 break-all">{newSecret}</code>
					<Button type="button" variant="outline" size="icon" class="h-8 w-8 shrink-0" onclick={() => copy(newSecret, 'secret')}>
						{#if copiedSecret}<Check class="h-3.5 w-3.5" />{:else}<Copy class="h-3.5 w-3.5" />{/if}
					</Button>
				</div>
			</div>
		{/if}

		{#if saveError}
			<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{saveError}</div>
		{/if}

		<!-- Permissions shortcut -->
		<a
			href="/{handle}/settings/apps/{appId}/permissions"
			class="flex items-center justify-between rounded-md border border-border bg-card px-5 py-4 mb-6 hover:bg-secondary/40 transition-colors group"
		>
			<div>
				<p class="text-sm font-semibold text-foreground">Permissions &amp; events</p>
				<p class="text-xs text-muted-foreground mt-0.5">View and update what this app can access</p>
			</div>
			<svg class="h-4 w-4 text-muted-foreground group-hover:text-foreground transition-colors" fill="none" stroke="currentColor" stroke-width="2" viewBox="0 0 24 24">
				<path stroke-linecap="round" stroke-linejoin="round" d="M9 5l7 7-7 7" />
			</svg>
		</a>

		<!-- App credentials -->
		<div class="rounded-md border border-border bg-card p-5 mb-6 space-y-3">
			<h2 class="text-sm font-semibold text-foreground">App credentials</h2>
			<div class="flex items-center gap-2">
				<span class="text-xs text-muted-foreground w-24 shrink-0">Client ID</span>
				<code class="flex-1 rounded border border-border bg-background/50 px-3 py-1.5 text-xs font-mono text-foreground">{app.client_id}</code>
				<Button type="button" variant="outline" size="icon" class="h-7 w-7 shrink-0" onclick={() => copy(app!.client_id, 'clientid')}>
					{#if copiedClientID}<Check class="h-3 w-3" />{:else}<Copy class="h-3 w-3" />{/if}
				</Button>
			</div>
			<div class="flex items-center gap-2">
				<span class="text-xs text-muted-foreground w-24 shrink-0">Client secret</span>
				<code class="flex-1 rounded border border-border bg-background/50 px-3 py-1.5 text-xs font-mono text-muted-foreground">••••••••••••••••••••••••</code>
				<Button type="button" variant="outline" size="sm" class="shrink-0 text-xs h-7" onclick={handleRegenerateSecret} disabled={regenerating}>
					{#if regenerating}<Loader class="h-3 w-3 animate-spin" />{/if}
					Regenerate
				</Button>
			</div>
		</div>

		<!-- Private keys -->
		<div class="rounded-md border border-border bg-card p-5 mb-6">
			<div class="flex items-center justify-between mb-4">
				<div>
					<h2 class="text-sm font-semibold text-foreground">Private keys</h2>
					<p class="text-xs text-muted-foreground mt-0.5">Used to sign JWTs for server-to-server auth. Up to 10 active keys.</p>
				</div>
				<Button type="button" variant="outline" size="sm" onclick={handleGenerateKey} disabled={generatingKey || keys.length >= 10}>
					{#if generatingKey}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Plus class="h-3.5 w-3.5" />{/if}
					Generate key
				</Button>
			</div>

			{#if newPrivateKey}
				<div class="mb-4 rounded-md border border-emerald-800/40 bg-emerald-900/20 p-4">
					<div class="flex items-start gap-2 mb-2">
						<AlertTriangle class="h-4 w-4 text-emerald-400 shrink-0 mt-0.5" />
						<p class="text-sm font-semibold text-emerald-300">Download your private key now</p>
					</div>
					<p class="text-xs text-emerald-400/80 mb-3">
						This key will not be stored. Fingerprint: <code class="font-mono">{newKeyFingerprint}</code>
					</p>
					<Button type="button" variant="outline" size="sm" onclick={downloadKey}>Download <code class="ml-1 text-xs">.pem</code></Button>
				</div>
			{/if}

			{#if keys.length === 0}
				<div class="rounded border border-border bg-secondary/30 px-4 py-6 text-center">
					<Key class="h-6 w-6 text-muted-foreground mx-auto mb-2" />
					<p class="text-xs text-muted-foreground">No private keys. Generate one to authenticate as this app.</p>
				</div>
			{:else}
				<div class="divide-y divide-border rounded border border-border overflow-hidden">
					{#each keys as k}
						<div class="flex items-center gap-3 px-4 py-3">
							<Key class="h-4 w-4 text-muted-foreground shrink-0" />
							<div class="flex-1 min-w-0">
								<p class="text-xs font-mono text-foreground truncate">{k.fingerprint}</p>
								<p class="text-xs text-muted-foreground">Added {new Date(k.created_at).toLocaleDateString()}</p>
							</div>
							<Button type="button" variant="ghost" size="icon" class="h-7 w-7 text-muted-foreground hover:text-red-400" onclick={() => handleDeleteKey(k.id)}>
								<Trash2 class="h-3.5 w-3.5" />
							</Button>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Installations -->
		<div class="rounded-md border border-border bg-card p-5 mb-6">
			<h2 class="text-sm font-semibold text-foreground mb-1">Installations</h2>
			<p class="text-xs text-muted-foreground mb-4">{installations.length} installation{installations.length !== 1 ? 's' : ''}</p>
			{#if installations.length === 0}
				<div class="rounded border border-border bg-secondary/30 px-4 py-6 text-center">
					<Boxes class="h-6 w-6 text-muted-foreground mx-auto mb-2" />
					<p class="text-xs text-muted-foreground">No installations yet.</p>
				</div>
			{:else}
				<div class="divide-y divide-border rounded border border-border overflow-hidden">
					{#each installations as inst}
						<div class="flex items-center gap-3 px-4 py-3">
							<div class="flex-1 min-w-0">
								<p class="text-xs text-foreground">{inst.account_type === 'org' ? 'Organization' : 'User'} #{inst.account_id}</p>
								<p class="text-xs text-muted-foreground">{inst.repository_selection === 'all' ? 'All repositories' : 'Selected repositories'}</p>
								{#if inst.suspended_at}
									<Badge variant="destructive" class="text-xs mt-1">Suspended</Badge>
								{/if}
							</div>
							<div class="flex gap-1 shrink-0">
								{#if inst.suspended_at}
									<Button
										type="button"
										variant="outline"
										size="sm"
										class="text-xs h-7"
										onclick={async () => {
											await gitpierApps.unsuspendInstallation(inst.id);
											installations = await gitpierApps.listInstallations(appId).then((r) => r.installations ?? []);
										}}>Unsuspend</Button
									>
								{:else}
									<Button
										type="button"
										variant="outline"
										size="sm"
										class="text-xs h-7"
										onclick={async () => {
											await gitpierApps.suspendInstallation(inst.id);
											installations = await gitpierApps.listInstallations(appId).then((r) => r.installations ?? []);
										}}>Suspend</Button
									>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			{/if}
		</div>

		<!-- App settings form -->
		<form onsubmit={handleSave} class="space-y-6">
			<div class="rounded-md border border-border bg-card p-6 space-y-5">
				<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Basic information</h2>
				<div>
					<label for="name" class="block text-sm font-semibold text-foreground mb-1.5">App name <span class="text-red-400">*</span></label>
					<input
						id="name"
						type="text"
						bind:value={name}
						required
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<div>
					<label for="desc" class="block text-sm font-semibold text-foreground mb-1.5">Description</label>
					<textarea
						id="desc"
						bind:value={description}
						rows={3}
						class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none"
					></textarea>
				</div>
				<div>
					<label for="homepage" class="block text-sm font-semibold text-foreground mb-1.5">Homepage URL <span class="text-red-400">*</span></label>
					<input
						id="homepage"
						type="url"
						bind:value={homepageURL}
						required
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<div>
					<label for="logo" class="block text-sm font-semibold text-foreground mb-1.5">Logo URL</label>
					<input
						id="logo"
						type="url"
						bind:value={logoURL}
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<label class="flex items-center gap-2 cursor-pointer">
					<input type="checkbox" bind:checked={isPublic} class="rounded border-border" />
					<span class="text-sm font-semibold text-foreground">Make this app public</span>
				</label>
			</div>

			<div class="rounded-md border border-border bg-card p-6 space-y-4">
				<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Callback URLs</h2>
				<div class="space-y-2">
					{#each callbackURLs as _, i}
						<div class="flex gap-2">
							<input
								type="url"
								bind:value={callbackURLs[i]}
								placeholder="https://example.com/callback"
								class="h-9 flex-1 rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
							/>
							{#if callbackURLs.length > 1}
								<Button
									type="button"
									variant="outline"
									size="icon"
									class="h-9 w-9 shrink-0"
									onclick={() => {
										callbackURLs = callbackURLs.filter((_, idx) => idx !== i);
									}}
								>
									<Trash2 class="h-3.5 w-3.5" />
								</Button>
							{/if}
						</div>
					{/each}
				</div>
				<Button
					type="button"
					variant="outline"
					size="sm"
					onclick={() => {
						callbackURLs = [...callbackURLs, ''];
					}}
				>
					<Plus class="h-3.5 w-3.5" /> Add URL
				</Button>
				<div class="space-y-2 pt-2 border-t border-border">
					<label class="flex items-center gap-2 cursor-pointer"
						><input type="checkbox" bind:checked={expireUserTokens} class="rounded border-border" /><span class="text-sm text-foreground">Expire user authorization tokens</span></label
					>
					<label class="flex items-center gap-2 cursor-pointer"
						><input type="checkbox" bind:checked={requestUserAuth} class="rounded border-border" /><span class="text-sm text-foreground"
							>Request user authorization during installation</span
						></label
					>
					<label class="flex items-center gap-2 cursor-pointer"
						><input type="checkbox" bind:checked={enableDeviceFlow} class="rounded border-border" /><span class="text-sm text-foreground">Enable Device Flow</span></label
					>
				</div>
			</div>

			<div class="rounded-md border border-border bg-card p-6 space-y-4">
				<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Post installation</h2>
				<div>
					<label for="setup" class="block text-sm font-semibold text-foreground mb-1.5">Setup URL</label>
					<input
						id="setup"
						type="url"
						bind:value={setupURL}
						placeholder="https://example.com/setup"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<label class="flex items-center gap-2 cursor-pointer">
					<input type="checkbox" bind:checked={redirectOnUpdate} class="rounded border-border" />
					<span class="text-sm text-foreground">Redirect on update</span>
				</label>
			</div>

			<div class="rounded-md border border-border bg-card p-6 space-y-4">
				<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Webhook</h2>
				<label class="flex items-center gap-2 cursor-pointer">
					<input type="checkbox" bind:checked={webhookActive} class="rounded border-border" />
					<span class="text-sm text-foreground">Active</span>
				</label>
				{#if webhookActive}
					<div>
						<label for="wh-url" class="block text-sm font-semibold text-foreground mb-1.5">Webhook URL</label>
						<input
							id="wh-url"
							type="url"
							bind:value={webhookURL}
							placeholder="https://example.com/events"
							class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
						/>
					</div>
				{/if}
			</div>

			<div class="rounded-md border border-border bg-card p-6 space-y-5">
				<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Permissions</h2>
				<h3 class="text-sm font-semibold text-foreground">Repository</h3>
				<div class="space-y-3">
					{#each REPO_PERMS as perm}
						<div class="flex items-center justify-between gap-4">
							<p class="text-sm text-foreground">{PERM_LABELS[perm]}</p>
							<select
								bind:value={repoPerms[perm]}
								class="h-8 rounded-md border border-border bg-background px-2 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
							>
								<option value="none">No access</option>
								<option value="read">Read-only</option>
								{#if perm !== 'metadata' && perm !== 'workflows'}<option value="write">Read & write</option>{/if}
							</select>
						</div>
					{/each}
				</div>
				<div class="pt-3 border-t border-border">
					<h3 class="text-sm font-semibold text-foreground mb-3">Organization</h3>
					<div class="space-y-3">
						{#each ORG_PERMS as perm}
							<div class="flex items-center justify-between gap-4">
								<p class="text-sm text-foreground">{PERM_LABELS[perm]}</p>
								<select
									bind:value={orgPerms[perm]}
									class="h-8 rounded-md border border-border bg-background px-2 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
								>
									<option value="none">No access</option>
									<option value="read">Read-only</option>
									<option value="write">Read & write</option>
								</select>
							</div>
						{/each}
					</div>
				</div>
				<div class="pt-3 border-t border-border">
					<h3 class="text-sm font-semibold text-foreground mb-3">Account</h3>
					<div class="space-y-3">
						{#each ACCT_PERMS as perm}
							<div class="flex items-center justify-between gap-4">
								<p class="text-sm text-foreground">{PERM_LABELS[perm]}</p>
								<select
									bind:value={acctPerms[perm]}
									class="h-8 rounded-md border border-border bg-background px-2 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
								>
									<option value="none">No access</option>
									<option value="read">Read-only</option>
									{#if perm === 'profile' || perm === 'ssh_keys'}<option value="write">Read & write</option>{/if}
								</select>
							</div>
						{/each}
					</div>
				</div>
			</div>

			<div class="flex justify-end">
				<Button type="submit" variant="brand" disabled={saving}>
					{#if saving}<Loader class="h-4 w-4 animate-spin" />{/if}
					Save changes
				</Button>
			</div>
		</form>

		<!-- Danger zone -->
		<div class="mt-6 rounded-md border border-red-800/40 bg-card p-5">
			<h2 class="text-sm font-semibold text-red-400 mb-1">Danger zone</h2>
			{#if confirmDelete}
				<p class="text-sm text-foreground mb-3">Are you sure you want to delete <strong>{app.name}</strong>? This cannot be undone.</p>
				<div class="flex gap-2">
					<Button variant="destructive" size="sm" onclick={handleDeleteApp} disabled={deletingApp}>
						{#if deletingApp}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
						Yes, delete this app
					</Button>
					<Button variant="outline" size="sm" onclick={() => (confirmDelete = false)}>Cancel</Button>
				</div>
			{:else}
				<p class="text-xs text-muted-foreground mb-3">Permanently delete this app and all associated data including keys and installations.</p>
				<Button variant="outline" size="sm" class="border-red-800/40 text-red-400 hover:bg-red-900/20" onclick={() => (confirmDelete = true)}>Delete this GitPier App</Button>
			{/if}
		</div>
	{/if}
</div>
