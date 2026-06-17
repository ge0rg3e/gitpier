<script lang="ts">
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { auth, orgs } from '$lib/api/client';
	import InstanceMaintenanceNotice from '$lib/components/InstanceMaintenanceNotice.svelte';
	import { Globe, Building2, Loader } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { onMount } from 'svelte';

	let login = $state('');
	let displayName = $state('');
	let description = $state('');
	let loading = $state(false);
	let error = $state('');
	let showMaintenanceNotice = $state(false);
	let selfHostURL = $state('https://github.com/gitpier/gitpier');

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		try {
			const status = await auth.repoCreationStatus();
			showMaintenanceNotice = !status.can_create_repositories;
			if (status.self_host_url && status.self_host_url.trim() !== '') {
				selfHostURL = status.self_host_url.trim();
			}
		} catch (e: any) {
			if (e?.status === 401) {
				goto('/login');
				return;
			}
			showMaintenanceNotice = false;
		}
	});

	const loginValid = $derived(login.length >= 1 && login.length <= 39 && /^[a-zA-Z0-9]([a-zA-Z0-9\-]{0,37}[a-zA-Z0-9])?$|^[a-zA-Z0-9]$/.test(login));

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (loading || !loginValid || showMaintenanceNotice) return;
		error = '';
		loading = true;
		try {
			const org = await orgs.create({ login, display_name: displayName, description });
			goto(`/${org.login}`);
		} catch (e: any) {
			error = e.message ?? 'Failed to create organization';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Create a new organization · GitPier</title>
</svelte:head>

<div class="bg-background min-h-screen py-8 px-4">
	<div class="mx-auto max-w-2xl">
		{#if showMaintenanceNotice}
			<InstanceMaintenanceNotice {selfHostURL} />
		{:else}
			<div class="mb-6 flex items-center gap-3">
				<Building2 class="h-7 w-7 text-muted-foreground" />
				<div>
					<h1 class="text-2xl font-semibold text-foreground">Create a new organization</h1>
					<p class="text-sm text-muted-foreground mt-0.5">Organizations let teams collaborate across many projects at once.</p>
				</div>
			</div>

			<hr class="border-secondary mb-6" />

			{#if error}
				<div class="mb-6 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
			{/if}

			<form onsubmit={handleSubmit} class="space-y-5">
			<!-- Login / slug -->
			<div>
				<label class="block text-sm font-semibold text-foreground mb-1.5">
					Organization name <span class="text-red-400">*</span>
				</label>
				<input
					type="text"
					bind:value={login}
					required
					placeholder="my-organization"
					maxlength={39}
					class="h-9 w-full rounded-md border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
					class:border-red-500={login && !loginValid}
					class:border-[#3fb950]={login && loginValid}
					class:border-border={!login}
				/>
				{#if login && !loginValid}
					<p class="mt-1 text-xs text-red-400">Name must be 1–39 characters, alphanumeric or hyphens, cannot start or end with a hyphen.</p>
				{:else if login && loginValid}
					<p class="mt-1 text-xs text-[#3fb950]">✓ Available at /{login}</p>
				{/if}
				<p class="mt-1 text-xs text-muted-foreground">This will be your organization's URL slug.</p>
			</div>

			<!-- Display name -->
			<div>
				<label class="block text-sm font-semibold text-foreground mb-1.5">
					Display name <span class="text-muted-foreground font-normal">(optional)</span>
				</label>
				<input
					type="text"
					bind:value={displayName}
					placeholder="Acme Corp"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
				/>
			</div>

			<!-- Description -->
			<div>
				<label class="block text-sm font-semibold text-foreground mb-1.5">
					Description <span class="text-muted-foreground font-normal">(optional)</span>
				</label>
				<textarea
					bind:value={description}
					rows={3}
					placeholder="What does this organization do?"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all resize-none"
				></textarea>
			</div>

			<!-- Visibility note -->
			<div class="flex items-start gap-3 rounded-md border border-border bg-card p-4">
				<Globe class="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
				<div>
					<p class="text-sm font-semibold text-foreground">Public organization</p>
					<p class="text-xs text-muted-foreground mt-0.5">Your organization profile and public repositories will be visible to everyone.</p>
				</div>
			</div>

			<hr class="border-secondary" />

				<div class="flex items-center justify-end gap-3">
					<Button variant="ghost" type="button" onclick={() => history.back()}>Cancel</Button>
					<Button variant="brand" type="submit" disabled={loading || !loginValid}>
						{#if loading}
							<Loader class="h-4 w-4 animate-spin" />
						{/if}
						Create organization
					</Button>
				</div>
			</form>
		{/if}
	</div>
</div>
