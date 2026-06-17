<script lang="ts">
	import { page } from '$app/state';
	import { onMount, getContext } from 'svelte';
	import { orgs, type Team, type TeamMember, type TeamRepo, type OrgMember, type Repository, type Organization } from '$lib/api/client';
	import { Users, Book, Shield, Trash2, Loader, Plus, X } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import ConfirmPasswordDialog from '$lib/components/ConfirmPasswordDialog.svelte';

	const handle = $derived(page.params.username as string);

	const ctx = getContext<{
		org: Organization | null;
		isOwner: boolean;
		isMember: boolean;
		loading: boolean;
	}>('org');

	let teams = $state<Team[]>([]);
	let loading = $state(true);
	let error = $state('');

	let selectedTeam = $state<Team | null>(null);
	let teamMembers = $state<TeamMember[]>([]);
	let teamRepos = $state<TeamRepo[]>([]);
	let loadingTeam = $state(false);

	let orgMembers = $state<OrgMember[]>([]);
	let orgRepos = $state<Repository[]>([]);

	let showCreateForm = $state(false);
	let newTeamName = $state('');
	let pendingDeleteTeam = $state<Team | null>(null);
	let showDeleteTeamDialog = $state(false);
	let newTeamDesc = $state('');
	let newTeamPermission = $state('read');
	let creating = $state(false);
	let createError = $state('');

	let addMemberUsername = $state('');
	let addingMember = $state(false);
	let addRepoName = $state('');
	let addingRepo = $state(false);

	let initialLoadDone = false;

	$effect(() => {
		if (!ctx.loading && ctx.isMember && !initialLoadDone) {
			initialLoadDone = true;
			load();
		}
	});

	async function load() {
		loading = true;
		error = '';
		try {
			const [teamsData, membersData, reposData] = await Promise.all([orgs.teams.list(handle), orgs.members.list(handle), orgs.repos.list(handle)]);
			teams = teamsData;
			orgMembers = membersData;
			orgRepos = reposData;
		} catch (e: any) {
			error = e.message ?? 'Failed to load';
		} finally {
			loading = false;
		}
	}

	async function selectTeam(team: Team) {
		selectedTeam = team;
		loadingTeam = true;
		try {
			const [members, repos] = await Promise.all([orgs.teams.members.list(handle, team.id), orgs.teams.repos.list(handle, team.id)]);
			teamMembers = members;
			teamRepos = repos;
		} catch {
		} finally {
			loadingTeam = false;
		}
	}

	async function createTeam(e: Event) {
		e.preventDefault();
		if (!newTeamName.trim()) return;
		creating = true;
		createError = '';
		try {
			const team = await orgs.teams.create(handle, { name: newTeamName.trim(), description: newTeamDesc.trim(), permission: newTeamPermission });
			teams = [...teams, team];
			showCreateForm = false;
			newTeamName = '';
			newTeamDesc = '';
			newTeamPermission = 'read';
		} catch (e: any) {
			createError = e.message ?? 'Failed to create team';
		} finally {
			creating = false;
		}
	}

	async function deleteTeam(team: Team) {
		pendingDeleteTeam = team;
		showDeleteTeamDialog = true;
	}

	async function confirmDeleteTeam(password: string) {
		if (!pendingDeleteTeam) return;
		const team = pendingDeleteTeam;
		try {
			await orgs.teams.delete(handle, team.id, password);
			teams = teams.filter((t) => t.id !== team.id);
			if (selectedTeam?.id === team.id) selectedTeam = null;
		} finally {
			pendingDeleteTeam = null;
		}
	}

	async function addMemberToTeam() {
		if (!selectedTeam || !addMemberUsername.trim()) return;
		addingMember = true;
		try {
			const updated = await orgs.teams.members.add(handle, selectedTeam.id, addMemberUsername.trim());
			teamMembers = updated;
			addMemberUsername = '';
			teams = teams.map((t) => (t.id === selectedTeam!.id ? { ...t, member_count: (t.member_count ?? 0) + 1 } : t));
		} catch (e: any) {
			alert(e.message);
		} finally {
			addingMember = false;
		}
	}

	async function removeMemberFromTeam(m: TeamMember) {
		if (!selectedTeam) return;
		try {
			await orgs.teams.members.remove(handle, selectedTeam.id, m.user.username);
			teamMembers = teamMembers.filter((tm) => tm.id !== m.id);
			teams = teams.map((t) => (t.id === selectedTeam!.id ? { ...t, member_count: Math.max(0, (t.member_count ?? 1) - 1) } : t));
		} catch (e: any) {
			alert(e.message);
		}
	}

	async function addRepoToTeam() {
		if (!selectedTeam || !addRepoName.trim()) return;
		addingRepo = true;
		try {
			await orgs.teams.repos.add(handle, selectedTeam.id, addRepoName.trim());
			const updated = await orgs.teams.repos.list(handle, selectedTeam.id);
			teamRepos = updated;
			addRepoName = '';
			teams = teams.map((t) => (t.id === selectedTeam!.id ? { ...t, repo_count: (t.repo_count ?? 0) + 1 } : t));
		} catch (e: any) {
			alert(e.message);
		} finally {
			addingRepo = false;
		}
	}

	async function removeRepoFromTeam(tr: TeamRepo) {
		if (!selectedTeam) return;
		try {
			await orgs.teams.repos.remove(handle, selectedTeam.id, tr.repo_id);
			teamRepos = teamRepos.filter((r) => r.id !== tr.id);
			teams = teams.map((t) => (t.id === selectedTeam!.id ? { ...t, repo_count: Math.max(0, (t.repo_count ?? 1) - 1) } : t));
		} catch (e: any) {
			alert(e.message);
		}
	}

	const permissionColor: Record<string, string> = {
		read: 'text-blue-400 border-blue-800/40',
		write: 'text-yellow-400 border-yellow-800/40',
		admin: 'text-red-400 border-red-800/40'
	};
</script>

<svelte:head>
	<title>Teams</title>
</svelte:head>

{#if loading}
	<div class="text-center py-12 text-muted-foreground">Loading…</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
{:else}
	<div>
		<div class="flex items-center justify-between mb-4">
			<h1 class="text-xl font-semibold text-foreground">Teams ({teams.length})</h1>
			{#if ctx.isOwner}
				<Button variant="brand" size="sm" onclick={() => (showCreateForm = !showCreateForm)}>
					<Plus class="h-3.5 w-3.5" />New team
				</Button>
			{/if}
		</div>

		{#if showCreateForm && ctx.isOwner}
			<div class="mb-5 rounded-md border border-border bg-card p-4">
				<h2 class="text-sm font-semibold text-foreground mb-3">New team</h2>
				{#if createError}
					<div class="mb-3 rounded border border-red-800/40 bg-red-900/20 px-3 py-2 text-xs text-red-400">{createError}</div>
				{/if}
				<form onsubmit={createTeam} class="space-y-3">
					<input
						type="text"
						bind:value={newTeamName}
						placeholder="Team name"
						required
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
					<input
						type="text"
						bind:value={newTeamDesc}
						placeholder="Description (optional)"
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
					<div class="flex items-center gap-3">
						<span class="text-xs font-semibold text-foreground">Permission:</span>
						{#each ['read', 'write', 'admin'] as p}
							<label class="flex items-center gap-1.5 cursor-pointer">
								<input type="radio" bind:group={newTeamPermission} value={p} class="accent-primary" />
								<span class="text-xs text-foreground capitalize">{p}</span>
							</label>
						{/each}
					</div>
					<div class="flex items-center gap-2">
						<Button variant="brand" size="sm" type="submit" disabled={creating}>
							{#if creating}<Loader class="h-4 w-4 animate-spin" />{/if}
							Create team
						</Button>
						<Button variant="ghost" size="sm" type="button" onclick={() => (showCreateForm = false)}>Cancel</Button>
					</div>
				</form>
			</div>
		{/if}

		<div class="grid grid-cols-1 lg:grid-cols-5 gap-4">
			<div class="lg:col-span-2">
				{#if teams.length === 0}
					<div class="rounded-md border border-border bg-card p-10 text-center">
						<Shield class="mx-auto h-7 w-7 text-muted-foreground mb-3" />
						<p class="text-sm text-muted-foreground">No teams yet.</p>
						{#if ctx.isOwner}<Button variant="outline" size="sm" class="mt-3" onclick={() => (showCreateForm = true)}>Create a team</Button>{/if}
					</div>
				{:else}
					<div class="divide-y divide-secondary rounded-md border border-border overflow-hidden">
						{#each teams as team}
							<button
								onclick={() => selectTeam(team)}
								class="w-full text-left flex items-start gap-3 px-4 py-3 bg-card hover:bg-secondary transition-colors"
								class:bg-secondary={selectedTeam?.id === team.id}
							>
								<Shield class="h-4 w-4 text-muted-foreground mt-0.5 shrink-0" />
								<div class="flex-1 min-w-0">
									<p class="text-sm font-semibold text-foreground truncate">{team.name}</p>
									{#if team.description}<p class="text-xs text-muted-foreground truncate">{team.description}</p>{/if}
									<div class="flex items-center gap-2 mt-1">
										<span class="text-xs text-muted-foreground">{team.member_count ?? 0} members · {team.repo_count ?? 0} repos</span>
										<span class="text-xs border rounded-full px-1.5 {permissionColor[team.permission] ?? 'text-muted-foreground border-border'}">{team.permission}</span>
									</div>
								</div>
							</button>
						{/each}
					</div>
				{/if}
			</div>

			{#if selectedTeam}
				<div class="lg:col-span-3">
					<div class="rounded-md border border-border bg-card">
						<div class="flex items-center justify-between px-4 py-3 border-b border-border">
							<h2 class="text-sm font-semibold text-foreground flex items-center gap-2">
								<Shield class="h-4 w-4 text-muted-foreground" />{selectedTeam.name}
							</h2>
							{#if ctx.isOwner}
								<button onclick={() => deleteTeam(selectedTeam!)} class="text-muted-foreground hover:text-red-400 transition-colors" title="Delete team">
									<Trash2 class="h-4 w-4" />
								</button>
							{/if}
						</div>

						{#if loadingTeam}
							<div class="p-8 text-center text-muted-foreground text-sm">Loading…</div>
						{:else}
							<div class="p-4 border-b border-border">
								<h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide flex items-center gap-1 mb-3">
									<Users class="h-3.5 w-3.5" /> Members ({teamMembers.length})
								</h3>
								{#if ctx.isOwner}
									<div class="flex gap-2 mb-3">
										<select bind:value={addMemberUsername} class="h-8 flex-1 rounded border border-border bg-background px-2 text-xs text-foreground focus:outline-none">
											<option value="">Select member to add…</option>
											{#each orgMembers.filter((m) => !teamMembers.some((tm) => tm.user_id === m.user_id)) as m}
												<option value={m.user.username}>{m.user.username}</option>
											{/each}
										</select>
										<Button variant="outline" size="sm" onclick={addMemberToTeam} disabled={addingMember || !addMemberUsername}>
											{#if addingMember}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Plus class="h-3.5 w-3.5" />{/if}
										</Button>
									</div>
								{/if}
								{#if teamMembers.length === 0}
									<p class="text-xs text-muted-foreground">No members in this team.</p>
								{:else}
									<div class="space-y-1.5">
										{#each teamMembers as m}
											<div class="flex items-center gap-2">
												<div class="h-6 w-6 rounded-full bg-secondary border border-border overflow-hidden flex items-center justify-center shrink-0">
													{#if m.user.avatar_url}<img src={m.user.avatar_url} alt={m.user.username} class="h-full w-full object-cover" />{:else}<span
															class="text-[10px] font-bold text-primary">{m.user.username[0].toUpperCase()}</span
														>{/if}
												</div>
												<a href="/{m.user.username}" class="text-xs text-foreground hover:text-primary flex-1 truncate">{m.user.username}</a>
												{#if ctx.isOwner}
													<button onclick={() => removeMemberFromTeam(m)} class="text-muted-foreground hover:text-red-400 transition-colors"><X class="h-3.5 w-3.5" /></button
													>
												{/if}
											</div>
										{/each}
									</div>
								{/if}
							</div>

							<div class="p-4">
								<h3 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide flex items-center gap-1 mb-3">
									<Book class="h-3.5 w-3.5" /> Repositories ({teamRepos.length})
								</h3>
								{#if ctx.isOwner}
									<div class="flex gap-2 mb-3">
										<select bind:value={addRepoName} class="h-8 flex-1 rounded border border-border bg-background px-2 text-xs text-foreground focus:outline-none">
											<option value="">Select repository to add…</option>
											{#each orgRepos.filter((r) => !teamRepos.some((tr) => tr.repo_id === r.id)) as r}
												<option value={r.name}>{r.name}</option>
											{/each}
										</select>
										<Button variant="outline" size="sm" onclick={addRepoToTeam} disabled={addingRepo || !addRepoName}>
											{#if addingRepo}<Loader class="h-3.5 w-3.5 animate-spin" />{:else}<Plus class="h-3.5 w-3.5" />{/if}
										</Button>
									</div>
								{/if}
								{#if teamRepos.length === 0}
									<p class="text-xs text-muted-foreground">No repositories in this team.</p>
								{:else}
									<div class="space-y-1.5">
										{#each teamRepos as tr}
											<div class="flex items-center gap-2">
												<Book class="h-3.5 w-3.5 text-muted-foreground shrink-0" />
												<a href="/{handle}/{tr.repo.name}" class="text-xs text-primary hover:underline flex-1 truncate">{tr.repo.name}</a>
												{#if ctx.isOwner}
													<button onclick={() => removeRepoFromTeam(tr)} class="text-muted-foreground hover:text-red-400 transition-colors"><X class="h-3.5 w-3.5" /></button>
												{/if}
											</div>
										{/each}
									</div>
								{/if}
							</div>
						{/if}
					</div>
				</div>
			{/if}
		</div>
	</div>
{/if}
<ConfirmPasswordDialog
	bind:open={showDeleteTeamDialog}
	title="Delete team"
	description={pendingDeleteTeam ? `This will permanently delete the team "${pendingDeleteTeam.name}". Enter your password to confirm.` : 'Enter your password to confirm.'}
	confirmLabel="Delete team"
	onconfirm={confirmDeleteTeam}
/>
