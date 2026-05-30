<script lang="ts">
	import { browser } from '$app/environment';
	import { page } from '$app/state';
	import { setContext } from 'svelte';
	import { users, orgs, moderation, type User, type Organization, type Repository, type ModerationBlockedUser, type FollowListItem } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { addOrUpdateBrowserTab, getBrowserTabByUrl, type BrowserTabKind } from '$lib/stores/browser-tabs';
	import { mediaUrl } from '$lib/utils';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';

	let { children } = $props();

	const handle = $derived(page.params.username as string);

	let loading = $state(true);
	let profile = $state<User | null>(null);
	let followerCount = $state(0);
	let followingCount = $state(0);
	let isFollowingUser = $state(false);
	let followsYou = $state(false);
	let blockedEntry = $state<ModerationBlockedUser | null>(null);
	let blockLoading = $state(false);
	let blockError = $state('');
	let userFollowLoading = $state(false);

	// Org
	let isOrg = $state(false);
	let org = $state<Organization | null>(null);
	let isOwner = $state(false);
	let isMember = $state(false);
	let memberCount = $state(0);
	let repoCount = $state(0);

	// Follow dialog
	let showFollowDialog = $state(false);
	let followDialogMode = $state<'followers' | 'following'>('followers');
	let followDialogUsers = $state<FollowListItem[]>([]);
	let loadingFollowDialog = $state(false);
	let followDialogError = $state('');
	let followActionBusy = $state<Record<string, boolean>>({});

	const isOwn = $derived(authStore.user?.username === handle);
	// Org context (used by OrgTopNav and org child pages)
	const orgCtx = $state({
		isOrg: false,
		org: null as Organization | null,
		isOwner: false,
		isMember: false,
		memberCount: 0,
		repoCount: 0,
		followerCount: 0,
		isFollowing: false,
		loading: true
	});
	setContext('org', orgCtx);

	// User profile context (shared with child tab pages)
	const userProfileCtx = $state({
		profile: null as User | null,
		repos: [] as Repository[],
		followerCount: 0,
		followingCount: 0,
		isFollowing: false,
		followsYou: false,
		openFollowersDialog: () => Promise.resolve(),
		openFollowingDialog: () => Promise.resolve(),
		loading: true
	});
	setContext('userProfile', userProfileCtx);

	$effect(() => {
		const currentHandle = handle;

		// Reset all
		loading = true;
		isOrg = false;
		org = null;
		profile = null;
		blockedEntry = null;
		followerCount = 0;
		followingCount = 0;
		isFollowingUser = false;
		followsYou = false;

		orgCtx.isOrg = false;
		orgCtx.org = null;
		orgCtx.isOwner = false;
		orgCtx.isMember = false;
		orgCtx.memberCount = 0;
		orgCtx.repoCount = 0;
		orgCtx.loading = true;

		userProfileCtx.profile = null;
		userProfileCtx.repos = [];
		userProfileCtx.followerCount = 0;
		userProfileCtx.followingCount = 0;
		userProfileCtx.isFollowing = false;
		userProfileCtx.followsYou = false;
		userProfileCtx.loading = true;

		users
			.getProfile(currentHandle)
			.then((data) => {
				if (handle !== currentHandle) return;
				profile = data.user;
				followerCount = data.follower_count ?? 0;
				followingCount = data.following_count ?? 0;
				isFollowingUser = data.is_following ?? false;
				followsYou = data.follows_you ?? false;

				userProfileCtx.profile = data.user;
				userProfileCtx.repos = data.repos ?? [];
				userProfileCtx.followerCount = data.follower_count ?? 0;
				userProfileCtx.followingCount = data.following_count ?? 0;
				userProfileCtx.isFollowing = data.is_following ?? false;
				userProfileCtx.followsYou = data.follows_you ?? false;
				userProfileCtx.loading = false;
				orgCtx.loading = false;
				loading = false;
			})
			.catch(() => {
				orgs.get(currentHandle)
					.then((data) => {
						if (handle !== currentHandle) return;
						isOrg = true;
						org = data.org;
						isOwner = data.is_owner;
						isMember = data.is_member;
						memberCount = data.member_count;
						repoCount = data.repo_count;

						orgCtx.isOrg = true;
						orgCtx.org = data.org;
						orgCtx.isOwner = data.is_owner;
						orgCtx.isMember = data.is_member;
						orgCtx.memberCount = data.member_count;
						orgCtx.repoCount = data.repo_count;
						orgCtx.followerCount = data.follower_count ?? 0;
						orgCtx.isFollowing = data.is_following ?? false;
					})
					.catch(() => {})
					.finally(() => {
						if (handle === currentHandle) {
							loading = false;
							orgCtx.loading = false;
							userProfileCtx.loading = false;
						}
					});
			});
	});

	// Block detection
	$effect(() => {
		if (!authStore.isAuthenticated || isOwn || !profile) return;
		blockedEntry = null;
		moderation
			.user()
			.getPolicy()
			.then((res) => {
				blockedEntry = res.policy.blocked_users?.find((u: ModerationBlockedUser) => u.user?.username === profile!.username) ?? null;
			})
			.catch(() => {});
	});

	async function toggleUserFollow() {
		if (!profile || !authStore.isAuthenticated) return;
		userFollowLoading = true;
		try {
			if (isFollowingUser) {
				await users.unfollow(profile.username);
				isFollowingUser = false;
				followerCount = Math.max(0, followerCount - 1);
				userProfileCtx.isFollowing = false;
				userProfileCtx.followerCount = Math.max(0, userProfileCtx.followerCount - 1);
			} else {
				await users.follow(profile.username);
				isFollowingUser = true;
				followerCount += 1;
				userProfileCtx.isFollowing = true;
				userProfileCtx.followerCount += 1;
			}
		} finally {
			userFollowLoading = false;
		}
	}

	async function toggleBlock() {
		if (!profile) return;
		blockLoading = true;
		blockError = '';
		try {
			if (blockedEntry) {
				await moderation.user().unblockUser(blockedEntry.user_id);
				blockedEntry = null;
			} else {
				const res = await moderation.user().blockUser(profile.username);
				blockedEntry = res.blocked_user;
			}
		} catch (e: any) {
			blockError = e.message ?? 'Failed';
		} finally {
			blockLoading = false;
		}
	}

	async function openFollowersDialog() {
		if (!profile) return;
		showFollowDialog = true;
		followDialogMode = 'followers';
		followDialogError = '';
		loadingFollowDialog = true;
		try {
			const data = await users.listFollowers(profile.username);
			followDialogUsers = data.users ?? [];
		} catch (e: any) {
			followDialogUsers = [];
			followDialogError = e.message ?? 'Failed to load followers';
		} finally {
			loadingFollowDialog = false;
		}
	}

	async function openFollowingDialog() {
		if (!profile) return;
		showFollowDialog = true;
		followDialogMode = 'following';
		followDialogError = '';
		loadingFollowDialog = true;
		try {
			const data = await users.listFollowing(profile.username);
			followDialogUsers = data.items ?? data.users ?? [];
		} catch (e: any) {
			followDialogUsers = [];
			followDialogError = e.message ?? 'Failed to load following';
		} finally {
			loadingFollowDialog = false;
		}
	}

	userProfileCtx.openFollowersDialog = openFollowersDialog;
	userProfileCtx.openFollowingDialog = openFollowingDialog;

	$effect(() => {
		if (!browser || orgCtx.loading) return;
		const url = `${page.url.pathname}${page.url.search || ''}`;
		const existing = getBrowserTabByUrl(url);
		if (!existing) return;
		const expectedKind: BrowserTabKind = orgCtx.isOrg ? 'org' : 'profile';
		if (existing.kind === expectedKind) return;
		addOrUpdateBrowserTab({
			...existing,
			kind: expectedKind
		});
	});

	async function toggleFollowFromDialog(item: FollowListItem) {
		if (!authStore.isAuthenticated) return;
		if (item.entity_type === 'org' && item.org) {
			const orgLogin = item.org.login;
			if (!orgLogin) return;
			followActionBusy = { ...followActionBusy, [orgLogin]: true };
			try {
				if (item.is_following) await orgs.unfollow(orgLogin);
				else await orgs.follow(orgLogin);
				followDialogUsers = followDialogUsers.map((u) => (u.org?.login === orgLogin ? { ...u, is_following: !u.is_following } : u));
			} finally {
				followActionBusy = { ...followActionBusy, [orgLogin]: false };
			}
			return;
		}
		const uname = item.user?.username;
		if (!uname || uname === authStore.user?.username) return;
		followActionBusy = { ...followActionBusy, [uname]: true };
		try {
			if (item.is_following) await users.unfollow(uname);
			else await users.follow(uname);
			followDialogUsers = followDialogUsers.map((u) => (u.user?.username === uname ? { ...u, is_following: !u.is_following } : u));
			if (profile && profile.username === uname) {
				isFollowingUser = !item.is_following;
				followerCount = Math.max(0, followerCount + (item.is_following ? -1 : 1));
				userProfileCtx.isFollowing = !item.is_following;
				userProfileCtx.followerCount = Math.max(0, userProfileCtx.followerCount + (item.is_following ? -1 : 1));
			}
		} finally {
			followActionBusy = { ...followActionBusy, [uname]: false };
		}
	}
</script>

<!-- User profile layout: single-page profile content -->
{#if profile && !page.params.repo}
	<div class="bg-background">
		{@render children()}
	</div>
{:else}
	{@render children()}
{/if}

<!-- Follow / Following dialog -->
<Dialog.Root bind:open={showFollowDialog}>
	<Dialog.Content class="max-w-xl">
		<Dialog.Header>
			<Dialog.Title>{followDialogMode === 'followers' ? 'Followers' : 'Following'}</Dialog.Title>
			<Dialog.Description>People in this list.</Dialog.Description>
		</Dialog.Header>
		{#if loadingFollowDialog}
			<div class="space-y-2 py-2">
				{#each Array(4) as _}
					<div class="h-11 rounded-md bg-secondary animate-pulse"></div>
				{/each}
			</div>
		{:else if followDialogError}
			<p class="py-4 text-sm text-red-400">{followDialogError}</p>
		{:else if followDialogUsers.length === 0}
			<p class="py-4 text-sm text-muted-foreground">No users yet.</p>
		{:else}
			<div class="max-h-[26rem] overflow-y-auto space-y-1">
				{#each followDialogUsers as item}
					{@const isOrgItem = item.entity_type === 'org' && !!item.org}
					{@const itemHandle = isOrgItem ? item.org!.login : item.user?.username}
					{@const avatarUrl = isOrgItem ? item.org!.avatar_url : item.user?.avatar_url}
					{@const displayName = isOrgItem ? item.org!.display_name || item.org!.login : item.user?.display_name || item.user?.username}
					<div class="flex items-center gap-3 rounded-md border border-secondary bg-card px-3 py-2">
						<a href="/{itemHandle}" class="h-9 w-9 rounded-full border border-border bg-secondary flex items-center justify-center overflow-hidden shrink-0">
							{#if avatarUrl}
								<img src={mediaUrl(avatarUrl)} alt={displayName} class="h-full w-full object-cover" />
							{:else}
								<span class="text-xs font-semibold text-primary">{(itemHandle ?? '?')[0].toUpperCase()}</span>
							{/if}
						</a>
						<div class="min-w-0 flex-1">
							<a href="/{itemHandle}" class="text-sm font-medium text-foreground hover:underline truncate block">{itemHandle}</a>
							{#if displayName && displayName !== itemHandle}
								<p class="text-xs text-muted-foreground truncate">{displayName}</p>
							{/if}
							{#if !isOrgItem && item.follows_you && item.user?.username !== authStore.user?.username}
								<p class="text-[11px] text-muted-foreground">Follows you</p>
							{/if}
						</div>
						{#if authStore.isAuthenticated && itemHandle && (isOrgItem || item.user?.username !== authStore.user?.username)}
							<Button size="sm" variant={item.is_following ? 'outline' : 'brand'} disabled={followActionBusy[itemHandle]} onclick={() => toggleFollowFromDialog(item)}>
								{#if followActionBusy[itemHandle]}…
								{:else if item.is_following}Following
								{:else if !isOrgItem && item.follows_you}Follow back
								{:else}Follow{/if}
							</Button>
						{/if}
					</div>
				{/each}
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>

