<script lang="ts">
	import { page } from '$app/state';
	import { getContext, onDestroy } from 'svelte';
	import { workflows, type WorkflowDefinition, type WorkflowRun, type WorkflowStatus } from '$lib/api/client';
	import { timeAgo } from '$lib/utils';
	import { Zap, GitCommit, CheckCircle2, XCircle, Clock, Loader, Ban, Play } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	type WorkflowNavItem = WorkflowDefinition & {
		run_count: number;
		registered: boolean;
	};

	function buildWorkflowItems(runs: WorkflowRun[], definitions: WorkflowDefinition[]): WorkflowNavItem[] {
		const counts = new Map<string, number>();
		for (const run of runs) {
			counts.set(run.workflow_file, (counts.get(run.workflow_file) ?? 0) + 1);
		}

		const items = new Map<string, WorkflowNavItem>();
		for (const workflow of definitions) {
			items.set(workflow.path, {
				...workflow,
				run_count: counts.get(workflow.path) ?? 0,
				registered: true
			});
		}

		for (const run of runs) {
			if (items.has(run.workflow_file)) continue;
			items.set(run.workflow_file, {
				name: run.workflow_name || run.workflow_file,
				path: run.workflow_file,
				can_dispatch: false,
				supports_manual: false,
				run_count: counts.get(run.workflow_file) ?? 0,
				registered: false
			});
		}

		return Array.from(items.values()).sort((a, b) => {
			if (a.registered !== b.registered) return a.registered ? -1 : 1;
			return a.name.localeCompare(b.name);
		});
	}

	let runs = $state<WorkflowRun[]>([]);
	let total = $state(0);
	let loading = $state(true);
	let error = $state('');

	let workflowDefinitions = $state<WorkflowDefinition[]>([]);
	let loadingWorkflowDefinitions = $state(true);
	let workflowDefinitionsError = $state('');
	let selectedWorkflowPath = $state<'all' | string>('all');

	let dispatching = $state(false);
	let dispatchError = $state('');
	let dispatchSuccess = $state('');

	const { username, repo } = $derived(page.params);
	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const workflowRef = $derived(repoCtx?.currentBranch || repoCtx?.repo?.default_branch || '');
	const hasActiveRuns = $derived(runs.some((run) => run.status === 'pending' || run.status === 'running'));
	const workflowItems = $derived(buildWorkflowItems(runs, workflowDefinitions));
	const selectedWorkflow = $derived(selectedWorkflowPath === 'all' ? null : (workflowItems.find((workflow) => workflow.path === selectedWorkflowPath) ?? null));
	const filteredRuns = $derived(selectedWorkflowPath === 'all' ? runs : runs.filter((run) => run.workflow_file === selectedWorkflowPath));
	const canDispatchSelectedWorkflow = $derived(Boolean(selectedWorkflow?.can_dispatch));
	const activeRef = $derived(repoCtx?.currentBranch || repoCtx?.repo?.default_branch || '');

	async function loadRuns() {
		loading = true;
		error = '';
		try {
			const limit = 100;
			let nextOffset = 0;
			let nextTotal = 0;
			const nextRuns: WorkflowRun[] = [];

			while (true) {
				const data = await workflows.listRuns(username!, repo!, limit, nextOffset);
				if (nextOffset === 0) nextTotal = data.total ?? 0;
				const batch = data.runs ?? [];
				nextRuns.push(...batch);
				if (batch.length === 0 || nextRuns.length >= nextTotal) break;
				nextOffset += batch.length;
			}

			runs = nextRuns;
			total = nextTotal;
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	async function loadWorkflowDefinitions(ref: string) {
		if (!ref) return;
		loadingWorkflowDefinitions = true;
		workflowDefinitionsError = '';
		try {
			const data = await workflows.listDispatchable(username!, repo!, ref);
			workflowDefinitions = data.workflows ?? [];
		} catch (e: any) {
			workflowDefinitions = [];
			workflowDefinitionsError = e.message ?? 'Failed to load workflows';
		} finally {
			loadingWorkflowDefinitions = false;
		}
	}

	$effect(() => {
		loadRuns();
	});

	$effect(() => {
		if (workflowRef) loadWorkflowDefinitions(workflowRef);
	});

	$effect(() => {
		if (selectedWorkflowPath !== 'all' && !workflowItems.some((workflow) => workflow.path === selectedWorkflowPath)) {
			selectedWorkflowPath = 'all';
		}
	});

	const interval = setInterval(() => {
		if (hasActiveRuns) loadRuns();
	}, 5000);
	onDestroy(() => clearInterval(interval));

	async function runSelectedWorkflow() {
		if (isRepoArchived || !selectedWorkflow || !selectedWorkflow.can_dispatch || !activeRef) return;
		dispatchError = '';
		dispatchSuccess = '';
		dispatching = true;
		try {
			await workflows.dispatch(username!, repo!, activeRef, selectedWorkflow.path);
			dispatchSuccess = `Triggered ${selectedWorkflow.name} on ${activeRef}`;
			await loadRuns();
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

<div class="grid gap-6 xl:grid-cols-[260px_minmax(0,1fr)]">
	<aside class="overflow-hidden rounded-xl border border-border bg-card/60">
		<div class="border-b border-border px-4 py-4">
			<p class="text-lg font-semibold text-foreground">Actions</p>
			<p class="mt-1 text-sm text-muted-foreground">
				{workflowDefinitions.length || workflowItems.length} workflow{(workflowDefinitions.length || workflowItems.length) !== 1 ? 's' : ''} available
			</p>
		</div>

		<div class="space-y-1.5 p-2">
			<button
				type="button"
				onclick={() => (selectedWorkflowPath = 'all')}
				class={`w-full rounded-lg border px-3 py-3 text-left transition-colors ${
					selectedWorkflowPath === 'all' ? 'border-border bg-background text-foreground shadow-sm' : 'border-transparent bg-transparent text-foreground hover:bg-secondary/50'
				}`}
			>
				<p class="text-sm font-semibold">All workflows</p>
				<p class="mt-1 text-xs text-muted-foreground">{total} run{total !== 1 ? 's' : ''} across the repository</p>
			</button>

			{#if loadingWorkflowDefinitions && workflowItems.length === 0}
				{#each Array(4) as _}
					<div class="h-16 rounded-lg bg-background animate-pulse"></div>
				{/each}
			{:else}
				{#each workflowItems as workflow}
					<button
						type="button"
						onclick={() => (selectedWorkflowPath = workflow.path)}
						class={`w-full rounded-lg border px-3 py-3 text-left transition-colors ${
							selectedWorkflowPath === workflow.path ? 'border-border bg-background text-foreground shadow-sm' : 'border-transparent bg-transparent text-foreground hover:bg-secondary/50'
						}`}
					>
						<div class="flex items-start justify-between gap-3">
							<p class="min-w-0 truncate text-sm font-semibold">{workflow.name}</p>
							{#if workflow.supports_manual}
								<span class="shrink-0 rounded-full border border-emerald-500/25 bg-emerald-500/10 px-2 py-0.5 text-[10px] font-semibold uppercase tracking-wide text-emerald-400">
									Manual
								</span>
							{/if}
						</div>
						<p class="mt-1 truncate text-xs text-muted-foreground">{workflow.path}</p>
						<p class="mt-2 text-xs text-muted-foreground">
							{workflow.run_count} run{workflow.run_count !== 1 ? 's' : ''}{workflow.registered ? '' : ' in history'}
						</p>
					</button>
				{/each}
			{/if}
		</div>

		{#if workflowDefinitionsError}
			<div class="border-t border-border px-4 py-3 text-xs text-red-400">{workflowDefinitionsError}</div>
		{/if}
	</aside>

	<div class="space-y-4">
		<div class="flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
			<div>
				<h2 class="text-2xl font-semibold text-foreground">{selectedWorkflow?.name ?? 'All workflows'}</h2>
				{#if selectedWorkflow}
					<p class="mt-1 text-sm text-muted-foreground">{selectedWorkflow.path}</p>
				{:else}
					<p class="mt-1 text-sm text-muted-foreground">Showing runs from every workflow in this repository.</p>
				{/if}
			</div>

			{#if canDispatchSelectedWorkflow && !isRepoArchived}
				<div class="flex items-center gap-3">
					{#if activeRef}
						<span class="rounded-md border border-border bg-card px-3 py-2 text-xs text-muted-foreground">
							Branch: <span class="font-medium text-foreground">{activeRef}</span>
						</span>
					{/if}
					<Button variant="brand" onclick={runSelectedWorkflow} disabled={dispatching}>
						{#if dispatching}
							<Loader class="h-3.5 w-3.5 animate-spin" />
						{:else}
							<Play class="h-3.5 w-3.5" />
						{/if}
						Run workflow
					</Button>
				</div>
			{:else if selectedWorkflow && selectedWorkflow.supports_manual && isRepoArchived}
				<Button variant="brand" disabled>
					<Play class="h-3.5 w-3.5" />
					Run workflow
				</Button>
			{/if}
		</div>

		{#if dispatchSuccess}
			<div class="rounded-md border border-brand/40 bg-brand/10 px-4 py-2.5 text-sm text-[#3fb950]">{dispatchSuccess}</div>
		{/if}

		{#if loading}
			<div class="overflow-hidden rounded-xl border border-border bg-card">
				<div class="border-b border-border px-4 py-3">
					<div class="h-5 w-32 rounded bg-secondary animate-pulse"></div>
				</div>
				<div class="divide-y divide-secondary">
					{#each Array(5) as _}
						<div class="h-20 bg-background animate-pulse"></div>
					{/each}
				</div>
			</div>
		{:else if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
		{:else if filteredRuns.length === 0}
			<div class="rounded-xl border border-border bg-card/70 p-10 text-center">
				<Zap class="mx-auto mb-4 h-10 w-10 text-muted-foreground" />
				{#if runs.length === 0}
					<h3 class="text-base font-semibold text-foreground mb-2">Get started with Actions</h3>
					<p class="text-sm text-muted-foreground max-w-sm mx-auto">
						Add a workflow file to
						<code class="bg-secondary px-1 py-0.5 rounded text-xs">.workflows/</code>,
					</p>
				{:else}
					<h3 class="mb-2 text-base font-semibold text-foreground">No runs for this workflow yet</h3>
					<p class="text-sm text-muted-foreground max-w-sm mx-auto">This workflow is registered, but it has not produced any runs yet.</p>
				{/if}
			</div>
		{:else}
			<div class="overflow-hidden rounded-xl border border-border bg-card">
				<div
					class="grid grid-cols-[minmax(0,1fr)_140px] gap-3 border-b border-border bg-secondary/20 px-4 py-3 text-xs font-semibold uppercase tracking-wide text-muted-foreground sm:grid-cols-[minmax(0,1fr)_120px_180px]"
				>
					<span>Run</span>
					<span class="hidden sm:block">Branch</span>
					<span class="text-right">Started</span>
				</div>

				<div class="divide-y divide-secondary">
					{#each filteredRuns as run}
						{@const Icon = statusIcon(run.status)}
						<a
							href="/{username}/{repo}/actions/{run.id}"
							class="grid grid-cols-[minmax(0,1fr)_140px] gap-3 px-4 py-4 transition-colors hover:bg-secondary/20 sm:grid-cols-[minmax(0,1fr)_120px_180px]"
						>
							<div class="flex min-w-0 items-start gap-3">
								<span class="mt-0.5">
									<Icon class="h-4.5 w-4.5 {run.status === 'running' ? 'animate-spin' : ''}" style="color: {statusColor(run.status)}" />
								</span>
								<div class="min-w-0">
									<p class="truncate text-sm font-semibold text-foreground">{run.workflow_name}</p>
									<div class="mt-1 flex flex-wrap items-center gap-3 text-xs text-muted-foreground">
										<span class="flex items-center gap-1 font-mono"><GitCommit class="h-3 w-3" />{shortSHA(run.commit_sha)}</span>
										<span>{run.event}</span>
										<span class="font-medium capitalize" style="color: {statusColor(run.status)}">{run.status}</span>
									</div>
								</div>
							</div>
							<div class="hidden items-center text-sm text-muted-foreground sm:flex">
								<span class="truncate">{run.branch}</span>
							</div>
							<div class="flex items-center justify-end text-right text-xs text-muted-foreground">
								<span>{timeAgo(run.created_at)}</span>
							</div>
						</a>
					{/each}
				</div>
			</div>
		{/if}
	</div>
</div>
