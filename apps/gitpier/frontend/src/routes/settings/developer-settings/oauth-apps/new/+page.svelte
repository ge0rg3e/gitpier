<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { oauthApps } from '$lib/api/client';
	import { Loader, ArrowLeft } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	let loading = $state(false);
	let error = $state('');
	let name = $state('');
	let description = $state('');
	let homepageURL = $state('');
	let callbackURL = $state('');
	let enableDeviceFlow = $state(false);

	onMount(() => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
		}
	});

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			const res = await oauthApps.createUserApp({
				name,
				description,
				homepage_url: homepageURL,
				callback_url: callbackURL,
				enable_device_flow: enableDeviceFlow
			});
			// Navigate to the edit page, passing the secret via URL hash so it shows just once
			goto(`/settings/developer-settings/oauth-apps/${res.app.id}?new=1#secret=${res.client_secret}`);
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Register a new OAuth App</title>
</svelte:head>

<div class="max-w-2xl">
	<div class="flex items-center gap-3 mb-6">
		<a href="/settings/developer-settings/oauth-apps" class="text-muted-foreground hover:text-foreground transition-colors">
			<ArrowLeft class="h-4 w-4" />
		</a>
		<h1 class="text-xl font-semibold text-foreground">Register a new OAuth app</h1>
	</div>

	{#if error}
		<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{/if}

	<div class="rounded-md border border-border bg-card p-6">
		<form onsubmit={handleSubmit} class="space-y-5">
			<div>
				<label for="app-name" class="block text-sm font-semibold text-foreground mb-1.5">
					Application name <span class="text-red-400">*</span>
				</label>
				<input
					id="app-name"
					type="text"
					bind:value={name}
					placeholder="My App"
					required
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
				<p class="text-xs text-muted-foreground mt-1">Something users will recognize and trust.</p>
			</div>

			<div>
				<label for="homepage-url" class="block text-sm font-semibold text-foreground mb-1.5">
					Homepage URL <span class="text-red-400">*</span>
				</label>
				<input
					id="homepage-url"
					type="url"
					bind:value={homepageURL}
					placeholder="https://example.com"
					required
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
				<p class="text-xs text-muted-foreground mt-1">The full URL to your application homepage.</p>
			</div>

			<div>
				<label for="description" class="block text-sm font-semibold text-foreground mb-1.5">Application description</label>
				<textarea
					id="description"
					bind:value={description}
					rows={3}
					placeholder="Application description is optional"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none"
				></textarea>
				<p class="text-xs text-muted-foreground mt-1">This is displayed to all users of your application.</p>
			</div>

			<div>
				<label for="callback-url" class="block text-sm font-semibold text-foreground mb-1.5">
					Authorization callback URL <span class="text-red-400">*</span>
				</label>
				<input
					id="callback-url"
					type="url"
					bind:value={callbackURL}
					placeholder="https://example.com/callback"
					required
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
				<p class="text-xs text-muted-foreground mt-1">Your application's callback URL.</p>
			</div>

			<div class="flex items-start gap-3 pt-1">
				<input id="device-flow" type="checkbox" bind:checked={enableDeviceFlow} class="mt-0.5 h-4 w-4 rounded border-border accent-blue-500 cursor-pointer" />
				<div>
					<label for="device-flow" class="text-sm font-semibold text-foreground cursor-pointer">Enable Device Flow</label>
					<p class="text-xs text-muted-foreground mt-0.5">Allow this OAuth App to authorize users via the Device Flow.</p>
				</div>
			</div>

			<div class="flex items-center gap-3 pt-2 border-t border-border">
				<Button variant="brand" type="submit" disabled={loading}>
					{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
					Register application
				</Button>
				<Button variant="ghost" type="button" onclick={() => goto('/settings/developer-settings/oauth-apps')}>Cancel</Button>
			</div>
		</form>
	</div>
</div>
