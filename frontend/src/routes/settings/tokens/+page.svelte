<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { personalAccessTokens, type PersonalAccessToken } from '$lib/api/client';
	import { formatDate } from '$lib/utils';
	import { Button } from '$lib/components/ui/button/index.js';
	import ConfirmPasswordDialog from '$lib/components/ConfirmPasswordDialog.svelte';
	import { CheckCheck, Copy, KeyRound, Loader, Plus, Trash2 } from '@lucide/svelte';

	let tokens = $state<PersonalAccessToken[]>([]);
	let loading = $state(true);
	let error = $state('');
	let showForm = $state(false);
	let adding = $state(false);
	let addError = $state('');
	let name = $state('');
	let allowRead = $state(true);
	let allowWrite = $state(false);
	let password = $state('');
	let createdToken = $state('');
	let copied = $state(false);
	let pendingDeleteId = $state<string | null>(null);
	let showDeleteDialog = $state(false);

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		await loadTokens();
	});

	async function loadTokens() {
		loading = true;
		error = '';
		try {
			const data = await personalAccessTokens.list();
			tokens = data.tokens ?? [];
		} catch (e: any) {
			error = e.message ?? 'Failed to load personal access tokens.';
		} finally {
			loading = false;
		}
	}

	async function handleCreate(e: SubmitEvent) {
		e.preventDefault();
		adding = true;
		addError = '';
		createdToken = '';
		try {
			const scopes = allowWrite ? ['repo:read', 'repo:write'] : allowRead ? ['repo:read'] : [];
			const result = await personalAccessTokens.create({ name: name.trim(), scopes }, password);
			createdToken = result.token;
			tokens = [result.record, ...tokens];
			name = '';
			password = '';
			allowRead = true;
			allowWrite = false;
			showForm = false;
		} catch (e: any) {
			addError = e.message ?? 'Failed to create token.';
		} finally {
			adding = false;
		}
	}

	async function copyCreatedToken() {
		if (!createdToken) return;
		await navigator.clipboard.writeText(createdToken);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	function requestDelete(id: string) {
		pendingDeleteId = id;
		showDeleteDialog = true;
	}

	async function confirmDelete(password: string) {
		if (!pendingDeleteId) return;
		try {
			await personalAccessTokens.delete(pendingDeleteId, password);
			tokens = tokens.filter((token) => token.id !== pendingDeleteId);
		} finally {
			pendingDeleteId = null;
		}
	}

	function scopeLabel(scopes: string): string {
		const parts = scopes.split(/\s+/).filter(Boolean);
		if (parts.includes('repo:write')) return 'Repository read and write';
		return 'Repository read';
	}
</script>

<svelte:head>
	<title>Personal access tokens</title>
</svelte:head>

<div class="max-w-3xl">
	<div class="flex items-center justify-between mb-6">
		<div>
			<h1 class="text-2xl font-semibold text-foreground">Personal access tokens</h1>
			<p class="mt-1 text-sm text-muted-foreground">Use tokens for Git over HTTPS. Account passwords are not accepted by Git clients.</p>
		</div>
		<Button variant="brand" size="sm" onclick={() => (showForm = !showForm)}>
			<Plus class="h-4 w-4" />
			New token
		</Button>
	</div>

	{#if createdToken}
		<div class="mb-6 rounded-md border border-[#3fb950]/40 bg-[#3fb950]/10 p-4">
			<p class="text-sm font-semibold text-foreground">Copy your token now. It will not be shown again.</p>
			<div class="mt-3 flex items-center gap-2 rounded-md border border-border bg-background px-3 py-2">
				<code class="flex-1 truncate text-xs text-muted-foreground">{createdToken}</code>
				<button onclick={copyCreatedToken} class="text-muted-foreground hover:text-foreground" aria-label="Copy token">
					{#if copied}<CheckCheck class="h-4 w-4 text-[#3fb950]" />{:else}<Copy class="h-4 w-4" />{/if}
				</button>
			</div>
		</div>
	{/if}

	{#if showForm}
		<div class="rounded-md border border-border bg-card p-5 mb-6">
			<h2 class="text-base font-semibold text-foreground mb-4">Create personal access token</h2>
			{#if addError}
				<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{addError}</div>
			{/if}
			<form onsubmit={handleCreate} class="space-y-4">
				<div>
					<label for="token-name" class="block text-sm font-semibold text-foreground mb-1.5">Name</label>
					<input
						id="token-name"
						type="text"
						bind:value={name}
						placeholder="e.g. Work laptop"
						required
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<div>
					<p class="block text-sm font-semibold text-foreground mb-2">Scopes</p>
					<label class="flex items-start gap-2 rounded-md border border-border bg-background px-3 py-2">
						<input type="checkbox" bind:checked={allowRead} disabled={allowWrite} class="mt-0.5 rounded border-border" />
						<span>
							<span class="block text-sm font-semibold text-foreground">Repository read</span>
							<span class="block text-xs text-muted-foreground">Clone and pull repositories you can access.</span>
						</span>
					</label>
					<label class="mt-2 flex items-start gap-2 rounded-md border border-border bg-background px-3 py-2">
						<input type="checkbox" bind:checked={allowWrite} onchange={() => (allowRead = true)} class="mt-0.5 rounded border-border" />
						<span>
							<span class="block text-sm font-semibold text-foreground">Repository write</span>
							<span class="block text-xs text-muted-foreground">Push to repositories where you already have write permission.</span>
						</span>
					</label>
				</div>
				<div>
					<label for="confirm-password" class="block text-sm font-semibold text-foreground mb-1.5">Confirm password</label>
					<input
						id="confirm-password"
						type="password"
						bind:value={password}
						required
						autocomplete="current-password"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<div class="flex items-center gap-3">
					<Button variant="brand" type="submit" disabled={adding}>
						{#if adding}<Loader class="h-4 w-4 animate-spin" />{/if}
						Create token
					</Button>
					<Button variant="outline" type="button" onclick={() => (showForm = false)}>Cancel</Button>
				</div>
			</form>
		</div>
	{/if}

	{#if loading}
		<div class="space-y-3">
			{#each Array(2) as _}
				<div class="h-16 rounded-md border border-border bg-card animate-pulse"></div>
			{/each}
		</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{:else if tokens.length === 0}
		<div class="rounded-md border border-border bg-card p-8 text-center">
			<KeyRound class="h-8 w-8 text-muted-foreground mx-auto mb-3" />
			<p class="text-sm font-semibold text-foreground mb-1">No personal access tokens.</p>
			<p class="text-xs text-muted-foreground">Create one to clone, pull, and push over HTTPS.</p>
		</div>
	{:else}
		<div class="divide-y divide-secondary rounded-md border border-border bg-card overflow-hidden">
			{#each tokens as token}
				<div class="flex items-start gap-4 px-4 py-4">
					<KeyRound class="h-5 w-5 text-muted-foreground mt-0.5 shrink-0" />
					<div class="flex-1 min-w-0">
						<p class="text-sm font-semibold text-foreground">{token.name}</p>
						<p class="text-xs text-muted-foreground mt-0.5">Token ending in <span class="font-mono">{token.token_last}</span> · {scopeLabel(token.scopes)}</p>
						<p class="text-xs text-muted-foreground mt-1">
							Created {formatDate(token.created_at)}{token.last_used_at ? ` · Last used ${formatDate(token.last_used_at)}` : ''}
						</p>
					</div>
					<button
						onclick={() => requestDelete(token.id)}
						class="inline-flex h-7 items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 text-xs text-red-400 hover:bg-red-900/30 hover:border-red-800/50 transition-colors shrink-0"
					>
						<Trash2 class="h-3.5 w-3.5" />
						Delete
					</button>
				</div>
			{/each}
		</div>
	{/if}

	<div class="mt-8 rounded-md border border-border bg-card p-4">
		<h3 class="text-sm font-semibold text-foreground mb-2">Use with Git HTTPS</h3>
		<p class="text-sm text-muted-foreground">
			When Git asks for credentials, enter your username and use the personal access token as the password.
		</p>
	</div>
</div>

<ConfirmPasswordDialog
	bind:open={showDeleteDialog}
	title="Delete personal access token"
	description="Confirm your password to delete this token. Any Git clients using it will stop working immediately."
	confirmLabel="Delete token"
	onconfirm={confirmDelete}
/>
