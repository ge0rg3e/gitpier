<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { gitpierApps } from '$lib/api/client';
	import { ArrowLeft, Loader, Plus, Minus } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	// Form state
	let loading = $state(false);
	let error = $state('');

	let name = $state('');
	let description = $state('');
	let homepageURL = $state('');
	let logoURL = $state('');
	let setupURL = $state('');
	let redirectOnUpdate = $state(false);
	let webhookURL = $state('');
	let webhookSecret = $state('');
	let webhookActive = $state(true);
	let isPublic = $state(false);
	let callbackURLs = $state<string[]>(['']);
	let requestUserAuth = $state(false);
	let expireUserTokens = $state(true);
	let enableDeviceFlow = $state(false);

	// Permissions
	const REPO_PERMS = ['contents', 'issues', 'pull_requests', 'webhooks', 'releases', 'workflows', 'metadata', 'collaborators'];
	const ORG_PERMS = ['members'];
	const ACCT_PERMS = ['profile', 'email', 'ssh_keys'];

	const PERM_LABELS: Record<string, string> = {
		contents: 'Repository contents',
		issues: 'Issues',
		pull_requests: 'Pull requests',
		webhooks: 'Webhooks',
		releases: 'Releases',
		workflows: 'Workflow runs',
		metadata: 'Repository metadata',
		collaborators: 'Collaborators',
		members: 'Organization members',
		profile: 'User profile',
		email: 'Email addresses',
		ssh_keys: 'SSH keys'
	};

	const PERM_DESCRIPTIONS: Record<string, string> = {
		contents: 'Read and write access to repository code, commits, and branches',
		issues: 'Read and write access to issues and issue comments',
		pull_requests: 'Read and write access to pull requests, reviews, and comments',
		webhooks: 'Manage repository webhooks',
		releases: 'Read and write access to releases and release assets',
		workflows: 'Read access to workflow runs and job logs',
		metadata: 'Read-only access to repository metadata (always recommended)',
		collaborators: 'Manage repository collaborators',
		members: 'Read and write access to organization members and teams',
		profile: 'Read and write access to user profile information',
		email: 'Read access to user email addresses',
		ssh_keys: 'Read and write access to SSH keys'
	};

	let repoPerms = $state<Record<string, string>>(Object.fromEntries(REPO_PERMS.map((p) => [p, 'none'])));
	let orgPerms = $state<Record<string, string>>(Object.fromEntries(ORG_PERMS.map((p) => [p, 'none'])));
	let acctPerms = $state<Record<string, string>>(Object.fromEntries(ACCT_PERMS.map((p) => [p, 'none'])));

	onMount(() => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
		}
	});

	function addCallbackURL() {
		callbackURLs = [...callbackURLs, ''];
	}

	function removeCallbackURL(i: number) {
		callbackURLs = callbackURLs.filter((_, idx) => idx !== i);
	}

	async function handleSubmit(e: SubmitEvent) {
		e.preventDefault();
		loading = true;
		error = '';
		try {
			// Filter out empty callback URLs
			const filteredCallbacks = callbackURLs.filter((u) => u.trim() !== '');

			// Filter out 'none' permissions
			const filteredRepoPerms = Object.fromEntries(Object.entries(repoPerms).filter(([, v]) => v !== 'none'));
			const filteredOrgPerms = Object.fromEntries(Object.entries(orgPerms).filter(([, v]) => v !== 'none'));
			const filteredAcctPerms = Object.fromEntries(Object.entries(acctPerms).filter(([, v]) => v !== 'none'));

			const res = await gitpierApps.createUserApp({
				name,
				description,
				homepage_url: homepageURL,
				logo_url: logoURL || undefined,
				setup_url: setupURL || undefined,
				redirect_on_update: redirectOnUpdate,
				webhook_url: webhookURL || undefined,
				webhook_secret: webhookSecret || undefined,
				webhook_active: webhookActive,
				is_public: isPublic,
				callback_urls: filteredCallbacks,
				request_user_auth: requestUserAuth,
				expire_user_tokens: expireUserTokens,
				enable_device_flow: enableDeviceFlow,
				repo_permissions: filteredRepoPerms,
				org_permissions: filteredOrgPerms,
				account_permissions: filteredAcctPerms
			});

			goto(`/settings/developer-settings/apps/${res.app.id}?new=1#secret=${res.client_secret}`);
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}
</script>

<svelte:head>
	<title>Register a new GitPier App</title>
</svelte:head>

<div class="max-w-2xl">
	<div class="flex items-center gap-3 mb-6">
		<a href="/settings/developer-settings/apps" class="text-muted-foreground hover:text-foreground transition-colors">
			<ArrowLeft class="h-4 w-4" />
		</a>
		<h1 class="text-xl font-semibold text-foreground">Register a new GitPier App</h1>
	</div>

	{#if error}
		<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
	{/if}

	<form onsubmit={handleSubmit} class="space-y-8">
		<!-- Basic info -->
		<div class="rounded-md border border-border bg-card p-6 space-y-5">
			<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Basic information</h2>

			<div>
				<label for="app-name" class="block text-sm font-semibold text-foreground mb-1.5">
					GitPier App name <span class="text-red-400">*</span>
				</label>
				<input
					id="app-name"
					type="text"
					bind:value={name}
					required
					placeholder="My Awesome App"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
				<p class="text-xs text-muted-foreground mt-1">
					The name must be globally unique. It will be used as the app's slug (e.g. "My App" → <code class="font-mono">my-app</code>) and displayed to users when they install the app.
				</p>
			</div>

			<div>
				<label for="description" class="block text-sm font-semibold text-foreground mb-1.5">Description</label>
				<textarea
					id="description"
					bind:value={description}
					rows={3}
					placeholder="What does your app do?"
					class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary resize-none"
				></textarea>
				<p class="text-xs text-muted-foreground mt-1">This description is shown to users on the installation screen.</p>
			</div>

			<div>
				<label for="homepage-url" class="block text-sm font-semibold text-foreground mb-1.5">
					Homepage URL <span class="text-red-400">*</span>
				</label>
				<input
					id="homepage-url"
					type="url"
					bind:value={homepageURL}
					required
					placeholder="https://example.com"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<label for="logo-url" class="block text-sm font-semibold text-foreground mb-1.5">Logo URL (optional)</label>
				<input
					id="logo-url"
					type="url"
					bind:value={logoURL}
					placeholder="https://example.com/logo.png"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>

			<div>
				<label class="flex items-center gap-2 cursor-pointer">
					<input type="checkbox" bind:checked={isPublic} class="rounded border-border" />
					<span class="text-sm font-semibold text-foreground">Make this app public</span>
				</label>
				<p class="text-xs text-muted-foreground mt-1 ml-5">Public apps can be installed by any GitPier user or organization. Private apps can only be installed by the owner.</p>
			</div>
		</div>

		<!-- Identifying and authorizing users -->
		<div class="rounded-md border border-border bg-card p-6 space-y-5">
			<div class="flex items-center justify-between border-b border-border pb-3">
				<h2 class="text-base font-semibold text-foreground">Identifying and authorizing users</h2>
				<Button type="button" variant="outline" size="sm" onclick={addCallbackURL}>
					<Plus class="h-3.5 w-3.5" />
					Add Callback URL
				</Button>
			</div>
			<p class="text-xs text-muted-foreground">The full URL to redirect to after a user authorizes an installation. You can add up to 10 callback URLs.</p>

			<div class="space-y-2">
				{#each callbackURLs as url, i}
					<div class="flex gap-2">
						<input
							type="url"
							bind:value={callbackURLs[i]}
							placeholder="https://example.com/callback"
							class="h-9 flex-1 rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
						/>
						{#if callbackURLs.length > 1}
							<Button type="button" variant="outline" size="icon" class="h-9 w-9 shrink-0" onclick={() => removeCallbackURL(i)}>
								<Minus class="h-3.5 w-3.5" />
							</Button>
						{/if}
					</div>
				{/each}
			</div>

			<div class="space-y-3 pt-2 border-t border-border">
				<label class="flex items-start gap-2 cursor-pointer">
					<input type="checkbox" bind:checked={expireUserTokens} class="rounded border-border mt-0.5" />
					<div>
						<span class="text-sm font-semibold text-foreground block">Expire user authorization tokens</span>
						<span class="text-xs text-muted-foreground">Provides a refresh token that can be used to renew access tokens when they expire. Recommended.</span>
					</div>
				</label>

				<label class="flex items-start gap-2 cursor-pointer">
					<input type="checkbox" bind:checked={requestUserAuth} class="rounded border-border mt-0.5" />
					<div>
						<span class="text-sm font-semibold text-foreground block">Request user authorization (OAuth) during installation</span>
						<span class="text-xs text-muted-foreground">Users will be prompted to authorize your app when they install it. Useful for apps that act on behalf of users immediately.</span>
					</div>
				</label>

				<label class="flex items-start gap-2 cursor-pointer">
					<input type="checkbox" bind:checked={enableDeviceFlow} class="rounded border-border mt-0.5" />
					<div>
						<span class="text-sm font-semibold text-foreground block">Enable Device Flow</span>
						<span class="text-xs text-muted-foreground">Allow this app to authorize users via the Device Flow (for CLI tools and headless apps).</span>
					</div>
				</label>
			</div>
		</div>

		<!-- Post installation -->
		<div class="rounded-md border border-border bg-card p-6 space-y-5">
			<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Post installation</h2>

			<div>
				<label for="setup-url" class="block text-sm font-semibold text-foreground mb-1.5">Setup URL (optional)</label>
				<input
					id="setup-url"
					type="url"
					bind:value={setupURL}
					placeholder="https://example.com/setup"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
				<p class="text-xs text-muted-foreground mt-1">Users will be redirected here after installation to complete additional setup steps.</p>
			</div>

			<label class="flex items-start gap-2 cursor-pointer">
				<input type="checkbox" bind:checked={redirectOnUpdate} class="rounded border-border mt-0.5" />
				<div>
					<span class="text-sm font-semibold text-foreground block">Redirect on update</span>
					<span class="text-xs text-muted-foreground">Redirect users to the Setup URL after they update the installation (e.g., adding or removing repositories).</span>
				</div>
			</label>
		</div>

		<!-- Webhook -->
		<div class="rounded-md border border-border bg-card p-6 space-y-5">
			<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Webhook</h2>

			<label class="flex items-center gap-2 cursor-pointer">
				<input type="checkbox" bind:checked={webhookActive} class="rounded border-border" />
				<span class="text-sm font-semibold text-foreground">Active</span>
			</label>
			<p class="text-xs text-muted-foreground -mt-3 ml-5">GitPier will deliver event details when this hook is triggered.</p>

			{#if webhookActive}
				<div>
					<label for="webhook-url" class="block text-sm font-semibold text-foreground mb-1.5">
						Webhook URL <span class="text-red-400">*</span>
					</label>
					<input
						id="webhook-url"
						type="url"
						bind:value={webhookURL}
						placeholder="https://example.com/events"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
					<p class="text-xs text-muted-foreground mt-1">Events will be POSTed to this URL.</p>
				</div>

				<div>
					<label for="webhook-secret" class="block text-sm font-semibold text-foreground mb-1.5">Webhook secret</label>
					<input
						id="webhook-secret"
						type="password"
						bind:value={webhookSecret}
						placeholder="A random secret token"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
					<p class="text-xs text-muted-foreground mt-1">Used to sign webhook deliveries. Strongly recommended.</p>
				</div>
			{/if}
		</div>

		<!-- Permissions -->
		<div class="rounded-md border border-border bg-card p-6 space-y-5">
			<div>
				<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Permissions</h2>
				<p class="text-xs text-muted-foreground mt-3">Select the minimum permissions your app needs. Users will see and grant these permissions when they install the app.</p>
			</div>

			<!-- Repository permissions -->
			<div>
				<h3 class="text-sm font-semibold text-foreground mb-3">Repository permissions</h3>
				<div class="space-y-3">
					{#each REPO_PERMS as perm}
						<div class="flex items-center justify-between gap-4">
							<div class="flex-1">
								<p class="text-sm font-medium text-foreground">{PERM_LABELS[perm]}</p>
								<p class="text-xs text-muted-foreground">{PERM_DESCRIPTIONS[perm]}</p>
							</div>
							<select
								bind:value={repoPerms[perm]}
								class="h-8 rounded-md border border-border bg-background px-2 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
							>
								<option value="none">No access</option>
								<option value="read">Read-only</option>
								{#if perm !== 'metadata' && perm !== 'workflows'}
									<option value="write">Read & write</option>
								{/if}
							</select>
						</div>
					{/each}
				</div>
			</div>

			<!-- Organization permissions -->
			<div class="pt-4 border-t border-border">
				<h3 class="text-sm font-semibold text-foreground mb-3">Organization permissions</h3>
				<div class="space-y-3">
					{#each ORG_PERMS as perm}
						<div class="flex items-center justify-between gap-4">
							<div class="flex-1">
								<p class="text-sm font-medium text-foreground">{PERM_LABELS[perm]}</p>
								<p class="text-xs text-muted-foreground">{PERM_DESCRIPTIONS[perm]}</p>
							</div>
							<select
								bind:value={orgPerms[perm]}
								class="h-8 rounded-md border border-border bg-background px-2 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
							>
								<option value="none">No access</option>
								<option value="read">Read-only</option>
								<option value="write">Read & write</option>
							</select>
						</div>
					{/each}
				</div>
			</div>

			<!-- Account permissions -->
			<div class="pt-4 border-t border-border">
				<h3 class="text-sm font-semibold text-foreground mb-3">Account permissions</h3>
				<p class="text-xs text-muted-foreground mb-3">These permissions are granted on an individual user basis as part of the user authorization flow.</p>
				<div class="space-y-3">
					{#each ACCT_PERMS as perm}
						<div class="flex items-center justify-between gap-4">
							<div class="flex-1">
								<p class="text-sm font-medium text-foreground">{PERM_LABELS[perm]}</p>
								<p class="text-xs text-muted-foreground">{PERM_DESCRIPTIONS[perm]}</p>
							</div>
							<select
								bind:value={acctPerms[perm]}
								class="h-8 rounded-md border border-border bg-background px-2 text-xs text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
							>
								<option value="none">No access</option>
								<option value="read">Read-only</option>
								{#if perm === 'profile' || perm === 'ssh_keys'}
									<option value="write">Read & write</option>
								{/if}
							</select>
						</div>
					{/each}
				</div>
			</div>
		</div>

		<!-- Where can this app be installed -->
		<div class="rounded-md border border-border bg-card p-6 space-y-4">
			<h2 class="text-base font-semibold text-foreground border-b border-border pb-3">Where can this GitPier App be installed?</h2>
			<label class="flex items-start gap-3 cursor-pointer">
				<input type="radio" name="visibility" value={false} checked={!isPublic} onchange={() => (isPublic = false)} class="mt-0.5" />
				<div>
					<span class="text-sm font-semibold text-foreground block">Only on this account</span>
					<span class="text-xs text-muted-foreground">Only allow this GitPier App to be installed on your own account or organizations you own.</span>
				</div>
			</label>
			<label class="flex items-start gap-3 cursor-pointer">
				<input type="radio" name="visibility" value={true} checked={isPublic} onchange={() => (isPublic = true)} class="mt-0.5" />
				<div>
					<span class="text-sm font-semibold text-foreground block">Any account</span>
					<span class="text-xs text-muted-foreground">Allow this GitPier App to be installed by any user or organization.</span>
				</div>
			</label>
		</div>

		<div class="flex justify-end">
			<Button type="submit" variant="brand" disabled={loading}>
				{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
				Create GitPier App
			</Button>
		</div>
	</form>
</div>
