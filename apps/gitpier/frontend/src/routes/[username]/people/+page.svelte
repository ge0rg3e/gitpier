<script lang="ts">
	import { page } from '$app/state';
	import { onMount, getContext } from 'svelte';
	import { authStore } from '$lib/stores/auth.svelte';
	import { orgs, type OrgMember } from '$lib/api/client';
	import { Users, UserPlus, Crown, Trash2, Loader } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { mediaUrl } from '$lib/utils';
	const handle = $derived(page.params.username as string);
	const ctx = getContext<{ isOwner: boolean }>('org');
	let members = $state<OrgMember[]>([]);
	let loading = $state(true);
	let error = $state('');

	let inviteUsername = $state('');
	let inviteRole = $state('member');
	let inviting = $state(false);
	let inviteError = $state('');
	let inviteSuccess = $state('');

	onMount(async () => {
		loading = true;
		error = '';
		try {
			members = await orgs.members.list(handle);
		} catch (e: any) {
			error = e.message ?? 'Failed to load';
		} finally {
			loading = false;
		}
	});

	async function invite(e: Event) {
		e.preventDefault();
		if (!inviteUsername.trim()) return;
		inviting = true;
		inviteError = '';
		inviteSuccess = '';
		try {
			const m = await orgs.members.add(handle, inviteUsername.trim(), inviteRole);
			members = [...members, m];
			inviteSuccess = `${inviteUsername} added as ${inviteRole}.`;
			inviteUsername = '';
		} catch (e: any) {
			inviteError = e.message ?? 'Failed to add member';
		} finally {
			inviting = false;
		}
	}

	async function updateRole(member: OrgMember, role: string) {
		if (member.role === role) return;
		try {
			const updated = await orgs.members.updateRole(handle, member.user.username, role);
			members = members.map((m) => (m.id === member.id ? updated : m));
		} catch (e: any) {
			alert(e.message);
		}
	}

	async function removeMember(member: OrgMember) {
		if (!confirm(`Remove @${member.user.username} from ${handle}?`)) return;
		try {
			await orgs.members.remove(handle, member.user.username);
			members = members.filter((m) => m.id !== member.id);
		} catch (e: any) {
			alert(e.message);
		}
	}

	const owners = $derived(members.filter((m) => m.role === 'owner'));
	const regularMembers = $derived(members.filter((m) => m.role === 'member'));
</script>

<svelte:head>
	<title>People</title>
</svelte:head>

{#if loading}
	<div>
		<div class="flex items-center justify-between mb-4">
			<h1 class="text-xl font-semibold text-foreground flex items-center gap-2">
				<Users class="h-5 w-5 text-muted-foreground" />
				Members
			</h1>
		</div>

		{#if ctx.isOwner}
			<div class="mb-6 rounded-md border border-border bg-card p-4">
				<h2 class="text-sm font-semibold text-foreground mb-3 flex items-center gap-2"><UserPlus class="h-4 w-4" />Invite a member</h2>
				<form onsubmit={invite} class="flex gap-2">
					<input
						type="text"
						bind:value={inviteUsername}
						placeholder="Username"
						required
						class="h-9 flex-1 rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
					<select bind:value={inviteRole} class="h-9 rounded-md border border-border bg-background px-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary">
						<option value="member">Member</option>
						<option value="owner">Owner</option>
					</select>
					<Button variant="brand" size="sm" type="submit" disabled={inviting}>
						Add
					</Button>
				</form>
			</div>
		{/if}

		<div class="mb-2 text-xs font-semibold text-muted-foreground uppercase tracking-wide flex items-center gap-1.5">
			<Crown class="h-3.5 w-3.5 text-yellow-500" /> Owners
		</div>

		<div class="rounded-md border border-border overflow-hidden">
			<div class="flex items-center gap-3 px-4 py-3 bg-card border-b border-secondary">
				<div class="h-9 w-9 rounded-full border border-border bg-secondary/40"></div>
				<div class="flex-1">
					<div class="mb-1 h-4 w-40 rounded border border-border bg-secondary/40"></div>
					<div class="h-3 w-28 rounded border border-border bg-secondary/40"></div>
				</div>
				<div class="h-7 w-24 rounded border border-border bg-secondary/40"></div>
			</div>
			<div class="flex items-center gap-3 px-4 py-3 bg-card border-b border-secondary">
				<div class="h-9 w-9 rounded-full border border-border bg-secondary/40"></div>
				<div class="flex-1">
					<div class="mb-1 h-4 w-36 rounded border border-border bg-secondary/40"></div>
					<div class="h-3 w-24 rounded border border-border bg-secondary/40"></div>
				</div>
				<div class="h-7 w-24 rounded border border-border bg-secondary/40"></div>
			</div>
		</div>
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
{:else}
	<div>
		<div class="flex items-center justify-between mb-4">
			<h1 class="text-xl font-semibold text-foreground flex items-center gap-2">
				<Users class="h-5 w-5 text-muted-foreground" />
				Members <span class="text-muted-foreground text-base font-normal">({members.length})</span>
			</h1>
		</div>

		{#if ctx.isOwner}
			<div class="mb-6 rounded-md border border-border bg-card p-4">
				<h2 class="text-sm font-semibold text-foreground mb-3 flex items-center gap-2"><UserPlus class="h-4 w-4" />Invite a member</h2>
				{#if inviteError}
					<div class="mb-3 rounded border border-red-800/40 bg-red-900/20 px-3 py-2 text-xs text-red-400">{inviteError}</div>
				{/if}
				{#if inviteSuccess}
					<div class="mb-3 rounded border border-brand/40 bg-brand/10 px-3 py-2 text-xs text-[#3fb950]">{inviteSuccess}</div>
				{/if}
				<form onsubmit={invite} class="flex gap-2">
					<input
						type="text"
						bind:value={inviteUsername}
						placeholder="Username"
						required
						class="h-9 flex-1 rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
					<select bind:value={inviteRole} class="h-9 rounded-md border border-border bg-background px-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary">
						<option value="member">Member</option>
						<option value="owner">Owner</option>
					</select>
					<Button variant="brand" size="sm" type="submit" disabled={inviting}>
						{#if inviting}<Loader class="h-4 w-4 animate-spin" />{/if}
						Add
					</Button>
				</form>
			</div>
		{/if}

		{#if owners.length > 0}
			<div class="mb-5">
				<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2 flex items-center gap-1.5">
					<Crown class="h-3.5 w-3.5 text-yellow-500" /> Owners ({owners.length})
				</h2>
				<div class="divide-y divide-secondary rounded-md border border-border overflow-hidden">
					{#each owners as m}
						<div class="flex items-center gap-3 px-4 py-3 bg-card">
							<a href="/{m.user.username}">
								<div class="h-9 w-9 rounded-full border border-border bg-secondary overflow-hidden flex items-center justify-center shrink-0">
									{#if m.user.avatar_url}<img src={mediaUrl(m.user.avatar_url)} alt={m.user.username} class="h-full w-full object-cover" />
									{:else}<span class="text-xs font-bold text-primary">{m.user.username[0].toUpperCase()}</span>{/if}
								</div>
							</a>
							<div class="flex-1 min-w-0">
								<a href="/{m.user.username}" class="text-sm font-semibold text-foreground hover:text-primary truncate block">{m.user.display_name || m.user.username}</a>
								<p class="text-xs text-muted-foreground truncate">@{m.user.username}</p>
							</div>
							{#if ctx.isOwner && m.user.id !== authStore.user?.id}
								<div class="flex items-center gap-2">
									<select
										value={m.role}
										onchange={(e) => updateRole(m, (e.target as HTMLSelectElement).value)}
										class="h-7 rounded border border-border bg-background px-2 text-xs text-foreground focus:outline-none"
									>
										<option value="member">Member</option>
										<option value="owner">Owner</option>
									</select>
									<button
										onclick={() => removeMember(m)}
										class="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-red-400 hover:bg-red-900/20 transition-colors"
										title="Remove"
									>
										<Trash2 class="h-3.5 w-3.5" />
									</button>
								</div>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{/if}

		{#if regularMembers.length > 0}
			<div>
				<h2 class="text-xs font-semibold text-muted-foreground uppercase tracking-wide mb-2">Members ({regularMembers.length})</h2>
				<div class="divide-y divide-secondary rounded-md border border-border overflow-hidden">
					{#each regularMembers as m}
						<div class="flex items-center gap-3 px-4 py-3 bg-card">
							<a href="/{m.user.username}">
								<div class="h-9 w-9 rounded-full border border-border bg-secondary overflow-hidden flex items-center justify-center shrink-0">
									{#if m.user.avatar_url}<img src={mediaUrl(m.user.avatar_url)} alt={m.user.username} class="h-full w-full object-cover" />
									{:else}<span class="text-xs font-bold text-primary">{m.user.username[0].toUpperCase()}</span>{/if}
								</div>
							</a>
							<div class="flex-1 min-w-0">
								<a href="/{m.user.username}" class="text-sm font-semibold text-foreground hover:text-primary truncate block">{m.user.display_name || m.user.username}</a>
								<p class="text-xs text-muted-foreground truncate">@{m.user.username}</p>
							</div>
							{#if ctx.isOwner}
								<div class="flex items-center gap-2">
									<select
										value={m.role}
										onchange={(e) => updateRole(m, (e.target as HTMLSelectElement).value)}
										class="h-7 rounded border border-border bg-background px-2 text-xs text-foreground focus:outline-none"
									>
										<option value="member">Member</option>
										<option value="owner">Owner</option>
									</select>
									<button
										onclick={() => removeMember(m)}
										class="h-7 w-7 flex items-center justify-center rounded text-muted-foreground hover:text-red-400 hover:bg-red-900/20 transition-colors"
										title="Remove"
									>
										<Trash2 class="h-3.5 w-3.5" />
									</button>
								</div>
							{:else if m.user.id === authStore.user?.id}
								<button onclick={() => removeMember(m)} class="text-xs text-red-400 hover:underline">Leave</button>
							{/if}
						</div>
					{/each}
				</div>
			</div>
		{:else if owners.length === 0}
			<div class="rounded-md border border-border bg-card p-12 text-center">
				<Users class="mx-auto h-8 w-8 text-muted-foreground mb-3" />
				<p class="text-muted-foreground">No members yet.</p>
			</div>
		{/if}
	</div>
{/if}
