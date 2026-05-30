<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { oauthFlow, type OAuthDeviceInfo } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Input } from '$lib/components/ui/input';
	import { Label } from '$lib/components/ui/label';
	import { Separator } from '$lib/components/ui/separator';

	let userCode = $state($page.url.searchParams.get('user_code') ?? '');
	let deviceInfo = $state<OAuthDeviceInfo | null>(null);
	let lookupError = $state('');
	let lookupLoading = $state(false);
	let approving = $state(false);
	let denying = $state(false);
	let done = $state<'approved' | 'denied' | null>(null);
	let actionError = $state('');

	onMount(() => {
		// Pre-fill and auto-lookup if user_code is in the URL.
		if (userCode) {
			lookupCode();
		}
	});

	function formatCode(raw: string): string {
		const clean = raw.replace(/[^A-Za-z0-9]/g, '').toUpperCase();
		if (clean.length > 4) return `${clean.slice(0, 4)}-${clean.slice(4, 8)}`;
		return clean;
	}

	function handleInput(e: Event) {
		const input = e.currentTarget as HTMLInputElement;
		const formatted = formatCode(input.value);
		userCode = formatted;
		input.value = formatted;
		// Clear previous lookup result when user types.
		deviceInfo = null;
		lookupError = '';
	}

	async function lookupCode() {
		const code = userCode.replace(/-/g, '').trim();
		if (code.length < 8) {
			lookupError = 'Please enter an 8-character code.';
			return;
		}

		lookupLoading = true;
		lookupError = '';
		deviceInfo = null;
		try {
			deviceInfo = await oauthFlow.getDeviceInfo(userCode);
		} catch (e: unknown) {
			lookupError = e instanceof Error ? e.message : 'Code not found or expired.';
		} finally {
			lookupLoading = false;
		}
	}

	async function handleApprove() {
		if (!authStore.user) {
			const returnUrl = encodeURIComponent($page.url.href);
			goto(`/login?redirect=${returnUrl}`);
			return;
		}
		approving = true;
		actionError = '';
		try {
			await oauthFlow.approveDevice(userCode);
			done = 'approved';
		} catch (e: unknown) {
			actionError = e instanceof Error ? e.message : 'Failed to approve device.';
		} finally {
			approving = false;
		}
	}

	async function handleDeny() {
		denying = true;
		actionError = '';
		try {
			await oauthFlow.denyDevice(userCode);
			done = 'denied';
		} catch (e: unknown) {
			actionError = e instanceof Error ? e.message : 'Failed to deny device.';
		} finally {
			denying = false;
		}
	}

	const scopeLabels: Record<string, string> = {
		repo: 'Full repository access',
		public_repo: 'Public repository access',
		user: 'User profile (read/write)',
		'read:user': 'Read user profile',
		'user:email': 'Email addresses',
		'read:org': 'Read organization membership',
		'admin:repo_hook': 'Repository webhooks'
	};
</script>

<svelte:head>
	<title>Device Activation — GitPier</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center bg-background px-4 py-12">
	<div class="w-full max-w-md space-y-6">
		{#if done === 'approved'}
			<!-- Success state -->
			<div class="rounded-lg border border-border bg-card p-8 text-center space-y-4">
				<div class="w-16 h-16 rounded-full bg-green-500/10 flex items-center justify-center mx-auto">
					<svg class="w-8 h-8 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
					</svg>
				</div>
				<h1 class="text-xl font-semibold text-foreground">Device authorized!</h1>
				<p class="text-sm text-muted-foreground">
					You have successfully authorized
					<strong class="text-foreground">{deviceInfo?.app.name ?? 'the application'}</strong>. You can close this page and return to your device.
				</p>
			</div>
		{:else if done === 'denied'}
			<!-- Denied state -->
			<div class="rounded-lg border border-border bg-card p-8 text-center space-y-4">
				<div class="w-16 h-16 rounded-full bg-muted flex items-center justify-center mx-auto">
					<svg class="w-8 h-8 text-muted-foreground" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
						<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
					</svg>
				</div>
				<h1 class="text-xl font-semibold text-foreground">Authorization denied</h1>
				<p class="text-sm text-muted-foreground">The device authorization has been denied. You can close this page.</p>
			</div>
		{:else}
			<!-- Code entry + confirmation -->
			<div class="rounded-lg border border-border bg-card shadow-sm overflow-hidden">
				<div class="px-8 pt-8 pb-6 text-center">
					<div class="w-12 h-12 rounded-lg bg-brand/10 flex items-center justify-center mx-auto mb-4">
						<svg class="w-6 h-6 text-brand" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path
								stroke-linecap="round"
								stroke-linejoin="round"
								d="M9 12l2 2 4-4m5.618-4.016A11.955 11.955 0 0112 2.944a11.955 11.955 0 01-8.618 3.04A12.02 12.02 0 003 9c0 5.591 3.824 10.29 9 11.622 5.176-1.332 9-6.03 9-11.622 0-1.042-.133-2.052-.382-3.016z"
							/>
						</svg>
					</div>
					<h1 class="text-xl font-semibold text-foreground">Device Activation</h1>
					<p class="mt-1 text-sm text-muted-foreground">Enter the code shown on your device to authorize it.</p>
				</div>

				<Separator />

				<div class="px-8 py-6 space-y-4">
					<div class="space-y-2">
						<Label for="user-code">Device code</Label>
						<div class="flex gap-2">
							<Input
								id="user-code"
								type="text"
								placeholder="XXXX-XXXX"
								maxlength={9}
								class="font-mono text-center tracking-widest text-lg uppercase"
								value={userCode}
								oninput={handleInput}
								onkeydown={(e) => e.key === 'Enter' && lookupCode()}
							/>
							<Button variant="outline" onclick={lookupCode} disabled={lookupLoading}>
								{#if lookupLoading}
									<span class="animate-spin mr-1">⟳</span>
								{/if}
								Look up
							</Button>
						</div>
						{#if lookupError}
							<p class="text-xs text-destructive">{lookupError}</p>
						{/if}
					</div>

					{#if deviceInfo}
						<div class="rounded-lg border border-border bg-muted/30 p-4 space-y-3">
							<!-- App info -->
							<div class="flex items-center gap-3">
								{#if deviceInfo.app.logo_url}
									<img src={deviceInfo.app.logo_url} alt="" class="w-10 h-10 rounded-lg object-cover border border-border" />
								{:else}
									<div class="w-10 h-10 rounded-lg bg-muted flex items-center justify-center font-semibold text-muted-foreground">
										{deviceInfo.app.name.charAt(0).toUpperCase()}
									</div>
								{/if}
								<div>
									<p class="font-medium text-foreground">{deviceInfo.app.name}</p>
									{#if deviceInfo.app.homepage_url}
										<a href={deviceInfo.app.homepage_url} target="_blank" rel="noopener noreferrer" class="text-xs text-brand hover:underline">
											{deviceInfo.app.homepage_url}
										</a>
									{/if}
								</div>
							</div>

							<!-- Scopes -->
							{#if deviceInfo.scopes}
								<div>
									<p class="text-xs text-muted-foreground mb-2">Requesting permissions:</p>
									<div class="flex flex-wrap gap-1">
										{#each deviceInfo.scopes.split(' ').filter(Boolean) as s}
											<span class="inline-flex items-center px-2 py-0.5 rounded-full text-xs bg-brand/10 text-brand font-medium">
												{scopeLabels[s] ?? s}
											</span>
										{/each}
									</div>
								</div>
							{/if}
						</div>

						{#if actionError}
							<p class="text-xs text-destructive">{actionError}</p>
						{/if}

						{#if !authStore.user}
							<div class="rounded-lg border border-border bg-muted/30 p-4 text-sm text-center">
								<p class="text-muted-foreground">You need to be signed in to authorize this device.</p>
								<a href="/login?redirect={encodeURIComponent($page.url.href)}" class="text-brand hover:underline font-medium">Sign in</a>
							</div>
						{:else}
							<div class="flex gap-3">
								<Button class="flex-1 bg-brand hover:bg-brand/90 text-white" onclick={handleApprove} disabled={approving || denying}>
									{approving ? 'Authorizing…' : 'Authorize device'}
								</Button>
								<Button variant="outline" class="flex-1" onclick={handleDeny} disabled={approving || denying}>
									{denying ? 'Denying…' : 'Deny'}
								</Button>
							</div>
						{/if}
					{/if}
				</div>
			</div>

			<!-- Help text -->
			<p class="text-xs text-center text-muted-foreground">
				Your device will be granted access to your GitPier account based on the permissions above. Never enter a code you did not initiate yourself.
			</p>
		{/if}
	</div>
</div>
