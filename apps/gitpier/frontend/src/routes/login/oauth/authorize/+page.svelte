<script lang="ts">
	import { onMount } from 'svelte';
	import { page } from '$app/stores';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { oauthFlow, type OAuthConsentInfo } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button';
	import { Separator } from '$lib/components/ui/separator';

	const params = $page.url.searchParams;
	const clientId = params.get('client_id') ?? '';
	const redirectUri = params.get('redirect_uri') ?? '';
	const scope = params.get('scope') ?? '';
	const oauthState = params.get('state') ?? '';
	const codeChallenge = params.get('code_challenge') ?? '';
	const codeChallengeMethod = params.get('code_challenge_method') ?? '';

	let app = $state<OAuthConsentInfo | null>(null);
	let loadError = $state('');
	let authorizing = $state(false);
	let cancelling = $state(false);

	// Human-readable scope descriptions for GitPier's defined scopes.
	const scopeDescriptions: Record<string, { label: string; detail: string }> = {
		repo: { label: 'Full repository access', detail: 'Read and write access to all your repositories, including private ones.' },
		public_repo: { label: 'Public repository access', detail: 'Read and write access to your public repositories.' },
		user: { label: 'User profile (read/write)', detail: 'Read and update your profile information.' },
		'read:user': { label: 'Read user profile', detail: 'Read your public and private profile data.' },
		'user:email': { label: 'Email addresses', detail: 'Read your email address(es).' },
		'read:org': { label: 'Read organization membership', detail: 'See which organizations you belong to.' },
		'admin:repo_hook': { label: 'Repository webhooks', detail: 'Manage webhooks on your repositories.' }
	};

	function parsedScopes(): string[] {
		return scope
			.split(/[\s,]+/)
			.filter(Boolean)
			.filter((s) => s in scopeDescriptions);
	}

	onMount(async () => {
		if (!clientId) {
			loadError = 'Missing client_id parameter.';
			return;
		}

		// If not logged in, redirect to login with a return URL.
		if (!authStore.user && !authStore.loading) {
			const returnUrl = encodeURIComponent($page.url.href);
			goto(`/login?redirect=${returnUrl}`);
			return;
		}

		try {
			app = await oauthFlow.getAppInfo(clientId);
		} catch (e: unknown) {
			loadError = e instanceof Error ? e.message : 'Application not found.';
		}
	});

	async function handleAuthorize() {
		if (!app) return;
		authorizing = true;
		try {
			const result = await oauthFlow.authorize({
				client_id: clientId,
				redirect_uri: redirectUri,
				scope,
				state: oauthState,
				code_challenge: codeChallenge || undefined,
				code_challenge_method: codeChallengeMethod || undefined
			});
			window.location.href = result.redirect_uri;
		} catch (e: unknown) {
			loadError = e instanceof Error ? e.message : 'Authorization failed.';
			authorizing = false;
		}
	}

	function handleCancel() {
		cancelling = true;
		const target = redirectUri || app?.callback_url || '/';
		const sep = target.includes('?') ? '&' : '?';
		let url = `${target}${sep}error=access_denied`;
		if (oauthState) url += `&state=${encodeURIComponent(oauthState)}`;
		window.location.href = url;
	}
</script>

<svelte:head>
	<title>{app ? `Authorize ${app.name}` : 'Authorize Application'} — GitPier</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center bg-background px-4 py-12">
	<div class="w-full max-w-md">
		{#if loadError}
			<div class="rounded-lg border border-destructive/50 bg-destructive/10 p-6 text-center">
				<h2 class="text-lg font-semibold text-destructive mb-2">Error</h2>
				<p class="text-sm text-muted-foreground">{loadError}</p>
			</div>
		{:else if !app}
			<!-- Loading skeleton -->
			<div class="rounded-lg border border-border bg-card p-8 space-y-4 animate-pulse">
				<div class="h-12 w-12 rounded-full bg-muted mx-auto"></div>
				<div class="h-5 bg-muted rounded w-3/4 mx-auto"></div>
				<div class="h-4 bg-muted rounded w-full"></div>
				<div class="h-4 bg-muted rounded w-5/6"></div>
			</div>
		{:else}
			<div class="rounded-lg border border-border bg-card shadow-sm overflow-hidden">
				<!-- Header -->
				<div class="px-8 pt-8 pb-6 text-center">
					<!-- App logo -->
					{#if app.logo_url}
						<img src={app.logo_url} alt="{app.name} logo" class="w-16 h-16 rounded-lg object-cover mx-auto mb-4 border border-border" />
					{:else}
						<div class="w-16 h-16 rounded-lg bg-muted flex items-center justify-center mx-auto mb-4 text-2xl font-bold text-muted-foreground">
							{app.name.charAt(0).toUpperCase()}
						</div>
					{/if}

					<h1 class="text-xl font-semibold text-foreground">
						Authorize <span class="text-brand">{app.name}</span>
					</h1>
					{#if app.description}
						<p class="mt-1 text-sm text-muted-foreground">{app.description}</p>
					{/if}
					<a href={app.homepage_url} target="_blank" rel="noopener noreferrer" class="mt-1 text-xs text-brand hover:underline">
						{app.homepage_url}
					</a>
				</div>

				<Separator />

				<!-- Scopes -->
				<div class="px-8 py-6">
					{#if parsedScopes().length > 0}
						<p class="text-sm text-muted-foreground mb-4">
							<strong class="text-foreground">{app.name}</strong> is requesting the following permissions:
						</p>
						<ul class="space-y-3">
							{#each parsedScopes() as s}
								{@const info = scopeDescriptions[s]}
								<li class="flex items-start gap-3">
									<div class="mt-0.5 w-4 h-4 rounded-full bg-brand/20 flex items-center justify-center shrink-0">
										<div class="w-2 h-2 rounded-full bg-brand"></div>
									</div>
									<div>
										<p class="text-sm font-medium text-foreground">{info.label}</p>
										<p class="text-xs text-muted-foreground">{info.detail}</p>
									</div>
								</li>
							{/each}
						</ul>
					{:else}
						<p class="text-sm text-muted-foreground">
							<strong class="text-foreground">{app.name}</strong> is requesting basic read access to your public profile.
						</p>
					{/if}
				</div>

				<Separator />

				<!-- User info -->
				{#if authStore.user}
					<div class="px-8 py-4 bg-muted/30 flex items-center gap-3">
						{#if authStore.user.avatar_url}
							<img src={authStore.user.avatar_url} alt="Your avatar" class="w-8 h-8 rounded-full border border-border" />
						{:else}
							<div class="w-8 h-8 rounded-full bg-muted flex items-center justify-center text-xs font-semibold">
								{authStore.user.username.charAt(0).toUpperCase()}
							</div>
						{/if}
						<div class="text-sm">
							<p class="text-foreground font-medium">Authorizing as <strong>{authStore.user.username}</strong></p>
							<!-- <p class="text-muted-foreground text-xs">Not you? <a href="/login" class="text-brand hover:underline">Switch accounts</a></p> -->
						</div>
					</div>

					<Separator />
				{/if}

				<!-- Actions -->
				<div class="px-8 py-6 flex flex-col gap-3">
					<Button class="w-full bg-brand hover:bg-brand/90 text-white" onclick={handleAuthorize} disabled={authorizing || cancelling}>
						{#if authorizing}
							Authorizing…
						{:else}
							Authorize {app.name}
						{/if}
					</Button>
					<Button variant="outline" class="w-full" onclick={handleCancel} disabled={authorizing || cancelling}>Cancel</Button>
				</div>

				<!-- Footer note -->
				<div class="px-8 pb-6">
					<p class="text-xs text-muted-foreground text-center">
						Authorizing will redirect you to
						<span class="font-mono text-foreground">{redirectUri || app.callback_url}</span>. GitPier is not responsible for third-party applications.
					</p>
				</div>
			</div>
		{/if}
	</div>
</div>
