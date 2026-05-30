<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { repoEnv, repos, type RepoVariable, type RepoSecretInfo } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { Trash2, Plus, Lock, Variable, Eye, EyeOff, Loader, CheckCircle2, Settings, ArrowLeft } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	const username = $derived(page.params.username as string);
	const repoName = $derived(page.params.repo as string);
	// --- Tabs ---
	let activeTab = $state<'variables' | 'secrets'>('variables');

	// --- Variables ---
	let variables = $state<RepoVariable[]>([]);
	let varLoading = $state(true);
	let varError = $state('');

	let newVarName = $state('');
	let newVarValue = $state('');
	let savingVar = $state(false);
	let varSaveError = $state('');
	let varSaveSuccess = $state('');

	// Edit state
	let editingVar = $state<string | null>(null);
	let editVarValue = $state('');

	// --- Secrets ---
	let secrets = $state<RepoSecretInfo[]>([]);
	let secretLoading = $state(true);
	let secretError = $state('');

	let newSecretName = $state('');
	let newSecretValue = $state('');
	let showSecretValue = $state(false);
	let savingSecret = $state(false);
	let secretSaveError = $state('');
	let secretSaveSuccess = $state('');

	onMount(async () => {
		while (authStore.loading) await new Promise((r) => setTimeout(r, 10));
		if (!authStore.isAuthenticated) {
			goto('/login');
			return;
		}
		// Verify owner access
		try {
			const data = await repos.get(username, repoName);
			if (data.repo.owner_id !== authStore.user?.id) {
				goto(`/${username}/${repoName}`);
				return;
			}
		} catch {
			goto(`/${username}/${repoName}`);
			return;
		}
		await Promise.all([loadVariables(), loadSecrets()]);
	});

	async function loadVariables() {
		varLoading = true;
		varError = '';
		try {
			const data = await repoEnv.listVariables(username, repoName);
			variables = data.variables ?? [];
		} catch (e: any) {
			varError = e.message;
		} finally {
			varLoading = false;
		}
	}

	async function loadSecrets() {
		secretLoading = true;
		secretError = '';
		try {
			const data = await repoEnv.listSecrets(username, repoName);
			secrets = data.secrets ?? [];
		} catch (e: any) {
			secretError = e.message;
		} finally {
			secretLoading = false;
		}
	}

	// Validate env-style name
	function isValidName(name: string): boolean {
		return /^[A-Z_][A-Z0-9_]*$/.test(name);
	}

	async function addVariable(e: Event) {
		e.preventDefault();
		const name = newVarName.trim().toUpperCase();
		if (!name || !isValidName(name)) {
			varSaveError = 'Name must match [A-Z_][A-Z0-9_]*';
			return;
		}
		savingVar = true;
		varSaveError = '';
		varSaveSuccess = '';
		try {
			await repoEnv.setVariable(username, repoName, name, newVarValue);
			varSaveSuccess = `Variable "${name}" saved.`;
			newVarName = '';
			newVarValue = '';
			await loadVariables();
			setTimeout(() => (varSaveSuccess = ''), 3000);
		} catch (e: any) {
			varSaveError = e.message;
		} finally {
			savingVar = false;
		}
	}

	async function saveEditVariable(name: string) {
		savingVar = true;
		varSaveError = '';
		try {
			await repoEnv.setVariable(username, repoName, name, editVarValue);
			editingVar = null;
			await loadVariables();
		} catch (e: any) {
			varSaveError = e.message;
		} finally {
			savingVar = false;
		}
	}

	async function deleteVariable(name: string) {
		if (!confirm(`Delete variable "${name}"?`)) return;
		try {
			await repoEnv.deleteVariable(username, repoName, name);
			await loadVariables();
		} catch (e: any) {
			varError = e.message;
		}
	}

	async function addSecret(e: Event) {
		e.preventDefault();
		const name = newSecretName.trim().toUpperCase();
		if (!name || !isValidName(name)) {
			secretSaveError = 'Name must match [A-Z_][A-Z0-9_]*';
			return;
		}
		if (!newSecretValue) {
			secretSaveError = 'Secret value cannot be empty.';
			return;
		}
		savingSecret = true;
		secretSaveError = '';
		secretSaveSuccess = '';
		try {
			await repoEnv.setSecret(username, repoName, name, newSecretValue);
			secretSaveSuccess = `Secret "${name}" saved.`;
			newSecretName = '';
			newSecretValue = '';
			showSecretValue = false;
			await loadSecrets();
			setTimeout(() => (secretSaveSuccess = ''), 3000);
		} catch (e: any) {
			secretSaveError = e.message;
		} finally {
			savingSecret = false;
		}
	}

	async function deleteSecret(name: string) {
		if (!confirm(`Delete secret "${name}"? This cannot be undone.`)) return;
		try {
			await repoEnv.deleteSecret(username, repoName, name);
			await loadSecrets();
		} catch (e: any) {
			secretError = e.message;
		}
	}

	function startEditVar(v: RepoVariable) {
		editingVar = v.name;
		editVarValue = v.value;
	}
</script>

<svelte:head>
	<title>Environments · Settings · {repoName} · GitPier</title>
</svelte:head>

<div class="max-w-2xl">
	<!-- Tab switcher -->
	<div class="flex border-b border-border mb-6">
		<button
			class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {activeTab === 'variables'
				? 'border-primary text-foreground'
				: 'border-transparent text-muted-foreground hover:text-foreground'}"
			onclick={() => (activeTab = 'variables')}
		>
			<Variable class="inline h-4 w-4 mr-2" />
			Variables
		</button>
		<button
			class="px-4 py-2 text-sm font-medium border-b-2 transition-colors {activeTab === 'secrets'
				? 'border-primary text-foreground'
				: 'border-transparent text-muted-foreground hover:text-foreground'}"
			onclick={() => (activeTab = 'secrets')}
		>
			<Lock class="inline h-4 w-4 mr-2" />
			Secrets
		</button>
	</div>

	<!-- Variables panel -->
	{#if activeTab === 'variables'}
		<section>
			<!-- Add form -->
			<div class="rounded-md border border-border overflow-hidden mb-4">
				<div class="p-4 bg-card border-b border-border">
					<h2 class="text-sm font-semibold text-foreground mb-3">New variable</h2>
					<form onsubmit={addVariable} class="flex flex-col gap-3 sm:flex-row sm:items-end">
						<div class="flex-1">
							<label for="varName" class="block text-xs font-semibold text-muted-foreground mb-1">Name</label>
							<input
								id="varName"
								type="text"
								bind:value={newVarName}
								placeholder="MY_VARIABLE"
								autocomplete="off"
								class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
							/>
						</div>
						<div class="flex-1">
							<label for="varValue" class="block text-xs font-semibold text-muted-foreground mb-1">Value</label>
							<input
								id="varValue"
								type="text"
								bind:value={newVarValue}
								placeholder="value"
								autocomplete="off"
								class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
							/>
						</div>
						<Button variant="brand" size="sm" type="submit" disabled={savingVar || !newVarName.trim()}>
							{#if savingVar}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Plus class="h-3.5 w-3.5" />{/if}
							Add variable
						</Button>
					</form>
					{#if varSaveError}<p class="mt-2 text-xs text-red-400">{varSaveError}</p>{/if}
					{#if varSaveSuccess}
						<p class="mt-2 text-xs text-green-400 flex items-center gap-1"><CheckCircle2 class="h-3.5 w-3.5" />{varSaveSuccess}</p>
					{/if}
				</div>
			</div>

			<!-- List -->
			{#if varLoading}
				<div class="py-8 text-center text-muted-foreground"><Loader class="h-5 w-5 animate-spin inline-block" /></div>
			{:else if varError}
				<p class="text-sm text-red-400">{varError}</p>
			{:else if variables.length === 0}
				<div class="rounded-md border border-border py-8 text-center text-sm text-muted-foreground bg-background">
					No variables yet. Add one above to use it as <code class="font-mono text-xs">$&#123;&#123; vars.NAME &#125;&#125;</code> in workflows.
				</div>
			{:else}
				<div class="rounded-md border border-border overflow-hidden bg-background divide-y divide-border">
					{#each variables as v}
						<div class="flex items-center gap-3 px-4 py-3">
							{#if editingVar === v.name}
								<span class="w-44 shrink-0 font-mono text-sm text-foreground">{v.name}</span>
								<input
									type="text"
									bind:value={editVarValue}
									class="flex-1 h-7 rounded-md border border-border bg-background px-2 text-sm font-mono text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
								/>
								<button onclick={() => saveEditVariable(v.name)} disabled={savingVar} class="text-xs text-green-400 hover:text-green-300 font-semibold disabled:opacity-60">Save</button
								>
								<button onclick={() => (editingVar = null)} class="text-xs text-muted-foreground hover:text-foreground font-semibold">Cancel</button>
							{:else}
								<span class="w-44 shrink-0 font-mono text-sm text-foreground truncate">{v.name}</span>
								<span class="flex-1 font-mono text-sm text-muted-foreground truncate">{v.value || '(empty)'}</span>
								<button onclick={() => startEditVar(v)} class="text-xs text-muted-foreground hover:text-foreground font-semibold transition-colors">Edit</button>
								<button onclick={() => deleteVariable(v.name)} class="text-muted-foreground hover:text-red-400 transition-colors" aria-label="Delete variable">
									<Trash2 class="h-4 w-4" />
								</button>
							{/if}
						</div>
					{/each}
				</div>
			{/if}
		</section>
	{/if}

	<!-- Secrets panel -->
	{#if activeTab === 'secrets'}
		<section>
			<div class="mb-4 rounded-md border border-amber-700/40 bg-amber-950/20 px-4 py-3 text-xs text-amber-300 flex items-start gap-2">
				<Lock class="h-4 w-4 shrink-0 mt-0.5" />
				<span>
					Secret values are encrypted at rest and <strong>never exposed</strong> after saving
				</span>
			</div>

			<!-- Add form -->
			<div class="rounded-md border border-border overflow-hidden mb-4">
				<div class="p-4 bg-card border-b border-border">
					<h2 class="text-sm font-semibold text-foreground mb-3">New secret</h2>
					<form onsubmit={addSecret} class="flex flex-col gap-3 sm:flex-row sm:items-end">
						<div class="flex-1">
							<label for="secretName" class="block text-xs font-semibold text-muted-foreground mb-1">Name</label>
							<input
								id="secretName"
								type="text"
								bind:value={newSecretName}
								placeholder="MY_SECRET"
								autocomplete="off"
								class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
							/>
						</div>
						<div class="flex-1">
							<label for="secretValue" class="block text-xs font-semibold text-muted-foreground mb-1">Value</label>
							<div class="relative">
								{#if showSecretValue}
									<input
										id="secretValue"
										type="text"
										bind:value={newSecretValue}
										placeholder="secret value"
										autocomplete="off"
										class="h-8 w-full rounded-md border border-border bg-background px-3 pr-8 text-sm font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
									/>
								{:else}
									<input
										id="secretValue"
										type="password"
										bind:value={newSecretValue}
										placeholder="secret value"
										autocomplete="new-password"
										class="h-8 w-full rounded-md border border-border bg-background px-3 pr-8 text-sm font-mono text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
									/>
								{/if}
								<button
									type="button"
									tabindex="-1"
									onclick={() => (showSecretValue = !showSecretValue)}
									class="absolute right-2 top-1/2 -translate-y-1/2 text-muted-foreground hover:text-foreground transition-colors"
								>
									{#if showSecretValue}<EyeOff class="h-3.5 w-3.5" />{:else}<Eye class="h-3.5 w-3.5" />{/if}
								</button>
							</div>
						</div>
						<Button variant="brand" size="sm" type="submit" disabled={savingSecret || !newSecretName.trim() || !newSecretValue}>
							{#if savingSecret}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Plus class="h-3.5 w-3.5" />{/if}
							Add secret
						</Button>
					</form>
					{#if secretSaveError}<p class="mt-2 text-xs text-red-400">{secretSaveError}</p>{/if}
					{#if secretSaveSuccess}
						<p class="mt-2 text-xs text-green-400 flex items-center gap-1"><CheckCircle2 class="h-3.5 w-3.5" />{secretSaveSuccess}</p>
					{/if}
				</div>
			</div>

			<!-- List -->
			{#if secretLoading}
				<div class="py-8 text-center text-muted-foreground"><Loader class="h-5 w-5 animate-spin inline-block" /></div>
			{:else if secretError}
				<p class="text-sm text-red-400">{secretError}</p>
			{:else if secrets.length === 0}
				<div class="rounded-md border border-border py-8 text-center text-sm text-muted-foreground bg-background">
					No secrets yet. Add one above to use it as <code class="font-mono text-xs">$&#123;&#123; secrets.NAME &#125;&#125;</code> in workflows.
				</div>
			{:else}
				<div class="rounded-md border border-border overflow-hidden bg-background divide-y divide-border">
					{#each secrets as s}
						<div class="flex items-center gap-3 px-4 py-3">
							<Lock class="h-4 w-4 shrink-0 text-amber-400" />
							<span class="flex-1 font-mono text-sm text-foreground">{s.name}</span>
							<span class="text-xs text-muted-foreground">Updated {new Date(s.updated_at).toLocaleDateString()}</span>
							<button onclick={() => deleteSecret(s.name)} class="text-muted-foreground hover:text-red-400 transition-colors" aria-label="Delete secret">
								<Trash2 class="h-4 w-4" />
							</button>
						</div>
					{/each}
				</div>
			{/if}
		</section>
	{/if}
</div>
