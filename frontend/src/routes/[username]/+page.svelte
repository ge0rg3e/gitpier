<script lang="ts">
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { users, orgs, packages, starred, projects, type Star, type Repository, type Organization, type OrgMember, type ContainerRepository, type Project } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { timeAgo, mediaUrl } from '$lib/utils';
	import { Lock, Star as StarIcon, MapPin, Link, Users, Plus, UserPlus, Package, Settings, Building2, Globe2, ArrowRight } from '@lucide/svelte';
	import linkedinSvg from 'bootstrap-icons/icons/linkedin.svg?raw';
	import youtubeSvg from 'bootstrap-icons/icons/youtube.svg?raw';
	import twitterXSvg from 'bootstrap-icons/icons/twitter-x.svg?raw';
	import instagramSvg from 'bootstrap-icons/icons/instagram.svg?raw';
	import facebookSvg from 'bootstrap-icons/icons/facebook.svg?raw';
	import tiktokSvg from 'bootstrap-icons/icons/tiktok.svg?raw';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Skeleton } from '$lib/components/ui/skeleton/index.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import SearchSelect from '$lib/components/SearchSelect.svelte';
	import ContributionGraph from '$lib/components/ContributionGraph.svelte';

	const userProfileCtx = getContext<{
		profile: any;
		repos: Repository[];
		followerCount: number;
		followingCount: number;
		isFollowing: boolean;
		followsYou: boolean;
		openFollowersDialog: () => Promise<void>;
		openFollowingDialog: () => Promise<void>;
		loading: boolean;
	}>('userProfile');
	const orgCtx = getContext<{
		isOrg: boolean;
		org: Organization | null;
		isOwner: boolean;
		isMember: boolean;
		memberCount: number;
		repoCount: number;
		followerCount: number;
		isFollowing: boolean;
		loading: boolean;
	}>('org');

	const username = $derived(page.params.username);
	const isOwn = $derived(authStore.user?.username === username);
	const registryHost = $derived(typeof window !== 'undefined' ? window.location.host : 'localhost:8443');

	let contributions = $state<Record<string, number>>({});
	let loadingContribs = $state(true);

	let orgMembers = $state<OrgMember[]>([]);
	let orgRepos = $state<Repository[]>([]);
	let orgContainerRepos = $state<ContainerRepository[]>([]);
	let loadingOrgPackages = $state(true);
	let orgFollowLoading = $state(false);
	let orgRepoFilter = $state('');
	let orgRepoSort = $state<'updated' | 'name' | 'stars'>('updated');
	let userRepoFilter = $state('');
	let userRepoSort = $state<'updated' | 'name' | 'stars'>('updated');
	let loadingProjects = $state(true);
	let projectsError = $state('');
	let projectList = $state<Project[]>([]);
	let showCreateProjectDialog = $state(false);
	let creatingProject = $state(false);
	let createProjectError = $state('');
	let newProjectTitle = $state('');
	let newProjectDescription = $state('');
	let newProjectIsPublic = $state(true);

	// Org followers dialog
	let showOrgFollowersDialog = $state(false);
	let orgFollowerDialogUsers = $state<any[]>([]);
	let loadingOrgFollowDialog = $state(false);

	let profileFollowLoading = $state(false);
	let starredRepos = $state<Star[]>([]);
	let loadingStarred = $state(true);
	let userContainerRepos = $state<ContainerRepository[]>([]);
	let loadingUserPackages = $state(true);
	const canCreateProject = $derived(orgCtx.isOrg ? orgCtx.isOwner : isOwn);

	$effect(() => {
		if (!userProfileCtx.profile) return;
		const u = username;
		starredRepos = [];
		userContainerRepos = [];
		contributions = {};
		loadingContribs = true;
		loadingStarred = true;
		loadingUserPackages = true;

		users
			.getContributions(u!)
			.then((d) => {
				if (username !== u) return;
				contributions = d.contributions ?? {};
				loadingContribs = false;
			})
			.catch(() => {
				loadingContribs = false;
			});
		starred
			.listForUser(u!)
			.then((d) => {
				if (username !== u) return;
				starredRepos = d.stars ?? [];
			})
			.catch(() => {})
			.finally(() => {
				if (username === u) loadingStarred = false;
			});
		packages
			.list(u!)
			.then((pkgs) => {
				if (username !== u) return;
				userContainerRepos = pkgs ?? [];
			})
			.catch(() => {})
			.finally(() => {
				if (username === u) loadingUserPackages = false;
			});
	});

	$effect(() => {
		if (!orgCtx.isOrg || orgCtx.loading) return;
		const u = username;
		orgMembers = [];
		orgRepos = [];
		orgContainerRepos = [];
		loadingOrgPackages = true;

		Promise.all([orgs.members.list(u!).catch(() => []), orgs.repos.list(u!).catch(() => []), packages.list(u!).catch(() => [] as ContainerRepository[])])
			.then(([members, repos, pkgs]) => {
				if (username !== u) return;
				orgMembers = members;
				orgRepos = repos;
				orgContainerRepos = pkgs ?? [];
			})
			.finally(() => {
				if (username === u) loadingOrgPackages = false;
			});
	});

	$effect(() => {
		const u = username;
		if (!u) return;
		if (orgCtx.loading || userProfileCtx.loading) return;
		loadingProjects = true;
		projectsError = '';
		projectList = [];

		const req = orgCtx.isOrg ? projects.listForOrg(u) : projects.listForUser(u);
		req
			.then((data) => {
				if (username !== u) return;
				projectList = data.projects ?? [];
			})
			.catch((e: any) => {
				if (username !== u) return;
				projectsError = e?.message ?? 'Failed to load projects';
			})
			.finally(() => {
				if (username === u) loadingProjects = false;
			});
	});

	function resetCreateProjectForm() {
		newProjectTitle = '';
		newProjectDescription = '';
		newProjectIsPublic = true;
		createProjectError = '';
	}

	async function createProject() {
		if (!canCreateProject || !newProjectTitle.trim()) return;
		creatingProject = true;
		createProjectError = '';
		try {
			const payload = {
				title: newProjectTitle.trim(),
				description: newProjectDescription.trim(),
				is_public: newProjectIsPublic
			};
			const result = orgCtx.isOrg ? await projects.createForOrg(username!, payload) : await projects.createForUser(payload);
			projectList = [result.project, ...projectList];
			showCreateProjectDialog = false;
			resetCreateProjectForm();
		} catch (e: any) {
			createProjectError = e?.message ?? 'Failed to create project';
		} finally {
			creatingProject = false;
		}
	}

	async function toggleProfileFollow() {
		if (!userProfileCtx.profile || !username || !authStore.isAuthenticated || isOwn) return;
		profileFollowLoading = true;
		try {
			if (userProfileCtx.isFollowing) {
				await users.unfollow(username);
				userProfileCtx.isFollowing = false;
				userProfileCtx.followerCount = Math.max(0, userProfileCtx.followerCount - 1);
			} else {
				await users.follow(username);
				userProfileCtx.isFollowing = true;
				userProfileCtx.followerCount += 1;
			}
		} finally {
			profileFollowLoading = false;
		}
	}

	async function toggleOrgFollow() {
		if (!orgCtx.org || !authStore.isAuthenticated) return;
		orgFollowLoading = true;
		try {
			if (orgCtx.isFollowing) {
				await orgs.unfollow(orgCtx.org.login);
				orgCtx.isFollowing = false;
				orgCtx.followerCount = Math.max(0, orgCtx.followerCount - 1);
			} else {
				await orgs.follow(orgCtx.org.login);
				orgCtx.isFollowing = true;
				orgCtx.followerCount += 1;
			}
		} finally {
			orgFollowLoading = false;
		}
	}

	async function openOrgFollowersDialog() {
		if (!orgCtx.org) return;
		showOrgFollowersDialog = true;
		loadingOrgFollowDialog = true;
		try {
			const data = await orgs.listFollowers(orgCtx.org.login);
			orgFollowerDialogUsers = data.users ?? [];
		} catch {
			orgFollowerDialogUsers = [];
		} finally {
			loadingOrgFollowDialog = false;
		}
	}

	function getRepoOwner(r: Repository, fallback: string) {
		return r.owner?.username ?? fallback;
	}

	const LANG_COLORS: Record<string, string> = {
		Go: '#00ADD8',
		JavaScript: '#f1e05a',
		TypeScript: '#3178c6',
		Python: '#3572A5',
		Ruby: '#701516',
		Rust: '#dea584',
		Java: '#b07219',
		Kotlin: '#A97BFF',
		Swift: '#F05138',
		'C#': '#178600',
		'C++': '#f34b7d',
		C: '#555555',
		PHP: '#4F5D95',
		Shell: '#89e051',
		HTML: '#e34c26',
		CSS: '#563d7c',
		Dart: '#00B4AB',
		Scala: '#c22d40',
		Haskell: '#5e5086',
		Elixir: '#6e4a7e',
		Clojure: '#db5855',
		Lua: '#000080',
		R: '#198ce7',
		MATLAB: '#e16737',
		'Objective-C': '#438eff',
		Perl: '#0298c3',
		Groovy: '#4298b8',
		HCL: '#844FBA',
		Nix: '#7e7eff',
		OCaml: '#3be133',
		Fortran: '#4d41b1',
		Julia: '#a270ba',
		Zig: '#ec915c',
		Crystal: '#000100',
		Nim: '#ffc200',
		D: '#ba595e',
		V: '#5d87bf'
	};

	function langColor(lang: string): string {
		return LANG_COLORS[lang] ?? '#8b949e';
	}

	function formatCountShort(count: number): string {
		if (count >= 1000) return `${(count / 1000).toFixed(1).replace(/\.0$/, '')}k`;
		return String(count);
	}

	function displayLink(url: string): string {
		return url.replace(/^https?:\/\//, '').replace(/\/$/, '');
	}

	function hrefForLink(url: string): string {
		return url.startsWith('http') ? url : `https://${url}`;
	}

	type SocialMeta = {
		text: string;
		brandSvg: string | null;
		brandColor: string | null;
	};

	function socialMeta(url: string): SocialMeta {
		const fallback = { brandSvg: null, brandColor: null, text: displayLink(url) };
		try {
			const href = hrefForLink(url);
			const parsed = new URL(href);
			const host = parsed.hostname.toLowerCase().replace(/^www\./, '');
			const parts = parsed.pathname.split('/').filter(Boolean);
			const first = parts[0] ?? '';
			const firstLower = first.toLowerCase();
			const usernameLike = first.startsWith('@') ? first : `@${first}`;
			const handleFromSecond = parts.length >= 2 ? `@${parts[1].replace(/^@/, '')}` : fallback.text;

			if (host === 'linkedin.com' || host.endsWith('.linkedin.com')) {
				if (parts.length >= 2 && (firstLower === 'in' || firstLower === 'company')) {
					return { brandSvg: linkedinSvg, brandColor: '#0A66C2', text: handleFromSecond };
				}
				return { brandSvg: linkedinSvg, brandColor: '#0A66C2', text: fallback.text };
			}
			if (host === 'youtube.com' || host === 'youtu.be' || host.endsWith('.youtube.com')) {
				if (first.startsWith('@')) return { brandSvg: youtubeSvg, brandColor: '#FF0000', text: first };
				if (parts.length >= 2 && (firstLower === 'c' || firstLower === 'user' || firstLower === 'channel')) {
					return { brandSvg: youtubeSvg, brandColor: '#FF0000', text: handleFromSecond };
				}
				return { brandSvg: youtubeSvg, brandColor: '#FF0000', text: fallback.text };
			}
			if (host === 'x.com' || host === 'twitter.com' || host.endsWith('.x.com') || host.endsWith('.twitter.com')) {
				const reserved = ['home', 'explore', 'intent', 'share', 'search', 'hashtag', 'i'];
				if (first && !reserved.includes(firstLower)) return { brandSvg: twitterXSvg, brandColor: null, text: usernameLike };
				return { brandSvg: twitterXSvg, brandColor: null, text: fallback.text };
			}
			if (host === 'instagram.com' || host.endsWith('.instagram.com')) {
				if (first) return { brandSvg: instagramSvg, brandColor: '#E4405F', text: usernameLike };
				return { brandSvg: instagramSvg, brandColor: '#E4405F', text: fallback.text };
			}
			if (host === 'facebook.com' || host.endsWith('.facebook.com')) {
				if (first) return { brandSvg: facebookSvg, brandColor: '#1877F2', text: usernameLike };
				return { brandSvg: facebookSvg, brandColor: '#1877F2', text: fallback.text };
			}
			if (host === 'tiktok.com' || host.endsWith('.tiktok.com')) {
				const reserved = ['tag', 'music', 'discover', 'foryou', 'explore'];
				if (first && !reserved.includes(firstLower)) return { brandSvg: tiktokSvg, brandColor: null, text: usernameLike };
				return { brandSvg: tiktokSvg, brandColor: null, text: fallback.text };
			}
			return fallback;
		} catch {
			return fallback;
		}
	}

	type ContributionActivity = { date: string; count: number };

	function buildFallbackContributions(repos: Repository[]): Record<string, number> {
		const fallback: Record<string, number> = {};
		for (const repo of repos) {
			const date = new Date(repo.updated_at);
			if (Number.isNaN(date.getTime())) continue;
			const day = date.toISOString().slice(0, 10);
			fallback[day] = (fallback[day] ?? 0) + 1;
		}
		return fallback;
	}

	function buildYearActivity(contribs: Record<string, number>): ContributionActivity[] {
		const today = new Date();
		today.setHours(0, 0, 0, 0);
		const start = new Date(today);
		start.setDate(start.getDate() - 52 * 7);
		start.setDate(start.getDate() - start.getDay());
		const out: ContributionActivity[] = [];
		for (const cur = new Date(start); cur <= today; cur.setDate(cur.getDate() + 1)) {
			const y = cur.getFullYear();
			const m = String(cur.getMonth() + 1).padStart(2, '0');
			const d = String(cur.getDate()).padStart(2, '0');
			const key = `${y}-${m}-${d}`;
			out.push({ date: key, count: contribs[key] ?? 0 });
		}
		return out;
	}

	const displayContributions = $derived.by(() => {
		if (Object.keys(contributions).length > 0) return contributions;
		return buildFallbackContributions(userProfileCtx.repos ?? []);
	});
	const contributionActivity = $derived(buildYearActivity(displayContributions));
	const totalContributions = $derived.by(() => contributionActivity.reduce((sum, day) => sum + day.count, 0));
	const contributionTitleTemplate = '{{count}} contributions in the last year';

	const sortedFilteredOrgRepos = $derived.by(() => {
		let list = orgRepoFilter.trim()
			? orgRepos.filter((r) => r.name.toLowerCase().includes(orgRepoFilter.toLowerCase()) || (r.description ?? '').toLowerCase().includes(orgRepoFilter.toLowerCase()))
			: [...orgRepos];
		if (orgRepoSort === 'name') list.sort((a, b) => a.name.localeCompare(b.name));
		else if (orgRepoSort === 'stars') list.sort((a, b) => (b.star_count ?? 0) - (a.star_count ?? 0));
		else list.sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
		return list;
	});

	const sortedFilteredUserRepos = $derived.by(() => {
		let list = userRepoFilter.trim()
			? (userProfileCtx.repos ?? []).filter(
					(r) => r.name.toLowerCase().includes(userRepoFilter.toLowerCase()) || (r.description ?? '').toLowerCase().includes(userRepoFilter.toLowerCase())
				)
			: [...(userProfileCtx.repos ?? [])];
		if (userRepoSort === 'name') list.sort((a, b) => a.name.localeCompare(b.name));
		else if (userRepoSort === 'stars') list.sort((a, b) => (b.star_count ?? 0) - (a.star_count ?? 0));
		else list.sort((a, b) => new Date(b.updated_at).getTime() - new Date(a.updated_at).getTime());
		return list;
	});
</script>

<svelte:head>
	<title>{orgCtx.org ? orgCtx.org.login : username}</title>
</svelte:head>

{#if userProfileCtx.loading && !orgCtx.isOrg}
	<div class="h-full overflow-visible">
		<div class="relative mx-auto max-w-3xl px-6 pt-8">
			<div class="flex items-end justify-between">
				<Skeleton class="size-24 rounded-full border border-border sm:size-28" />
				<Skeleton class="mb-1 h-9 w-24" />
			</div>
			<div class="mt-4 space-y-4 pb-8">
				<div class="space-y-2">
					<Skeleton class="h-8 w-56" />
					<Skeleton class="h-5 w-72 max-w-full" />
				</div>
				<Skeleton class="h-4 w-[28rem] max-w-full" />
				<div class="flex gap-4">
					<Skeleton class="h-4 w-32" />
					<Skeleton class="h-4 w-28" />
				</div>
				<section class="space-y-3 pt-8">
					<Skeleton class="h-4 w-28" />
					<div class="space-y-3">
						{#each Array(2) as _}
							<div class="rounded-md border border-border bg-background p-4">
								<Skeleton class="h-5 w-48 max-w-full" />
								<Skeleton class="mt-2 h-4 w-full" />
								<div class="mt-3 flex gap-3">
									<Skeleton class="h-3 w-16" />
									<Skeleton class="h-3 w-20" />
									<Skeleton class="h-3 w-24" />
								</div>
							</div>
						{/each}
					</div>
				</section>
			</div>
		</div>
	</div>
{:else if userProfileCtx.profile}
	<div class="h-full overflow-visible">
		<div class="relative mx-auto max-w-3xl px-6 pt-8">
			<div class="flex items-end justify-between">
				<div class="size-24 overflow-hidden rounded-full border border-border bg-secondary sm:size-28">
					{#if userProfileCtx.profile.avatar_url}
						<img src={mediaUrl(userProfileCtx.profile.avatar_url)} alt={userProfileCtx.profile.username} class="h-full w-full object-cover" />
					{:else}
						<div class="flex h-full w-full items-center justify-center text-4xl font-bold text-primary">
							{(userProfileCtx.profile.display_name || userProfileCtx.profile.username).charAt(0).toUpperCase()}
						</div>
					{/if}
				</div>

				<div class="mb-1 flex items-center gap-1.5">
					{#if isOwn}
						<Button variant="ghost" size="icon" class="size-8" href="/settings/profile"><Settings /></Button>
					{:else if authStore.isAuthenticated}
						<Button variant={userProfileCtx.isFollowing ? 'outline' : 'brand'} size="sm" disabled={profileFollowLoading} onclick={toggleProfileFollow}>
							{#if profileFollowLoading}...{:else if userProfileCtx.isFollowing}Following{:else if userProfileCtx.followsYou}Follow back{:else}Follow{/if}
						</Button>
					{/if}
				</div>
			</div>

			<div class="mt-3 flex flex-col gap-3 pb-8">
				<div class="flex min-w-0 flex-1 flex-col gap-0.5">
					<h1 class="text-2xl font-semibold tracking-tight">{userProfileCtx.profile.display_name || userProfileCtx.profile.username}</h1>
					<div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-base text-muted-foreground">
						<span>@{userProfileCtx.profile.username}</span>
						<span class="text-border">.</span>
						<span class="text-sm">
							Joined {new Date(userProfileCtx.profile.created_at).toLocaleDateString('en-US', { month: 'long', year: 'numeric' })}
						</span>
					</div>
				</div>

				{#if userProfileCtx.profile.bio}
					<p class="max-w-lg text-sm leading-relaxed">{userProfileCtx.profile.bio}</p>
				{/if}

				<div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
					{#if userProfileCtx.profile.location}
						<span class="flex items-center gap-1.5"><MapPin class="h-3.5 w-3.5" />{userProfileCtx.profile.location}</span>
					{/if}
					{#if userProfileCtx.profile.website}
						<a
							href={userProfileCtx.profile.website.startsWith('http') ? userProfileCtx.profile.website : `https://${userProfileCtx.profile.website}`}
							target="_blank"
							rel="noopener noreferrer"
							class="flex items-center gap-1.5 transition-colors hover:text-foreground"
						>
							<Link class="h-3.5 w-3.5" />
							{userProfileCtx.profile.website.replace(/^https?:\/\//, '').replace(/\/$/, '')}
						</a>
					{/if}
				</div>

				<div class="flex items-center gap-4 text-sm">
					<button
						type="button"
						class="flex cursor-pointer items-center gap-1.5 text-muted-foreground transition-colors hover:text-foreground hover:underline"
						onclick={userProfileCtx.openFollowersDialog}
					>
						<Users class="h-3.5 w-3.5" />
						<span class="font-semibold text-foreground">{formatCountShort(userProfileCtx.followerCount)}</span>
						<span>{userProfileCtx.followerCount === 1 ? 'follower' : 'followers'}</span>
					</button>
					<span class="text-border">.</span>
					<button type="button" class="cursor-pointer text-muted-foreground transition-colors hover:text-foreground hover:underline" onclick={userProfileCtx.openFollowingDialog}>
						<span class="font-semibold text-foreground">{formatCountShort(userProfileCtx.followingCount)}</span> following
					</button>
				</div>

				<section class="pt-2">
					{#if loadingContribs}
						<div class="rounded-md border border-border bg-card p-4">
							<Skeleton class="h-32 w-full" />
						</div>
					{:else}
						<ContributionGraph
							data={contributionActivity}
							totalCount={totalContributions}
							labels={{
								totalCount: contributionTitleTemplate,
								legend: { less: 'Less', more: 'More' }
							}}
						/>
					{/if}
				</section>

				<section class="flex flex-col gap-3 pt-8">
					<div class="flex items-center gap-2 flex-wrap">
						<input
							type="search"
							placeholder="Find a repository..."
							bind:value={userRepoFilter}
							class="h-9 flex-1 min-w-[180px] rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
						/>
						<SearchSelect
							bind:value={userRepoSort}
							options={[
								{ value: 'updated', label: 'Last updated' },
								{ value: 'name', label: 'Name' },
								{ value: 'stars', label: 'Stars' }
							]}
						/>
						{#if isOwn}
							<Button variant="brand" size="sm" href="/new"><Plus class="h-3.5 w-3.5" />New</Button>
						{/if}
					</div>

					{#if sortedFilteredUserRepos.length === 0}
						<div class="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">
							{userRepoFilter ? 'No repositories match.' : isOwn ? "You don't have any repositories yet." : `${username} doesn't have any public repositories.`}
							{#if isOwn && !userRepoFilter}
								<div class="mt-3">
									<Button variant="brand" size="sm" href="/new">Create a repository</Button>
								</div>
							{/if}
						</div>
					{:else}
						<div class="divide-y divide-secondary overflow-hidden rounded-md border border-border bg-background">
							{#each sortedFilteredUserRepos as repo}
								{@const repoOwner = getRepoOwner(repo, username!)}
								<div class="px-4 py-4 transition-colors hover:bg-card/60">
									<div class="flex items-center gap-2">
										<a href="/{repoOwner}/{repo.name}" class="text-base font-semibold text-primary hover:underline">{repo.name}</a>
										<span class="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">{repo.is_private ? 'Private' : 'Public'}</span>
									</div>
									{#if repo.description}<p class="mt-1 text-sm text-muted-foreground">{repo.description}</p>{/if}
									<div class="mt-2 flex items-center gap-4 text-xs text-muted-foreground">
										{#if repo.language}
											<span class="flex items-center gap-1.5">
												<span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background-color:{langColor(repo.language)}"></span>
												{repo.language}
											</span>
										{/if}
										{#if (repo.star_count ?? 0) > 0}<span class="flex items-center gap-1"><StarIcon class="h-3 w-3" />{repo.star_count}</span>{/if}
										<span>Updated {timeAgo(repo.updated_at)}</span>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</section>

				<section class="flex flex-col gap-3 pt-8">
					<div class="flex items-center justify-between">
						<h2 class="text-sm font-medium text-muted-foreground">Projects</h2>
						{#if canCreateProject}
							<Button variant="brand" size="sm" onclick={() => (showCreateProjectDialog = true)}><Plus class="h-3.5 w-3.5" />Create</Button>
						{/if}
					</div>
					{#if loadingProjects}
						<div class="space-y-3">
							{#each Array(2) as _}
								<div class="rounded-md border border-border bg-card p-4">
									<Skeleton class="h-5 w-56 max-w-full" />
									<Skeleton class="mt-2 h-4 w-full" />
								</div>
							{/each}
						</div>
					{:else if projectsError}
						<div class="rounded-md border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive">{projectsError}</div>
					{:else if projectList.length === 0}
						<div class="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">
							No projects yet.
							{#if canCreateProject}
								<div class="mt-3">
									<Button variant="brand" size="sm" onclick={() => (showCreateProjectDialog = true)}>Create first project</Button>
								</div>
							{/if}
						</div>
					{:else}
						<div class="space-y-3">
							{#each projectList as project}
								<a href="/{username}/projects/{project.id}" class="block rounded-md border border-secondary bg-card p-4 transition-colors hover:border-border">
									<div class="flex items-start justify-between gap-3">
										<h3 class="line-clamp-1 text-base font-semibold text-foreground">{project.title}</h3>
										<span class="inline-flex items-center rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground">
											{#if project.is_public}<Globe2 class="mr-1 h-3 w-3" />Public{:else}<Lock class="mr-1 h-3 w-3" />Private{/if}
										</span>
									</div>
									{#if project.description}<p class="mt-1 line-clamp-2 text-sm text-muted-foreground">{project.description}</p>{/if}
									<div class="mt-3 flex items-center justify-between text-xs text-muted-foreground">
										<span>{project.columns?.length ?? 0} columns</span>
										<span class="inline-flex items-center gap-1 text-primary">Open board <ArrowRight class="h-3.5 w-3.5" /></span>
									</div>
								</a>
							{/each}
						</div>
					{/if}
				</section>

				<section class="flex flex-col gap-3 pt-8">
					<h2 class="text-sm font-medium text-muted-foreground">Stars</h2>
					{#if loadingStarred}
						<div class="space-y-3">
							{#each Array(2) as _}
								<div class="rounded-md border border-border bg-card p-4">
									<Skeleton class="h-5 w-60 max-w-full" />
									<Skeleton class="mt-2 h-4 w-full" />
									<div class="mt-3 flex gap-3">
										<Skeleton class="h-3 w-16" />
										<Skeleton class="h-3 w-24" />
									</div>
								</div>
							{/each}
						</div>
					{:else if starredRepos.length === 0}
						<div class="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">{username} hasn't starred any repositories yet.</div>
					{:else}
						<div class="space-y-3">
							{#each starredRepos as star}
								{@const repo = star.repo}
								{@const repoOwner = repo.org?.login ?? repo.owner?.username ?? username}
								<div class="rounded-md border border-secondary bg-card p-4 transition-colors hover:border-border">
									<div class="min-w-0">
										<div class="flex flex-wrap items-center gap-2">
											<a href="/{repoOwner}/{repo.name}" class="truncate text-base font-semibold text-primary hover:underline">{repoOwner}/{repo.name}</a>
											<span class="inline-flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">
												{#if repo.is_private}<Lock class="h-2.5 w-2.5" />Private{:else}Public{/if}
											</span>
										</div>
										{#if repo.description}<p class="mt-1 text-xs text-muted-foreground">{repo.description}</p>{/if}
									</div>
									<div class="mt-3 flex items-center gap-4 text-xs text-muted-foreground">
										{#if repo.language}
											<span class="flex items-center gap-1.5">
												<span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background-color:{langColor(repo.language)}"></span>
												{repo.language}
											</span>
										{/if}
										<span>Updated {timeAgo(repo.updated_at)}</span>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</section>

				<section class="flex flex-col gap-3 pt-8">
					<h2 class="text-sm font-medium text-muted-foreground">Packages</h2>
					{#if loadingUserPackages}
						<div class="space-y-3">
							{#each Array(2) as _}
								<div class="rounded-md border border-border bg-card p-4">
									<div class="flex items-start gap-3">
										<Skeleton class="mt-0.5 h-5 w-5 rounded-sm" />
										<div class="flex-1 space-y-2">
											<Skeleton class="h-5 w-44 max-w-full" />
											<Skeleton class="h-4 w-full" />
										</div>
									</div>
								</div>
							{/each}
						</div>
					{:else if userContainerRepos.length === 0}
						<div class="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">No packages published yet.</div>
					{:else}
						<div class="space-y-3">
							{#each userContainerRepos as pkg}
								<div class="rounded-md border border-secondary bg-card p-4 transition-colors hover:border-border">
									<div class="flex items-start gap-3">
										<Package class="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
										<div class="min-w-0 flex-1">
											<div class="flex flex-wrap items-center gap-2">
												<a href="/{pkg.namespace}/packages/{pkg.name}" class="text-base font-semibold text-primary hover:underline">{pkg.name}</a>
												<span class="inline-flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">
													{pkg.is_public ? 'Public' : 'Private'}
												</span>
											</div>
											<p class="mt-1 overflow-x-auto text-xs font-mono text-muted-foreground">docker pull {registryHost}/{pkg.namespace}/{pkg.name}:latest</p>
										</div>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</section>
			</div>
		</div>
	</div>
{:else if orgCtx.isOrg && orgCtx.org}
	<div class="h-full overflow-visible">
		<div class="relative mx-auto max-w-3xl px-6 pt-8">
			<div class="flex items-end justify-between">
				<div class="size-28 overflow-hidden rounded-3xl border border-border bg-secondary sm:size-32">
					{#if orgCtx.org.avatar_url}
						<img src={mediaUrl(orgCtx.org.avatar_url)} alt={orgCtx.org.login} class="h-full w-full object-cover" />
					{:else}
						<div class="flex h-full w-full items-center justify-center text-4xl font-bold text-primary">
							{(orgCtx.org.display_name || orgCtx.org.login).charAt(0).toUpperCase()}
						</div>
					{/if}
				</div>
				<div class="mb-1 flex items-center gap-1.5">
					{#if authStore.isAuthenticated}
						<Button variant={orgCtx.isFollowing ? 'outline' : 'brand'} size="sm" disabled={orgFollowLoading} onclick={toggleOrgFollow}>
							{#if orgFollowLoading}...{:else if orgCtx.isFollowing}Following{:else}Follow{/if}
						</Button>
						{#if orgCtx.isMember}
							<Button variant="ghost" size="icon" class="size-8" href="/{username}/settings" aria-label="Organization settings">
								<Settings class="h-4 w-4" />
							</Button>
						{/if}
					{/if}
				</div>
			</div>

			<div class="mt-3 flex flex-col gap-3 pb-8">
				<div class="flex min-w-0 flex-1 flex-col gap-0.5">
					<h1 class="text-2xl font-semibold tracking-tight">{orgCtx.org.display_name || orgCtx.org.login}</h1>
					<div class="flex flex-wrap items-center gap-x-2 gap-y-0.5 text-base text-muted-foreground">
						<span>@{orgCtx.org.login}</span>
						<span class="text-border">.</span>
						<span class="flex items-center gap-1 text-sm">
							<Building2 class="h-3.5 w-3.5" />
							Organization
						</span>
					</div>
				</div>

				{#if orgCtx.org.description}
					<p class="max-w-lg text-sm leading-relaxed">{orgCtx.org.description}</p>
				{/if}

				<div class="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm text-muted-foreground">
					<span class="flex items-center gap-1.5"><Users class="h-3.5 w-3.5" />{orgCtx.memberCount} member{orgCtx.memberCount !== 1 ? 's' : ''}</span>
					<button type="button" class="flex cursor-pointer items-center gap-1.5 transition-colors hover:text-foreground hover:underline" onclick={openOrgFollowersDialog}>
						<UserPlus class="h-3.5 w-3.5" />
						<span class="font-semibold text-foreground">{formatCountShort(orgCtx.followerCount)}</span>
						<span>{orgCtx.followerCount === 1 ? 'follower' : 'followers'}</span>
					</button>
					{#if orgCtx.org.location}
						<span class="flex items-center gap-1.5"><MapPin class="h-3.5 w-3.5" />{orgCtx.org.location}</span>
					{/if}
					{#if orgCtx.org.website}
						<a
							href={hrefForLink(orgCtx.org.website)}
							target="_blank"
							rel="noopener noreferrer"
							class="flex items-center gap-1.5 transition-colors hover:text-foreground"
						>
							<Link class="h-3.5 w-3.5" />
							{displayLink(orgCtx.org.website)}
						</a>
					{/if}
					{#each orgCtx.org.social_links ?? [] as social}
						{#if social?.url?.trim()}
							{@const meta = socialMeta(social.url)}
							<a href={hrefForLink(social.url)} target="_blank" rel="noopener noreferrer" class="flex items-center gap-1.5 transition-colors hover:text-foreground">
								{#if meta.brandSvg}
									<span class="social-brand-icon shrink-0" style:color={meta.brandColor ?? undefined} aria-hidden="true">
										{@html meta.brandSvg}
									</span>
								{:else}
									<Link class="h-3.5 w-3.5" />
								{/if}
								{meta.text}
							</a>
						{/if}
					{/each}
				</div>

				<div class="flex items-center gap-2 pt-1">
					<div class="flex items-center -space-x-2">
						{#each orgMembers.slice(0, 12) as m}
							<a href="/{m.user.username}" title="@{m.user.username}" class="relative">
								<div class="size-8 overflow-hidden rounded-full border-2 border-card bg-secondary">
									{#if m.user.avatar_url}
										<img src={mediaUrl(m.user.avatar_url)} alt={m.user.username} class="h-full w-full object-cover" />
									{:else}
										<div class="flex h-full w-full items-center justify-center text-xs font-semibold text-primary">{m.user.username[0].toUpperCase()}</div>
									{/if}
								</div>
							</a>
						{/each}
						{#if orgMembers.length > 12}
							<a href="/{username}/people" class="flex h-8 w-8 items-center justify-center rounded-full border-2 border-card bg-muted text-xs font-semibold text-muted-foreground">
								+{orgMembers.length - 12}
							</a>
						{/if}
					</div>
				</div>

				<section class="flex flex-col gap-3 pt-8">
					<div class="flex items-center gap-2 flex-wrap">
						<input
							type="search"
							placeholder="Find a repository..."
							bind:value={orgRepoFilter}
							class="h-9 flex-1 min-w-[180px] rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
						/>
						<SearchSelect
							bind:value={orgRepoSort}
							options={[
								{ value: 'updated', label: 'Last updated' },
								{ value: 'name', label: 'Name' },
								{ value: 'stars', label: 'Stars' }
							]}
						/>
						{#if orgCtx.isMember}
							<Button variant="brand" size="sm" href="/{username}/new"><Plus class="h-3.5 w-3.5" />New</Button>
						{/if}
					</div>

					{#if sortedFilteredOrgRepos.length === 0}
						<div class="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">
							{orgRepoFilter ? 'No repositories match.' : 'No repositories yet.'}
							{#if orgCtx.isMember && !orgRepoFilter}
								<div class="mt-3">
									<Button variant="brand" size="sm" href="/{username}/new">Create a repository</Button>
								</div>
							{/if}
						</div>
					{:else}
						<div class="divide-y divide-secondary overflow-hidden rounded-md border border-border bg-background">
							{#each sortedFilteredOrgRepos as repo}
								<div class="px-4 py-4 transition-colors hover:bg-card/60">
									<div class="flex items-center gap-2">
										<a href="/{username}/{repo.name}" class="text-base font-semibold text-primary hover:underline">{repo.name}</a>
										<span class="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">{repo.is_private ? 'Private' : 'Public'}</span>
									</div>
									{#if repo.description}<p class="mt-1 text-sm text-muted-foreground">{repo.description}</p>{/if}
									<div class="mt-2 flex items-center gap-4 text-xs text-muted-foreground">
										{#if repo.language}
											<span class="flex items-center gap-1.5">
												<span class="h-2.5 w-2.5 shrink-0 rounded-full" style="background-color:{langColor(repo.language)}"></span>
												{repo.language}
											</span>
										{/if}
										{#if (repo.star_count ?? 0) > 0}<span class="flex items-center gap-1"><StarIcon class="h-3 w-3" />{repo.star_count}</span>{/if}
										<span>Updated {timeAgo(repo.updated_at)}</span>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</section>

				<section class="flex flex-col gap-3 pt-8">
					<div class="flex items-center justify-between">
						<h2 class="text-sm font-medium text-muted-foreground">Projects</h2>
						{#if canCreateProject}
							<Button variant="brand" size="sm" onclick={() => (showCreateProjectDialog = true)}><Plus class="h-3.5 w-3.5" />Create</Button>
						{/if}
					</div>
					{#if loadingProjects}
						<div class="space-y-3">
							{#each Array(2) as _}
								<div class="rounded-md border border-border bg-card p-4">
									<Skeleton class="h-5 w-56 max-w-full" />
									<Skeleton class="mt-2 h-4 w-full" />
								</div>
							{/each}
						</div>
					{:else if projectsError}
						<div class="rounded-md border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive">{projectsError}</div>
					{:else if projectList.length === 0}
						<div class="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">
							No projects yet.
							{#if canCreateProject}
								<div class="mt-3">
									<Button variant="brand" size="sm" onclick={() => (showCreateProjectDialog = true)}>Create first project</Button>
								</div>
							{/if}
						</div>
					{:else}
						<div class="space-y-3">
							{#each projectList as project}
								<a href="/{username}/projects/{project.id}" class="block rounded-md border border-secondary bg-card p-4 transition-colors hover:border-border">
									<div class="flex items-start justify-between gap-3">
										<h3 class="line-clamp-1 text-base font-semibold text-foreground">{project.title}</h3>
										<span class="inline-flex items-center rounded-full border border-border px-2 py-0.5 text-[11px] text-muted-foreground">
											{#if project.is_public}<Globe2 class="mr-1 h-3 w-3" />Public{:else}<Lock class="mr-1 h-3 w-3" />Private{/if}
										</span>
									</div>
									{#if project.description}<p class="mt-1 line-clamp-2 text-sm text-muted-foreground">{project.description}</p>{/if}
									<div class="mt-3 flex items-center justify-between text-xs text-muted-foreground">
										<span>{project.columns?.length ?? 0} columns</span>
										<span class="inline-flex items-center gap-1 text-primary">Open board <ArrowRight class="h-3.5 w-3.5" /></span>
									</div>
								</a>
							{/each}
						</div>
					{/if}
				</section>

				<section class="flex flex-col gap-3 pt-8">
					<h2 class="text-sm font-medium text-muted-foreground">Packages</h2>
					{#if loadingOrgPackages}
						<div class="space-y-3">
							{#each Array(2) as _}
								<div class="rounded-md border border-border bg-card p-4">
									<div class="flex items-start gap-3">
										<Skeleton class="mt-0.5 h-5 w-5 rounded-sm" />
										<div class="flex-1 space-y-2">
											<Skeleton class="h-5 w-44 max-w-full" />
											<Skeleton class="h-4 w-full" />
										</div>
									</div>
								</div>
							{/each}
						</div>
					{:else if orgContainerRepos.length === 0}
						<div class="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">No packages published yet.</div>
					{:else}
						<div class="space-y-3">
							{#each orgContainerRepos as pkg}
								<div class="rounded-md border border-secondary bg-card p-4 transition-colors hover:border-border">
									<div class="flex items-start gap-3">
										<Package class="mt-0.5 h-5 w-5 shrink-0 text-muted-foreground" />
										<div class="min-w-0 flex-1">
											<div class="flex flex-wrap items-center gap-2">
												<a href="/{pkg.namespace}/packages/{pkg.name}" class="text-base font-semibold text-primary hover:underline">{pkg.name}</a>
												<span class="inline-flex items-center gap-1 rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">
													{pkg.is_public ? 'Public' : 'Private'}
												</span>
											</div>
											<p class="mt-1 overflow-x-auto text-xs font-mono text-muted-foreground">docker pull {registryHost}/{pkg.namespace}/{pkg.name}:latest</p>
										</div>
									</div>
								</div>
							{/each}
						</div>
					{/if}
				</section>
			</div>
		</div>
	</div>
{/if}

<!-- Org followers dialog -->
<Dialog.Root bind:open={showOrgFollowersDialog}>
	<Dialog.Content class="max-w-xl">
		<Dialog.Header>
			<Dialog.Title>Organization followers</Dialog.Title>
			<Dialog.Description>People following this organization.</Dialog.Description>
		</Dialog.Header>
		{#if loadingOrgFollowDialog}
			<div class="space-y-2 py-2">
				{#each Array(3) as _}<div class="h-11 rounded-md bg-secondary animate-pulse"></div>{/each}
			</div>
		{:else if orgFollowerDialogUsers.length === 0}
			<p class="py-4 text-sm text-muted-foreground">No followers yet.</p>
		{:else}
			<div class="max-h-[26rem] overflow-y-auto space-y-1">
				{#each orgFollowerDialogUsers as item}
					{@const u = item.user}
					<div class="flex items-center gap-3 rounded-md border border-secondary bg-card px-3 py-2">
						<a href="/{u?.username}" class="h-9 w-9 rounded-full border border-border bg-secondary flex items-center justify-center overflow-hidden shrink-0">
							{#if u?.avatar_url}<img src={mediaUrl(u.avatar_url)} alt={u.username} class="h-full w-full object-cover" />{:else}<span class="text-xs font-semibold text-primary"
									>{(u?.username ?? '?')[0].toUpperCase()}</span
								>{/if}
						</a>
						<a href="/{u?.username}" class="text-sm font-medium text-foreground hover:underline">{u?.username}</a>
					</div>
				{/each}
			</div>
		{/if}
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={showCreateProjectDialog}>
	<Dialog.Content class="max-w-lg">
		<Dialog.Header>
			<Dialog.Title>Create project</Dialog.Title>
			<Dialog.Description>Start with a board and default workflow columns.</Dialog.Description>
		</Dialog.Header>
		<div class="space-y-3 pt-2">
			<div>
				<label class="mb-1 block text-xs text-muted-foreground" for="project-title">Title</label>
				<input
					id="project-title"
					bind:value={newProjectTitle}
					class="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring"
					placeholder="Project UI/UX"
				/>
			</div>
			<div>
				<label class="mb-1 block text-xs text-muted-foreground" for="project-description">Description</label>
				<textarea
					id="project-description"
					bind:value={newProjectDescription}
					rows="4"
					class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring"
					placeholder="What is this board used for?"
				></textarea>
			</div>
			<label class="flex items-center gap-2 rounded-md border border-input bg-background px-3 py-2 text-sm">
				<input type="checkbox" bind:checked={newProjectIsPublic} />
				<span>Public project</span>
			</label>
			{#if createProjectError}<p class="text-sm text-destructive">{createProjectError}</p>{/if}
		</div>
		<Dialog.Footer class="mt-4 gap-2">
			<Button variant="outline" onclick={() => (showCreateProjectDialog = false)} disabled={creatingProject}>Cancel</Button>
			<Button variant="brand" onclick={createProject} disabled={creatingProject || !newProjectTitle.trim()}>{creatingProject ? 'Creating...' : 'Create project'}</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<style>
	.social-brand-icon :global(svg) {
		display: block;
		width: 0.875rem;
		height: 0.875rem;
		fill: currentColor;
	}
</style>
