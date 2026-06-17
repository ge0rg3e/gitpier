<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { repos, pullRequests, type CommitInfo, type FileDiff, type Collaborator, type Repository, type User } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { timeAgo, commitAuthorAvatarUrl, commitAuthorInitial, commitAuthorName } from '$lib/utils';
	import { GitPullRequestArrow, ArrowLeft, ArrowLeftRight, AlertCircle, GitCommitHorizontal, FileCode, Loader, GitMerge, Users, Check, ChevronDown } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { renderMarkdownHtml } from '$lib/markdown';
	import DiffViewer from '$lib/components/DiffViewer.svelte';
	import SearchSelect from '$lib/components/SearchSelect.svelte';
	import { getContext } from 'svelte';
	import { handleMarkdownPaste, handleMarkdownDrop, handleMarkdownDragOver, openMarkdownAssetPicker } from '$lib/hooks/markdown-assets';
	import { mentionAutocomplete } from '$lib/hooks/mention-autocomplete';
	import { mentionHoverCard } from '$lib/hooks/mention-hover-card';

	let loading = $state(true);
	let branches = $state<string[]>([]);
	let headBranches = $state<string[]>([]);
	let repoData = $state<any>(null);
	let collaborators = $state<Collaborator[]>([]);
	let headRepoOptions = $state<Array<{ id: string; owner: string; name: string; defaultBranch: string; label: string }>>([]);

	let baseRef = $state('');
	let headRepoID = $state('');
	let headRef = $state('');

	// Track current compare key to prevent re-triggering
	let compareKey = $state('');

	// Phase: 'compare' = show comparison; 'form' = show PR form
	let phase = $state<'compare' | 'form'>('compare');

	// Comparison data
	let comparing = $state(false);
	let compareData = $state<{ commits: CommitInfo[]; files: FileDiff[]; mergeable: boolean; contributors: number } | null>(null);
	let compareError = $state('');

	// PR form
	let title = $state('');
	let description = $state('');
	let descriptionTab = $state<'write' | 'preview'>('write');
	let descriptionTextareaEl = $state<HTMLTextAreaElement | null>(null);
	let markdownUploading = $state(false);
	let markdownUploadError = $state('');
	let isDraft = $state(false);
	let submitting = $state(false);
	let submitError = $state('');
	let showDraftDropdown = $state(false);

	const { username, repo } = $derived(page.params);
	const currentRepoID = $derived(String(repoData?.id ?? ''));
	const sameBranchSelection = $derived(headRepoID === currentRepoID && headRef === baseRef);
	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const isLoggedIn = $derived(authStore.user != null);
	const possibleAssignees = $derived.by(() => {
		const result: User[] = [];
		if (authStore.user) result.push(authStore.user);
		for (const c of collaborators) {
			if (c.user && c.user_id !== authStore.user?.id) result.push(c.user);
		}
		return result;
	});
	const mentionUsers = $derived(possibleAssignees.map((u) => ({ username: u.username, avatar_url: u.avatar_url })));

	async function loadBranches() {
		loading = true;
		try {
			const [repoResp, branchesResp, collabResp, forksResp] = await Promise.all([
				repos.get(username!, repo!),
				repos.branches.list(username!, repo!),
				repos.collaborators.list(username!, repo!).catch(() => ({ collaborators: [] })),
				repos.forks.list(username!, repo!, 200).catch(() => ({ forks: [] as Repository[] }))
			]);
			repoData = repoResp.repo;
			branches = branchesResp.branches;
			collaborators = collabResp.collaborators ?? [];
			baseRef = repoData.default_branch || branches[0] || 'main';

			const currentOption = {
				id: String(repoResp.repo.id),
				owner: repoResp.repo.org?.login ?? repoResp.repo.owner.username,
				name: repoResp.repo.name,
				defaultBranch: repoResp.repo.default_branch || 'main',
				label: `${repoResp.repo.org?.login ?? repoResp.repo.owner.username}/${repoResp.repo.name}`
			};

			const options = [currentOption];
			for (const forkRepo of forksResp.forks ?? []) {
				const forkID = String(forkRepo.id);
				if (options.some((o) => o.id === forkID)) continue;
				options.push({
					id: forkID,
					owner: forkRepo.org?.login ?? forkRepo.owner.username,
					name: forkRepo.name,
					defaultBranch: forkRepo.default_branch || 'main',
					label: `${forkRepo.org?.login ?? forkRepo.owner.username}/${forkRepo.name}`
				});
			}
			headRepoOptions = options;

			const preferredFork = options.find((o) => o.id !== currentOption.id && o.owner === authStore.user?.username);
			await selectHeadRepo(preferredFork?.id ?? currentOption.id);
		} catch (e: any) {
			compareError = e.message;
		} finally {
			loading = false;
		}
	}

	async function selectHeadRepo(repoID: string) {
		headRepoID = repoID;
		const selected = headRepoOptions.find((o) => o.id === repoID);
		if (!selected) {
			headBranches = [];
			headRef = '';
			return;
		}

		let candidateBranches: string[] = [];
		if (repoID === currentRepoID) {
			candidateBranches = branches;
		} else {
			const resp = await repos.branches.list(selected.owner, selected.name);
			candidateBranches = resp.branches ?? [];
		}
		headBranches = candidateBranches;

		const currentHeadRef = headRef;
		headRef = candidateBranches.includes(currentHeadRef)
			? currentHeadRef
			: candidateBranches.find((b) => b !== baseRef) ?? candidateBranches[0] ?? '';
	}

	$effect(() => {
		if (username && repo) loadBranches();
	});

	$effect(() => {
		const key = headRepoID + '::' + baseRef + '::' + headRef;
		if (baseRef && headRef && !sameBranchSelection && key !== compareKey) {
			compareKey = key;
			runCompare();
		}
	});

	async function runCompare() {
		comparing = true;
		compareError = '';
		compareData = null;
		try {
			compareData = await repos.compare(username!, repo!, baseRef, headRef, headRepoID && headRepoID !== currentRepoID ? headRepoID : undefined);
			// Pre-fill title from latest commit message if comparison has commits
			if (compareData?.commits?.length && !title) {
				title = compareData.commits[0].message.split('\n')[0];
			}
		} catch (e: any) {
			compareError = e.message;
		} finally {
			comparing = false;
		}
	}

	async function handleSubmit() {
		if (isRepoArchived) {
			submitError = 'This repository is archived and read-only.';
			return;
		}
		if (!title.trim()) return;
		submitting = true;
		submitError = '';
		try {
			const pr = await pullRequests.create(username!, repo!, {
				title: title.trim(),
				description: description || undefined,
				head_ref: headRef,
				base_ref: baseRef,
				head_repo_id: headRepoID && headRepoID !== currentRepoID ? headRepoID : undefined,
				is_draft: isDraft
			});
			goto(`/${username}/${repo}/pulls/${pr.number || pr.id}`);
		} catch (e: any) {
			submitError = e.message;
		} finally {
			submitting = false;
		}
	}

	const identical = $derived(compareData?.commits?.length === 0 && compareData?.files?.length === 0);
	const canCreate = $derived(!!headRef && !sameBranchSelection && compareData !== null && !identical);

	const renderedPreview = $derived(renderMarkdownHtml(description));

	function descriptionField() {
		if (!descriptionTextareaEl) return null;
		return {
			username: username!,
			repo: repo!,
			textarea: descriptionTextareaEl,
			getValue: () => description,
			setValue: (next: string) => (description = next),
			onUploadState: (uploading: boolean) => (markdownUploading = uploading),
			onError: (message: string) => (markdownUploadError = message)
		};
	}

	async function handleDescriptionPaste(e: ClipboardEvent) {
		const field = descriptionField();
		if (!field) return;
		await handleMarkdownPaste(e, field);
	}

	async function handleDescriptionDrop(e: DragEvent) {
		const field = descriptionField();
		if (!field) return;
		await handleMarkdownDrop(e, field);
	}

	async function pickDescriptionFiles() {
		const field = descriptionField();
		if (!field) return;
		await openMarkdownAssetPicker(field);
	}

	// Total additions/deletions from files
	const totalAdditions = $derived((compareData?.files ?? []).reduce((s, f) => s + (f.additions || 0), 0));
	const totalDeletions = $derived((compareData?.files ?? []).reduce((s, f) => s + (f.deletions || 0), 0));
</script>

<svelte:head>
	<title>Compare · {username}/{repo} · GitPier</title>
</svelte:head>

{#if !isLoggedIn}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">You must be signed in to create a pull request.</div>
{:else if isRepoArchived}
	<div class="rounded-md border border-amber-700/40 bg-amber-900/20 p-4 text-sm text-amber-300">This repository is archived and read-only. New pull requests cannot be created.</div>
{:else if loading}
	<div class="space-y-4">
		<div class="h-7 w-56 rounded bg-card animate-pulse"></div>
		<div class="h-12 rounded-md border border-border bg-card animate-pulse"></div>
	</div>
{:else}
	<!-- ── Header ─────────────────────────────────────────────────────────── -->
	<div class="mb-1">
		<h2 class="text-2xl font-semibold text-foreground">
			{phase === 'compare' ? 'Compare changes' : 'Open a pull request'}
		</h2>
		<p class="text-sm text-muted-foreground mt-1">
			{#if phase === 'compare'}
				Choose two branches to see what's changed or to start a new pull request.
			{:else}
				Create a new pull request by comparing changes across two branches.
			{/if}
		</p>
	</div>

	<hr class="border-border my-4" />

	<!-- ── Branch selector row ───────────────────────────────────────────── -->
	<div class="rounded-md border border-border bg-card px-4 py-3 flex items-center gap-3 flex-wrap mb-4">
		<ArrowLeftRight class="h-4 w-4 text-muted-foreground shrink-0" />

		<!-- base -->
		<div class="flex items-center gap-1.5">
			<span class="text-sm text-muted-foreground">base:</span>
			<SearchSelect bind:value={baseRef} options={branches.map((b) => ({ value: b }))} size="sm" />
		</div>

		<span class="text-muted-foreground text-lg">...</span>

		<!-- compare -->
		<div class="flex items-center gap-1.5 flex-wrap">
			<span class="text-sm text-muted-foreground">compare:</span>
			<SearchSelect
				bind:value={headRepoID}
				options={headRepoOptions.map((r) => ({ value: r.id, label: r.label }))}
				size="sm"
				onchange={(value) => {
					void selectHeadRepo(value).catch((e) => {
						compareError = e?.message ?? 'Failed to load branches';
						headBranches = [];
						headRef = '';
					});
				}}
			/>
			<span class="text-sm text-muted-foreground">:</span>
			<SearchSelect bind:value={headRef} options={[{ value: '', label: 'Choose a branch…' }, ...headBranches.map((b) => ({ value: b }))]} size="sm" />
		</div>

		<!-- Able to merge indicator (when diff loaded) -->
		{#if compareData && !identical}
			{#if compareData.mergeable}
				<span class="flex items-center gap-1.5 text-sm text-[#3fb950]">
					<Check class="h-4 w-4" />
					Able to merge. These branches can be automatically merged.
				</span>
			{:else}
				<span class="flex items-center gap-1.5 text-sm text-[#d29922]">
					<AlertCircle class="h-4 w-4" />
					Can't automatically merge. Resolve conflicts manually.
				</span>
			{/if}
		{/if}
	</div>

	<!-- ── COMPARE PHASE ─────────────────────────────────────────────────── -->
	{#if phase === 'compare'}
		{#if comparing}
			<div class="flex items-center justify-center py-16">
				<Loader class="h-6 w-6 animate-spin text-muted-foreground" />
			</div>
		{:else if !headRef || sameBranchSelection}
			<!-- No head selected yet or same branch -->
			<div class="rounded-md border border-[#5a3e1b]/60 bg-[#1f1500]/60 px-5 py-4 flex items-center justify-between">
				<span class="text-sm text-foreground">
					Choose different branches or forks above to discuss and review changes.
					<a href="https://docs.github.com/pull-requests" target="_blank" class="text-primary hover:underline ml-1">Learn about pull requests</a>
				</span>
				<Button variant="brand" size="sm" disabled>Create pull request</Button>
			</div>

			<!-- Placeholder icon -->
			<div class="flex justify-center py-16">
				<GitPullRequestArrow class="h-12 w-12 text-muted-foreground/30" />
			</div>
		{:else if compareData}
			{#if identical}
				<!-- Identical branches -->
				<div class="text-center py-16">
					<GitPullRequestArrow class="mx-auto h-12 w-12 text-muted-foreground mb-4" />
					<h3 class="text-lg font-semibold text-foreground mb-1">There isn't anything to compare.</h3>
					<p class="text-sm text-muted-foreground"><strong>{baseRef}</strong> and <strong>{headRef}</strong> are identical.</p>
				</div>
				<!-- Stats bar even when identical -->
				<div class="rounded-md border border-border overflow-hidden mb-4">
					<div class="flex divide-x divide-border bg-card">
						<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
							<GitCommitHorizontal class="h-4 w-4" /> 0 commits
						</div>
						<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
							<FileCode class="h-4 w-4" /> 0 files changed
						</div>
						<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
							<Users class="h-4 w-4" /> 0 contributors
						</div>
					</div>
				</div>
				<div class="text-sm text-muted-foreground mb-3">
					Showing <span class="font-semibold text-foreground">0 changed files</span>
					with <span class="text-[#3fb950] font-semibold">0 additions</span> and
					<span class="text-[#f85149] font-semibold">0 deletions</span>.
				</div>
			{:else}
				<!-- Has differences -->
				<div class="rounded-md border border-[#163b6e]/60 bg-[#0d1f38]/60 px-5 py-4 flex items-center justify-between mb-4">
					<span class="text-sm text-foreground">
						Discuss and review the changes in this comparison with others.
						<a href="https://docs.github.com/pull-requests" target="_blank" class="text-primary hover:underline ml-1">Learn about pull requests</a>
					</span>
					<Button
						variant="brand"
						size="sm"
						onclick={() => {
							phase = 'form';
						}}
					>
						Create pull request
					</Button>
				</div>

				<!-- Stats bar -->
				<div class="rounded-md border border-border overflow-hidden mb-4">
					<div class="flex divide-x divide-border bg-card">
						<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
							<GitCommitHorizontal class="h-4 w-4" />
							<span><span class="font-semibold text-foreground">{compareData.commits.length}</span> commit{compareData.commits.length !== 1 ? 's' : ''}</span>
						</div>
						<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
							<FileCode class="h-4 w-4" />
							<span><span class="font-semibold text-foreground">{compareData.files.length}</span> file{compareData.files.length !== 1 ? 's' : ''} changed</span>
						</div>
						<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
							<Users class="h-4 w-4" />
							<span><span class="font-semibold text-foreground">{compareData.contributors}</span> contributor{compareData.contributors !== 1 ? 's' : ''}</span>
						</div>
					</div>
				</div>

				<!-- Commits list -->
				{#if compareData.commits.length > 0}
					<!-- Group header (date from first commit) -->
					<div class="flex items-center gap-2 mb-2 text-xs text-muted-foreground">
						<GitCommitHorizontal class="h-3.5 w-3.5" />
						Commits on {new Date(compareData.commits[0].author.date).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
					</div>
					<div class="rounded-md border border-border bg-card overflow-hidden mb-4">
						{#each compareData.commits as commit, i}
							<a
								href="/{username}/{repo}/commit/{commit.sha}"
								class="flex items-center gap-3 px-4 py-3 hover:bg-secondary transition-colors
{i < compareData.commits.length - 1 ? 'border-b border-border' : ''}"
							>
								<GitCommitHorizontal class="h-4 w-4 text-muted-foreground shrink-0" />
								<span class="text-sm font-semibold text-foreground flex-1 truncate">{commit.message.split('\n')[0]}</span>
								<span class="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
									<div class="flex h-5 w-5 items-center justify-center overflow-hidden rounded-full border border-border bg-secondary text-[11px] font-semibold text-foreground">
										{#if commitAuthorAvatarUrl(commit.author)}
											<img src={commitAuthorAvatarUrl(commit.author)} alt={commitAuthorName(commit.author)} class="h-full w-full object-cover" />
										{:else}
											{commitAuthorInitial(commit.author)}
										{/if}
									</div>
									<span title={commit.author?.username ? undefined : 'Not registered on GitPier'}>{commitAuthorName(commit.author)} · {timeAgo(commit.author.date)}</span>
								</span>
								<span class="font-mono text-xs text-primary shrink-0 bg-secondary px-2 py-0.5 rounded">{commit.sha.slice(0, 7)}</span>
							</a>
						{/each}
					</div>
				{/if}

				<!-- Files changed summary + diff -->
				<div class="flex items-center justify-between mb-3 text-sm text-muted-foreground">
					<span>
						Showing <span class="font-semibold text-foreground">{compareData.files.length} changed file{compareData.files.length !== 1 ? 's' : ''}</span>
						with <span class="text-[#3fb950] font-semibold">{totalAdditions} addition{totalAdditions !== 1 ? 's' : ''}</span> and
						<span class="text-[#f85149] font-semibold">{totalDeletions} deletion{totalDeletions !== 1 ? 's' : ''}</span>.
					</span>
					<div class="flex rounded overflow-hidden border border-border text-xs">
						<button class="px-3 py-1 bg-card text-muted-foreground cursor-default">Split</button>
						<button class="px-3 py-1 bg-secondary text-foreground font-semibold border-l border-border">Unified</button>
					</div>
				</div>

				{#each compareData.files as file}
					<div class="mb-3 rounded-md border border-border overflow-hidden">
						<div class="flex items-center gap-3 px-4 py-2.5 bg-accent border-b border-border">
							<span class="text-sm font-semibold text-foreground mr-1">▾ {compareData.files.indexOf(file) + 1}</span>
							<div class="flex-1 min-w-0">
								<span class="font-mono text-sm text-foreground truncate">
									{#if file.old_path && file.old_path !== file.path}
										<span class="text-muted-foreground">{file.old_path}</span>
										<span class="mx-1">→</span>
									{/if}
									{file.path}
								</span>
							</div>
							<div class="flex items-center gap-1.5 text-xs shrink-0">
								<span class="text-[#3fb950]">+{file.additions}</span>
								<span class="text-muted-foreground">−</span>
								<span class="text-[#f85149]">{file.deletions}</span>
							</div>
						</div>
						{#if file.patch}
							<DiffViewer patch={file.patch} filePath={file.path} />
						{:else}
							<div class="border-t border-border px-4 py-6 text-center text-sm text-muted-foreground bg-background">No preview available.</div>
						{/if}
					</div>
				{/each}
			{/if}
		{/if}

		<!-- ── FORM PHASE ─────────────────────────────────────────────────────── -->
	{:else}
		<div class="flex gap-6">
			<!-- Left: form + preview -->
			<div class="flex-1 min-w-0">
				<!-- Avatar + form -->
				<div class="flex gap-3">
					<div class="w-9 h-9 rounded-full bg-primary/20 flex items-center justify-center text-primary font-bold text-sm shrink-0">
						{(authStore.user?.username ?? 'U')[0].toUpperCase()}
					</div>
					<div class="flex-1 rounded-md border border-border overflow-hidden">
						<!-- Title -->
						<div class="border-b border-border p-3">
							<input
								type="text"
								bind:value={title}
								required
								placeholder="Add a title *"
								class="w-full bg-transparent text-sm font-semibold text-foreground placeholder:text-muted-foreground focus:outline-none"
							/>
						</div>

						<!-- Description with Write/Preview tabs -->
						<div>
							<div class="flex border-b border-border px-3 pt-2 gap-0">
								<button
									onclick={() => (descriptionTab = 'write')}
									class="px-3 py-1.5 text-xs rounded-t border {descriptionTab === 'write'
										? 'border-border bg-card border-b-card text-foreground font-semibold'
										: 'border-transparent text-muted-foreground hover:text-foreground'}">Write</button
								>
								<button
									onclick={() => (descriptionTab = 'preview')}
									class="px-3 py-1.5 text-xs rounded-t border {descriptionTab === 'preview'
										? 'border-border bg-card border-b-card text-foreground font-semibold'
										: 'border-transparent text-muted-foreground hover:text-foreground'}">Preview</button
								>
							</div>

							{#if descriptionTab === 'write'}
								<textarea
									bind:this={descriptionTextareaEl}
									bind:value={description}
									use:mentionAutocomplete={{ users: mentionUsers }}
									rows="10"
									placeholder="Add your description here..."
									onpaste={handleDescriptionPaste}
									ondragover={handleMarkdownDragOver}
									ondrop={handleDescriptionDrop}
									class="w-full bg-transparent px-3 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none resize-y"
								></textarea>
							{:else}
								<div
									use:mentionHoverCard
									class="px-3 py-3 min-h-40 prose prose-invert max-w-none text-sm
prose-headings:text-foreground prose-a:text-primary
prose-code:text-foreground prose-code:bg-secondary prose-code:rounded prose-code:px-1
prose-pre:bg-secondary prose-pre:border prose-pre:border-border
prose-strong:text-foreground"
								>
									{#if renderedPreview}
										{@html renderedPreview}
									{:else}
										<span class="text-muted-foreground italic text-xs">Nothing to preview.</span>
									{/if}
								</div>
							{/if}

							<div class="flex items-center justify-between px-3 py-2 border-t border-border bg-secondary/30 text-xs text-muted-foreground">
								<button type="button" class="hover:text-foreground transition-colors" onclick={pickDescriptionFiles} disabled={markdownUploading}>
									{markdownUploading ? 'Uploading…' : 'Paste, drop, or click to add files'}
								</button>
								{#if markdownUploadError}
									<span class="text-red-400">{markdownUploadError}</span>
								{/if}
							</div>
						</div>

						<!-- Submit area -->
						{#if submitError}
							<div class="px-3 py-2 border-t border-border text-sm text-red-400 flex items-center gap-2">
								<AlertCircle class="h-4 w-4 shrink-0" />{submitError}
							</div>
						{/if}

						<div class="flex items-center justify-end gap-2 px-3 py-3 border-t border-border">
							<Button variant="ghost" size="sm" onclick={() => (phase = 'compare')}>
								<ArrowLeft class="h-4 w-4" />
								Back
							</Button>

							<!-- Split button: create / create draft -->
							<div class="flex items-stretch">
								<Button variant="brand" size="sm" disabled={submitting || !title.trim()} onclick={handleSubmit} class="rounded-r-none">
									{#if submitting}<Loader class="h-4 w-4 animate-spin" />{/if}
									{isDraft ? 'Create draft pull request' : 'Create pull request'}
								</Button>
								<button
									onclick={() => (showDraftDropdown = !showDraftDropdown)}
									class="px-2 border-l border-brand/60 bg-brand hover:bg-[#2ea043] text-white rounded-r-md transition-colors text-xs"
									disabled={submitting}
								>
									<ChevronDown class="h-4 w-4" />
								</button>
							</div>

							{#if showDraftDropdown}
								<div class="absolute z-20 mt-8 w-64 rounded-md border border-border bg-popover shadow-lg p-1">
									<button
										onclick={() => {
											isDraft = false;
											showDraftDropdown = false;
										}}
										class="w-full flex items-start gap-2 px-3 py-2 text-left rounded hover:bg-secondary transition-colors"
									>
										{#if !isDraft}<Check class="h-4 w-4 text-primary mt-0.5" />{:else}<div class="h-4 w-4 mt-0.5"></div>{/if}
										<div>
											<div class="text-sm font-semibold text-foreground">Create pull request</div>
											<div class="text-xs text-muted-foreground">Opens a pull request ready to review.</div>
										</div>
									</button>
									<button
										onclick={() => {
											isDraft = true;
											showDraftDropdown = false;
										}}
										class="w-full flex items-start gap-2 px-3 py-2 text-left rounded hover:bg-secondary transition-colors"
									>
										{#if isDraft}<Check class="h-4 w-4 text-primary mt-0.5" />{:else}<div class="h-4 w-4 mt-0.5"></div>{/if}
										<div>
											<div class="text-sm font-semibold text-foreground">Create draft pull request</div>
											<div class="text-xs text-muted-foreground">Cannot be merged until marked ready for review.</div>
										</div>
									</button>
								</div>
							{/if}
						</div>
					</div>
				</div>

				<!-- Commits + files preview below form -->
				{#if compareData}
					<!-- Stats bar -->
					<div class="mt-6 rounded-md border border-border overflow-hidden">
						<div class="flex divide-x divide-border bg-card">
							<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
								<GitCommitHorizontal class="h-4 w-4" />
								<span><span class="font-semibold text-foreground">{compareData.commits.length}</span> commit{compareData.commits.length !== 1 ? 's' : ''}</span>
							</div>
							<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
								<FileCode class="h-4 w-4" />
								<span><span class="font-semibold text-foreground">{compareData.files.length}</span> file{compareData.files.length !== 1 ? 's' : ''} changed</span>
							</div>
							<div class="flex-1 flex items-center justify-center gap-2 py-2.5 text-sm text-muted-foreground">
								<Users class="h-4 w-4" />
								<span><span class="font-semibold text-foreground">{compareData.contributors}</span> contributor{compareData.contributors !== 1 ? 's' : ''}</span>
							</div>
						</div>
					</div>

					<!-- Commits -->
					{#if compareData.commits.length > 0}
						<div class="mt-4">
							<div class="flex items-center gap-2 mb-2 text-xs text-muted-foreground">
								<GitCommitHorizontal class="h-3.5 w-3.5" />
								Commits on {new Date(compareData.commits[0].author.date).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
							</div>
							<div class="rounded-md border border-border bg-card overflow-hidden">
								{#each compareData.commits as commit, i}
									<a
										href="/{username}/{repo}/commit/{commit.sha}"
										class="flex items-center gap-3 px-4 py-3 hover:bg-secondary transition-colors
{i < compareData.commits.length - 1 ? 'border-b border-border' : ''}"
									>
										<GitCommitHorizontal class="h-4 w-4 text-muted-foreground shrink-0" />
										<span class="text-sm font-semibold text-foreground flex-1 truncate">{commit.message.split('\n')[0]}</span>
										<span class="flex shrink-0 items-center gap-2 text-xs text-muted-foreground">
											<div
												class="flex h-5 w-5 items-center justify-center overflow-hidden rounded-full border border-border bg-secondary text-[11px] font-semibold text-foreground"
											>
												{#if commitAuthorAvatarUrl(commit.author)}
													<img src={commitAuthorAvatarUrl(commit.author)} alt={commitAuthorName(commit.author)} class="h-full w-full object-cover" />
												{:else}
													{commitAuthorInitial(commit.author)}
												{/if}
											</div>
											<span title={commit.author?.username ? undefined : 'Not registered on GitPier'}>{commitAuthorName(commit.author)} · {timeAgo(commit.author.date)}</span>
										</span>
										<span class="font-mono text-xs text-primary shrink-0 bg-secondary px-2 py-0.5 rounded">{commit.sha.slice(0, 7)}</span>
									</a>
								{/each}
							</div>
						</div>
					{/if}
				{/if}
			</div>

			<!-- Right: sidebar -->
			<div class="w-60 shrink-0 space-y-4">
				{#each [{ label: 'Reviewers', value: 'No reviews' }, { label: 'Assignees', value: 'No one—assign yourself' }, { label: 'Labels', value: 'None yet' }, { label: 'Projects', value: 'None yet' }] as item}
					<div class="border-b border-border pb-4">
						<div class="flex items-center justify-between mb-1.5">
							<span class="text-xs font-semibold text-foreground">{item.label}</span>
							<button class="text-muted-foreground hover:text-foreground transition-colors p-0.5">
								<svg class="h-3.5 w-3.5" viewBox="0 0 16 16" fill="currentColor">
									<path d="M8 9.5a1.5 1.5 0 1 0 0-3 1.5 1.5 0 0 0 0 3ZM8 0a8 8 0 1 1 0 16A8 8 0 0 1 8 0Zm0 1.5a6.5 6.5 0 1 0 0 13 6.5 6.5 0 0 0 0-13Z" />
								</svg>
							</button>
						</div>
						<span class="text-xs text-muted-foreground">{item.value}</span>
					</div>
				{/each}

				<div class="border-b border-border pb-4">
					<div class="text-xs font-semibold text-foreground mb-1.5">Development</div>
					<p class="text-xs text-muted-foreground">
						Use <span class="text-primary">closing keywords</span> in the description to automatically close issues.
					</p>
				</div>

				<div>
					<div class="text-xs font-semibold text-foreground mb-2">Helpful resources</div>
					<a href="https://docs.github.com/pull-requests" target="_blank" class="block text-xs text-primary hover:underline"> GitHub Community Guidelines </a>
				</div>
			</div>
		</div>
	{/if}
{/if}
