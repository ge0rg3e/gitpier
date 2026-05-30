<script lang="ts">
	import { page } from '$app/stores';
	import { onMount } from 'svelte';
	import CodeViewer from '$lib/components/CodeViewer.svelte';
	import { Button } from '$lib/components/ui/button';

	const code = $page.url.searchParams.get('code') ?? '';
	const oauthState = $page.url.searchParams.get('state') ?? '';
	const error = $page.url.searchParams.get('error') ?? '';
	const errorDescription = $page.url.searchParams.get('error_description') ?? '';

	let copied = $state(false);

	function copySnippet() {
		navigator.clipboard.writeText(curlSnippet).then(() => {
			copied = true;
			setTimeout(() => (copied = false), 2000);
		});
	}

	const curlSnippet = `curl -X POST http://localhost:8080/login/oauth/access_token \\
  -H "Accept: application/json" \\
  -d "client_id=YOUR_CLIENT_ID" \\
  -d "client_secret=YOUR_CLIENT_SECRET" \\
  -d "code=${code}"`;
</script>

<svelte:head>
	<title>OAuth Callback — GitPier</title>
</svelte:head>

<div class="min-h-screen flex items-center justify-center bg-background px-4 py-12">
	<div class="w-full max-w-lg space-y-4">
		{#if error}
			<!-- Error from the authorization server -->
			<div class="rounded-lg border border-destructive/50 bg-destructive/10 p-6 space-y-3">
				<div class="flex items-center gap-3">
					<div class="w-8 h-8 rounded-full bg-destructive/20 flex items-center justify-center shrink-0">
						<svg class="w-4 h-4 text-destructive" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
							<path stroke-linecap="round" stroke-linejoin="round" d="M6 18L18 6M6 6l12 12" />
						</svg>
					</div>
					<h1 class="text-base font-semibold text-foreground">Authorization denied</h1>
				</div>
				<p class="text-sm text-muted-foreground">
					<span class="font-mono text-destructive">{error}</span>
					{#if errorDescription}
						— {errorDescription}
					{/if}
				</p>
				<a href="/" class="text-sm text-brand hover:underline">← Back to GitPier</a>
			</div>
		{:else if code}
			<!-- Success: code received -->
			<div class="rounded-lg border border-border bg-card shadow-sm overflow-hidden">
				<div class="px-6 py-5 border-b border-border">
					<div class="flex items-center gap-3">
						<div class="w-8 h-8 rounded-full bg-green-500/10 flex items-center justify-center shrink-0">
							<svg class="w-4 h-4 text-green-500" fill="none" viewBox="0 0 24 24" stroke="currentColor" stroke-width="2">
								<path stroke-linecap="round" stroke-linejoin="round" d="M5 13l4 4L19 7" />
							</svg>
						</div>
						<h1 class="text-base font-semibold text-foreground">Authorization code received</h1>
					</div>
				</div>

				<div class="px-6 py-5 space-y-5">
					<p class="text-sm text-muted-foreground">
						GitPier has returned an authorization code to this callback URL. In a real app, your server would exchange this code for an access token. This page is a built-in demo landing
						page — it shows the code and lets you exchange it manually.
					</p>

					<!-- Code value -->
					<div class="space-y-1">
						<p class="text-xs font-medium text-muted-foreground uppercase tracking-wide">Authorization Code</p>
						<div class="flex items-center gap-2">
							<code class="flex-1 font-mono text-sm bg-muted rounded px-3 py-2 break-all text-foreground">
								{code}
							</code>
						</div>
						<p class="text-xs text-muted-foreground">Expires in 10 minutes. Single use.</p>
					</div>

					{#if oauthState}
						<div class="space-y-1">
							<p class="text-xs font-medium text-muted-foreground uppercase tracking-wide">State</p>
							<code class="block font-mono text-xs bg-muted rounded px-3 py-2 break-all text-foreground">
								{oauthState}
							</code>
							<p class="text-xs text-muted-foreground">Verify this matches the value your app generated before the redirect.</p>
						</div>
					{/if}

					<!-- Exchange snippet -->
					<div class="space-y-2">
						<p class="text-xs font-medium text-muted-foreground uppercase tracking-wide">Exchange for a token</p>
						<div class="relative">
							<CodeViewer code={curlSnippet} filePath="oauth-token-exchange.sh" containerClass="bg-muted" />
							<button
								onclick={copySnippet}
								class="absolute top-2 right-2 px-2 py-1 text-xs rounded bg-background border border-border text-muted-foreground hover:text-foreground transition-colors"
							>
								{copied ? 'Copied!' : 'Copy'}
							</button>
						</div>
						<p class="text-xs text-muted-foreground">
							Replace <code class="font-mono">YOUR_CLIENT_ID</code> and <code class="font-mono">YOUR_CLIENT_SECRET</code> with your app's credentials. The response will contain a
							<code class="font-mono">glo_…</code> access token.
						</p>
					</div>
				</div>

				<div class="px-6 py-4 border-t border-border bg-muted/30 flex items-center justify-between">
					<a href="/" class="text-sm text-brand hover:underline">← Back to GitPier</a>
					<a href="/docs/oauth-apps" class="text-sm text-muted-foreground hover:text-foreground transition-colors">OAuth docs</a>
				</div>
			</div>
		{:else}
			<!-- Landed here with no params -->
			<div class="rounded-lg border border-border bg-card p-6 text-center space-y-3">
				<h1 class="text-base font-semibold text-foreground">OAuth Callback</h1>
				<p class="text-sm text-muted-foreground">This is the OAuth callback endpoint. It should be reached via a redirect from the GitPier authorization page, not visited directly.</p>
				<a href="/" class="text-sm text-brand hover:underline">← Back to GitPier</a>
			</div>
		{/if}
	</div>
</div>
