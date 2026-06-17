<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { auth, users } from '$lib/api/client';
	import { Loader, Camera, Zap } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { mediaUrl } from '$lib/utils';
	let displayName = $state('');
	let bio = $state('');
	let avatarUrl = $state('');
	let location = $state('');
	let website = $state('');
	let saving = $state(false);
	let uploadingAvatar = $state(false);
	let avatarError = $state('');
	let error = $state('');
	let success = $state(false);
	let actionsUsage = $state<{ used_minutes: number; limit_minutes: number; remaining_minutes: number; month: string } | null>(null);
	let actionsUsageError = $state('');

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		if (authStore.user) {
			displayName = authStore.user.display_name ?? '';
			bio = authStore.user.bio ?? '';
			avatarUrl = authStore.user.avatar_url ?? '';
			location = authStore.user.location ?? '';
			website = authStore.user.website ?? '';
		}
		try {
			actionsUsage = await users.getActionsUsage();
		} catch (usageErr: any) {
			actionsUsageError = usageErr?.message ?? 'Failed to load Actions usage';
		}
	});

	function formatMinutes(minutes: number): string {
		if (minutes < 60) return `${minutes} min`;
		const hours = Math.floor(minutes / 60);
		const mins = minutes % 60;
		return mins === 0 ? `${hours}h` : `${hours}h ${mins}m`;
	}

	async function handleAvatarChange(e: Event) {
		const file = (e.target as HTMLInputElement).files?.[0];
		if (!file) return;
		uploadingAvatar = true;
		avatarError = '';
		try {
			const res = await users.uploadAvatar(file);
			avatarUrl = res.avatar_url;
			if (authStore.user) authStore.user = { ...authStore.user, avatar_url: res.avatar_url };
		} catch (e: any) {
			avatarError = e.message ?? 'Upload failed';
		} finally {
			uploadingAvatar = false;
		}
	}

	async function handleSave(e: Event) {
		e.preventDefault();
		saving = true;
		error = '';
		success = false;
		try {
			await users.updateProfile({ display_name: displayName, bio, location, website });
			// Re-fetch canonical auth user to avoid dropping fields like avatar_url if PATCH returns a partial payload.
			const refreshed = await auth.me();
			authStore.user = refreshed;
			avatarUrl = refreshed.avatar_url ?? avatarUrl;
			success = true;
			setTimeout(() => (success = false), 3000);
		} catch (e: any) {
			error = e.message;
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>Profile</title>
</svelte:head>

<div class="max-w-xl">
	<h1 class="text-2xl font-semibold text-foreground mb-6">Public profile</h1>

	{#if error}
		<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{/if}
	{#if success}
		<div class="mb-4 rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">Profile saved successfully.</div>
	{/if}

	<form onsubmit={handleSave} class="space-y-5">
		<!-- Avatar -->
		{#if authStore.user}
			<div class="flex items-center gap-4 p-4 rounded-md border border-border bg-card">
				<div class="relative h-16 w-16 shrink-0">
					<div class="h-16 w-16 rounded-full bg-secondary border border-border flex items-center justify-center overflow-hidden">
						{#if avatarUrl}
							<img src={mediaUrl(avatarUrl)} alt={authStore.user.username} class="h-full w-full object-cover" />
						{:else}
							<span class="text-2xl font-bold text-primary">{authStore.user.username[0].toUpperCase()}</span>
						{/if}
					</div>
					{#if uploadingAvatar}
						<div class="absolute inset-0 rounded-full bg-black/50 flex items-center justify-center">
							<Loader class="h-5 w-5 animate-spin text-white" />
						</div>
					{/if}
				</div>
				<div class="flex-1 min-w-0">
					<p class="text-sm font-semibold text-foreground">{authStore.user.username}</p>
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
		{/if}

		<!-- Display name -->
		<div>
			<label for="display_name" class="block text-sm font-semibold text-foreground mb-1.5">Name</label>
			<input
				id="display_name"
				type="text"
				bind:value={displayName}
				maxlength={60}
				placeholder="Your full name"
				class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
			/>
		</div>

		<!-- Bio -->
		<div>
			<label for="bio" class="block text-sm font-semibold text-foreground mb-1.5">Bio</label>
			<textarea
				id="bio"
				bind:value={bio}
				rows={3}
				maxlength={160}
				placeholder="Tell us a little bit about yourself"
				class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none"
			></textarea>
			<p class="text-xs text-muted-foreground mt-1">{bio.length}/160</p>
		</div>

		<!-- Location -->
		<div>
			<label for="location" class="block text-sm font-semibold text-foreground mb-1.5">Location</label>
			<input
				id="location"
				type="text"
				bind:value={location}
				maxlength={100}
				placeholder="City, Country"
				class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
			/>
		</div>

		<!-- Website -->
		<div>
			<label for="website" class="block text-sm font-semibold text-foreground mb-1.5">Website</label>
			<input
				id="website"
				type="url"
				bind:value={website}
				maxlength={255}
				placeholder="https://yourwebsite.com"
				class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
			/>
		</div>

		<Button variant="brand" type="submit" disabled={saving}>
			{#if saving}<Loader class="h-4 w-4 animate-spin" />{/if}
			Update profile
		</Button>
	</form>

	<hr class="border-secondary my-8" />

	<div class="mb-8">
		<h2 class="text-sm font-semibold text-foreground mb-2 flex items-center gap-2"><Zap class="h-4 w-4" />Actions</h2>
		<p class="text-xs text-muted-foreground mb-4">
			Actions usage is capped at {formatMinutes(Math.max(1, actionsUsage?.limit_minutes ?? 1))} per month for your account. When the limit is reached, workflows stop running until the next month.
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
</div>
