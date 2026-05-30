<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { moderation, users, type ModerationPolicy, type ModerationBlockedUser, type ModerationBlockedKeyword } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { repos } from '$lib/api/client';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Switch } from '$lib/components/ui/switch/index.js';
	import { Badge } from '$lib/components/ui/badge/index.js';
	import { Separator } from '$lib/components/ui/separator/index.js';
	import { mediaUrl } from '$lib/utils';
	import { Shield, UserX, Hash, AlertTriangle, Loader, Plus, Trash2, Users, GitPullRequest, MessageSquare, GitCommitHorizontal, Clock, Activity, ArrowLeft, Info, ChevronDown } from '@lucide/svelte';

	const username = $derived(page.params.username as string);
	const repoName = $derived(page.params.repo as string);

	let policy = $state<ModerationPolicy | null>(null);
	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let saveSuccess = $state(false);

	// Block user form
	let blockUsername = $state('');
	let blockReason = $state('');
	let blocking = $state(false);
	let blockError = $state('');

	// Keyword form
	let newKeyword = $state('');
	let keywordApplyTo = $state('all');
	let addingKeyword = $state(false);
	let keywordError = $state('');

	const api = $derived(moderation.repo(username, repoName));

	onMount(async () => {
		while (authStore.loading) await new Promise((r) => setTimeout(r, 10));
		if (!authStore.isAuthenticated) {
			goto('/login');
			return;
		}
		try {
			const [repoData, policyData] = await Promise.all([repos.get(username, repoName), api.getPolicy()]);
			if (repoData.repo.owner_id !== authStore.user?.id) {
				goto(`/${username}/${repoName}`);
				return;
			}
			policy = policyData.policy;
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	});

	async function savePolicy() {
		if (!policy) return;
		saving = true;
		saveSuccess = false;
		error = '';
		try {
			const updated = await api.updatePolicy({
				inherit_from_owner: policy.inherit_from_owner,
				block_issues: policy.block_issues,
				block_prs: policy.block_prs,
				block_pushes: policy.block_pushes,
				block_comments: policy.block_comments,
				max_issues_per_day: policy.max_issues_per_day,
				max_prs_per_day: policy.max_prs_per_day,
				max_comments_per_day: policy.max_comments_per_day,
				min_account_age_days: policy.min_account_age_days,
				require_min_activity: policy.require_min_activity,
				min_commits: policy.min_commits,
				min_contributions: policy.min_contributions
			});
			policy = updated.policy;
			saveSuccess = true;
			setTimeout(() => (saveSuccess = false), 3000);
		} catch (e: any) {
			error = e.message;
		} finally {
			saving = false;
		}
	}

	async function blockUser(e: Event) {
		e.preventDefault();
		if (!blockUsername.trim()) return;
		blocking = true;
		blockError = '';
		try {
			const res = await api.blockUser(blockUsername.trim(), blockReason.trim() || undefined);
			policy!.blocked_users = [...(policy!.blocked_users ?? []), res.blocked_user];
			blockUsername = '';
			blockReason = '';
		} catch (e: any) {
			blockError = e.message;
		} finally {
			blocking = false;
		}
	}

	async function unblockUser(userID: number) {
		try {
			await api.unblockUser(userID);
			policy!.blocked_users = policy!.blocked_users.filter((u) => u.user_id !== userID);
		} catch (e: any) {
			alert(e.message);
		}
	}

	async function addKeyword(e: Event) {
		e.preventDefault();
		if (!newKeyword.trim()) return;
		addingKeyword = true;
		keywordError = '';
		try {
			const res = await api.addKeyword(newKeyword.trim(), keywordApplyTo);
			policy!.blocked_keywords = [...(policy!.blocked_keywords ?? []), res.keyword];
			newKeyword = '';
		} catch (e: any) {
			keywordError = e.message;
		} finally {
			addingKeyword = false;
		}
	}

	async function removeKeyword(id: number) {
		try {
			await api.removeKeyword(id);
			policy!.blocked_keywords = policy!.blocked_keywords.filter((k) => k.id !== id);
		} catch (e: any) {
			alert(e.message);
		}
	}

	function applyToLabel(t: string) {
		return { all: 'All', issues: 'Issues', prs: 'Pull Requests', commits: 'Commits' }[t] ?? t;
	}
</script>

<svelte:head>
	<title>Moderation</title>
</svelte:head>

{#if loading}
	<div class="text-center py-12 text-muted-foreground flex items-center justify-center gap-2">
		<Loader class="h-4 w-4 animate-spin" />Loading…
	</div>
{:else if error && !policy}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
{:else if policy}
	<div class="max-w-2xl space-y-6">
		<!-- Header -->
		<div class="flex items-center gap-3">
			<Shield class="h-6 w-6 text-primary" />
			<div>
				<h1 class="text-xl font-semibold text-foreground">Moderation</h1>
				<p class="text-sm text-muted-foreground">Control who can interact with this repository and how.</p>
			</div>
		</div>

		{#if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
		{/if}
		{#if saveSuccess}
			<div class="rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">Moderation settings saved.</div>
		{/if}

		<!-- Inherit from owner -->
		<section class="rounded-lg border border-border bg-card p-5 space-y-4">
			<div class="flex items-start justify-between gap-4">
				<div>
					<p class="text-sm font-semibold text-foreground">Inherit owner's moderation rules</p>
					<p class="text-xs text-muted-foreground mt-0.5">Also apply the rules configured on your account (or organization) to this repository.</p>
				</div>
				<Switch bind:checked={policy.inherit_from_owner} />
			</div>
		</section>

		<!-- Interaction locks -->
		<section class="rounded-lg border border-border bg-card p-5 space-y-4">
			<h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
				<Shield class="h-4 w-4 text-muted-foreground" />
				Interaction locks
			</h2>
			<Separator />

			<div class="space-y-3">
				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm text-foreground flex items-center gap-1.5"><Users class="h-4 w-4 text-muted-foreground" /> Block new issues</p>
						<p class="text-xs text-muted-foreground">No one (except you) can open new issues.</p>
					</div>
					<Switch bind:checked={policy.block_issues} />
				</div>

				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm text-foreground flex items-center gap-1.5"><GitPullRequest class="h-4 w-4 text-muted-foreground" /> Block new pull requests</p>
						<p class="text-xs text-muted-foreground">No one (except you) can open new pull requests.</p>
					</div>
					<Switch bind:checked={policy.block_prs} />
				</div>

				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm text-foreground flex items-center gap-1.5"><MessageSquare class="h-4 w-4 text-muted-foreground" /> Block new comments</p>
						<p class="text-xs text-muted-foreground">No one (except you) can post comments.</p>
					</div>
					<Switch bind:checked={policy.block_comments} />
				</div>

				<div class="flex items-center justify-between">
					<div>
						<p class="text-sm text-foreground flex items-center gap-1.5"><GitCommitHorizontal class="h-4 w-4 text-muted-foreground" /> Block pushes</p>
						<p class="text-xs text-muted-foreground">No one (except you) can push commits.</p>
					</div>
					<Switch bind:checked={policy.block_pushes} />
				</div>
			</div>
		</section>

		<!-- Rate limits -->
		<section class="rounded-lg border border-border bg-card p-5 space-y-4">
			<div class="flex items-start justify-between gap-4">
				<div>
					<h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
						<Clock class="h-4 w-4 text-muted-foreground" />
						Rate limits
					</h2>
					<p class="text-xs text-muted-foreground mt-0.5">Per-user limits enforced per calendar day. Set to 0 to disable.</p>
				</div>
			</div>
			<Separator />

			<div class="grid grid-cols-1 sm:grid-cols-3 gap-4">
				<div>
					<label class="block text-xs font-semibold text-muted-foreground mb-1">Max issues per user / day</label>
					<input
						type="number"
						min="0"
						bind:value={policy.max_issues_per_day}
						class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
					/>
				</div>
				<div>
					<label class="block text-xs font-semibold text-muted-foreground mb-1">Max PRs per user / day</label>
					<input
						type="number"
						min="0"
						bind:value={policy.max_prs_per_day}
						class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
					/>
				</div>
				<div>
					<label class="block text-xs font-semibold text-muted-foreground mb-1">Max comments per user / day</label>
					<input
						type="number"
						min="0"
						bind:value={policy.max_comments_per_day}
						class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
					/>
				</div>
			</div>
		</section>

		<!-- Account requirements -->
		<section class="rounded-lg border border-border bg-card p-5 space-y-4">
			<h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
				<Activity class="h-4 w-4 text-muted-foreground" />
				Account requirements
			</h2>
			<Separator />

			<div>
				<label class="block text-xs font-semibold text-muted-foreground mb-1">Minimum account age (days, 0 = no requirement)</label>
				<input
					type="number"
					min="0"
					bind:value={policy.min_account_age_days}
					class="h-8 w-48 rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
				/>
				{#if policy.min_account_age_days > 0}
					<p class="mt-1 text-xs text-muted-foreground">
						= {Math.floor(policy.min_account_age_days / 30)} months {policy.min_account_age_days % 30} days
					</p>
				{/if}
			</div>

			<div class="flex items-start gap-3">
				<Switch bind:checked={policy.require_min_activity} id="require-activity" />
				<div>
					<label for="require-activity" class="text-sm font-medium text-foreground cursor-pointer">Require minimum activity</label>
					<p class="text-xs text-muted-foreground">Only allow users who meet the activity thresholds below.</p>
				</div>
			</div>

			{#if policy.require_min_activity}
				<div class="grid grid-cols-1 sm:grid-cols-2 gap-4 pl-9">
					<div>
						<label class="block text-xs font-semibold text-muted-foreground mb-1">Minimum commits</label>
						<input
							type="number"
							min="0"
							bind:value={policy.min_commits}
							class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
						/>
					</div>
					<div>
						<label class="block text-xs font-semibold text-muted-foreground mb-1">Minimum contributions</label>
						<input
							type="number"
							min="0"
							bind:value={policy.min_contributions}
							class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
						/>
					</div>
				</div>
			{/if}
		</section>

		<!-- Save button -->
		<div class="flex justify-end">
			<Button onclick={savePolicy} disabled={saving} class="gap-2">
				{#if saving}<Loader class="h-4 w-4 animate-spin" />{/if}
				Save changes
			</Button>
		</div>

		<!-- Blocked users -->
		<section class="rounded-lg border border-border bg-card p-5 space-y-4">
			<h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
				<UserX class="h-4 w-4 text-muted-foreground" />
				Blocked users
				{#if (policy.blocked_users?.length ?? 0) > 0}
					<Badge variant="secondary">{policy.blocked_users.length}</Badge>
				{/if}
			</h2>
			<Separator />

			<form onsubmit={blockUser} class="flex gap-2 flex-wrap">
				<input
					type="text"
					bind:value={blockUsername}
					placeholder="Username to block"
					class="h-8 flex-1 min-w-[160px] rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
				/>
				<input
					type="text"
					bind:value={blockReason}
					placeholder="Reason (optional)"
					class="h-8 flex-1 min-w-[160px] rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
				/>
				<Button type="submit" size="sm" disabled={blocking || !blockUsername.trim()} class="gap-1.5 shrink-0">
					{#if blocking}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Plus class="h-3.5 w-3.5" />{/if}
					Block
				</Button>
			</form>
			{#if blockError}
				<p class="text-xs text-red-400">{blockError}</p>
			{/if}

			{#if (policy.blocked_users?.length ?? 0) === 0}
				<p class="text-sm text-muted-foreground text-center py-4">No blocked users.</p>
			{:else}
				<ul class="space-y-2">
					{#each policy.blocked_users as bu (bu.id)}
						<li class="flex items-center justify-between rounded-md border border-border px-3 py-2">
							<div class="flex items-center gap-2 min-w-0">
								<div class="h-7 w-7 rounded-full bg-secondary border border-border flex items-center justify-center shrink-0 overflow-hidden text-xs font-bold text-primary">
									{#if bu.user?.avatar_url}
										<img src={mediaUrl(bu.user.avatar_url)} alt={bu.user.username} class="h-full w-full object-cover" />
									{:else}
										{bu.user?.username?.[0]?.toUpperCase() ?? '?'}
									{/if}
								</div>
								<div class="min-w-0">
									<p class="text-sm font-medium text-foreground truncate">{bu.user?.username ?? `#${bu.user_id}`}</p>
									{#if bu.reason}
										<p class="text-xs text-muted-foreground truncate">{bu.reason}</p>
									{/if}
								</div>
							</div>
							<Button variant="ghost" size="icon" onclick={() => unblockUser(bu.user_id)} class="h-7 w-7 text-muted-foreground hover:text-red-400 shrink-0">
								<Trash2 class="h-3.5 w-3.5" />
							</Button>
						</li>
					{/each}
				</ul>
			{/if}
		</section>

		<!-- Blocked keywords -->
		<section class="rounded-lg border border-border bg-card p-5 space-y-4">
			<h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
				<Hash class="h-4 w-4 text-muted-foreground" />
				Blocked keywords
				{#if (policy.blocked_keywords?.length ?? 0) > 0}
					<Badge variant="secondary">{policy.blocked_keywords.length}</Badge>
				{/if}
			</h2>
			<Separator />
			<p class="text-xs text-muted-foreground">Content containing these words will be rejected.</p>

			<form onsubmit={addKeyword} class="flex gap-2 flex-wrap">
				<input
					type="text"
					bind:value={newKeyword}
					placeholder="Keyword or phrase"
					class="h-8 flex-1 min-w-[160px] rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
				/>
				<select bind:value={keywordApplyTo} class="h-8 rounded-md border border-border bg-background px-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary">
					<option value="all">All</option>
					<option value="issues">Issues</option>
					<option value="prs">Pull Requests</option>
					<option value="commits">Commits</option>
				</select>
				<Button type="submit" size="sm" disabled={addingKeyword || !newKeyword.trim()} class="gap-1.5 shrink-0">
					{#if addingKeyword}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Plus class="h-3.5 w-3.5" />{/if}
					Add
				</Button>
			</form>
			{#if keywordError}
				<p class="text-xs text-red-400">{keywordError}</p>
			{/if}

			{#if (policy.blocked_keywords?.length ?? 0) === 0}
				<p class="text-sm text-muted-foreground text-center py-4">No blocked keywords.</p>
			{:else}
				<div class="flex flex-wrap gap-2">
					{#each policy.blocked_keywords as kw (kw.id)}
						<span class="inline-flex items-center gap-1.5 rounded-full border border-border bg-secondary px-3 py-1 text-xs text-foreground">
							<Hash class="h-3 w-3 text-muted-foreground" />
							{kw.keyword}
							<Badge variant="outline" class="text-[10px] px-1 py-0">{applyToLabel(kw.apply_to)}</Badge>
							<button type="button" onclick={() => removeKeyword(kw.id)} class="ml-0.5 text-muted-foreground hover:text-red-400 transition-colors" aria-label="Remove keyword">
								<Trash2 class="h-3 w-3" />
							</button>
						</span>
					{/each}
				</div>
			{/if}
		</section>
	</div>
{/if}
