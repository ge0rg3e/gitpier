<script lang="ts">
	import { adminSystem, type AdminSystemStats } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Building2, FolderGit2, GitPullRequest, HardDrive, RefreshCw, Server, Users } from '@lucide/svelte';

	let password = $state('');
	let sessionPassword = $state('');
	let stats = $state<AdminSystemStats | null>(null);
	let loading = $state(false);
	let error = $state('');

	async function unlockDashboard(event: SubmitEvent) {
		event.preventDefault();
		error = '';

		const inputPassword = password.trim();
		if (!inputPassword) {
			error = 'Please enter the admin password.';
			return;
		}

		sessionPassword = inputPassword;
		await refreshStats();
	}

	async function refreshStats() {
		if (!sessionPassword) return;
		loading = true;
		error = '';
		try {
			stats = await adminSystem.getStats(sessionPassword);
			password = '';
		} catch (e: any) {
			stats = null;
			if (e?.status === 401) {
				error = 'Incorrect admin password.';
			} else if (e?.status === 503) {
				error = 'SYSTEM_ADMIN_PASSWORD is not configured in backend env.';
			} else {
				error = e?.message ?? 'Failed to load system dashboard.';
			}
		} finally {
			loading = false;
		}
	}

	function formatBytes(bytes: number) {
		if (!Number.isFinite(bytes) || bytes < 0) return '0 B';
		const units = ['B', 'KB', 'MB', 'GB', 'TB'];
		let size = bytes;
		let unitIndex = 0;
		while (size >= 1024 && unitIndex < units.length - 1) {
			size /= 1024;
			unitIndex++;
		}
		return `${size.toFixed(unitIndex === 0 ? 0 : 2)} ${units[unitIndex]}`;
	}

	function formatDate(iso: string) {
		const dt = new Date(iso);
		return Number.isNaN(dt.getTime()) ? '-' : dt.toLocaleString();
	}

	function averageFilesystemRepoSize() {
		if (!stats || stats.repositories.total <= 0) return 0;
		return Math.floor(stats.repositories.filesystem_total_size_bytes / stats.repositories.total);
	}

	$effect(() => {
		if (!stats || !sessionPassword) return;

		const intervalId = setInterval(() => {
			void refreshStats();
		}, 10000);

		return () => {
			clearInterval(intervalId);
		};
	});
</script>

<svelte:head>
	<title>System Dashboard - GitPier</title>
</svelte:head>

<div class="mx-auto w-full max-w-6xl space-y-5 px-2 py-4">
	<div class="rounded-2xl border border-border bg-card p-5">
		<div class="flex flex-wrap items-center justify-between gap-4">
			<div class="flex items-center gap-3">
				<div class="rounded-xl bg-primary/15 p-2 text-primary">
					<Server class="h-5 w-5" />
				</div>
				<div>
					<h1 class="text-xl font-semibold text-foreground">System Dashboard</h1>
					<p class="text-sm text-muted-foreground">Repository and platform health overview.</p>
				</div>
			</div>
			{#if stats}
				<div class="flex items-center gap-2">
					<Button variant="outline" class="gap-2" onclick={refreshStats} disabled={loading}>
						<RefreshCw class={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
						Refresh
					</Button>
				</div>
			{/if}
		</div>
	</div>

	{#if !stats}
		<form onsubmit={unlockDashboard} class="mx-auto max-w-xl rounded-2xl border border-border bg-card p-5">
			<h2 class="text-sm font-semibold text-foreground">Admin Authentication</h2>
			<div class="mt-4 space-y-3">
				<input
					type="password"
					bind:value={password}
					placeholder="Admin password"
					autocomplete="current-password"
					class="h-10 w-full rounded-lg border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
				/>
				<Button type="submit" class="w-full" disabled={loading}>
					{loading ? 'Unlocking...' : 'Unlock dashboard'}
				</Button>
				{#if error}
					<p class="rounded-lg border border-red-800/40 bg-red-950/30 px-3 py-2 text-xs text-red-400">{error}</p>
				{/if}
			</div>
		</form>
	{:else}
		<div class="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
			<div class="rounded-xl border border-border bg-card p-4">
				<div class="flex items-center justify-between">
					<p class="text-xs text-muted-foreground">Total Repositories</p>
					<FolderGit2 class="h-4 w-4 text-muted-foreground" />
				</div>
				<p class="mt-2 text-2xl font-semibold text-foreground">{stats.repositories.total}</p>
				<p class="mt-1 text-xs text-muted-foreground">{stats.repositories.public} public | {stats.repositories.private} private</p>
			</div>
			<div class="rounded-xl border border-border bg-card p-4">
				<div class="flex items-center justify-between">
					<p class="text-xs text-muted-foreground">Repository Storage</p>
					<HardDrive class="h-4 w-4 text-muted-foreground" />
				</div>
				<p class="mt-2 text-2xl font-semibold text-foreground">{formatBytes(stats.repositories.filesystem_total_size_bytes)}</p>
				<p class="mt-1 text-xs text-muted-foreground">Average: {formatBytes(averageFilesystemRepoSize())}</p>
			</div>
			<div class="rounded-xl border border-border bg-card p-4">
				<div class="flex items-center justify-between">
					<p class="text-xs text-muted-foreground">Users</p>
					<Users class="h-4 w-4 text-muted-foreground" />
				</div>
				<p class="mt-2 text-2xl font-semibold text-foreground">{stats.users.total}</p>
				<p class="mt-1 text-xs text-muted-foreground">Suspended: {stats.users.suspended}</p>
			</div>
			<div class="rounded-xl border border-border bg-card p-4">
				<div class="flex items-center justify-between">
					<p class="text-xs text-muted-foreground">Organizations</p>
					<Building2 class="h-4 w-4 text-muted-foreground" />
				</div>
				<p class="mt-2 text-2xl font-semibold text-foreground">{stats.organizations.total}</p>
				<p class="mt-1 text-xs text-muted-foreground">Suspended: {stats.organizations.suspended}</p>
			</div>
		</div>

		<div class="grid gap-3 lg:grid-cols-3">
			<div class="rounded-xl border border-border bg-card p-4">
				<h3 class="text-sm font-semibold text-foreground">Repository Health</h3>
				<div class="mt-3 space-y-2 text-sm text-muted-foreground">
					<p>Archived: <span class="text-foreground">{stats.repositories.archived}</span></p>
					<p>Suspended: <span class="text-foreground">{stats.repositories.suspended}</span></p>
					<p>Filesystem size: <span class="text-foreground">{formatBytes(stats.repositories.filesystem_total_size_bytes)}</span></p>
					<p>Scan errors: <span class="text-foreground">{stats.repositories.filesystem_scan_errors}</span></p>
				</div>
			</div>
			<div class="rounded-xl border border-border bg-card p-4">
				<h3 class="flex items-center gap-2 text-sm font-semibold text-foreground">
					<GitPullRequest class="h-4 w-4 text-muted-foreground" />
					Pull Requests
				</h3>
				<div class="mt-3 space-y-2 text-sm text-muted-foreground">
					<p>Total: <span class="text-foreground">{stats.pull_requests.total}</span></p>
					<p>Open: <span class="text-foreground">{stats.pull_requests.open}</span></p>
					<p>Merged: <span class="text-foreground">{stats.pull_requests.merged}</span></p>
					<p>Closed: <span class="text-foreground">{stats.pull_requests.closed}</span></p>
				</div>
			</div>
			<div class="rounded-xl border border-border bg-card p-4">
				<h3 class="text-sm font-semibold text-foreground">Workflow Runs</h3>
				<div class="mt-3 space-y-2 text-sm text-muted-foreground">
					<p>Total: <span class="text-foreground">{stats.workflow_runs.total}</span></p>
					<p>Running: <span class="text-foreground">{stats.workflow_runs.running}</span></p>
					<p>Success: <span class="text-foreground">{stats.workflow_runs.success}</span></p>
					<p>Failure: <span class="text-foreground">{stats.workflow_runs.failure}</span></p>
				</div>
			</div>
		</div>

		<div class="rounded-xl border border-border bg-card p-4">
			<h3 class="text-sm font-semibold text-foreground">Largest Repositories</h3>
			{#if stats.largest_repositories.length === 0}
				<p class="mt-3 text-sm text-muted-foreground">No repositories found.</p>
			{:else}
				<div class="mt-3 overflow-x-auto">
					<table class="min-w-full text-left text-sm">
						<thead class="text-xs uppercase text-muted-foreground">
							<tr>
								<th class="px-3 py-2 font-medium">Repository</th>
								<th class="px-3 py-2 font-medium">Size</th>
							</tr>
						</thead>
						<tbody>
							{#each stats.largest_repositories as repo}
								<tr class="border-t border-border/80">
									<td class="px-3 py-2 text-foreground">{repo.full_name}</td>
									<td class="px-3 py-2 text-foreground">{formatBytes(repo.size_bytes)}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{/if}
		</div>

		<p class="text-xs text-muted-foreground">Updated: {formatDate(stats.generated_at)}</p>
	{/if}
</div>
