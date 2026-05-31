<script lang="ts">
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { goto } from '$app/navigation';
	import { orgs, type ActionsUsage, type OrgSocialLink, type Organization } from '$lib/api/client';
	import { Settings, Trash2, Loader, AlertTriangle, Camera, Zap, Plus, X } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import ConfirmPasswordDialog from '$lib/components/ConfirmPasswordDialog.svelte';
	import { mediaUrl } from '$lib/utils';

	const routeHandle = $derived(page.params.username as string);
	const ctx = getContext<any>('org');

	let saving = $state(false);
	let deleting = $state(false);
	let saveError = $state('');
	let success = $state('');
	let uploadingAvatar = $state(false);
	let avatarError = $state('');

	let displayName = $state('');
	let description = $state('');
	let avatarUrl = $state('');
	let website = $state('');
	let socialLinks = $state<OrgSocialLink[]>([]);
	let location = $state('');

	let showDeleteDialog = $state(false);
	let actionsUsage = $state<ActionsUsage | null>(null);
	let actionsUsageError = $state('');

	$effect(() => {
		if (!ctx.loading && !ctx.isOwner) {
			goto(`/${ctx.org?.login ?? routeHandle ?? ''}`);
		}
	});

	$effect(() => {
		if (ctx.org) {
			displayName = ctx.org.display_name ?? '';
			description = ctx.org.description ?? '';
			avatarUrl = ctx.org.avatar_url ?? '';
			website = ctx.org.website ?? '';
			socialLinks = Array.isArray(ctx.org.social_links) ? ctx.org.social_links.map((link: OrgSocialLink) => ({ label: link.label ?? '', url: link.url ?? '' })) : [];
			location = ctx.org.location ?? '';
		}
	});

	function addSocialLink() {
		socialLinks = [...socialLinks, { label: '', url: '' }];
	}

	function removeSocialLink(index: number) {
		socialLinks = socialLinks.filter((_, i) => i !== index);
	}

	function updateSocialLink(index: number, key: 'label' | 'url', value: string) {
		socialLinks = socialLinks.map((link, i) => (i === index ? { ...link, [key]: value } : link));
	}

	$effect(() => {
		if (!ctx.loading && ctx.org && ctx.isOwner && actionsUsage === null) {
			orgs.getActionsUsage(ctx.org.login)
				.then((usage) => {
					actionsUsage = usage;
				})
				.catch((usageErr: any) => {
					actionsUsageError = usageErr?.message ?? 'Failed to load Actions usage';
				});
		}
	});

	function formatMinutes(minutes: number): string {
		if (minutes < 60) return `${minutes} min`;
		const hours = Math.floor(minutes / 60);
		const mins = minutes % 60;
		return mins === 0 ? `${hours}h` : `${hours}h ${mins}m`;
	}

	async function handleSave(e: Event) {
		e.preventDefault();
		const orgLogin = ctx.org?.login ?? routeHandle;
		if (!orgLogin) {
			saveError = 'Organization context is missing. Please refresh and try again.';
			return;
		}
		saving = true;
		saveError = '';
		success = '';
		try {
			const updated = await orgs.update(orgLogin, {
				display_name: displayName,
				description,
				website,
				social_links: socialLinks.map((link) => ({ label: '', url: link.url.trim() })).filter((link) => link.url.length > 0),
				location
			});
			// Keep existing fields (e.g. avatar/login) and guard against malformed/partial payloads.
			if (updated && typeof updated === 'object' && 'login' in updated) {
				ctx.org = { ...(ctx.org ?? {}), ...updated };
			} else {
				const fresh = await orgs.get(orgLogin);
				ctx.org = fresh.org;
			}
			success = 'Organization settings saved.';
			setTimeout(() => (success = ''), 3000);
		} catch (e: any) {
			saveError = e.message ?? 'Failed to save';
		} finally {
			saving = false;
		}
	}

	async function handleAvatarChange(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file) return;
		const orgLogin = ctx.org?.login ?? routeHandle;
		if (!orgLogin) {
			avatarError = 'Organization context is missing. Please refresh and try again.';
			return;
		}
		uploadingAvatar = true;
		avatarError = '';
		try {
			const res = await orgs.uploadAvatar(orgLogin, file);
			avatarUrl = res.avatar_url;
			if (ctx.org) ctx.org = { ...ctx.org, avatar_url: res.avatar_url };
		} catch (e: any) {
			avatarError = e.message ?? 'Upload failed';
		} finally {
			uploadingAvatar = false;
		}
	}

	async function handleDelete(password: string) {
		const orgLogin = ctx.org?.login ?? routeHandle;
		if (!orgLogin) {
			saveError = 'Organization context is missing. Please refresh and try again.';
			throw new Error('organization context missing');
		}
		deleting = true;
		try {
			await orgs.delete(orgLogin, password);
			goto('/');
		} catch (e: any) {
			saveError = e.message ?? 'Failed to delete organization';
			deleting = false;
			throw e;
		}
	}
</script>

<svelte:head>
	<title>Settings</title>
</svelte:head>

{#if ctx.org}
	<div class="max-w-xl">
		<h1 class="text-xl font-semibold text-foreground mb-6 flex items-center gap-2">
			<Settings class="h-5 w-5 text-muted-foreground" /> Organization settings
		</h1>

		{#if saveError}
			<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{saveError}</div>
		{/if}
		{#if success}
			<div class="mb-4 rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">{success}</div>
		{/if}

		<form onsubmit={handleSave} class="space-y-5">
			<div class="flex items-center gap-4 p-4 rounded-md border border-border bg-card">
				<div class="relative h-16 w-16 shrink-0">
					<div class="h-16 w-16 rounded-md border border-border bg-secondary flex items-center justify-center overflow-hidden">
						{#if avatarUrl}
							<img src={mediaUrl(avatarUrl)} alt={ctx.org.login} class="h-full w-full object-cover" />
						{:else}
							<span class="text-2xl font-bold text-primary">{ctx.org.login[0].toUpperCase()}</span>
						{/if}
					</div>
					{#if uploadingAvatar}
						<div class="absolute inset-0 rounded-md bg-black/50 flex items-center justify-center">
							<Loader class="h-5 w-5 animate-spin text-white" />
						</div>
					{/if}
				</div>
				<div class="flex-1 min-w-0">
					<p class="text-sm font-semibold text-foreground">{ctx.org.login}</p>
					<p class="text-xs text-muted-foreground mt-0.5 mb-2">JPEG, PNG, GIF or WebP · max 2 MB</p>
					<label
						class="inline-flex items-center gap-1.5 h-7 cursor-pointer rounded-md border border-border bg-secondary px-2.5 text-xs font-semibold text-foreground hover:bg-border transition-colors"
					>
						<Camera class="h-3.5 w-3.5" />
						Upload photo
						<input type="file" accept="image/jpeg,image/png,image/gif,image/webp" class="sr-only" onchange={handleAvatarChange} disabled={uploadingAvatar} />
					</label>
					{#if avatarError}
						<p class="text-xs text-red-400 mt-1.5">{avatarError}</p>
					{/if}
				</div>
			</div>

			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">Display name</p>
				<input
					type="text"
					bind:value={displayName}
					placeholder="Acme Corp"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<p class="block text-sm font-semibold text-foreground mb-1.5">Description</p>
				<textarea
					bind:value={description}
					rows={3}
					placeholder="What does this organization do?"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none"
				></textarea>
			</div>

			<div class="grid grid-cols-2 gap-4">
				<div>
					<p class="block text-sm font-semibold text-foreground mb-1.5">Website</p>
					<input
						type="url"
						bind:value={website}
						placeholder="https://example.com"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
				<div>
					<p class="block text-sm font-semibold text-foreground mb-1.5">Location</p>
					<input
						type="text"
						bind:value={location}
						placeholder="San Francisco, CA"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>
			</div>

			<div>
				<div class="mb-2 flex items-center justify-between">
					<p class="block text-sm font-semibold text-foreground">Social links</p>
					<Button variant="outline" type="button" size="sm" onclick={addSocialLink}>
						<Plus class="h-3.5 w-3.5" />
						Add link
					</Button>
				</div>
				{#if socialLinks.length === 0}
					<p class="text-xs text-muted-foreground">Add YouTube, LinkedIn, or any other social profile link.</p>
				{:else}
					<div class="space-y-2">
						{#each socialLinks as link, index}
							<div class="grid grid-cols-[1fr_auto] gap-2">
								<input
									type="url"
									value={link.url}
									oninput={(e) => updateSocialLink(index, 'url', (e.currentTarget as HTMLInputElement).value)}
									placeholder="https://linkedin.com/company/example"
									class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
								/>
								<Button variant="ghost" type="button" size="icon" class="h-9 w-9" onclick={() => removeSocialLink(index)} aria-label="Remove social link">
									<X class="h-4 w-4" />
								</Button>
							</div>
						{/each}
					</div>
				{/if}
			</div>

			<div class="flex justify-end">
				<Button variant="brand" type="submit" disabled={saving}>
					{#if saving}<Loader class="h-4 w-4 animate-spin" />{/if}
					Save changes
				</Button>
			</div>
		</form>

		<hr class="border-secondary my-8" />

		<div class="mb-8">
			<h2 class="text-sm font-semibold text-foreground mb-2 flex items-center gap-2"><Zap class="h-4 w-4" />Actions</h2>
			<p class="text-xs text-muted-foreground mb-4">
				Actions usage is capped at {formatMinutes(Math.max(1, actionsUsage?.limit_minutes ?? 1))} per month for your organization. When the limit is reached, workflows stop running until the next month.
			</p>

			{#if actionsUsageError}
				<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{actionsUsageError}</div>
			{/if}

			<div class="rounded-md border border-border bg-card p-4 mb-4">
				{#if actionsUsage}
					<p class="text-sm text-foreground mb-3">
						Monthly usage: <strong>{formatMinutes(Math.max(0, actionsUsage.used_minutes))}</strong> / {formatMinutes(Math.max(1, actionsUsage.limit_minutes))}
					</p>
					<div class="w-full bg-secondary rounded-full h-2 mb-3">
						<div
							class="bg-primary h-2 rounded-full transition-all"
							style="width: {Math.min(100, (Math.max(0, actionsUsage.used_minutes) / Math.max(1, actionsUsage.limit_minutes)) * 100)}%"
						></div>
					</div>
					<p class="text-xs text-muted-foreground">
						{Math.min(100, (Math.max(0, actionsUsage.used_minutes) / Math.max(1, actionsUsage.limit_minutes)) * 100).toFixed(1)}% used, {formatMinutes(
							Math.max(0, actionsUsage.remaining_minutes)
						)} remaining ({actionsUsage.month})
					</p>
				{:else}
					<p class="text-xs text-muted-foreground">No Actions usage recorded for this month yet.</p>
				{/if}
			</div>

		</div>

		<hr class="border-secondary my-8" />

		<div class="rounded-md border border-red-800/40 p-4">
			<h2 class="text-sm font-semibold text-red-400 mb-1 flex items-center gap-2">
				<AlertTriangle class="h-4 w-4" />Danger Zone
			</h2>
			<p class="text-xs text-muted-foreground mb-4">Once you delete an organization, there is no going back.</p>
			<Button variant="outline" size="sm" onclick={() => (showDeleteDialog = true)} class="border-red-800/40 text-red-400 hover:bg-red-900/20">
				<Trash2 class="h-3.5 w-3.5" />Delete this organization
			</Button>
		</div>

		<ConfirmPasswordDialog
			bind:open={showDeleteDialog}
			title="Delete organization"
			description="This will permanently delete {ctx.org?.login ?? routeHandle} and all of its repositories. Enter your password to confirm."
			confirmLabel="Delete organization"
			onconfirm={handleDelete}
		/>
	</div>
{/if}
