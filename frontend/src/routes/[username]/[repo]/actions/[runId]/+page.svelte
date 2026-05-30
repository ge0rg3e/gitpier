<script lang="ts">
	import { page } from '$app/state';
	import { onDestroy } from 'svelte';
	import { workflows, API_BASE, type WorkflowRun, type WorkflowJob, type WorkflowStep, type WorkflowStatus } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import CodeViewer from '$lib/components/CodeViewer.svelte';
	import { timeAgo } from '$lib/utils';
	import { Zap, GitBranch, GitCommit, CheckCircle2, XCircle, Clock, Loader, Ban, ChevronDown, ChevronRight, AlertCircle, RotateCcw, Trash2, Download } from '@lucide/svelte';

	let run = $state<WorkflowRun | null>(null);
	let loading = $state(true);
	let error = $state('');
	let cancelling = $state(false);
	let rerunning = $state(false);
	let deleting = $state(false);
	let downloading = $state(false);
	let expanded = $state<Record<string, boolean>>({});

	const { username, repo, runId } = $derived(page.params);
	const isActive = $derived(run?.status === 'pending' || run?.status === 'running');
	const isRecentlyCreated = $derived.by(() => {
		if (!run?.created_at) return false;
		const created = new Date(run.created_at).getTime();
		return Date.now() - created < 30000;
	});

	const artifactUrl = $derived(run?.id && run.status === 'success' ? `${API_BASE}/artifact-files/artifacts/run${run.id}` : '');

	async function load() {
		if (!runId) {
			error = 'Invalid workflow run ID';
			loading = false;
			return;
		}
		try {
			const data = await workflows.getRun(username!, repo!, runId);
			run = data.run;
			if (run?.jobs) {
				for (const job of run.jobs) {
					if (job.status === 'running' || job.status === 'failure' || job.status === 'pending') {
						expanded[job.id] = true;
					}
				}
			}
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
		if (isActive) load();
	}, 2000);
	onDestroy(() => clearInterval(interval));

	async function cancelRun() {
		if (!run) return;
		cancelling = true;
		try {
			await workflows.cancelRun(username!, repo!, run.id);
			await load();
		} catch {}
		cancelling = false;
	}

	async function rerun() {
		if (!run) return;
		rerunning = true;
		error = '';
		try {
			await workflows.rerun(username!, repo!, run.id);
			await load();
		} catch (e: any) {
			error = e.message;
		} finally {
			rerunning = false;
		}
	}

	async function deleteRun() {
		if (!run || !confirm('Delete this run?')) return;
		deleting = true;
		error = '';
		try {
			await workflows.deleteRun(username!, repo!, run.id);
			window.location.href = `/${username}/${repo}/actions`;
		} catch (e: any) {
			error = e.message;
		} finally {
			deleting = false;
		}
	}

	async function downloadArtifact() {
		if (!artifactUrl) return;
		downloading = true;
		error = '';
		try {
			const res = await fetch(artifactUrl);
			if (!res.ok) {
				throw new Error(`Download failed: ${res.status} ${res.statusText}`);
			}
			const blob = await res.blob();
			let filename = `${repo}-artifact-${runId}.zip`;
			const cd = res.headers.get('Content-Disposition');
			if (cd) {
				const match = cd.match(/filename="?([^";]+)"?/);
				if (match) filename = match[1];
			}
			const url = URL.createObjectURL(blob);
			const a = document.createElement('a');
			a.href = url;
			a.download = filename;
			document.body.appendChild(a);
			a.click();
			setTimeout(() => {
				document.body.removeChild(a);
				URL.revokeObjectURL(url);
			}, 100);
		} catch (e: any) {
			error = e?.message || 'Download failed';
		} finally {
			downloading = false;
		}
	}

	function toggleJob(jobId: string) {
		expanded[jobId] = !expanded[jobId];
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

	function duration(start?: string, end?: string): string {
		if (!start) return '';
		const s = new Date(start).getTime();
		const e = end ? new Date(end).getTime() : Date.now();
		const secs = Math.round((e - s) / 1000);
		if (secs < 60) return `${secs}s`;
		return `${Math.floor(secs / 60)}m ${secs % 60}s`;
	}

	function shortSHA(sha: string): string {
		return sha?.slice(0, 7) ?? '';
	}
</script>

<div class="space-y-5">
	{#if loading && !run}
		<div class="space-y-3">
			<div class="h-28 rounded-md border border-border bg-card animate-pulse"></div>
			<div class="h-16 rounded-md border border-border bg-card animate-pulse"></div>
		</div>
	{:else if error}
		<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400 flex items-center gap-2">
			<AlertCircle class="h-4 w-4 shrink-0" />
			{error}
		</div>
	{:else if run}
		{@const RunStatusIcon = statusIcon(run.status)}
		<!-- Run summary header -->
		<div class="rounded-md border border-border bg-card p-5">
			<div class="flex items-start justify-between gap-4 mb-4">
				<div class="flex items-center gap-3">
					<RunStatusIcon class="h-6 w-6 {run.status === 'running' ? 'animate-spin' : ''}" style="color: {statusColor(run.status)}" />
					<div>
						<h2 class="text-lg font-semibold text-foreground">{run.workflow_name}</h2>
						<p class="text-xs text-muted-foreground font-mono">{run.workflow_file}</p>
						{#if run.status === 'success'}
							<button
								onclick={downloadArtifact}
								class="inline-flex mt-2 h-7 items-center gap-1.5 rounded-md border border-green-800/50 bg-green-900/20 px-3 text-xs text-green-400 hover:bg-green-900/40 transition-colors"
							>
								<Download class="h-3.5 w-3.5" />
								Download artifact
							</button>
						{/if}
					</div>
				</div>
				{#if authStore.isAuthenticated}
					<div class="flex items-center gap-2 shrink-0">
						<button
							onclick={deleteRun}
							disabled={deleting}
							class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-secondary px-3 text-xs text-red-400 hover:bg-red-900/20 hover:border-red-800/50 disabled:opacity-60 transition-colors"
						>
							<Trash2 class="h-3.5 w-3.5 {deleting ? 'animate-spin' : ''}" />
							Delete
						</button>
						{#if rerunning}
							<button
								disabled
								class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-secondary px-3 text-xs text-foreground opacity-60 cursor-wait transition-colors"
							>
								<RotateCcw class="h-3.5 w-3.5 animate-spin" />
								Re-running...
							</button>
						{:else if isActive || isRecentlyCreated}
							<button
								disabled
								class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-secondary px-3 text-xs text-muted-foreground opacity-60 cursor-not-allowed transition-colors"
							>
								<RotateCcw class="h-3.5 w-3.5" />
								{isActive ? 'Running...' : 'Starting...'}
							</button>
						{:else}
							<button
								onclick={rerun}
								class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-secondary px-3 text-xs text-foreground hover:bg-accent transition-colors"
							>
								<RotateCcw class="h-3.5 w-3.5" />
								Re-run
							</button>
						{/if}
						{#if isActive}
							<button
								onclick={cancelRun}
								disabled={cancelling}
								class="inline-flex h-8 items-center gap-1.5 rounded-md border border-border bg-secondary px-3 text-xs text-red-400 hover:bg-red-900/20 hover:border-red-800/50 disabled:opacity-60 transition-colors"
							>
								<Ban class="h-3.5 w-3.5" />
								Cancel run
							</button>
						{/if}
					</div>
				{/if}
			</div>
			<div class="flex items-center gap-4 text-xs text-muted-foreground border-t border-secondary pt-3 flex-wrap">
				<span class="flex items-center gap-1.5"><GitBranch class="h-3.5 w-3.5" /><span class="text-foreground font-medium">{run.branch}</span></span>
				<span class="flex items-center gap-1.5 font-mono"><GitCommit class="h-3.5 w-3.5" />{shortSHA(run.commit_sha)}</span>
				<span class="capitalize">{run.event}</span>
				<span>Triggered {timeAgo(run.created_at)}</span>
			</div>
		</div>

		<!-- Jobs -->
		{#if run.jobs && run.jobs.length > 0}
			<div class="space-y-2">
				{#each run.jobs as job}
					{@const JobIcon = statusIcon(job.status)}
					<div class="rounded-md border border-border overflow-hidden">
						<button onclick={() => toggleJob(job.id)} class="w-full flex items-center gap-3 px-4 py-3 bg-card hover:bg-accent transition-colors text-left">
							<JobIcon class="h-4 w-4 shrink-0 {job.status === 'running' ? 'animate-spin' : ''}" style="color: {statusColor(job.status)}" />
							<span class="flex-1 text-sm font-semibold text-foreground">{job.name}</span>
							{#if job.started_at}
								<span class="text-xs text-muted-foreground">{duration(job.started_at, job.finished_at ?? undefined)}</span>
							{/if}
							{#if expanded[job.id]}
								<ChevronDown class="h-4 w-4 text-muted-foreground" />
							{:else}
								<ChevronRight class="h-4 w-4 text-muted-foreground" />
							{/if}
						</button>

						{#if expanded[job.id] && job.steps && job.steps.length > 0}
							<div class="border-t border-border divide-y divide-secondary bg-background">
								{#each job.steps as step}
									{@const StepIcon = statusIcon(step.status)}
									<div class="px-4 py-2.5">
										<div class="flex items-center gap-2.5">
											<StepIcon class="h-3.5 w-3.5 shrink-0 {step.status === 'running' ? 'animate-spin' : ''}" style="color: {statusColor(step.status)}" />
											<span class="text-sm text-foreground flex-1 truncate">{step.name}</span>
											<div class="flex items-center gap-2 text-xs text-muted-foreground shrink-0">
												{#if step.started_at}
													<span>{duration(step.started_at, step.finished_at ?? undefined)}</span>
												{/if}
												{#if step.exit_code != null && step.exit_code !== 0}
													<span class="text-destructive">exit {step.exit_code}</span>
												{/if}
											</div>
										</div>
										{#if step.log && step.log.trim() !== ''}
											<div class="mt-2 ml-6 max-h-96 overflow-y-auto">
												<CodeViewer code={step.log} filePath={`${step.name}.log`} containerClass="bg-[#010409]" />
											</div>
										{/if}
									</div>
								{/each}
							</div>
						{/if}
					</div>
				{/each}
			</div>
		{:else}
			<div class="rounded-md border border-border bg-card p-10 text-center">
				<Zap class="h-8 w-8 mx-auto mb-3 text-muted-foreground" />
				<p class="text-sm text-muted-foreground">No jobs found for this run.</p>
			</div>
		{/if}
	{/if}
</div>
