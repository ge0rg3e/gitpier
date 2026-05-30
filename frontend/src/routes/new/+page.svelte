<script lang="ts">
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { auth, repos, orgs, type Organization } from '$lib/api/client';
	import InstanceMaintenanceNotice from '$lib/components/InstanceMaintenanceNotice.svelte';
	import { Lock, Globe, Loader, ChevronDown, Building2, User } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { onMount } from 'svelte';
	import { mediaUrl } from '$lib/utils';

	let name = $state('');
	let description = $state('');
	let isPrivate = $state(false);
	let initializeWithReadme = $state(false);
	let loading = $state(false);
	let error = $state('');

	let myOrgs = $state<Organization[]>([]);
	let selectedOwner = $state('');
	let ownerDropdownOpen = $state(false);
	let showMaintenanceNotice = $state(false);
	let selfHostURL = $state('https://github.com/gitpier/gitpier');

	const isOrgOwner = $derived(selectedOwner !== '' && selectedOwner !== authStore.user?.username);

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}

		selectedOwner = authStore.user?.username ?? '';

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

		if (showMaintenanceNotice) {
			return;
		}

		try {
			myOrgs = await orgs.listMyOrgs();
		} catch {
			myOrgs = [];
		}
	});

	const nameValid = $derived(name.length >= 1 && name.length <= 100 && /^[a-zA-Z0-9][a-zA-Z0-9\-._]*$/.test(name));

	function selectOwner(owner: string) {
		selectedOwner = owner;
		ownerDropdownOpen = false;
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (loading || !nameValid || showMaintenanceNotice) return;
		error = '';
		loading = true;
		try {
			let repoName: string;
			let ownerName: string;
			if (isOrgOwner) {
				const repo = await orgs.repos.create(selectedOwner, { name, description, is_private: isPrivate, initialize_with_readme: initializeWithReadme });
				repoName = repo.name;
				ownerName = selectedOwner;
			} else {
				const repo = await repos.create({ name, description, is_private: isPrivate, initialize_with_readme: initializeWithReadme });
				repoName = repo.name;
				ownerName = repo.owner?.username ?? authStore.user?.username ?? selectedOwner;
			}
			goto(`/${ownerName}/${repoName}`);
		} catch (e: any) {
			error = e.message ?? 'Failed to create repository';
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Create a new repository · GitPier</title>
</svelte:head>

<div class="bg-background min-h-screen py-8 px-4">
	<div class="mx-auto max-w-2xl">
		{#if showMaintenanceNotice}
			<InstanceMaintenanceNotice {selfHostURL} />
		{:else}
			<div class="mb-6">
				<h1 class="text-2xl font-semibold text-foreground">Create a new repository</h1>
				<p class="text-sm text-muted-foreground mt-1">A repository contains all project files, including the revision history.</p>
			</div>

			<hr class="border-secondary mb-6" />

			{#if error}
				<div class="mb-6 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
			{/if}

			<form onsubmit={handleSubmit} class="space-y-5">
			<!-- Owner / name row -->
			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">
					Owner / Repository name <span class="text-red-400">*</span>
				</p>
				<div class="flex items-center gap-2">
					<!-- Owner dropdown -->
					<div class="relative shrink-0">
						<button
							type="button"
							onclick={() => (ownerDropdownOpen = !ownerDropdownOpen)}
							class="flex h-9 items-center gap-2 rounded-md border border-border bg-secondary px-3 text-sm font-semibold text-foreground hover:bg-secondary/80 transition-colors focus:outline-none focus:ring-1 focus:ring-primary"
						>
							{#if isOrgOwner}
								<Building2 class="h-4 w-4 text-muted-foreground shrink-0" />
							{:else if authStore.user?.avatar_url}
								<img src={mediaUrl(authStore.user.avatar_url)} alt="" class="h-4 w-4 rounded-full object-cover shrink-0" />
							{:else}
								<User class="h-4 w-4 text-muted-foreground shrink-0" />
							{/if}
							{selectedOwner || '…'}
							<ChevronDown class="h-3.5 w-3.5 text-muted-foreground" />
						</button>

						{#if ownerDropdownOpen}
							<!-- Backdrop -->
							<button type="button" class="fixed inset-0 z-10" onclick={() => (ownerDropdownOpen = false)} tabindex="-1" aria-hidden="true"></button>
							<div class="absolute left-0 top-10 z-20 min-w-50 rounded-md border border-border bg-popover shadow-lg py-1">
								<p class="px-3 py-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wider">Personal account</p>
								<button
									type="button"
									onclick={() => selectOwner(authStore.user?.username ?? '')}
									class="flex w-full items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-secondary transition-colors"
									class:font-semibold={selectedOwner === authStore.user?.username}
								>
									{#if authStore.user?.avatar_url}
										<img src={mediaUrl(authStore.user.avatar_url)} alt="" class="h-5 w-5 rounded-full object-cover shrink-0" />
									{:else}
										<div class="h-5 w-5 rounded-full bg-primary/20 flex items-center justify-center shrink-0">
											<User class="h-3 w-3 text-primary" />
										</div>
									{/if}
									{authStore.user?.username}
									{#if selectedOwner === authStore.user?.username}
										<span class="ml-auto text-primary text-xs">✓</span>
									{/if}
								</button>

								{#if myOrgs.length > 0}
									<div class="my-1 border-t border-border"></div>
									<p class="px-3 py-1.5 text-xs font-semibold text-muted-foreground uppercase tracking-wider">Organizations</p>
									{#each myOrgs as org}
										<button
											type="button"
											onclick={() => selectOwner(org.login)}
											class="flex w-full items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-secondary transition-colors"
											class:font-semibold={selectedOwner === org.login}
										>
											{#if org.avatar_url}
												<img src={mediaUrl(org.avatar_url)} alt="" class="h-5 w-5 rounded-md object-cover shrink-0" />
											{:else}
												<div class="h-5 w-5 rounded-md bg-brand/20 flex items-center justify-center shrink-0">
													<Building2 class="h-3 w-3 text-brand" />
												</div>
											{/if}
											{org.display_name || org.login}
											{#if selectedOwner === org.login}
												<span class="ml-auto text-primary text-xs">✓</span>
											{/if}
										</button>
									{/each}
								{/if}
							</div>
						{/if}
					</div>

					<span class="text-muted-foreground font-semibold text-sm">/</span>

					<input
						type="text"
						bind:value={name}
						required
						placeholder="my-awesome-project"
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
					<p class="mt-1 text-xs text-[#3fb950]">✓ {selectedOwner}/{name}</p>
				{/if}
			</div>

			<!-- Description -->
			<div>
				<div class="flex items-center justify-between mb-1.5">
					<label for="desc" class="text-sm font-semibold text-foreground">Description <span class="text-muted-foreground font-normal">(optional)</span></label>
				</div>
				<input
					id="desc"
					type="text"
					bind:value={description}
					placeholder="Short description of your project"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary transition-all"
				/>
			</div>

			<hr class="border-secondary" />

			<!-- Visibility -->
			<div class="space-y-3">
				<label
					class="flex items-start gap-3 p-3 rounded-md border cursor-pointer transition-colors"
					class:border-primary={!isPrivate}
					class:bg-card={!isPrivate}
					class:border-border={isPrivate}
				>
					<input type="radio" bind:group={isPrivate} value={false} class="mt-0.5 accent-primary" />
					<div>
						<div class="flex items-center gap-2">
							<Globe class="h-4 w-4 text-muted-foreground" />
							<span class="text-sm font-semibold text-foreground">Public</span>
						</div>
						<p class="text-xs text-muted-foreground mt-0.5">Anyone on the internet can see this repository. You choose who can commit.</p>
					</div>
				</label>
				<label
					class="flex items-start gap-3 p-3 rounded-md border cursor-pointer transition-colors"
					class:border-primary={isPrivate}
					class:bg-card={isPrivate}
					class:border-border={!isPrivate}
				>
					<input type="radio" bind:group={isPrivate} value={true} class="mt-0.5 accent-primary" />
					<div>
						<div class="flex items-center gap-2">
							<Lock class="h-4 w-4 text-muted-foreground" />
							<span class="text-sm font-semibold text-foreground">Private</span>
						</div>
						<p class="text-xs text-muted-foreground mt-0.5">You choose who can see and commit to this repository.</p>
					</div>
				</label>
			</div>

			<hr class="border-secondary" />

			<div class="space-y-2">
				<p class="block text-sm font-semibold text-foreground">Initialize this repository</p>
				<label class="flex items-center gap-2 text-sm text-foreground">
					<input type="checkbox" bind:checked={initializeWithReadme} class="accent-primary" />
					Add a README file
				</label>
			</div>

			<hr class="border-secondary" />

				<div class="flex items-center gap-3 pt-1">
					<Button variant="brand" type="submit" disabled={loading || !nameValid}>
						{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
						Create repository
					</Button>
				</div>
			</form>
		{/if}
	</div>
</div>
