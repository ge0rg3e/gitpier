<script lang="ts">
	import { page } from '$app/state';
	import { getContext, onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { repos, type Repository } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { Trash2, AlertTriangle, Loader, Settings, Archive, RotateCcw, Lock, Unlock } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import ConfirmPasswordDialog from '$lib/components/ConfirmPasswordDialog.svelte';

	let repo = $state<Repository | null>(null);
	let loading = $state(true);
	let error = $state('');

	let repoNameValue = $state('');
	let description = $state('');
	let website = $state('');
	let isPrivate = $state(false);
	let defaultBranch = $state('main');
	let saving = $state(false);
	let saveError = $state('');
	let saveSuccess = $state(false);

	let deleting = $state(false);
	let showDeleteDialog = $state(false);

	let archiving = $state(false);
	let unarchiving = $state(false);
	let archiveError = $state('');
	let archiveSuccess = $state('');
	let showArchiveDialog = $state(false);
	let archiveAction = $state<'archive' | 'unarchive'>('archive');

	let showPrivacyDialog = $state(false);
	let togglingPrivacy = $state(false);

	async function togglePrivacy(password: string) {
		togglingPrivacy = true;
		try {
			const updated = await repos.setVisibility(username, repoName, !repo!.is_private, password);
			repo = updated;
			isPrivate = updated.is_private;
			await repoLayout?.reloadMetadata?.();
		} catch (e: any) {
			throw e;
		} finally {
			togglingPrivacy = false;
		}
	}

	const username = $derived(page.params.username as string);
	const repoName = $derived(page.params.repo as string);
	const repoLayout = getContext<{ reloadMetadata?: (ref?: string) => Promise<void> } | null>('repoLayout');

	onMount(async () => {
		while (authStore.loading) await new Promise((r) => setTimeout(r, 10));
		if (!authStore.isAuthenticated) {
			goto('/login');
			return;
		}
		try {
			const repoData = await repos.get(username, repoName);
			repo = repoData.repo;
			if (repo.owner_id !== authStore.user?.id) {
				goto(`/${username}/${repoName}`);
				return;
			}
			repoNameValue = repo.name;
			description = repo.description ?? '';
			website = repo.website ?? '';
			isPrivate = repo.is_private;
			defaultBranch = repo.default_branch;
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function saveGeneral(e: Event) {
		e.preventDefault();
		if (repo?.is_archived) {
			saveError = 'Repository is archived and read-only. Unarchive it to change settings.';
			return;
		}
		saving = true;
		saveError = '';
		saveSuccess = false;
		try {
			const nameChanged = repoNameValue.trim() && repoNameValue !== repoName;
			const updated = await repos.update(username, repoName, {
				name: nameChanged ? repoNameValue.trim() : undefined,
				description,
				website,
				is_private: isPrivate,
				default_branch: defaultBranch
			});
			repo = updated;
			await repoLayout?.reloadMetadata?.();
			if (nameChanged) goto(`/${username}/${repoNameValue}/settings`, { replaceState: true });
			saveSuccess = true;
			setTimeout(() => (saveSuccess = false), 3000);
		} catch (e: any) {
			saveError = e.message;
		} finally {
			saving = false;
		}
	}

	async function deleteRepo(password: string) {
		deleting = true;
		try {
			await repos.delete(username, repoName, password);
			goto(`/${username}`);
		} catch (e: any) {
			deleting = false;
			throw e;
		}
	}

	function formatBytes(bytes: number): string {
		if (bytes === 0) return '0 B';
		const k = 1024;
		const sizes = ['B', 'KB', 'MB', 'GB'];
		const i = Math.floor(Math.log(bytes) / Math.log(k));
		return parseFloat((bytes / Math.pow(k, i)).toFixed(2)) + ' ' + sizes[i];
	}

	function getStorageLimit(): number {
		return repo?.size_limit_bytes || 1;
	}

	function formatArchivedAt(archivedAt?: string): string {
		if (!archivedAt) return '';
		return new Date(archivedAt).toLocaleString();
	}

	function openArchiveDialog(action: 'archive' | 'unarchive') {
		archiveAction = action;
		showArchiveDialog = true;
	}

	async function submitArchiveStatusChange(password: string) {
		if (!repo) return;
		archiveError = '';
		archiveSuccess = '';

		if (archiveAction === 'archive') {
			archiving = true;
			try {
				repo = await repos.archive(username, repoName, password);
				await repoLayout?.reloadMetadata?.();
				archiveSuccess = 'Repository archived. It is now read-only.';
			} catch (e: any) {
				archiveError = e.message ?? 'Failed to archive repository';
				throw e;
			} finally {
				archiving = false;
			}
			return;
		}

		unarchiving = true;
		try {
			repo = await repos.unarchive(username, repoName, password);
			await repoLayout?.reloadMetadata?.();
			archiveSuccess = 'Repository unarchived. Write actions are enabled again.';
		} catch (e: any) {
			archiveError = e.message ?? 'Failed to unarchive repository';
			throw e;
		} finally {
			unarchiving = false;
		}
	}
</script>

<svelte:head>
	<title>General settings</title>
</svelte:head>

{#if loading}
	<div class="max-w-2xl animate-pulse">
		<div class="mb-5 flex items-center gap-2">
			<div class="h-5 w-5 rounded bg-secondary"></div>
			<div class="h-6 w-24 rounded bg-secondary"></div>
		</div>

		<div class="space-y-5">
			<div>
				<div class="mb-1.5 h-4 w-28 rounded bg-secondary"></div>
				<div class="h-9 w-full rounded-md border border-border bg-card"></div>
			</div>
			<div>
				<div class="mb-1.5 h-4 w-36 rounded bg-secondary"></div>
				<div class="h-9 w-full rounded-md border border-border bg-card"></div>
			</div>
			<div>
				<div class="mb-1.5 h-4 w-32 rounded bg-secondary"></div>
				<div class="h-9 w-full rounded-md border border-border bg-card"></div>
			</div>
			<div>
				<div class="mb-1.5 h-4 w-28 rounded bg-secondary"></div>
				<div class="h-9 w-48 rounded-md border border-border bg-card"></div>
			</div>
			<div class="flex justify-end">
				<div class="h-9 w-24 rounded-md bg-secondary"></div>
			</div>
		</div>

		<hr class="border-secondary my-8" />

		<div class="rounded-md border border-red-800/40 bg-card p-4">
			<div class="mb-4 h-4 w-28 rounded bg-secondary"></div>
			<div class="mb-4 h-24 rounded-md border border-border bg-card"></div>
			<div class="h-24 rounded-md border border-border bg-card"></div>
		</div>
	</div>
{:else if error && !repo}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
{:else if repo}
	<div class="max-w-2xl">
		{#if error}
			<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
		{/if}
		{#if saveSuccess}
			<div class="mb-4 rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">Settings saved.</div>
		{/if}
		{#if archiveSuccess}
			<div class="mb-4 rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">{archiveSuccess}</div>
		{/if}
		{#if repo.is_archived}
			<div class="mb-4 rounded-md border border-amber-700/40 bg-amber-900/20 px-4 py-3 text-sm text-amber-300">
				This repository is archived and read-only.
				{#if repo.archived_at}
					Archived at {formatArchivedAt(repo.archived_at)}.
				{/if}
			</div>
		{/if}

		<h1 class="text-xl font-semibold text-foreground mb-5 flex items-center gap-2">
			<Settings class="h-5 w-5 text-muted-foreground" />
			General
		</h1>

		<form onsubmit={saveGeneral} class="space-y-5">
			{#if saveError}
				<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{saveError}</div>
			{/if}

			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">Repository name</p>
				<input
					type="text"
					bind:value={repoNameValue}
					disabled={repo.is_archived}
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">Description <span class="font-normal text-muted-foreground">(optional)</span></p>
				<input
					type="text"
					bind:value={description}
					disabled={repo.is_archived}
					placeholder="Short description of your project"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">Website URL <span class="font-normal text-muted-foreground">(optional)</span></p>
				<input
					type="url"
					bind:value={website}
					disabled={repo.is_archived}
					placeholder="https://example.com"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">Default branch</p>
				<input
					type="text"
					bind:value={defaultBranch}
					disabled={repo.is_archived}
					placeholder="main"
					class="h-9 w-48 rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div class="flex justify-end">
				<Button variant="brand" type="submit" disabled={saving || repo.is_archived}>
					{#if saving}<Loader class="h-4 w-4 animate-spin" />{/if}
					Save changes
				</Button>
			</div>
		</form>

		<hr class="border-secondary my-8" />

		<div>
			<h2 class="text-base font-semibold text-red-400 mb-3 flex items-center gap-2"><AlertTriangle class="h-4 w-4" />Danger Zone</h2>
			{#if archiveError}
				<div class="mb-3 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{archiveError}</div>
			{/if}
			<div class="mb-4 rounded-md border border-border bg-card p-4">
				<div class="flex items-center justify-between">
					<div>
						<h3 class="text-sm font-semibold text-foreground mb-1 flex items-center gap-2">
							{#if repo.is_private}<Lock class="h-4 w-4 text-amber-400" />{:else}<Unlock class="h-4 w-4 text-muted-foreground" />{/if}
							{repo.is_private ? 'Make repository public' : 'Make repository private'}
						</h3>
						<p class="text-xs text-muted-foreground">
							{#if repo.is_private}
								Making this repository public means anyone can see it.
							{:else}
								Only you and collaborators will be able to see it.
							{/if}
						</p>
					</div>
					<Button
						variant="outline"
						size="sm"
						onclick={() => (showPrivacyDialog = true)}
						disabled={togglingPrivacy || repo.is_archived}
						class="ml-4 shrink-0 {repo.is_private ? 'border-amber-700/40 text-amber-300 hover:bg-amber-900/20' : ''} "
					>
						{#if togglingPrivacy}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
						{repo.is_private ? 'Make public' : 'Make private'}
					</Button>
				</div>
			</div>
			<div class="mb-4 rounded-md border border-border bg-card p-4">
				<h3 class="text-sm font-semibold text-foreground mb-1 flex items-center gap-2">
					<Archive class="h-4 w-4 text-amber-400" />
					Repository archive
				</h3>
				<p class="text-xs text-muted-foreground mb-3">Archiving makes this repository read-only. Nobody can push code or change repository settings while it is archived.</p>
				{#if repo.is_archived}
					<Button variant="outline" size="sm" onclick={() => openArchiveDialog('unarchive')} disabled={unarchiving}>
						{#if unarchiving}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<RotateCcw class="h-3.5 w-3.5" />{/if}
						Unarchive repository
					</Button>
				{:else}
					<Button variant="outline" size="sm" onclick={() => openArchiveDialog('archive')} disabled={archiving} class="border-amber-700/40 text-amber-300 hover:bg-amber-900/20">
						{#if archiving}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Archive class="h-3.5 w-3.5" />{/if}
						Archive repository
					</Button>
				{/if}
			</div>
			<p class="text-xs text-muted-foreground mb-4">Once you delete a repository, there is no going back.</p>
			<Button variant="outline" size="sm" onclick={() => (showDeleteDialog = true)} disabled={repo.is_archived} class="border-red-800/40 text-red-400 hover:bg-red-900/20">
				<Trash2 class="h-3.5 w-3.5" />Delete this repository
			</Button>
			{#if repo.is_archived}
				<p class="text-xs text-muted-foreground mt-2">Unarchive this repository before deleting it.</p>
			{/if}
		</div>

		<ConfirmPasswordDialog
			bind:open={showDeleteDialog}
			title="Delete repository"
			description="This will permanently delete {repoName} and all of its contents. Enter your password to confirm."
			confirmLabel="Delete repository"
			onconfirm={deleteRepo}
		/>
		<ConfirmPasswordDialog
			bind:open={showPrivacyDialog}
			title={repo.is_private ? 'Make repository public' : 'Make repository private'}
			description={repo.is_private
				? 'This will make the repository visible to everyone. Enter your password to confirm.'
				: 'This will restrict access to you and collaborators only. Enter your password to confirm.'}
			confirmLabel={repo.is_private ? 'Make public' : 'Make private'}
			onconfirm={togglePrivacy}
		/>
		<ConfirmPasswordDialog
			bind:open={showArchiveDialog}
			title={archiveAction === 'archive' ? 'Archive repository' : 'Unarchive repository'}
			description={archiveAction === 'archive'
				? 'This will make the repository read-only for all users. Enter your password to confirm.'
				: 'This will restore write actions for this repository. Enter your password to confirm.'}
			confirmLabel={archiveAction === 'archive' ? 'Archive repository' : 'Unarchive repository'}
			onconfirm={submitArchiveStatusChange}
		/>
	</div>
{/if}
