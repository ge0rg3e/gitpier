<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { auth, type Session } from '$lib/api/client';
	import { formatDate } from '$lib/utils';
	import { Loader, Monitor, Smartphone, Globe, LogOut, ShieldAlert, RefreshCw } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	let sessions = $state<Session[]>([]);
	let loading = $state(true);
	let error = $state('');
	let success = $state('');
	let revokingId = $state<string | null>(null);
	let revokingAll = $state(false);

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		await loadSessions();
	});

	async function loadSessions() {
		loading = true;
		error = '';
		try {
			const data = await auth.sessions.list();
			sessions = data.sessions ?? [];
		} catch (e: any) {
			error = e.message ?? 'Failed to load sessions.';
		} finally {
			loading = false;
		}
	}

	async function revokeSession(tokenId: string) {
		revokingId = tokenId;
		error = '';
		success = '';
		try {
			await auth.sessions.revoke(tokenId);
			sessions = sessions.filter((s) => s.token_id !== tokenId);
			success = 'Session revoked.';
		} catch (e: any) {
			error = e.message ?? 'Failed to revoke session.';
		} finally {
			revokingId = null;
		}
	}

	async function revokeOtherSessions() {
		revokingAll = true;
		error = '';
		success = '';
		try {
			await auth.sessions.revokeOthers();
			await loadSessions();
			success = 'All other sessions have been signed out.';
		} catch (e: any) {
			error = e.message ?? 'Failed to revoke sessions.';
		} finally {
			revokingAll = false;
		}
	}

	function timeAgo(dateStr: string): string {
		const diff = Date.now() - new Date(dateStr).getTime();
		const mins = Math.floor(diff / 60000);
		if (mins < 1) return 'Just now';
		if (mins < 60) return `${mins}m ago`;
		const hrs = Math.floor(mins / 60);
		if (hrs < 24) return `${hrs}h ago`;
		const days = Math.floor(hrs / 24);
		if (days < 30) return `${days}d ago`;
		return formatDate(dateStr);
	}
</script>

<svelte:head>
	<title>Active sessions — GitPier</title>
</svelte:head>

<div class="max-w-2xl space-y-6">
	<div class="flex items-start justify-between gap-4">
		<div>
			<h1 class="text-2xl font-semibold text-foreground">Active sessions</h1>
			<p class="text-sm text-muted-foreground mt-1">These are the devices currently signed into your account. Revoke any session you don't recognise.</p>
		</div>
		<Button variant="outline" size="icon" onclick={loadSessions} disabled={loading} title="Refresh">
			<RefreshCw class={`h-4 w-4 ${loading ? 'animate-spin' : ''}`} />
		</Button>
	</div>

	{#if error}
		<div class="rounded-md border border-red-800/50 bg-red-900/30 px-4 py-3 text-sm text-red-400">{error}</div>
	{/if}
	{#if success}
		<div class="rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">{success}</div>
	{/if}

	{#if loading}
		<div class="flex items-center gap-2 text-sm text-muted-foreground py-6">
			<Loader class="h-4 w-4 animate-spin" />
			Loading sessions…
		</div>
	{:else if sessions.length === 0}
		<div class="rounded-md border border-border bg-card px-5 py-8 text-center text-sm text-muted-foreground">No active sessions found.</div>
	{:else}
		<div class="space-y-3">
			{#each sessions as session (session.token_id)}
				<div class="rounded-md border {session.is_current ? 'border-brand/50 bg-brand/5' : 'border-border bg-card'} px-5 py-4">
					<div class="flex items-start gap-4">
						<!-- Device icon -->
						<div class="mt-0.5 text-muted-foreground shrink-0">
							{#if session.is_mobile}
								<Smartphone class="h-5 w-5" />
							{:else}
								<Monitor class="h-5 w-5" />
							{/if}
						</div>

						<!-- Details -->
						<div class="flex-1 min-w-0 space-y-1">
							<div class="flex items-center gap-2 flex-wrap">
								<span class="text-sm font-semibold text-foreground">
									{session.browser} on {session.os}
								</span>
								{#if session.is_current}
									<span class="text-xs font-semibold rounded-full border border-brand/40 bg-brand/10 text-brand px-2 py-0.5">This device</span>
								{/if}
							</div>

							<!-- IP — blurred by default, unblur on hover -->
							<div class="flex items-center gap-1.5 text-xs text-muted-foreground">
								<Globe class="h-3 w-3 shrink-0" />
								<span class="blur-sm hover:blur-none transition-all duration-200 cursor-pointer select-none font-mono" title="Hover to reveal IP address">
									{session.ip_address || 'Unknown'}
								</span>
							</div>

							<div class="text-xs text-muted-foreground space-y-0.5">
								<div>Last active: {timeAgo(session.last_seen_at)}</div>
								<div class="text-[11px]">Signed in: {formatDate(session.created_at)}</div>
							</div>

							<!-- UA -->
							{#if session.user_agent}
								<div class="text-[11px] text-muted-foreground/60 truncate max-w-xs">
									{session.user_agent}
								</div>
							{/if}
						</div>

						<!-- Revoke button -->
						{#if !session.is_current}
							<Button
								variant="outline"
								size="sm"
								class="shrink-0 text-red-400 border-red-800/40 hover:bg-red-900/20 hover:text-red-300"
								onclick={() => revokeSession(session.token_id)}
								disabled={revokingId === session.token_id}
							>
								{#if revokingId === session.token_id}
									<Loader class="h-3.5 w-3.5 animate-spin" />
								{:else}
									<LogOut class="h-3.5 w-3.5" />
								{/if}
								Sign out
							</Button>
						{/if}
					</div>
				</div>
			{/each}
		</div>

		<!-- Revoke all others -->
		{#if sessions.filter((s) => !s.is_current).length > 0}
			<div class="rounded-md border border-border bg-card px-5 py-4 flex items-start gap-4">
				<ShieldAlert class="h-5 w-5 text-amber-400 mt-0.5 shrink-0" />
				<div class="flex-1">
					<p class="text-sm font-semibold text-foreground">Sign out all other sessions</p>
					<p class="text-xs text-muted-foreground mt-0.5 mb-3">If you don't recognise a session, sign out of all devices except this one.</p>
					<Button variant="destructive" size="sm" onclick={revokeOtherSessions} disabled={revokingAll}>
						{#if revokingAll}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
						Sign out all other sessions
					</Button>
				</div>
			</div>
		{/if}
	{/if}
</div>
