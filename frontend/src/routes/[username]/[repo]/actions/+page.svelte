<script lang="ts">
	import { page } from '$app/state';
	import { getContext, onDestroy } from 'svelte';
	import { workflows, repos, type WorkflowRun, type WorkflowStatus } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { timeAgo } from '$lib/utils';
	import { Zap, GitBranch, GitCommit, CheckCircle2, XCircle, Clock, Loader, Ban, Play, ChevronDown, FileCode2 } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	let runs = $state<WorkflowRun[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state('');

	let dispatchOpen = $state(false);
	let dispatchBranches = $state<string[]>([]);
	let dispatchRef = $state('');
	let dispatchWorkflows = $state<string[]>([]);
	let dispatchWorkflowFile = $state('');
	let loadingWorkflows = $state(false);
	let dispatching = $state(false);
	let dispatchError = $state('');
	let dispatchSuccess = $state('');

	const { username, repo } = $derived(page.params);
	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const isLoggedIn = $derived(!!authStore.user);
	const hasActiveRuns = $derived(runs.some((r) => r.status === 'pending' || r.status === 'running'));

	async function load() {
		try {
			const data = await workflows.listRuns(username!, repo!);
			runs = data.runs ?? [];
			total = data.total;
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	const interval = setInterval(() => {
		if (hasActiveRuns) load();
	}, 5000);
	onDestroy(() => clearInterval(interval));

	async function openDispatch() {
		if (isRepoArchived) return;
		dispatchError = '';
		dispatchSuccess = '';
		dispatchWorkflows = [];
		dispatchWorkflowFile = '';
		dispatchOpen = true;
		if (dispatchBranches.length === 0) {
			try {
				const data = await repos.get(username!, repo!);
				dispatchBranches = data.branches ?? [];
				dispatchRef = data.repo.default_branch || dispatchBranches[0] || '';
			} catch {
				dispatchBranches = [];
			}
		}
		if (dispatchRef) await loadWorkflowsForRef(dispatchRef);
	}

	async function loadWorkflowsForRef(ref: string) {
		if (!ref) return;
		loadingWorkflows = true;
		dispatchWorkflows = [];
		dispatchWorkflowFile = '';
		try {
			const data = await workflows.listDispatchable(username!, repo!, ref);
			dispatchWorkflows = data.workflows ?? [];
			dispatchWorkflowFile = dispatchWorkflows[0] ?? '';
		} catch {
			dispatchWorkflows = [];
		} finally {
			loadingWorkflows = false;
		}
	}

	async function onRefChange(ref: string) {
		dispatchRef = ref;
		await loadWorkflowsForRef(ref);
	}

	async function runDispatch() {
		if (isRepoArchived) return;
		if (!dispatchRef || !dispatchWorkflowFile) return;
		dispatching = true;
		dispatchError = '';
		dispatchSuccess = '';
		try {
			await workflows.dispatch(username!, repo!, dispatchRef, dispatchWorkflowFile);
			dispatchSuccess = `Triggered ${dispatchWorkflowFile} on ${dispatchRef}`;
			dispatchOpen = false;
			await load();
		} catch (e: any) {
			dispatchError = e.message;
		} finally {
			dispatching = false;
		}
	}

	function statusIcon(status: WorkflowStatus) {
		switch (status) {
			case 'success':
				return CheckCircle2;
			case 'failure':
				return XCircle;
			case 'running':
				return Loader;
			case 'cancelled':
				return Ban;
			default:
				return Clock;
		}
	}

	function statusColor(status: WorkflowStatus): string {
		switch (status) {
			case 'success':
				return '#3fb950';
			case 'failure':
				return '#f85149';
			case 'running':
				return '#58a6ff';
			case 'cancelled':
				return '#848d97';
			default:
				return '#d29922';
		}
	}

	function shortSHA(sha: string): string {
		return sha?.slice(0, 7) ?? '';
	}
</script>

<div class="space-y-4">
	<div class="flex items-center justify-between">
		<div>
			<h2 class="text-xl font-semibold text-foreground">All workflows</h2>
			{#if total > 0}
				<p class="text-sm text-muted-foreground mt-0.5">{total} workflow run{total !== 1 ? 's' : ''}</p>
			{/if}
		</div>
		{#if isLoggedIn && !isRepoArchived}
			<Button variant="brand" onclick={openDispatch}>
				<Play class="h-3.5 w-3.5" />
				Run workflow
			</Button>
		{/if}
	</div>

	<!-- Dispatch panel -->
	{#if dispatchOpen && !isRepoArchived}
		<div class="rounded-md border border-border bg-card p-4 space-y-4">
			<p class="text-sm font-semibold text-foreground">Run workflow</p>

			<div class="space-y-1.5">
				<label class="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
					<FileCode2 class="h-3.5 w-3.5" />
					Use workflow from
				</label>
				{#if loadingWorkflows}
					<div class="h-9 rounded-md bg-secondary animate-pulse"></div>
				{:else if dispatchWorkflows.length === 0}
					<p class="text-xs text-muted-foreground italic">No dispatchable workflow files found on this branch.</p>
				{:else}
					<div class="relative">
						<select
							bind:value={dispatchWorkflowFile}
							class="w-full appearance-none h-9 rounded-md border border-border bg-background px-3 pr-8 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
						>
							{#each dispatchWorkflows as wf}<option value={wf}>{wf}</option>{/each}
						</select>
						<ChevronDown class="absolute right-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
					</div>
				{/if}
			</div>

			<div class="space-y-1.5">
				<label class="flex items-center gap-1.5 text-xs font-semibold text-muted-foreground">
					<GitBranch class="h-3.5 w-3.5" />
					Branch
				</label>
				{#if dispatchBranches.length > 0}
					<div class="relative">
						<select
							value={dispatchRef}
							onchange={(e) => onRefChange((e.target as HTMLSelectElement).value)}
							class="w-full appearance-none h-9 rounded-md border border-border bg-background px-3 pr-8 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
						>
							{#each dispatchBranches as b}<option value={b}>{b}</option>{/each}
						</select>
						<ChevronDown class="absolute right-2.5 top-1/2 -translate-y-1/2 h-3.5 w-3.5 text-muted-foreground pointer-events-none" />
					</div>
				{:else}
					<input
						bind:value={dispatchRef}
						onblur={() => loadWorkflowsForRef(dispatchRef)}
						placeholder="Branch or tag"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
					/>
				{/if}
			</div>

			{#if dispatchError}
				<div class="rounded-md border border-red-800/40 bg-red-900/20 px-3 py-2 text-sm text-red-400">{dispatchError}</div>
			{/if}

			<div class="flex items-center gap-2">
				<Button variant="brand" onclick={runDispatch} disabled={dispatching || !dispatchRef || !dispatchWorkflowFile}>
					{#if dispatching}<Loader class="h-4 w-4 animate-spin" />{:else}<Play class="h-4 w-4" />{/if}
					Run workflow
				</Button>
				<Button variant="outline" onclick={() => (dispatchOpen = false)}>Cancel</Button>
			</div>
		</div>
	{/if}

	{#if dispatchSuccess}
		<div class="rounded-md border border-brand/40 bg-brand/10 px-4 py-2.5 text-sm text-[#3fb950]">{dispatchSuccess}</div>
	{/if}

	{#if loading}
		<div class="divide-y divide-secondary rounded-md border border-border overflow-hidden">
			{#each Array(4) as _}
				<div class="h-16 bg-card animate-pulse"></div>
			{/each}
		</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
	{:else if runs.length === 0}
		<div class="rounded-md border border-border bg-card p-12 text-center">
			<Zap class="h-10 w-10 mx-auto mb-4 text-muted-foreground" />
			<h3 class="text-base font-semibold text-foreground mb-2">Get started with Actions</h3>
			<p class="text-sm text-muted-foreground max-w-sm mx-auto">
				Add a workflow file to
				<code class="bg-secondary px-1 py-0.5 rounded text-xs">.workflows/</code> to get started.
			</p>
		</div>
	{:else}
		<div class="divide-y divide-secondary rounded-md border border-border overflow-hidden">
			{#each runs as run}
				{@const Icon = statusIcon(run.status)}
				<a href="/{username}/{repo}/actions/{run.id}" class="flex items-center gap-4 px-4 py-3 bg-background hover:bg-card transition-colors">
					<span>
						<Icon class="h-5 w-5 {run.status === 'running' ? 'animate-spin' : ''}" style="color: {statusColor(run.status)}" />
					</span>
					<div class="flex-1 min-w-0">
						<p class="text-sm font-semibold text-foreground truncate">{run.workflow_name}</p>
						<div class="flex items-center gap-3 mt-0.5 text-xs text-muted-foreground flex-wrap">
							<span class="flex items-center gap-1"><GitBranch class="h-3 w-3" />{run.branch}</span>
							<span class="flex items-center gap-1 font-mono"><GitCommit class="h-3 w-3" />{shortSHA(run.commit_sha)}</span>
							<span>{run.event}</span>
							<span>{timeAgo(run.created_at)}</span>
						</div>
					</div>
					<span class="hidden sm:block text-xs text-muted-foreground font-mono shrink-0 truncate max-w-[180px]">{run.workflow_file}</span>
				</a>
			{/each}
		</div>
	{/if}
</div>
