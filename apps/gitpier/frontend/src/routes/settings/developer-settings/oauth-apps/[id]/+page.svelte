<script lang="ts">
	import { onMount } from 'svelte';
	import { goto, afterNavigate } from '$app/navigation';
	import { page } from '$app/stores';
	import { authStore } from '$lib/stores/auth.svelte';
	import { oauthApps, type OAuthApp } from '$lib/api/client';
	import { formatDate } from '$lib/utils';
	import { Loader, ArrowLeft, Copy, RotateCcw, Trash2, AppWindow, CheckCircle } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	const appId = $derived(Number($page.params.id));

	let app = $state<OAuthApp | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let saveError = $state('');

	// Form fields
	let name = $state('');
	let description = $state('');
	let homepageURL = $state('');
	let callbackURL = $state('');
	let logoURL = $state('');
	let enableDeviceFlow = $state(false);

	// Secret management
	let revealedSecret = $state('');
	let copiedSecret = $state(false);
	let regenerating = $state(false);
	let showRegenConfirm = $state(false);

	// Delete
	let showDeleteConfirm = $state(false);
	let deleting = $state(false);

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}

		// Check if this is a fresh creation — secret is in the URL hash
		const hash = window.location.hash;
		if (hash.startsWith('#secret=')) {
			revealedSecret = decodeURIComponent(hash.slice('#secret='.length));
			// Remove the secret from the URL immediately
			history.replaceState(null, '', window.location.pathname + (window.location.search || ''));
		}

		try {
			const data = await oauthApps.get(appId);
			app = data.app;
			name = app.name;
			description = app.description ?? '';
			homepageURL = app.homepage_url;
			callbackURL = app.callback_url;
			logoURL = app.logo_url ?? '';
			enableDeviceFlow = app.enable_device_flow;
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function handleSave(e: SubmitEvent) {
		e.preventDefault();
		saving = true;
		saveError = '';
		try {
			const data = await oauthApps.update(appId, {
				name,
				description,
				homepage_url: homepageURL,
				callback_url: callbackURL,
				logo_url: logoURL,
				enable_device_flow: enableDeviceFlow
			});
			app = data.app;
		} catch (e: any) {
			saveError = e.message;
		} finally {
			saving = false;
		}
	}

	async function handleRegenerate() {
		regenerating = true;
		try {
			const data = await oauthApps.regenerateSecret(appId);
			revealedSecret = data.client_secret;
			showRegenConfirm = false;
		} catch (e: any) {
			saveError = e.message;
		} finally {
			regenerating = false;
		}
	}

	async function handleDelete() {
		deleting = true;
		try {
			await oauthApps.delete(appId);
			goto('/settings/developer-settings/oauth-apps');
		} catch (e: any) {
			saveError = e.message;
			deleting = false;
		}
	}

	function copySecret() {
		if (!revealedSecret) return;
		navigator.clipboard.writeText(revealedSecret).then(() => {
			copiedSecret = true;
			setTimeout(() => (copiedSecret = false), 2000);
		});
	}
</script>

<svelte:head>
	<title>{app ? app.name : 'OAuth App'} · Settings</title>
</svelte:head>

<div class="max-w-2xl">
	<div class="flex items-center gap-3 mb-6">
		<a href="/settings/developer-settings/oauth-apps" class="text-muted-foreground hover:text-foreground transition-colors">
			<ArrowLeft class="h-4 w-4" />
		</a>
		<h1 class="text-xl font-semibold text-foreground">{app?.name ?? 'OAuth App'}</h1>
	</div>

	{#if loading}
		<div class="space-y-4">
			{#each Array(4) as _}
				<div class="h-12 rounded-md border border-border bg-card animate-pulse"></div>
			{/each}
		</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if app}
		<!-- Client credentials card -->
		<div class="rounded-md border border-border bg-card p-5 mb-5">
			<h2 class="text-sm font-semibold text-foreground mb-4">Client credentials</h2>
			<div class="space-y-3">
				<div>
					<p class="text-xs text-muted-foreground mb-1">Client ID</p>
					<div class="flex items-center gap-2">
						<code class="flex-1 rounded border border-border bg-secondary/50 px-3 py-1.5 text-xs font-mono text-foreground">{app.client_id}</code>
						<button
							onclick={() => navigator.clipboard.writeText(app!.client_id)}
							class="inline-flex h-7 w-7 items-center justify-center rounded border border-border bg-secondary text-muted-foreground hover:text-foreground transition-colors"
							title="Copy client ID"
						>
							<Copy class="h-3.5 w-3.5" />
						</button>
					</div>
				</div>

				<div>
					<p class="text-xs text-muted-foreground mb-1">Client secret</p>
					{#if revealedSecret}
						<div class="rounded-md border border-yellow-800/40 bg-yellow-900/10 px-4 py-3 mb-2">
							<p class="text-xs text-yellow-400 mb-2 font-semibold">Make sure to copy your client secret now. You won't be able to see it again.</p>
							<div class="flex items-center gap-2">
								<code class="flex-1 rounded border border-border bg-secondary/50 px-3 py-1.5 text-xs font-mono text-foreground break-all">{revealedSecret}</code>
								<button
									onclick={copySecret}
									class="inline-flex h-7 w-7 shrink-0 items-center justify-center rounded border border-border bg-secondary text-muted-foreground hover:text-foreground transition-colors"
									title="Copy secret"
								>
									{#if copiedSecret}
										<CheckCircle class="h-3.5 w-3.5 text-green-400" />
									{:else}
										<Copy class="h-3.5 w-3.5" />
									{/if}
								</button>
							</div>
						</div>
					{:else}
						<p class="text-xs text-muted-foreground italic mb-2">The client secret is not displayed for security reasons.</p>
					{/if}

					{#if showRegenConfirm}
						<div class="rounded-md border border-red-800/40 bg-red-900/10 p-3 mt-2">
							<p class="text-xs text-red-300 mb-3">
								Regenerating the client secret will invalidate the current secret. Any applications using the old secret will stop working immediately.
							</p>
							<div class="flex gap-2">
								<Button variant="destructive" size="sm" onclick={handleRegenerate} disabled={regenerating}>
									{#if regenerating}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
									Yes, regenerate secret
								</Button>
								<Button variant="outline" size="sm" onclick={() => (showRegenConfirm = false)}>Cancel</Button>
							</div>
						</div>
					{:else}
						<button onclick={() => (showRegenConfirm = true)} class="inline-flex items-center gap-1.5 text-xs text-blue-400 hover:text-blue-300 transition-colors mt-1">
							<RotateCcw class="h-3 w-3" />
							Generate a new client secret
						</button>
					{/if}
				</div>
			</div>

			<div class="mt-4 pt-4 border-t border-border flex items-center gap-4 text-xs text-muted-foreground">
				<span>{app.authorization_count} user{app.authorization_count !== 1 ? 's' : ''}</span>
				<span>·</span>
				<span>Created {formatDate(app.created_at)}</span>
			</div>
		</div>

		<!-- Edit form -->
		{#if saveError}
			<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{saveError}</div>
		{/if}

		<form onsubmit={handleSave} class="rounded-md border border-border bg-card p-5 space-y-5">
			<h2 class="text-sm font-semibold text-foreground">Application settings</h2>

			<div>
				<label for="app-name" class="block text-sm font-semibold text-foreground mb-1.5">
					Application name <span class="text-red-400">*</span>
				</label>
				<input
					id="app-name"
					type="text"
					bind:value={name}
					required
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<label for="homepage-url" class="block text-sm font-semibold text-foreground mb-1.5">
					Homepage URL <span class="text-red-400">*</span>
				</label>
				<input
					id="homepage-url"
					type="url"
					bind:value={homepageURL}
					required
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<label for="description" class="block text-sm font-semibold text-foreground mb-1.5">Application description</label>
				<textarea
					id="description"
					bind:value={description}
					rows={3}
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none"
				></textarea>
			</div>

			<div>
				<label for="callback-url" class="block text-sm font-semibold text-foreground mb-1.5">
					Authorization callback URL <span class="text-red-400">*</span>
				</label>
				<input
					id="callback-url"
					type="url"
					bind:value={callbackURL}
					required
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<label for="logo-url" class="block text-sm font-semibold text-foreground mb-1.5">Application logo URL</label>
				<input
					id="logo-url"
					type="url"
					bind:value={logoURL}
					placeholder="https://example.com/logo.png"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div class="flex items-start gap-3">
				<input id="device-flow" type="checkbox" bind:checked={enableDeviceFlow} class="mt-0.5 h-4 w-4 rounded border-border accent-blue-500 cursor-pointer" />
				<div>
					<label for="device-flow" class="text-sm font-semibold text-foreground cursor-pointer">Enable Device Flow</label>
					<p class="text-xs text-muted-foreground mt-0.5">Allow this OAuth App to authorize users via the Device Flow.</p>
				</div>
			</div>

			<div class="flex items-center gap-3 pt-2 border-t border-border">
				<Button variant="brand" type="submit" disabled={saving}>
					{#if saving}<Loader class="h-4 w-4 animate-spin" />{/if}
					Save changes
				</Button>
			</div>
		</form>

		<!-- Danger zone -->
		<div class="rounded-md border border-red-800/40 bg-card p-5 mt-5">
			<h2 class="text-sm font-semibold text-red-400 mb-3">Danger Zone</h2>
			{#if showDeleteConfirm}
				<p class="text-xs text-muted-foreground mb-3">Deleting this OAuth app will revoke all existing authorizations. This action cannot be undone.</p>
				<div class="flex gap-2">
					<Button variant="destructive" size="sm" onclick={handleDelete} disabled={deleting}>
						{#if deleting}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
						Yes, delete this app
					</Button>
					<Button variant="outline" size="sm" onclick={() => (showDeleteConfirm = false)}>Cancel</Button>
				</div>
			{:else}
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm font-semibold text-foreground">Delete this OAuth app</p>
						<p class="text-xs text-muted-foreground mt-0.5">Once deleted, all user tokens and authorizations will be revoked.</p>
					</div>
					<Button variant="destructive" size="sm" onclick={() => (showDeleteConfirm = true)}>
						<Trash2 class="h-4 w-4" />
						Delete app
					</Button>
				</div>
			{/if}
		</div>
	{/if}
</div>
