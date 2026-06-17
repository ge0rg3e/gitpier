<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { orgs } from '$lib/api/client';
	import { Lock, Globe, Loader, ArrowLeft } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { onMount } from 'svelte';

	const handle = $derived(page.params.username as string);

	let name = $state('');
	let description = $state('');
	let isPrivate = $state(false);
	let initializeWithReadme = $state(false);
	let loading = $state(false);
	let error = $state('');

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		try {
			const data = await orgs.get(handle);
			if (!data.is_member) goto(`/${handle}`);
		} catch {
			goto('/');
		}
	});

	const nameValid = $derived(name.length >= 1 && name.length <= 100 && /^[a-zA-Z0-9][a-zA-Z0-9\-._]*$/.test(name));

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (loading || !nameValid) return;
		error = '';
		loading = true;
		try {
			const repo = await orgs.repos.create(handle, { name, description, is_private: isPrivate, initialize_with_readme: initializeWithReadme });
			goto(`/${handle}/${repo.name}`);
		} catch (e: any) {
			error = e.message ?? 'Failed to create repository';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>New repository · {handle} · GitPier</title>
</svelte:head>

<div class="bg-background min-h-screen py-8 px-4">
	<div class="mx-auto max-w-2xl">
		<div class="mb-6">
			<h1 class="text-2xl font-semibold text-foreground">Create a new repository</h1>
			<p class="text-sm text-muted-foreground mt-1">This repository will belong to the <strong>{handle}</strong> organization.</p>
		</div>

		<hr class="border-secondary mb-6" />

		{#if error}
			<div class="mb-6 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
		{/if}

		<form onsubmit={handleSubmit} class="space-y-5">
			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">
					Repository name <span class="text-red-400">*</span>
				</p>
				<div class="flex items-center gap-2">
					<div class="flex h-9 items-center rounded-md border border-border bg-secondary px-3 text-sm text-muted-foreground shrink-0 font-semibold">
						{handle} /
					</div>
					<input
						type="text"
						bind:value={name}
						required
						placeholder="my-project"
						maxlength={100}
						class="h-9 flex-1 rounded-md border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
						class:border-red-500={name && !nameValid}
						class:border-[#3fb950]={name && nameValid}
						class:border-border={!name}
					/>
				</div>
				{#if name && !nameValid}
					<p class="mt-1 text-xs text-red-400">Repository names can only contain alphanumeric characters, hyphens, underscores, and dots.</p>
				{:else if name && nameValid}
					<p class="mt-1 text-xs text-[#3fb950]">✓ {handle}/{name}</p>
				{/if}
			</div>

			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">Description <span class="text-muted-foreground font-normal">(optional)</span></p>
				<input
					type="text"
					bind:value={description}
					placeholder="Short description of your project"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
				/>
			</div>

			<div class="space-y-2">
				<p class="block text-sm font-semibold text-foreground">Visibility</p>
				<label
					class="flex items-start gap-3 p-3 rounded-md border cursor-pointer transition-colors {!isPrivate ? 'border-primary bg-primary/5' : 'border-border bg-card hover:border-border/80'}"
				>
					<input type="radio" bind:group={isPrivate} value={false} class="mt-0.5 accent-primary" />
					<div>
						<div class="flex items-center gap-2"><Globe class="h-4 w-4 text-muted-foreground" /><span class="text-sm font-semibold text-foreground">Public</span></div>
						<p class="text-xs text-muted-foreground mt-0.5">Anyone can see this repository.</p>
					</div>
				</label>
				<label
					class="flex items-start gap-3 p-3 rounded-md border cursor-pointer transition-colors {isPrivate ? 'border-primary bg-primary/5' : 'border-border bg-card hover:border-border/80'}"
				>
					<input type="radio" bind:group={isPrivate} value={true} class="mt-0.5 accent-primary" />
					<div>
						<div class="flex items-center gap-2"><Lock class="h-4 w-4 text-muted-foreground" /><span class="text-sm font-semibold text-foreground">Private</span></div>
						<p class="text-xs text-muted-foreground mt-0.5">Only org members with access can see this repository.</p>
					</div>
				</label>
			</div>

			<div class="space-y-2">
				<p class="block text-sm font-semibold text-foreground">Initialize this repository</p>
				<label class="flex items-center gap-2 text-sm text-foreground">
					<input type="checkbox" bind:checked={initializeWithReadme} class="accent-primary" />
					Add a README file
				</label>
			</div>

			<hr class="border-secondary" />

			<div class="flex items-center justify-end gap-3">
				<Button variant="ghost" type="button" onclick={() => history.back()}>Cancel</Button>
				<Button variant="brand" type="submit" disabled={loading || !nameValid}>
					{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
					Create repository
				</Button>
			</div>
		</form>
	</div>
</div>
