<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { packages, type ContainerPackageDetails } from '$lib/api/client';
	import { Package, Lock, Globe, Trash2, ArrowLeft } from '@lucide/svelte';

	let loading = $state(true);
	let error = $state('');
	let details = $state<ContainerPackageDetails | null>(null);
	let saving = $state(false);
	let deleting = $state(false);
	let saveMsg = $state('');
	let selectedVisibility = $state<'public' | 'private'>('public');

	const username = $derived(page.params.username);
	const imageName = $derived(page.params.image);

	$effect(() => {
		const u = username;
		const i = imageName;
		loading = true;
		error = '';
		details = null;
		saveMsg = '';

		packages
			.get(u!, i!)
			.then((res) => {
				if (username !== u || imageName !== i) return;
				details = res;
				selectedVisibility = res.package.is_public ? 'public' : 'private';
			})
			.catch((e: any) => {
				if (username !== u || imageName !== i) return;
				error = e.message ?? 'Failed to load package';
			})
			.finally(() => {
				if (username === u && imageName === i) loading = false;
			});
	});

	async function saveVisibility() {
		if (!details || saving) return;
		saving = true;
		saveMsg = '';
		try {
			const isPublic = selectedVisibility === 'public';
			const updated = await packages.update(username!, imageName!, { is_public: isPublic });
			details = {
				...details,
				package: {
					...details.package,
					is_public: updated.is_public
				}
			};
			saveMsg = 'Saved';
		} catch (e: any) {
			saveMsg = e.message ?? 'Failed to save package settings';
		} finally {
			saving = false;
		}
	}

	async function deletePackage() {
		if (deleting) return;
		const ok = confirm(`Delete package ${username}/${imageName}? This cannot be undone.`);
		if (!ok) return;

		deleting = true;
		saveMsg = '';
		try {
			await packages.delete(username!, imageName!);
			await goto(`/${username}`);
		} catch (e: any) {
			saveMsg = e.message ?? 'Failed to delete package';
			deleting = false;
		}
	}
</script>

<svelte:head>
	<title>{username}/{imageName} - Settings - GitPier</title>
</svelte:head>

<div class="min-h-screen py-6 px-4">
	<div class="mx-auto max-w-2xl">
		<a href="/{username}/packages/{imageName}" class="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors">
			<ArrowLeft class="h-4 w-4" />
			Back to package
		</a>

		<div class="mb-6 flex items-center gap-2 flex-wrap">
			<Package class="h-5 w-5 text-muted-foreground" />
			<a href="/{username}/packages/{imageName}" class="text-xl font-semibold text-foreground hover:underline">{imageName}</a>
			<span class="text-muted-foreground">/</span>
			<span class="text-xl font-semibold text-foreground">Settings</span>
		</div>

		{#if loading}
			<div class="rounded-md border border-border bg-card p-8 animate-pulse space-y-3">
				<div class="h-5 w-48 rounded bg-secondary"></div>
				<div class="h-10 w-full rounded bg-secondary"></div>
			</div>
		{:else if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
		{:else if details}
			<div class="rounded-md border border-border bg-card overflow-hidden mb-4">
				<div class="px-4 py-3 border-b border-border">
					<h2 class="text-sm font-semibold text-foreground">Package visibility</h2>
				</div>
				<div class="px-4 py-4 space-y-3 text-sm">
					<label class="flex items-start gap-3 cursor-pointer">
						<input type="radio" name="visibility" value="public" bind:group={selectedVisibility} class="mt-0.5" />
						<div>
							<div class="flex items-center gap-1.5 font-medium text-foreground"><Globe class="h-4 w-4" /> Public</div>
							<p class="text-xs text-muted-foreground mt-0.5">Anyone can pull this package.</p>
						</div>
					</label>
					<label class="flex items-start gap-3 cursor-pointer">
						<input type="radio" name="visibility" value="private" bind:group={selectedVisibility} class="mt-0.5" />
						<div>
							<div class="flex items-center gap-1.5 font-medium text-foreground"><Lock class="h-4 w-4" /> Private</div>
							<p class="text-xs text-muted-foreground mt-0.5">Only you and collaborators can pull this package.</p>
						</div>
					</label>
					<div class="pt-1">
						<button
							type="button"
							onclick={saveVisibility}
							disabled={saving}
							class="rounded-md bg-primary text-primary-foreground px-3 py-1.5 text-xs font-medium hover:bg-primary/90 disabled:opacity-50 transition-colors"
						>
							{saving ? 'Saving...' : 'Save'}
						</button>
						{#if saveMsg}
							<span class="ml-3 text-xs {saveMsg === 'Saved' ? 'text-[#3fb950]' : 'text-red-400'}">{saveMsg}</span>
						{/if}
					</div>
				</div>
			</div>

			<div class="rounded-md border border-red-500/30 overflow-hidden">
				<div class="px-4 py-3 border-b border-red-500/30 bg-red-950/20">
					<h2 class="text-sm font-semibold text-red-400">Danger zone</h2>
				</div>
				<div class="px-4 py-4 flex items-center justify-between gap-4 bg-card">
					<div>
						<p class="text-sm font-medium text-foreground">Delete this package</p>
						<p class="text-xs text-muted-foreground">Once deleted, all versions and tags will be removed. This cannot be undone.</p>
					</div>
					<button
						type="button"
						onclick={deletePackage}
						disabled={deleting}
						class="shrink-0 rounded-md border border-red-500/60 bg-red-950/30 text-red-400 hover:bg-red-900/40 px-3 py-1.5 text-xs font-medium transition-colors flex items-center gap-1.5 disabled:opacity-50"
					>
						<Trash2 class="h-3.5 w-3.5" />
						{deleting ? 'Deleting...' : 'Delete package'}
					</button>
				</div>
			</div>
		{/if}
	</div>
</div>
