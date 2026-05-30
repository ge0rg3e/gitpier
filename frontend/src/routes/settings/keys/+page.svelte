<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { sshKeys, type SSHKey } from '$lib/api/client';
	import { formatDate } from '$lib/utils';
	import { Key, Trash2, Plus, Loader } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import ConfirmPasswordDialog from '$lib/components/ConfirmPasswordDialog.svelte';

	let keys = $state<SSHKey[]>([]);
	let loading = $state(true);
	let error = $state('');
	let title = $state('');
	let keyContent = $state('');
	let adding = $state(false);
	let addError = $state('');
	let showForm = $state(false);
	let pendingDeleteId = $state<number | null>(null);
	let showDeleteDialog = $state(false);

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		try {
			const data = await sshKeys.list();
			keys = data.keys ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function handleAdd(e: SubmitEvent) {
		e.preventDefault();
		adding = true;
		addError = '';
		try {
			await sshKeys.add(title, keyContent);
			const data = await sshKeys.list();
			keys = data.keys ?? [];
			title = '';
			keyContent = '';
			showForm = false;
		} catch (e: any) {
			addError = e.message;
		} finally {
			adding = false;
		}
	}

	async function handleDelete(id: number) {
		pendingDeleteId = id;
		showDeleteDialog = true;
	}

	async function confirmDeleteKey(password: string) {
		if (pendingDeleteId === null) return;
		try {
			await sshKeys.delete(pendingDeleteId, password);
			keys = keys.filter((k) => k.id !== pendingDeleteId);
		} finally {
			pendingDeleteId = null;
		}
	}
</script>

<svelte:head>
	<title>SSH and GPG keys</title>
</svelte:head>

<div class="max-w-3xl">
	<div class="flex items-center justify-between mb-6">
		<h1 class="text-2xl font-semibold text-foreground">SSH keys</h1>
		<Button variant="brand" size="sm" onclick={() => (showForm = !showForm)}>
			<Plus class="h-4 w-4" />
			New SSH key
		</Button>
	</div>

	{#if showForm}
		<div class="rounded-md border border-border bg-card p-5 mb-6">
			<h2 class="text-base font-semibold text-foreground mb-4">Add new SSH key</h2>
			{#if addError}
				<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{addError}</div>
			{/if}
			<form onsubmit={handleAdd} class="space-y-4">
				<div>
					<label for="key-title" class="block text-sm font-semibold text-foreground mb-1.5">Title</label>
					<input
						id="key-title"
						type="text"
						bind:value={title}
						placeholder="e.g. Personal MacBook"
						required
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<div>
					<label for="key-content" class="block text-sm font-semibold text-foreground mb-1.5">Key</label>
					<textarea
						id="key-content"
						bind:value={keyContent}
						rows={5}
						placeholder="Begins with 'ssh-rsa', 'ecdsa-sha2-nistp256', 'ecdsa-sha2-nistp384', 'ecdsa-sha2-nistp521', 'ssh-ed25519', 'sk-ecdsa-sha2-nistp256@openssh.com', or 'sk-ssh-ed25519@openssh.com'"
						required
						class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none font-mono text-xs"
					></textarea>
				</div>
				<div class="flex items-center gap-3">
					<Button variant="brand" type="submit" disabled={adding}>
						{#if adding}<Loader class="h-4 w-4 animate-spin" />{/if}
						Add SSH key
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
	{:else if keys.length === 0}
		<div class="rounded-md border border-border bg-card p-8 text-center">
			<Key class="h-8 w-8 text-muted-foreground mx-auto mb-3" />
			<p class="text-sm font-semibold text-foreground mb-1">There are no SSH keys associated with your account.</p>
			<p class="text-xs text-muted-foreground">Add a new public key to allow SSH access to your repositories.</p>
		</div>
	{:else}
		<div class="divide-y divide-secondary rounded-md border border-border bg-card overflow-hidden">
			{#each keys as key}
				<div class="flex items-start gap-4 px-4 py-4">
					<Key class="h-5 w-5 text-muted-foreground mt-0.5 shrink-0" />
					<div class="flex-1 min-w-0">
						<p class="text-sm font-semibold text-foreground">{key.title || 'Untitled key'}</p>
						<p class="text-xs text-muted-foreground mt-0.5 font-mono">{key.key ?? '-'}…</p>
						<p class="text-xs text-muted-foreground mt-1">Added {formatDate(key.created_at)}</p>
					</div>
					<button
						onclick={() => handleDelete(key.id)}
						class="inline-flex h-7 items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 text-xs text-red-400 hover:bg-red-900/30 hover:border-red-800/50 transition-colors shrink-0"
					>
						<Trash2 class="h-3.5 w-3.5" />
						Delete
					</button>
				</div>
			{/each}
		</div>
	{/if}

	<!-- SSH setup instructions -->
	<div class="mt-8 rounded-md border border-border bg-card p-4">
		<h3 class="text-sm font-semibold text-foreground mb-2">How to generate and add an SSH key</h3>

		<div class="space-y-4 text-sm text-foreground">
			<div>
				<p class="font-semibold mb-1">1. Generate a new key on your computer</p>
				<pre class="rounded-md border border-border bg-background px-3 py-2 text-xs text-muted-foreground overflow-x-auto"><code>ssh-keygen -t ed25519 -C "your_email@example.com"</code></pre>
			</div>

			<div>
				<p class="font-semibold mb-1">2. Copy your public key</p>
				<pre class="rounded-md border border-border bg-background px-3 py-2 text-xs text-muted-foreground overflow-x-auto"><code>cat ~/.ssh/id_ed25519.pub</code></pre>
			</div>
		</div>
	</div>
</div>

<ConfirmPasswordDialog
	bind:open={showDeleteDialog}
	title="Delete SSH key"
	description="This will permanently remove the SSH key from your account. Enter your password to confirm."
	confirmLabel="Delete key"
	onconfirm={confirmDeleteKey}
/>
