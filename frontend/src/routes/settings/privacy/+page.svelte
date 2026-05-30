<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { users } from '$lib/api/client';
	import { Loader, Download, Trash2 } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	let exporting = $state(false);
	let exportError = $state('');

	let deletePassword = $state('');
	let deleting = $state(false);
	let deleteError = $state('');
	let showDeleteConfirm = $state(false);

	onMount(() => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
		}
	});

	async function handleExport() {
		exporting = true;
		exportError = '';
		try {
			const data = await users.exportData();
			const blob = new Blob([JSON.stringify(data, null, 2)], { type: 'application/json' });
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = 'GitPier-data-export.json';
			a.click();
			URL.revokeObjectURL(url);
		} catch (e: any) {
			exportError = e.message ?? 'Export failed. Please try again.';
		} finally {
			exporting = false;
		}
	}

	async function handleDeleteAccount() {
		if (!deletePassword) {
			deleteError = 'Please enter your password to confirm.';
			return;
		}
		deleting = true;
		deleteError = '';
		try {
			await users.deleteAccount(deletePassword);
			authStore.logout();
			goto('/');
		} catch (e: any) {
			deleteError = e.message ?? 'Failed to delete account. Please try again.';
		} finally {
			deleting = false;
		}
	}
</script>

<svelte:head>
	<title>Privacy &amp; Data – Settings</title>
</svelte:head>

<div class="max-w-xl space-y-8">
	<h1 class="text-2xl font-semibold text-foreground">Privacy &amp; Data</h1>

	<!-- Data export -->
	<section class="rounded-md border border-border bg-card px-5 py-4 space-y-3">
		<h2 class="text-sm font-semibold text-foreground">Export your data</h2>
		<p class="text-xs text-muted-foreground leading-relaxed">
			Download a JSON archive of all personal data we hold about you, including your account details, repositories, and SSH keys. This is your right under GDPR Article 20 (data portability).
		</p>
		{#if exportError}
			<div class="rounded-md border border-red-800/50 bg-red-900/30 px-3 py-2 text-xs text-red-400">{exportError}</div>
		{/if}
		<Button variant="outline" onclick={handleExport} disabled={exporting}>
			{#if exporting}
				<Loader class="h-3.5 w-3.5 animate-spin" />
			{:else}
				<Download class="h-3.5 w-3.5" />
			{/if}
			{exporting ? 'Preparing export…' : 'Download data export'}
		</Button>
	</section>

	<!-- Account deletion -->
	<section class="rounded-md border border-red-800/40 bg-card px-5 py-4 space-y-3">
		<h2 class="text-sm font-semibold text-red-400">Delete account</h2>
		<p class="text-xs text-muted-foreground leading-relaxed">
			Permanently delete your account and all associated data. This action is irreversible and cannot be undone. Your data will be erased within 30 days as required by GDPR Article 17 (right to
			erasure).
		</p>

		{#if !showDeleteConfirm}
			<Button variant="destructive" onclick={() => (showDeleteConfirm = true)}>
				<Trash2 class="h-3.5 w-3.5" />
				Delete my account
			</Button>
		{:else}
			<div class="space-y-3 rounded-md border border-red-800/40 bg-red-900/10 p-4">
				<p class="text-xs font-semibold text-red-400">Are you absolutely sure? This cannot be undone.</p>
				<div>
					<label for="delete_password" class="block text-xs font-semibold text-foreground mb-1.5"> Confirm with your password </label>
					<input
						id="delete_password"
						type="password"
						bind:value={deletePassword}
						placeholder="Your current password"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-red-500 focus:border-red-500"
					/>
				</div>
				{#if deleteError}
					<div class="rounded-md border border-red-800/50 bg-red-900/30 px-3 py-2 text-xs text-red-400">{deleteError}</div>
				{/if}
				<div class="flex gap-2">
					<Button variant="destructive" onclick={handleDeleteAccount} disabled={deleting || !deletePassword}>
						{#if deleting}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Trash2 class="h-3.5 w-3.5" />{/if}
						{deleting ? 'Deleting…' : 'Permanently delete account'}
					</Button>
					<Button
						variant="outline"
						onclick={() => {
							showDeleteConfirm = false;
							deletePassword = '';
							deleteError = '';
						}}
					>
						Cancel
					</Button>
				</div>
			</div>
		{/if}
	</section>
</div>
