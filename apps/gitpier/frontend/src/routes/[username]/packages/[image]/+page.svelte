<script lang="ts">
	import { page } from '$app/state';
	import { packages, users, orgs, type ContainerPackageDetails, type User, type Organization, API_BASE } from '$lib/api/client';
	import { timeAgo } from '$lib/utils';
	import { Package, Tag, Copy, Box, Scale, CircleDot, Settings, Download, ArrowLeft } from '@lucide/svelte';

	let loading = $state(true);
	let error = $state('');
	let details = $state<ContainerPackageDetails | null>(null);
	let copied = $state(false);
	let copiedDigest = $state<string | null>(null);
	let ownerAvatar = $state<string | null>(null);
	let ownerDisplayName = $state<string>('');

	const username = $derived(page.params.username);
	const imageName = $derived(page.params.image);

	$effect(() => {
		const currentUsername = username;
		const currentImageName = imageName;
		loading = true;
		error = '';
		details = null;
		ownerAvatar = null;
		ownerDisplayName = '';

		packages
			.get(currentUsername!, currentImageName!)
			.then((res) => {
				if (username !== currentUsername || imageName !== currentImageName) return;
				details = res;
				// Fetch owner avatar - try user first, then org
				users
					.getProfile(currentUsername!)
					.then((u) => {
						ownerAvatar = u.user.avatar_url || null;
						ownerDisplayName = u.user.display_name || u.user.username;
					})
					.catch(() => {
						orgs.get(currentUsername!)
							.then((o) => {
								ownerAvatar = o.org.avatar_url || null;
								ownerDisplayName = o.org.display_name || o.org.login;
							})
							.catch(() => {});
					});
			})
			.catch((e: any) => {
				if (username !== currentUsername || imageName !== currentImageName) return;
				error = e.message ?? 'Failed to load package';
			})
			.finally(() => {
				if (username === currentUsername && imageName === currentImageName) {
					loading = false;
				}
			});
	});

	async function copyPullCommand() {
		if (!pullCommand) return;
		try {
			await navigator.clipboard.writeText(pullCommand);
			copied = true;
			setTimeout(() => (copied = false), 1500);
		} catch {
			copied = false;
		}
	}

	async function copyDigest(digest: string) {
		try {
			await navigator.clipboard.writeText(digest);
			copiedDigest = digest;
			setTimeout(() => (copiedDigest = null), 1500);
		} catch {
			/* ignore */
		}
	}

	function shortDigest(digest: string) {
		if (!digest) return '';
		const hash = digest.includes(':') ? digest.split(':')[1] : digest;
		return hash.slice(0, 12);
	}

	const latestTag = $derived(details?.tags?.[0]?.tag ?? 'latest');
	const lastPublished = $derived(details?.tags?.[0]?.updated_at ? timeAgo(details.tags[0].updated_at) : null);
	const totalDownloads = $derived(details?.tags?.reduce((sum, t) => sum + (t.pull_count ?? 0), 0) ?? 0);
	const registryHost = $derived(typeof window !== 'undefined' ? window.location.host : 'localhost:8828');
	const pullCommand = $derived(details?.pull_command || (details ? `docker pull ${registryHost}/${details.package.namespace}/${details.package.name}:${latestTag}` : ''));

	// Derive real bar heights from per-tag pull counts (last 7 tags)
	const downloadBars = $derived(
		!details?.tags?.length || totalDownloads === 0
			? []
			: (() => {
					const vals = details.tags.slice(0, 7).map((t) => t.pull_count ?? 0);
					const max = Math.max(...vals, 1);
					return vals.map((v) => Math.round(4 + (v / max) * 14));
				})()
	);

	const avatarSrc = $derived(ownerAvatar ? (ownerAvatar.startsWith('http') ? ownerAvatar : API_BASE + ownerAvatar) : null);
</script>

<svelte:head>
	<title>{username}/{imageName} · Package · GitPier</title>
</svelte:head>

<div class="min-h-screen py-6 px-4">
	<div class="mx-auto max-w-5xl">
		<!-- Back button -->
		<a href="/{username}/packages" class="mb-4 inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors">
			<ArrowLeft class="h-4 w-4" />
			Back to packages
		</a>

		{#if loading}
			<div class="rounded-md border border-border bg-card p-8 animate-pulse space-y-3">
				<div class="h-5 w-56 rounded bg-secondary"></div>
				<div class="h-4 w-40 rounded bg-secondary"></div>
				<div class="h-20 w-full rounded bg-secondary"></div>
			</div>
		{:else if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{error}</div>
		{:else if details}
			<!-- Page header -->
			<div class="mb-5 flex items-center gap-2 flex-wrap">
				<Package class="h-5 w-5 text-muted-foreground" />
				<h1 class="text-xl font-semibold text-foreground">{details.package.name}</h1>
				<span class="text-base font-normal text-muted-foreground">{latestTag}</span>
				<span class="rounded-full border border-border px-2 py-0.5 text-xs text-muted-foreground">{details.package.is_public ? 'Public' : 'Private'}</span>
				<span class="rounded-full border border-border px-2 py-0.5 text-xs text-foreground font-medium">Latest</span>
			</div>

			<div class="grid grid-cols-1 lg:grid-cols-[minmax(0,1fr)_296px] gap-6">
				<!-- Main column -->
				<div class="space-y-4">
					<!-- Installation card -->
					<div class="rounded-md border border-border bg-card overflow-hidden">
						<div class="px-4 border-b border-border">
							<div class="flex items-end">
								<span class="py-2.5 text-sm font-medium border-b-2 border-[#f78166] text-foreground -mb-px mr-auto">Installation</span>
								<a href="https://docs.docker.com/reference/cli/docker/image/pull/" target="_blank" rel="noopener" class="py-2.5 text-sm text-[#58a6ff] hover:underline"
									>Learn more about packages</a
								>
							</div>
						</div>
						<div class="p-4">
							<p class="text-xs text-muted-foreground mb-2 flex items-center gap-1.5">
								<Box class="h-3.5 w-3.5" />
								Install from the command line
							</p>
							<div class="rounded-md border border-border bg-muted px-3 py-2.5 flex items-center gap-3">
								<code class="flex-1 text-sm text-foreground font-mono break-all min-w-0">
									<span class="select-none text-muted-foreground">$ </span>{pullCommand}
								</code>
								<button
									onclick={copyPullCommand}
									class="shrink-0 rounded-md border border-border bg-background hover:bg-secondary px-2.5 py-1 text-xs text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1.5"
								>
									<Copy class="h-3 w-3" />{copied ? 'Copied!' : 'Copy'}
								</button>
							</div>
						</div>
					</div>

					<!-- Recent tagged image versions card -->
					<div class="rounded-md border border-border bg-card overflow-hidden">
						<div class="px-4 py-3 border-b border-border">
							<h2 class="text-sm font-semibold text-foreground">Recent tagged image versions</h2>
						</div>
						{#if details.tags.length === 0}
							<div class="px-4 py-6 text-sm text-muted-foreground">
								No versions yet — push an image to populate this.
								<code class="ml-1 text-xs font-mono">docker push {registryHost}/{details.package.namespace}/{details.package.name}:latest</code>
							</div>
						{:else}
							<div class="divide-y divide-border">
								{#each details.tags as tag, i}
									<div class="px-4 py-3">
										<div class="flex items-start justify-between gap-4">
											<div class="min-w-0">
												<div class="flex items-center gap-1.5 flex-wrap mb-1">
													{#if i === 0}
														<span class="rounded-full bg-[#1a3a2a] text-[#3fb950] border border-[#2ea043]/50 px-2 py-0.5 text-xs font-medium">latest</span>
													{/if}
													<span class="rounded-full border border-[#30363d] px-2 py-0.5 text-xs font-medium text-[#58a6ff]">{tag.tag}</span>
												</div>
												<div class="text-xs text-muted-foreground flex items-center gap-1.5 flex-wrap">
													<span>Published {timeAgo(tag.updated_at)}</span>
													<span>·</span>
													<span>Digest</span>
													<button onclick={() => copyDigest(tag.digest)} class="font-mono hover:text-foreground transition-colors" title={tag.digest}
														>{shortDigest(tag.digest)}</button
													>
													<button onclick={() => copyDigest(tag.digest)} class="hover:text-foreground transition-colors px-0.5" title="Copy full digest"
														>{copiedDigest === tag.digest ? '✓' : '···'}</button
													>
												</div>
											</div>
											<div class="shrink-0 text-xs text-muted-foreground flex items-center gap-1 pt-0.5">
												<Download class="h-3.5 w-3.5" />{tag.pull_count ?? 0}
											</div>
										</div>
									</div>
								{/each}
							</div>
							<a href="/{details.package.namespace}/packages/{details.package.name}/versions" class="block px-4 py-2.5 text-sm text-[#58a6ff] hover:underline border-t border-border"
								>View and manage all versions</a
							>
						{/if}
					</div>
				</div>

				<!-- Sidebar — single card with dividers -->
				<aside>
					<div class="rounded-md border border-border bg-card overflow-hidden text-sm">
						<!-- Details header -->
						<div class="px-4 pt-3 pb-2 border-b border-border">
							<h2 class="font-semibold text-foreground">Details</h2>
						</div>

						<!-- Details items -->
						<div class="px-4 py-3 space-y-2.5 border-b border-border">
							<div class="flex items-center gap-2">
								{#if avatarSrc}
									<img src={avatarSrc} alt={details.package.namespace} class="h-4 w-4 rounded-full shrink-0 object-cover" />
								{:else}
									<div class="h-4 w-4 rounded-full bg-orange-600 shrink-0 flex items-center justify-center text-[9px] font-bold text-white">
										{details.package.namespace[0]?.toUpperCase()}
									</div>
								{/if}
								<a href="/{details.package.namespace}" class="text-foreground hover:underline">{details.package.namespace}</a>
							</div>
							<div class="flex items-center gap-2 text-muted-foreground">
								<Box class="h-4 w-4 shrink-0" />
								<span class="text-foreground">{details.package.name}</span>
							</div>
							<div class="flex items-start gap-2 text-muted-foreground">
								<Scale class="h-4 w-4 shrink-0 mt-0.5" />
								<span>License not specified</span>
							</div>
						</div>

						<!-- Last published + stats -->
						<div class="px-4 py-3 space-y-3 border-b border-border">
							<div>
								<p class="text-xs text-muted-foreground">Last published</p>
								{#if lastPublished}
									<p class="text-base font-semibold text-foreground mt-0.5">{lastPublished}</p>
								{:else}
									<p class="text-sm text-muted-foreground mt-0.5 italic">No versions yet</p>
								{/if}
							</div>
							<div class="grid grid-cols-2 gap-2">
								<div>
									<p class="text-xs text-muted-foreground">Discussions</p>
									<p class="text-base font-semibold text-foreground">0</p>
								</div>
								<div>
									<p class="text-xs text-muted-foreground">Issues</p>
									<p class="text-base font-semibold text-foreground">0</p>
								</div>
							</div>
							<div>
								<p class="text-xs text-muted-foreground mb-1.5">Total downloads</p>
								<div class="flex items-end gap-2">
									<p class="text-base font-semibold text-foreground">{totalDownloads}</p>
									{#if downloadBars.length > 0}
										<div class="flex items-end gap-0.5 h-5 ml-auto" aria-hidden="true">
											{#each downloadBars as h}
												<span class="w-1.5 rounded-sm bg-[#3fb950]" style="height:{h}px"></span>
											{/each}
										</div>
									{/if}
								</div>
							</div>
						</div>

						<!-- Contributors -->
						<div class="px-4 py-3 border-b border-border">
							<div class="flex items-center gap-2 mb-2.5">
								<span class="font-semibold text-foreground">Contributors</span>
								<span class="rounded-full border border-border bg-secondary/60 px-1.5 text-[10px] font-medium leading-4">1</span>
							</div>
							<div class="flex items-center gap-2">
								<a href="/{details.package.namespace}">
									{#if avatarSrc}
										<img src={avatarSrc} alt={details.package.namespace} class="h-8 w-8 rounded-full shrink-0 object-cover" />
									{:else}
										<div class="h-8 w-8 rounded-full bg-secondary flex items-center justify-center text-sm font-semibold text-primary shrink-0">
											{details.package.namespace[0]?.toUpperCase()}
										</div>
									{/if}
								</a>
								<div>
									<a href="/{details.package.namespace}" class="font-semibold text-foreground hover:underline">
										{ownerDisplayName || details.package.namespace}
									</a>
									{#if ownerDisplayName && ownerDisplayName !== details.package.namespace}
										<span class="text-muted-foreground"> {details.package.namespace}</span>
									{/if}
								</div>
							</div>
						</div>

						<!-- Action links -->
						<div class="px-4 py-3 space-y-2">
							<a href="/{details.package.namespace}/packages/{details.package.name}/settings" class="flex items-center gap-2 text-muted-foreground hover:text-foreground">
								<Settings class="h-4 w-4 shrink-0" />Package settings
							</a>
						</div>
					</div>
				</aside>
			</div>
		{/if}
	</div>
</div>
