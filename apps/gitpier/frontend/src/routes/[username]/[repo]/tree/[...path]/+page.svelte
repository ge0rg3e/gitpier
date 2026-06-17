<script lang="ts">
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { repos, type FileEntry, type CommitInfo } from '$lib/api/client';
	import type { Repository } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { resolveRepoTreeIconUrl } from '$lib/icons/fileIcons';
	import { timeAgo } from '$lib/utils';
	import { Folder, File, ChevronRight, FilePlus } from '@lucide/svelte';
	import FileTreeSidebar from '$lib/components/FileTreeSidebar.svelte';

	let files = $state<FileEntry[]>([]);
	let headCommit = $state<CommitInfo | null>(null);
	let loading = $state(true);
	let error = $state('');
	let loadSeq = 0;
	const repoLayout = getContext<{ repo: Repository | null } | null>('repoLayout');

	const { username, repo } = $derived(page.params);
	const path = $derived(page.params.path ?? '');
	const ref = $derived(page.url.searchParams.get('ref') ?? undefined);
	const effectiveRef = $derived(ref ?? (/^[a-f0-9]{7,40}$/i.test(path) && !path.includes('/') ? path : undefined));
	const effectivePath = $derived(ref ? path : effectiveRef === path ? '' : path);

	async function loadFiles() {
		const seq = ++loadSeq;
		loading = true;
		error = '';
		try {
			const data = await repos.tree(username!, repo!, effectiveRef, effectivePath, { includeMeta: false, includeHead: true });
			if (seq !== loadSeq) return;
			files = data.files ?? [];
			headCommit = data.head_commit;
			loading = false;

			void repos
				.tree(username!, repo!, effectiveRef, effectivePath, { includeMeta: true, includeHead: false })
				.then((metaData) => {
					if (seq !== loadSeq) return;
					files = metaData.files ?? files;
				})
				.catch(() => {});
		} catch (e: any) {
			if (seq !== loadSeq) return;
			error = e.message;
			loading = false;
		}
	}

	const currentUser = $derived(authStore.user);
	const isCommitSHA = $derived(/^[0-9a-f]{40}$/i.test(effectiveRef ?? ''));
	const canAddFile = $derived(currentUser != null && !isCommitSHA && !repoLayout?.repo?.is_archived);
	const addFileHref = $derived(
		effectivePath ? `/${username}/${repo}/new/${effectivePath}${effectiveRef ? `?ref=${effectiveRef}` : ''}` : `/${username}/${repo}/new${effectiveRef ? `?ref=${effectiveRef}` : ''}`
	);

	$effect(() => {
		loadFiles();
	});

	const segments = $derived(effectivePath ? effectivePath.split('/').filter(Boolean) : []);

	const sortedFiles = $derived([
		...files.filter((f) => f.type === 'tree').sort((a, b) => a.name.localeCompare(b.name)),
		...files.filter((f) => f.type === 'blob').sort((a, b) => a.name.localeCompare(b.name))
	]);
</script>

<!-- Breadcrumb -->
<nav class="flex items-center gap-1 mb-3 text-sm flex-wrap">
	<a href="/{username}/{repo}" class="text-primary hover:underline font-semibold">{repo}</a>
	{#each segments as seg, i}
		<ChevronRight class="h-3.5 w-3.5 text-muted-foreground" />
		{#if i < segments.length - 1}
			<a href="/{username}/{repo}/tree/{segments.slice(0, i + 1).join('/')}{ref ? `?ref=${ref}` : ''}" class="text-primary hover:underline">{seg}</a>
		{:else}
			<span class="font-semibold text-foreground">{seg}</span>
		{/if}
	{/each}
</nav>

<div class="flex gap-4 items-start">
	<!-- File tree sidebar -->
	<div class="hidden xl:block shrink-0">
		<FileTreeSidebar username={username!} repo={repo!} {ref} currentPath={effectivePath} />
	</div>

	<!-- Main content -->
	<div class="flex-1 min-w-0">
		{#if loading}
			<div class="rounded-md border border-border overflow-hidden">
				<div class="h-10 bg-card border-b border-border animate-pulse"></div>
				{#each Array(6) as _}
					<div class="h-10 border-b border-secondary bg-background animate-pulse last:border-0"></div>
				{/each}
			</div>
		{:else if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
		{:else}
			<div class="rounded-md border border-border overflow-hidden">
				{#if headCommit}
					<div class="flex items-center gap-3 border-b border-border bg-card px-4 py-2.5 text-sm">
						<a href="/{username}/{repo}/commit/{headCommit.sha}" class="text-foreground font-medium truncate hover:text-primary hover:underline">{headCommit.message.split('\n')[0]}</a>
						<div class="ml-auto flex items-center gap-2 shrink-0">
							{#if canAddFile}
								<a
									href={addFileHref}
									class="flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground hover:border-primary"
								>
									<FilePlus class="h-3.5 w-3.5" />
									Add file
								</a>
							{/if}
							<span class="text-xs text-muted-foreground font-mono">{headCommit.sha.slice(0, 7)}</span>
						</div>
					</div>
				{/if}

				{#if path}
					<a
						href={segments.length > 1 ? `/${username}/${repo}/tree/${segments.slice(0, -1).join('/')}${ref ? `?ref=${ref}` : ''}` : `/${username}/${repo}`}
						class="flex items-center gap-3 px-4 py-2 hover:bg-card border-b border-secondary transition-colors"
					>
						<Folder class="h-4 w-4 text-primary" />
						<span class="text-sm text-muted-foreground">..</span>
					</a>
				{/if}

				<div class="divide-y divide-secondary">
					{#each sortedFiles as file}
						{@const commitMessage = file.commit_message ?? file.message ?? ''}
						{@const commitDate = file.commit_date ?? file.date}
						{@const commitSHA = file.commit_sha ?? ''}
						<div class="flex items-center gap-3 px-4 py-2 hover:bg-card transition-colors group">
							<a href="/{username}/{repo}/{file.type === 'tree' ? 'tree' : 'blob'}/{file.path}{ref ? `?ref=${ref}` : ''}" class="flex min-w-0 flex-1 items-center gap-3">
								<span class="shrink-0">
									{#if file.type === 'tree'}
										{#if resolveRepoTreeIconUrl(file.name, 'tree')}
											<img src={resolveRepoTreeIconUrl(file.name, 'tree')} alt="" class="h-4 w-4" />
										{:else}
											<Folder class="h-4 w-4 text-primary" />
										{/if}
									{:else}
										{#if resolveRepoTreeIconUrl(file.name, 'blob')}
											<img src={resolveRepoTreeIconUrl(file.name, 'blob')} alt="" class="h-4 w-4" />
										{:else}
											<File class="h-4 w-4 text-muted-foreground" />
										{/if}
									{/if}
								</span>
								<span class="text-sm group-hover:underline truncate">{file.name}</span>
							</a>
							{#if commitMessage}
								{#if commitSHA}
									<a href="/{username}/{repo}/commit/{commitSHA}" class="text-xs text-muted-foreground hidden md:block truncate max-w-xs hover:text-primary hover:underline"
										>{commitMessage}</a
									>
								{:else}
									<span class="text-xs text-muted-foreground hidden md:block truncate max-w-xs">{commitMessage}</span>
								{/if}
							{/if}
							<span class="text-xs text-muted-foreground shrink-0 ml-4">{commitDate ? timeAgo(commitDate) : ''}</span>
						</div>
					{/each}
				</div>
			</div>
		{/if}
	</div>
</div>
