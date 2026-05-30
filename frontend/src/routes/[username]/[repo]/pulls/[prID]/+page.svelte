<script lang="ts">
	import { page } from '$app/state';
	import {
		pullRequests,
		labels,
		repos,
		type PullRequest,
		type CommitInfo,
		type PRComment,
		type PRReview,
		type FileDiff,
		type Label,
		type Collaborator,
		type User
	} from '$lib/api/client';
	import { timeAgo, mediaUrl, commitAuthorAvatarUrl, commitAuthorInitial, commitAuthorName } from '$lib/utils';
	import { authStore } from '$lib/stores/auth.svelte';
	import {
		GitMerge,
		XCircle,
		CircleDot,
		AlertCircle,
		RotateCcw,
		GitBranch,
		GitCommitHorizontal,
		FileCode,
		MessageSquare,
		ChevronDown,
		Check,
		X,
		Pencil,
		Trash2,
		Send,
		Loader,
		Users,
		Settings
	} from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { renderMarkdownHtml } from '$lib/markdown';
	import DiffViewer from '$lib/components/DiffViewer.svelte';
	import { getContext } from 'svelte';
	import { handleMarkdownPaste, handleMarkdownDrop, handleMarkdownDragOver, openMarkdownAssetPicker } from '$lib/hooks/markdown-assets';
	import { mentionAutocomplete } from '$lib/hooks/mention-autocomplete';
	import { mentionHoverCard } from '$lib/hooks/mention-hover-card';

	let pr = $state<PullRequest | null>(null);
	let mergeable = $state(false);
	let loading = $state(true);
	let error = $state('');
	let actionLoading = $state(false);
	let actionError = $state('');

	let activeTab = $state<'conversation' | 'commits' | 'files'>('conversation');

	let commits = $state<CommitInfo[]>([]);
	let commitsLoading = $state(false);
	let commitsLoaded = $state(false);

	let files = $state<FileDiff[]>([]);
	let filesLoading = $state(false);
	let filesLoaded = $state(false);
	let expandedFiles = $state(new Set<string>());

	let comments = $state<PRComment[]>([]);
	let reviews = $state<PRReview[]>([]);
	let convLoading = $state(false);
	let convLoaded = $state(false);

	let newCommentBody = $state('');
	let newCommentTab = $state<'write' | 'preview'>('write');
	let commentSubmitting = $state(false);
	let editingCommentID = $state<number | null>(null);
	let editingCommentBody = $state('');
	let editCommentTab = $state<'write' | 'preview'>('write');
	let newCommentTextareaEl = $state<HTMLTextAreaElement | null>(null);
	let reviewTextareaEl = $state<HTMLTextAreaElement | null>(null);
	let markdownUploading = $state(false);
	let markdownUploadError = $state('');

	let mergeMethod = $state<'merge' | 'squash' | 'rebase'>('merge');
	let showMergeOptions = $state(false);

	let reviewState = $state<'APPROVED' | 'CHANGES_REQUESTED' | 'COMMENTED'>('COMMENTED');
	let reviewBody = $state('');
	let reviewSubmitting = $state(false);
	let showReviewForm = $state(false);

	let labelList = $state<Label[]>([]);
	let collaborators = $state<Collaborator[]>([]);
	let openSidebarDropdown = $state<'assignees' | 'labels' | null>(null);
	let selectedLabelIds = $state<number[]>([]);
	let labelUpdating = $state(false);
	let showCreateLabel = $state(false);
	let newLabelName = $state('');
	let newLabelColor = $state('#1f6feb');
let labelCreating = $state(false);

	const { username, repo } = $derived(page.params);
	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const prNumber = $derived(Number(page.params.prID || page.params.number));
	const isLoggedIn = $derived(authStore.user != null);
	const isAuthor = $derived(isLoggedIn && pr?.author_id === authStore.user?.id);
	const isOwner = $derived(isLoggedIn && pr?.repo?.owner_id === authStore.user?.id);
const canMerge = $derived(isOwner && !isRepoArchived);
const canClose = $derived((isAuthor || isOwner) && !isRepoArchived);
const isCollaborator = $derived(!!authStore.user && collaborators.some((c) => c.user_id === authStore.user!.id));
const canSubmitDecisionReview = $derived(isOwner || isCollaborator);
const reviewOptions = $derived.by(() => {
	const all = [
		{ state: 'APPROVED' as const, label: 'Approve', icon: Check },
		{ state: 'CHANGES_REQUESTED' as const, label: 'Request changes', icon: X },
		{ state: 'COMMENTED' as const, label: 'Comment', icon: MessageSquare }
	];
	if (canSubmitDecisionReview) return all;
	return all.filter((opt) => opt.state === 'COMMENTED');
});
const headRepoIsDifferent = $derived(!!pr?.head_repo_id && pr.head_repo_id !== pr.repo_id);
	const headBranchLabel = $derived.by(() => {
		if (!pr) return '';
		if (!headRepoIsDifferent) return pr.head_ref;
		const ns = pr.head_repo?.org?.login ?? pr.head_repo?.owner?.username ?? username!;
		const name = pr.head_repo?.name ?? repo!;
		return `${ns}/${name}:${pr.head_ref}`;
	});
	const baseBranchLabel = $derived.by(() => {
		if (!pr) return '';
		const ns = pr.repo?.org?.login ?? pr.repo?.owner?.username ?? username!;
		const name = pr.repo?.name ?? repo!;
		return headRepoIsDifferent ? `${ns}/${name}:${pr.base_ref}` : pr.base_ref;
	});
	const possibleAssignees = $derived.by(() => {
		const result: User[] = [];
		if (authStore.user) result.push(authStore.user);
		for (const c of collaborators) {
			if (c.user && c.user_id !== authStore.user?.id) result.push(c.user);
		}
		return result;
	});
	const mentionUsers = $derived(possibleAssignees.map((u) => ({ username: u.username, avatar_url: u.avatar_url })));

	async function loadPR() {
		loading = true;
		error = '';
		try {
			const data = await pullRequests.get(username!, repo!, prNumber);
			pr = data.pull_request;
			mergeable = data.mergeable;
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		if (prNumber) loadPR();
	});

	// Load conversation when PR first loads
	$effect(() => {
		if (pr && !convLoaded) {
			convLoaded = true;
			loadConversation();
		}
	});

	$effect(() => {
		if (pr && !commitsLoaded && !commitsLoading) {
			void loadCommits();
		}
		if (pr && !filesLoaded && !filesLoading) {
			void loadFiles();
		}
	});

	$effect(() => {
		if (!canSubmitDecisionReview && reviewState !== 'COMMENTED') {
			reviewState = 'COMMENTED';
		}
	});

	async function loadConversation() {
		convLoading = true;
		try {
			const [cData, rData] = await Promise.all([pullRequests.listComments(username!, repo!, prNumber), pullRequests.listReviews(username!, repo!, prNumber)]);
			comments = cData.comments ?? [];
			reviews = rData.reviews ?? [];
		} catch {
		} finally {
			convLoading = false;
		}
	}

	async function loadCommits() {
		if (commitsLoading || commitsLoaded) return;
		commitsLoading = true;
		try {
			const data = await pullRequests.getCommits(username!, repo!, prNumber);
			commits = data.commits ?? [];
		} catch {
		} finally {
			commitsLoading = false;
			commitsLoaded = true;
		}
	}

	async function loadFiles() {
		if (filesLoading || filesLoaded) return;
		filesLoading = true;
		try {
			const data = await pullRequests.getFiles(username!, repo!, prNumber);
			files = data.files ?? [];
			if (files.length <= 10) expandedFiles = new Set(files.map((f) => f.path));
		} catch {
		} finally {
			filesLoading = false;
			filesLoaded = true;
		}
	}

	async function handleClose() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!confirm('Close this pull request?')) return;
		actionLoading = true;
		actionError = '';
		try {
			await pullRequests.close(username!, repo!, prNumber);
			await loadPR();
		} catch (e: any) {
			actionError = e.message;
		} finally {
			actionLoading = false;
		}
	}

	async function handleReopen() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		actionLoading = true;
		actionError = '';
		try {
			await pullRequests.reopen(username!, repo!, prNumber);
			await loadPR();
		} catch (e: any) {
			actionError = e.message;
		} finally {
			actionLoading = false;
		}
	}

	async function handleMerge() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!confirm(`Merge via "${mergeMethod}"?`)) return;
		actionLoading = true;
		actionError = '';
		try {
			await pullRequests.merge(username!, repo!, prNumber, mergeMethod);
			await loadPR();
		} catch (e: any) {
			actionError = e.message;
		} finally {
			actionLoading = false;
		}
	}

	async function submitComment() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!newCommentBody.trim()) return;
		commentSubmitting = true;
		try {
			const c = await pullRequests.createComment(username!, repo!, prNumber, newCommentBody.trim());
			comments = [...comments, c];
			newCommentBody = '';
			newCommentTab = 'write';
		} catch (e: any) {
			actionError = e.message;
		} finally {
			commentSubmitting = false;
		}
	}

	async function saveEditComment(c: PRComment) {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!editingCommentBody.trim()) return;
		try {
			const updated = await pullRequests.updateComment(username!, repo!, prNumber, c.id, editingCommentBody.trim());
			comments = comments.map((x) => (x.id === c.id ? updated : x));
			editingCommentID = null;
		} catch (e: any) {
			actionError = e.message;
		}
	}

	async function deleteComment(c: PRComment) {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!confirm('Delete this comment?')) return;
		try {
			await pullRequests.deleteComment(username!, repo!, prNumber, c.id);
			comments = comments.filter((x) => x.id !== c.id);
		} catch (e: any) {
			actionError = e.message;
		}
	}

	async function loadSidebarData() {
		try {
			const [labelData, collabData] = await Promise.all([
				labels.list(username!, repo!),
				repos.collaborators.list(username!, repo!).catch(() => ({ collaborators: [] }))
			]);
			labelList = labelData.labels ?? [];
			collaborators = collabData.collaborators ?? [];
		} catch {}
	}

	$effect(() => {
		if (pr && isLoggedIn) loadSidebarData();
	});

	$effect(() => {
		if (pr) selectedLabelIds = (pr.labels ?? []).map((l) => l.id);
	});

	function avatarLetter(u: string | undefined) {
		return (u ?? 'U')[0].toUpperCase();
	}

	function textColorForBg(hex: string): string {
		const r = parseInt(hex.slice(1, 3), 16);
		const g = parseInt(hex.slice(3, 5), 16);
		const b = parseInt(hex.slice(5, 7), 16);
		const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
		return luminance > 0.5 ? '#000000' : '#ffffff';
	}

	async function updateAssignee(userId: number) {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!pr) return;
		const isSame = pr.assignee_id === userId;
		try {
			const data = await pullRequests.update(username!, repo!, prNumber, isSame ? { clear_assignee: true } : { assignee_id: userId });
			pr = data.pull_request;
		} catch (e: any) {
			actionError = e.message;
		}
	}

	async function toggleLabel(labelId: number) {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		labelUpdating = true;
		const next = selectedLabelIds.includes(labelId) ? selectedLabelIds.filter((id) => id !== labelId) : [...selectedLabelIds, labelId];
		try {
			const data = await pullRequests.update(username!, repo!, prNumber, { label_ids: next });
			pr = data.pull_request;
			selectedLabelIds = (pr.labels ?? []).map((l) => l.id);
		} catch (e: any) {
			actionError = e.message;
		} finally {
			labelUpdating = false;
		}
	}

	async function createLabel() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!newLabelName.trim()) return;
		labelCreating = true;
		try {
			const data = await labels.create(username!, repo!, { name: newLabelName.trim(), color: newLabelColor });
			labelList = [...labelList, data.label];
			newLabelName = '';
			newLabelColor = '#1f6feb';
			showCreateLabel = false;
		} catch (e: any) {
			actionError = e.message;
		} finally {
			labelCreating = false;
		}
	}

	async function submitReview() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		reviewSubmitting = true;
		try {
			const state = canSubmitDecisionReview ? reviewState : 'COMMENTED';
			const r = await pullRequests.createReview(username!, repo!, prNumber, { state, body: reviewBody.trim() || undefined });
			reviews = [...reviews, r];
			reviewBody = '';
			showReviewForm = false;
			if (state !== 'COMMENTED') await loadPR();
		} catch (e: any) {
			actionError = e.message;
		} finally {
			reviewSubmitting = false;
		}
	}

	let diffMode = $state<'unified' | 'split'>('unified');
	function toggleFile(path: string) {
		const next = new Set(expandedFiles);
		if (next.has(path)) next.delete(path);
		else next.add(path);
		expandedFiles = next;
	}

	const renderedDescription = $derived(renderMarkdownHtml(pr?.description));
	const renderedNewCommentPreview = $derived(renderMarkdownHtml(newCommentBody));
	const renderedEditingCommentPreview = $derived(renderMarkdownHtml(editingCommentBody));

	const totalAdditions = $derived(files.reduce((s, f) => s + (f.additions || 0), 0));
	const totalDeletions = $derived(files.reduce((s, f) => s + (f.deletions || 0), 0));

	const latestReviewByUser = $derived(() => {
		const map = new Map<number, PRReview>();
		for (const r of reviews) map.set(r.author_id, r);
		return [...map.values()];
	});

	const hasApproval = $derived(latestReviewByUser().some((r) => r.state === 'APPROVED'));
	const hasChangesRequested = $derived(latestReviewByUser().some((r) => r.state === 'CHANGES_REQUESTED'));

	const mergeMethodInfo: Record<string, { label: string; desc: string }> = {
		merge: { label: 'Merge pull request', desc: 'Create a merge commit' },
		squash: { label: 'Squash and merge', desc: 'Combine commits into one' },
		rebase: { label: 'Rebase and merge', desc: 'Replay commits onto base' }
	};

	function markdownField(textarea: HTMLTextAreaElement, getValue: () => string, setValue: (next: string) => void) {
		return {
			username: username!,
			repo: repo!,
			textarea,
			getValue,
			setValue,
			onUploadState: (uploading: boolean) => (markdownUploading = uploading),
			onError: (message: string) => (markdownUploadError = message)
		};
	}

	async function handleNewCommentPaste(e: ClipboardEvent) {
		if (!newCommentTextareaEl) return;
		await handleMarkdownPaste(
			e,
			markdownField(
				newCommentTextareaEl,
				() => newCommentBody,
				(next) => (newCommentBody = next)
			)
		);
	}

	async function handleNewCommentDrop(e: DragEvent) {
		if (!newCommentTextareaEl) return;
		await handleMarkdownDrop(
			e,
			markdownField(
				newCommentTextareaEl,
				() => newCommentBody,
				(next) => (newCommentBody = next)
			)
		);
	}

	async function handleEditCommentPaste(e: ClipboardEvent) {
		const textarea = e.currentTarget as HTMLTextAreaElement | null;
		if (!textarea) return;
		await handleMarkdownPaste(
			e,
			markdownField(
				textarea,
				() => editingCommentBody,
				(next) => (editingCommentBody = next)
			)
		);
	}

	async function handleEditCommentDrop(e: DragEvent) {
		const textarea = e.currentTarget as HTMLTextAreaElement | null;
		if (!textarea) return;
		await handleMarkdownDrop(
			e,
			markdownField(
				textarea,
				() => editingCommentBody,
				(next) => (editingCommentBody = next)
			)
		);
	}

	async function handleReviewPaste(e: ClipboardEvent) {
		if (!reviewTextareaEl) return;
		await handleMarkdownPaste(
			e,
			markdownField(
				reviewTextareaEl,
				() => reviewBody,
				(next) => (reviewBody = next)
			)
		);
	}

	async function handleReviewDrop(e: DragEvent) {
		if (!reviewTextareaEl) return;
		await handleMarkdownDrop(
			e,
			markdownField(
				reviewTextareaEl,
				() => reviewBody,
				(next) => (reviewBody = next)
			)
		);
	}

	async function pickNewCommentFiles() {
		if (!newCommentTextareaEl) return;
		await openMarkdownAssetPicker(
			markdownField(
				newCommentTextareaEl,
				() => newCommentBody,
				(next) => (newCommentBody = next)
			)
		);
	}
</script>

<svelte:head>
	<title>Pull Request #{prNumber}{pr ? ' · ' + pr.title : ''}</title>
</svelte:head>

{#if loading}
	<div class="space-y-3">
		<div class="h-8 w-2/3 rounded bg-card animate-pulse"></div>
		<div class="h-48 rounded-md border border-border bg-card animate-pulse"></div>
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else if pr}
	{#if isRepoArchived}
		<div class="mb-4 rounded-md border border-amber-700/40 bg-amber-900/20 px-4 py-3 text-sm text-amber-300">This repository is archived. Pull request actions and comments are read-only.</div>
	{/if}

	<!-- ── Title + status row ─────────────────────────────────────────── -->
	<div class="mb-4 flex items-start justify-between gap-4 flex-wrap">
		<div>
			<h2 class="text-2xl font-semibold text-foreground leading-snug">
				{pr.title}
				<span class="text-muted-foreground font-normal">#{pr.number}</span>
			</h2>
			<div class="flex items-center gap-2 mt-1.5 flex-wrap text-sm text-muted-foreground">
				<!-- Status badge -->
				{#if pr.status === 'merged'}
					<span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-[#6e40c9] text-white">
						<GitMerge class="h-3.5 w-3.5" /> Merged
					</span>
				{:else if pr.status === 'closed'}
					<span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-[#da3633] text-white">
						<XCircle class="h-3.5 w-3.5" /> Closed
					</span>
				{:else if pr.is_draft}
					<span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-[#848d97] text-white">
						<CircleDot class="h-3.5 w-3.5" /> Draft
					</span>
				{:else}
					<span class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded-full text-xs font-semibold bg-brand text-white">
						<CircleDot class="h-3.5 w-3.5" /> Open
					</span>
				{/if}

				{#if pr.author}<a href="/{pr.author.username}" class="font-semibold text-foreground hover:underline">{pr.author.username}</a>{/if}
				wants to merge into
				<span class="inline-flex items-center gap-1 bg-[#163b6e]/40 text-primary rounded-full px-1.5 py-0 text-xs font-mono border border-primary/20">
					<GitBranch class="h-2.5 w-2.5" />{baseBranchLabel}
				</span>
				from
				<span class="inline-flex items-center gap-1 bg-[#163b6e]/40 text-primary rounded-full px-1.5 py-0 text-xs font-mono border border-primary/20">
					<GitBranch class="h-2.5 w-2.5" />{headBranchLabel}
				</span>
				· {timeAgo(pr.created_at)}
			</div>
		</div>

		<!-- Ready to merge / Merged status button (top right like GitHub) -->
		{#if pr.status === 'open' && !pr.is_draft && canMerge && mergeable}
			<Button variant="brand" size="sm" onclick={handleMerge} disabled={actionLoading}>
				<Check class="h-4 w-4" /> Ready to merge
			</Button>
		{/if}
	</div>

	{#if actionError}
		<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400 flex items-center gap-2">
			<AlertCircle class="h-4 w-4 shrink-0" />{actionError}
		</div>
	{/if}

	<!-- ── Tabs ───────────────────────────────────────────────────────── -->
	<div class="border-b border-border mb-0">
		<div class="flex gap-0">
			{#each [{ id: 'conversation', label: 'Conversation', icon: MessageSquare, count: comments.length + reviews.length }, { id: 'commits', label: 'Commits', icon: GitCommitHorizontal, count: commits.length }, { id: 'files', label: 'Files changed', icon: FileCode, count: files.length }] as tab}
				<button
					onclick={() => {
						activeTab = tab.id as any;
						if (tab.id === 'commits' && !commitsLoaded) loadCommits();
						if (tab.id === 'files' && !filesLoaded) loadFiles();
					}}
					class="flex items-center gap-1.5 px-4 py-2.5 text-sm border-b-2 -mb-px transition-colors"
					class:border-primary={activeTab === tab.id}
					class:text-foreground={activeTab === tab.id}
					class:font-semibold={activeTab === tab.id}
					class:border-transparent={activeTab !== tab.id}
					class:text-muted-foreground={activeTab !== tab.id}
					class:hover:text-foreground={activeTab !== tab.id}
				>
					<tab.icon class="h-4 w-4" />
					{tab.label}
					{#if tab.count > 0}
						<span class="ml-1 rounded-full bg-secondary px-1.5 py-0.5 text-xs">{tab.count}</span>
					{/if}
				</button>
			{/each}
		</div>
	</div>

	<!-- ── Main layout: left content + right sidebar ─────────────────── -->
	<div class="flex gap-6 pt-5">
		<!-- ════════════════════════════════════════════════════════════════ -->
		<!-- LEFT COLUMN -->
		<!-- ════════════════════════════════════════════════════════════════ -->
		<div class="flex-1 min-w-0">
			<!-- ── CONVERSATION ───────────────────────────────────────────── -->
			{#if activeTab === 'conversation'}
				<!-- Description -->
				<div class="flex gap-3 mb-4">
					<div class="w-9 h-9 rounded-full bg-primary/20 flex items-center justify-center text-primary font-bold text-sm shrink-0 overflow-hidden">
						{#if pr.author?.avatar_url}
							<img src={mediaUrl(pr.author.avatar_url)} alt={pr.author?.username ?? 'author'} class="h-full w-full object-cover" />
						{:else}
							{(pr.author?.username ?? 'U')[0].toUpperCase()}
						{/if}
					</div>
					<div class="flex-1 rounded-md border border-border overflow-hidden">
						<div class="flex items-center justify-between border-b border-border bg-accent px-4 py-2">
							<span class="text-sm">
								{#if pr.author}<a href="/{pr.author.username}" class="font-semibold text-foreground hover:underline">{pr.author.username}</a>{/if}
								<span class="text-muted-foreground ml-1">commented · {timeAgo(pr.created_at)}</span>
							</span>
						</div>
						{#if pr.description}
							<div
								use:mentionHoverCard
								class="p-4 prose prose-invert max-w-none text-sm
prose-headings:text-foreground prose-a:text-primary
prose-code:text-foreground prose-code:bg-secondary prose-code:rounded prose-code:px-1
prose-pre:bg-card prose-pre:border prose-pre:border-border
prose-strong:text-foreground"
							>
								{@html renderedDescription}
							</div>
						{:else}
							<div class="px-4 py-3 text-sm text-muted-foreground italic">No description provided.</div>
						{/if}
					</div>
				</div>

				<!-- Review summaries -->
				{#each latestReviewByUser() as review}
					<div class="flex gap-3 mb-3">
						<div class="w-9 shrink-0 flex justify-center pt-1">
							{#if review.state === 'APPROVED'}
								<div class="w-6 h-6 rounded-full bg-brand flex items-center justify-center">
									<Check class="h-3.5 w-3.5 text-white" />
								</div>
							{:else if review.state === 'CHANGES_REQUESTED'}
								<div class="w-6 h-6 rounded-full bg-[#da3633] flex items-center justify-center">
									<X class="h-3.5 w-3.5 text-white" />
								</div>
							{:else}
								<div class="w-6 h-6 rounded-full bg-secondary flex items-center justify-center">
									<MessageSquare class="h-3.5 w-3.5 text-muted-foreground" />
								</div>
							{/if}
						</div>
						<div class="flex-1 min-w-0 pt-0.5">
							<span class="text-sm">
								<a href="/{review.author.username}" class="font-semibold text-foreground hover:underline">{review.author.username}</a>
								{#if review.state === 'APPROVED'}<span class="text-muted-foreground"> approved these changes</span>{/if}
								{#if review.state === 'CHANGES_REQUESTED'}<span class="text-muted-foreground"> requested changes</span>{/if}
								{#if review.state === 'COMMENTED'}<span class="text-muted-foreground"> left a comment</span>{/if}
								<span class="text-muted-foreground text-xs ml-1">· {timeAgo(review.created_at)}</span>
							</span>
							{#if review.body}
								<div class="mt-1 rounded-md border border-border bg-card px-3 py-2 text-sm text-foreground">{review.body}</div>
							{/if}
						</div>
					</div>
				{/each}

				<!-- Comments -->
				{#if convLoading}
					<div class="space-y-3">
						{#each Array(2) as _}
							<div class="h-24 rounded-md border border-border bg-card animate-pulse"></div>
						{/each}
					</div>
				{:else}
					{#each comments as comment}
						<div class="flex gap-3 mb-4">
							<div class="w-9 h-9 rounded-full bg-primary/20 flex items-center justify-center text-primary font-bold text-sm shrink-0 overflow-hidden">
								{#if comment.author?.avatar_url}
									<img src={mediaUrl(comment.author.avatar_url)} alt={comment.author?.username ?? 'author'} class="h-full w-full object-cover" />
								{:else}
									{(comment.author?.username ?? 'U')[0].toUpperCase()}
								{/if}
							</div>
							<div class="flex-1 rounded-md border border-border overflow-hidden">
								<div class="flex items-center justify-between border-b border-border bg-accent px-4 py-2">
									<span class="text-sm">
										<a href="/{comment.author.username}" class="font-semibold text-foreground hover:underline">{comment.author.username}</a>
										<span class="text-muted-foreground ml-1">commented · {timeAgo(comment.created_at)}</span>
										{#if comment.updated_at !== comment.created_at}<span class="text-muted-foreground ml-1 text-xs">· edited</span>{/if}
									</span>
									{#if isLoggedIn && !isRepoArchived && (comment.author_id === authStore.user?.id || isOwner)}
										<div class="flex items-center gap-1">
											{#if comment.author_id === authStore.user?.id}
												<button
													onclick={() => {
														editingCommentID = comment.id;
														editingCommentBody = comment.body;
														editCommentTab = 'write';
													}}
													class="p-1 rounded hover:bg-secondary text-muted-foreground hover:text-foreground"
												>
													<Pencil class="h-3.5 w-3.5" />
												</button>
											{/if}
											<button onclick={() => deleteComment(comment)} class="p-1 rounded hover:bg-secondary text-muted-foreground hover:text-red-400">
												<Trash2 class="h-3.5 w-3.5" />
											</button>
										</div>
									{/if}
								</div>
								{#if editingCommentID === comment.id}
									<div class="p-3">
										<div class="rounded-md border border-border overflow-hidden">
											<div class="flex items-center gap-0.5 border-b border-border bg-secondary/30 px-2 py-1">
												<Button
													variant="ghost"
													size="sm"
													class={editCommentTab === 'write' ? 'bg-card text-foreground' : 'text-muted-foreground'}
													onclick={() => (editCommentTab = 'write')}>Write</Button
												>
												<Button
													variant="ghost"
													size="sm"
													class={editCommentTab === 'preview' ? 'bg-card text-foreground' : 'text-muted-foreground'}
													onclick={() => (editCommentTab = 'preview')}>Preview</Button
												>
											</div>
											{#if editCommentTab === 'write'}
												<textarea
													bind:value={editingCommentBody}
													use:mentionAutocomplete={{ users: mentionUsers }}
													rows="4"
													onpaste={handleEditCommentPaste}
													ondragover={handleMarkdownDragOver}
													ondrop={handleEditCommentDrop}
													class="w-full border-0 bg-background px-3 py-2 text-sm text-foreground focus:outline-none resize-none"
												></textarea>
											{:else}
												<div class="min-h-20 bg-background px-3 py-2">
													{#if renderedEditingCommentPreview}
														<div
															use:mentionHoverCard
															class="prose prose-invert prose-sm max-w-none text-foreground prose-a:text-primary prose-code:bg-secondary prose-code:px-1 prose-code:rounded prose-pre:bg-secondary prose-pre:border prose-pre:border-border"
														>
															{@html renderedEditingCommentPreview}
														</div>
													{:else}
														<p class="text-sm text-muted-foreground italic">Nothing to preview.</p>
													{/if}
												</div>
											{/if}
										</div>
										<div class="flex items-center gap-2 mt-2">
											<Button variant="brand" size="sm" onclick={() => saveEditComment(comment)}>Save</Button>
											<Button variant="ghost" size="sm" onclick={() => (editingCommentID = null)}>Cancel</Button>
										</div>
									</div>
								{:else}
									<div
										use:mentionHoverCard
										class="p-4 prose prose-invert max-w-none text-sm
prose-headings:text-foreground prose-a:text-primary
prose-code:text-foreground prose-code:bg-secondary prose-code:rounded prose-code:px-1
prose-strong:text-foreground"
									>
										{@html renderMarkdownHtml(comment.body)}
									</div>
								{/if}
							</div>
						</div>
					{/each}
				{/if}

				<!-- Merge / check area -->
				<div class="ml-12 mt-2 mb-4">
					{#if pr.status === 'open' && !pr.is_draft}
						<!-- Review status banners -->
						{#if hasApproval && !hasChangesRequested}
							<div class="flex items-center gap-2 mb-3 rounded-md border border-brand/40 bg-[#0d2b0d] px-4 py-3">
								<Check class="h-5 w-5 text-[#3fb950] shrink-0" />
								<div>
									<p class="text-sm font-semibold text-[#3fb950]">All checks have passed</p>
									<p class="text-xs text-muted-foreground">Approved — ready to merge.</p>
								</div>
							</div>
						{:else if hasChangesRequested}
							<div class="flex items-center gap-2 mb-3 rounded-md border border-[#da3633]/40 bg-[#2d0d0d] px-4 py-3">
								<X class="h-5 w-5 text-[#f85149] shrink-0" />
								<div>
									<p class="text-sm font-semibold text-[#f85149]">Changes requested</p>
									<p class="text-xs text-muted-foreground">Reviewers have requested changes.</p>
								</div>
							</div>
						{/if}

						<!-- Mergeable check box -->
						<div class="rounded-md border {mergeable ? 'border-brand/40 bg-[#0d2b0d]' : 'border-[#5a3e1b]/60 bg-[#1f1500]/60'} px-4 py-3 mb-3">
							<div class="flex items-center gap-2">
								{#if mergeable}
									<Check class="h-5 w-5 text-[#3fb950] shrink-0" />
									<div>
										<p class="text-sm font-semibold text-foreground">No conflicts with base branch</p>
										<p class="text-xs text-muted-foreground">Merging can be performed automatically.</p>
									</div>
								{:else}
									<AlertCircle class="h-5 w-5 text-[#d29922] shrink-0" />
									<div>
										<p class="text-sm font-semibold text-foreground">This branch has conflicts</p>
										<p class="text-xs text-muted-foreground">Must be resolved before merging.</p>
									</div>
								{/if}
							</div>
						</div>

						<!-- Merge button row (only if owner) -->
						{#if canMerge}
							<div class="flex items-center gap-2 mb-3">
								<div class="flex items-stretch">
									<Button variant="brand" disabled={actionLoading || !mergeable} onclick={handleMerge} class="rounded-r-none">
										<GitMerge class="h-4 w-4" />
										{mergeMethodInfo[mergeMethod].label}
									</Button>
									<button
										onclick={() => (showMergeOptions = !showMergeOptions)}
										class="px-2 border-l border-brand/50 bg-brand hover:bg-[#2ea043] text-white rounded-r-md transition-colors"
										disabled={actionLoading}
									>
										<ChevronDown class="h-4 w-4" />
									</button>
								</div>
								<span class="text-xs text-muted-foreground">You can also merge with the command line.</span>
							</div>

							{#if showMergeOptions}
								<div class="mb-3 rounded-md border border-border bg-background overflow-hidden w-80">
									{#each ['merge', 'squash', 'rebase'] as const as m}
										<button
											onclick={() => {
												mergeMethod = m;
												showMergeOptions = false;
											}}
											class="w-full flex items-start gap-3 px-4 py-3 text-left hover:bg-secondary transition-colors {mergeMethod === m ? 'bg-secondary' : ''}"
										>
											<div class="mt-0.5">
												{#if mergeMethod === m}<Check class="h-4 w-4 text-primary" />{:else}<div class="h-4 w-4"></div>{/if}
											</div>
											<div>
												<div class="text-sm font-semibold text-foreground">{mergeMethodInfo[m].label}</div>
												<div class="text-xs text-muted-foreground mt-0.5">{mergeMethodInfo[m].desc}</div>
											</div>
										</button>
									{/each}
								</div>
							{/if}
						{/if}

						<!-- Close / Review buttons -->
						<div class="flex items-center gap-2 flex-wrap">
							{#if canClose}
								<Button variant="outline" size="sm" onclick={handleClose} disabled={actionLoading}>
									<XCircle class="h-4 w-4" /> Close pull request
								</Button>
							{/if}
							{#if isLoggedIn && !isRepoArchived}
								<Button variant="outline" size="sm" onclick={() => (showReviewForm = !showReviewForm)}>
									<MessageSquare class="h-4 w-4" />
									{showReviewForm ? 'Cancel review' : 'Review changes'}
								</Button>
							{/if}
						</div>

						{#if showReviewForm}
							<div class="mt-3 rounded-md border border-border p-4 space-y-3">
								<div class="flex gap-2 flex-wrap">
									{#each reviewOptions as opt}
										<button
											onclick={() => (reviewState = opt.state)}
											class="flex items-center gap-1.5 px-3 py-1.5 rounded-md border text-sm transition-colors
{reviewState === opt.state ? 'border-primary bg-primary/10 text-foreground' : 'border-border hover:bg-secondary text-muted-foreground'}"
										>
											<opt.icon class="h-3.5 w-3.5" />{opt.label}
										</button>
									{/each}
								</div>
								<textarea
									bind:this={reviewTextareaEl}
									bind:value={reviewBody}
									use:mentionAutocomplete={{ users: mentionUsers }}
									rows="3"
									placeholder="Leave a review comment (optional)…"
									onpaste={handleReviewPaste}
									ondragover={handleMarkdownDragOver}
									ondrop={handleReviewDrop}
									class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground focus:outline-none focus:ring-1 focus:ring-primary resize-none"
								></textarea>
								<Button variant="brand" size="sm" disabled={reviewSubmitting} onclick={submitReview}>
									{#if reviewSubmitting}<Loader class="h-4 w-4 animate-spin" />{/if}
									Submit review
								</Button>
							</div>
						{/if}
					{:else if pr.status === 'closed'}
						<div class="flex items-center gap-3 rounded-md border border-border bg-card px-4 py-3">
							<XCircle class="h-5 w-5 text-muted-foreground shrink-0" />
							<span class="text-sm text-muted-foreground flex-1">Pull request closed{pr.closed_at ? ' ' + timeAgo(pr.closed_at) : ''}.</span>
							{#if canClose}
								<Button variant="brand" size="sm" onclick={handleReopen} disabled={actionLoading}>
									<RotateCcw class="h-4 w-4" /> Reopen
								</Button>
							{/if}
						</div>
					{:else if pr.status === 'merged'}
						<div class="flex items-center gap-3 rounded-md border border-[#6e40c9]/40 bg-[#1a0b3b] px-4 py-3">
							<GitMerge class="h-5 w-5 text-[#a371f7] shrink-0" />
							<div>
								<p class="text-sm font-semibold text-foreground">Pull request successfully merged{pr.merge_sha ? ` as ${pr.merge_sha.slice(0, 7)}` : ''}.</p>
								{#if pr.merged_by}<p class="text-xs text-muted-foreground">
										Merged by <a href="/{pr.merged_by.username}" class="hover:underline text-foreground">{pr.merged_by.username}</a>{pr.merged_at
											? ' · ' + timeAgo(pr.merged_at)
											: ''}
									</p>{/if}
							</div>
						</div>
					{/if}
				</div>

				<!-- New comment box -->
				{#if isLoggedIn && !isRepoArchived}
					<div class="flex gap-3 mt-2">
						<div class="w-9 h-9 rounded-full bg-primary/20 flex items-center justify-center text-primary font-bold text-sm shrink-0 overflow-hidden">
							{#if authStore.user?.avatar_url}
								<img src={mediaUrl(authStore.user.avatar_url)} alt={authStore.user.username} class="h-full w-full object-cover" />
							{:else}
								{(authStore.user?.username ?? 'U')[0].toUpperCase()}
							{/if}
						</div>
						<div class="flex-1 rounded-md border border-border overflow-hidden">
							<div class="border-b border-border bg-accent px-4 py-2 text-sm font-semibold text-foreground">Add a comment</div>
							<div class="flex items-center gap-0.5 border-b border-border bg-secondary/30 px-2 py-1">
								<Button variant="ghost" size="sm" class={newCommentTab === 'write' ? 'bg-card text-foreground' : 'text-muted-foreground'} onclick={() => (newCommentTab = 'write')}
									>Write</Button
								>
								<Button variant="ghost" size="sm" class={newCommentTab === 'preview' ? 'bg-card text-foreground' : 'text-muted-foreground'} onclick={() => (newCommentTab = 'preview')}
									>Preview</Button
								>
							</div>
							{#if newCommentTab === 'write'}
								<textarea
									bind:this={newCommentTextareaEl}
									bind:value={newCommentBody}
									use:mentionAutocomplete={{ users: mentionUsers }}
									rows="4"
									placeholder="Leave a comment…"
									onpaste={handleNewCommentPaste}
									ondragover={handleMarkdownDragOver}
									ondrop={handleNewCommentDrop}
									class="w-full bg-transparent px-4 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none resize-none"
								></textarea>
							{:else}
								<div class="min-h-24 bg-background px-4 py-3">
									{#if renderedNewCommentPreview}
										<div
											use:mentionHoverCard
											class="prose prose-invert prose-sm max-w-none text-foreground prose-a:text-primary prose-code:bg-secondary prose-code:px-1 prose-code:rounded prose-pre:bg-secondary prose-pre:border prose-pre:border-border"
										>
											{@html renderedNewCommentPreview}
										</div>
									{:else}
										<p class="text-sm text-muted-foreground italic">Nothing to preview.</p>
									{/if}
								</div>
							{/if}
							<div class="flex items-center justify-between px-3 py-2 border-t border-border bg-secondary/30">
								<div class="flex items-center gap-3">
									<button
										type="button"
										class="text-xs text-muted-foreground hover:text-foreground transition-colors"
										onclick={pickNewCommentFiles}
										disabled={markdownUploading || newCommentTab === 'preview'}
									>
										{markdownUploading ? 'Uploading…' : 'Paste, drop, or click to add files'}
									</button>
									{#if markdownUploadError}
										<span class="text-xs text-red-400">{markdownUploadError}</span>
									{/if}
								</div>
								<Button variant="brand" size="sm" disabled={commentSubmitting || !newCommentBody.trim()} onclick={submitComment}>
									{#if commentSubmitting}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
									<Send class="h-3.5 w-3.5" /> Comment
								</Button>
							</div>
						</div>
					</div>
				{:else if isLoggedIn}
					<div class="ml-12 mt-2 text-sm text-muted-foreground">Comments are disabled because this repository is archived.</div>
				{/if}
			{:else if activeTab === 'commits'}
				{#if commitsLoading}
					<div class="space-y-2">
						{#each Array(4) as _}<div class="h-10 rounded-md border border-border bg-card animate-pulse"></div>{/each}
					</div>
				{:else if commits.length === 0}
					<div class="rounded-md border border-dashed border-border py-16 text-center text-sm text-muted-foreground">No commits found.</div>
				{:else}
					<!-- Group by date -->
					{@const dateLabel = new Date(commits[0].author.date).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
					<div class="flex items-center gap-2 mb-2 text-xs text-muted-foreground">
						<GitCommitHorizontal class="h-3.5 w-3.5" /> Commits on {dateLabel}
					</div>
					<div class="rounded-md border border-border bg-card overflow-hidden">
						{#each commits as commit, i}
							<a
								href="/{username}/{repo}/commit/{commit.sha}"
								class="flex items-center gap-3 px-4 py-3 hover:bg-secondary transition-colors {i < commits.length - 1 ? 'border-b border-border' : ''}"
							>
								<GitCommitHorizontal class="h-4 w-4 text-muted-foreground shrink-0" />
								<div class="flex-1 min-w-0">
									<p class="text-sm font-semibold text-foreground truncate">{commit.message.split('\n')[0]}</p>
									<div class="flex items-center gap-2 text-xs text-muted-foreground">
										<div class="flex h-5 w-5 items-center justify-center overflow-hidden rounded-full border border-border bg-secondary text-[11px] font-semibold text-foreground">
											{#if commitAuthorAvatarUrl(commit.author)}
												<img src={commitAuthorAvatarUrl(commit.author)} alt={commitAuthorName(commit.author)} class="h-full w-full object-cover" />
											{:else}
												{commitAuthorInitial(commit.author)}
											{/if}
										</div>
										<span title={commit.author?.username ? undefined : 'Not registered on GitPier'}>{commitAuthorName(commit.author)} · {timeAgo(commit.author.date)}</span>
									</div>
								</div>
								<span class="font-mono text-xs text-primary shrink-0 bg-secondary px-2 py-0.5 rounded border border-border">{commit.sha.slice(0, 7)}</span>
							</a>
						{/each}
					</div>
				{/if}

				<!-- ── FILES CHANGED ──────────────────────────────────────── -->
			{:else if activeTab === 'files'}
				{#if filesLoading}
					<div class="space-y-2">
						{#each Array(3) as _}<div class="h-20 rounded-md border border-border bg-card animate-pulse"></div>{/each}
					</div>
				{:else if files.length === 0}
					<div class="rounded-md border border-dashed border-border py-16 text-center text-sm text-muted-foreground">No files changed.</div>
				{:else}
					<div class="flex items-center justify-between mb-3">
						<span class="text-sm text-muted-foreground">
							Showing <span class="font-semibold text-foreground">{files.length} changed file{files.length !== 1 ? 's' : ''}</span>
							with <span class="text-[#3fb950] font-semibold">{totalAdditions} addition{totalAdditions !== 1 ? 's' : ''}</span> and
							<span class="text-[#f85149] font-semibold">{totalDeletions} deletion{totalDeletions !== 1 ? 's' : ''}</span>.
						</span>
						<div class="flex rounded overflow-hidden border border-border text-xs">
							<button
								onclick={() => (diffMode = 'split')}
								class="px-3 py-1 transition-colors {diffMode === 'split' ? 'bg-secondary text-foreground font-semibold' : 'bg-card text-muted-foreground hover:text-foreground'}"
								>Split</button
							>
							<button
								onclick={() => (diffMode = 'unified')}
								class="px-3 py-1 border-l border-border transition-colors {diffMode === 'unified'
									? 'bg-secondary text-foreground font-semibold'
									: 'bg-card text-muted-foreground hover:text-foreground'}">Unified</button
							>
						</div>
					</div>

					{#each files as file}
						<div id="file-{file.path}" class="mb-3 rounded-md border border-border overflow-hidden">
							<button onclick={() => toggleFile(file.path)} class="w-full flex items-center gap-3 px-4 py-2.5 bg-accent hover:bg-secondary transition-colors border-b border-border">
								<span class="text-sm font-semibold text-foreground mr-1">
									{#if expandedFiles.has(file.path)}▾{:else}▸{/if}
									{files.indexOf(file) + 1}
								</span>
								<FileCode class="h-4 w-4 text-muted-foreground shrink-0" />
								<span class="font-mono text-sm text-foreground flex-1 text-left truncate">{file.path}</span>
								<div class="flex items-center gap-3 shrink-0 text-xs">
									<span class="text-[#3fb950]">+{file.additions}</span>
									<span class="text-[#f85149]">−{file.deletions}</span>
								</div>
							</button>
							{#if expandedFiles.has(file.path)}
								{#if file.patch}
									<DiffViewer patch={file.patch} filePath={file.path} diffStyle={diffMode} />
								{:else}
									<div class="border-t border-border px-4 py-6 text-center text-sm text-muted-foreground bg-background">
										{file.type === 'added' ? 'New file' : file.type === 'deleted' ? 'File deleted' : 'No diff available'}
									</div>
								{/if}
							{/if}
						</div>
					{/each}
				{/if}
			{/if}
		</div>

		<!-- ════════════════════════════════════════════════════════════════ -->
		<!-- RIGHT SIDEBAR (visible in all tabs) -->
		<!-- ════════════════════════════════════════════════════════════════ -->
		<!-- svelte-ignore a11y_click_events_have_key_events a11y_no_static_element_interactions -->
		<div
			class="w-56 shrink-0 divide-y divide-border"
			onclick={(e) => {
				if (!(e.target as HTMLElement).closest('.sidebar-section')) openSidebarDropdown = null;
			}}
		>
			<!-- Reviewers -->
			<div class="pb-4 pt-0">
				<div class="flex items-center justify-between mb-2">
					<span class="text-xs font-semibold text-foreground">Reviewers</span>
				</div>
				{#if latestReviewByUser().length > 0}
					<div class="space-y-1">
						{#each latestReviewByUser() as review}
							<div class="flex items-center gap-2 text-xs">
								<div class="w-4 h-4 rounded-full bg-primary/20 flex items-center justify-center text-primary font-bold text-[10px] overflow-hidden">
									{#if review.author?.avatar_url}
										<img src={mediaUrl(review.author.avatar_url)} alt={review.author?.username ?? 'reviewer'} class="h-full w-full object-cover" />
									{:else}
										{avatarLetter(review.author.username)}
									{/if}
								</div>
								<span class="text-foreground font-semibold">{review.author.username}</span>
								{#if review.state === 'APPROVED'}<Check class="h-3 w-3 text-[#3fb950] ml-auto" />{/if}
								{#if review.state === 'CHANGES_REQUESTED'}<X class="h-3 w-3 text-[#f85149] ml-auto" />{/if}
							</div>
						{/each}
					</div>
				{:else}
					<p class="text-xs text-muted-foreground">No reviews</p>
				{/if}
			</div>

			<!-- Assignees -->
			<div class="py-4 relative sidebar-section">
				<div class="flex items-center justify-between mb-2">
					<span class="text-xs font-semibold text-foreground">Assignees</span>
					{#if (isOwner || isAuthor) && !isRepoArchived}
						<button
							onclick={() => (openSidebarDropdown = openSidebarDropdown === 'assignees' ? null : 'assignees')}
							class="text-muted-foreground hover:text-foreground transition-colors p-0.5"
						>
							<Settings class="h-3.5 w-3.5" />
						</button>
					{/if}
				</div>
				{#if openSidebarDropdown === 'assignees'}
					<div class="absolute right-0 top-10 z-20 w-56 rounded-md border border-border bg-card shadow-lg overflow-hidden">
						{#each possibleAssignees as user}
							<button
								onclick={() => updateAssignee(user.id)}
								class="flex items-center gap-2 w-full px-3 py-2 text-sm hover:bg-secondary transition-colors text-left"
								class:bg-secondary={pr.assignee_id === user.id}
							>
								<div class="w-5 h-5 rounded-full bg-secondary border border-border flex items-center justify-center text-xs font-semibold shrink-0 overflow-hidden">
									{#if user.avatar_url}
										<img src={mediaUrl(user.avatar_url)} alt={user.username} class="h-full w-full object-cover" />
									{:else}
										{avatarLetter(user.username)}
									{/if}
								</div>
								<span class="text-foreground text-xs">{user.username}</span>
								{#if pr.assignee_id === user.id}<span class="ml-auto text-primary text-xs">✓</span>{/if}
							</button>
						{/each}
						{#if possibleAssignees.length === 0}
							<p class="px-3 py-2 text-xs text-muted-foreground">No assignees available</p>
						{/if}
					</div>
				{/if}
				{#if pr.assignee}
					<div class="flex items-center gap-2">
						<div class="w-5 h-5 rounded-full bg-secondary border border-border flex items-center justify-center text-xs font-semibold shrink-0 overflow-hidden">
							{#if pr.assignee.avatar_url}
								<img src={mediaUrl(pr.assignee.avatar_url)} alt={pr.assignee.username} class="h-full w-full object-cover" />
							{:else}
								{avatarLetter(pr.assignee.username)}
							{/if}
						</div>
						<a href="/{pr.assignee.username}" class="text-xs text-foreground hover:underline">{pr.assignee.username}</a>
					</div>
				{:else}
					<p class="text-xs text-muted-foreground">
						No one
						{#if isLoggedIn && !isRepoArchived}
							— <button onclick={() => updateAssignee(authStore.user!.id)} class="text-primary hover:underline">assign yourself</button>
						{/if}
					</p>
				{/if}
			</div>

			<!-- Labels -->
			<div class="py-4 relative sidebar-section">
				<div class="flex items-center justify-between mb-2">
					<span class="text-xs font-semibold text-foreground">Labels</span>
					{#if (isOwner || isAuthor) && !isRepoArchived}
						<button onclick={() => (openSidebarDropdown = openSidebarDropdown === 'labels' ? null : 'labels')} class="text-muted-foreground hover:text-foreground transition-colors p-0.5">
							<Settings class="h-3.5 w-3.5" />
						</button>
					{/if}
				</div>
				{#if openSidebarDropdown === 'labels'}
					<div class="absolute right-0 top-10 z-20 w-56 rounded-md border border-border bg-card shadow-lg overflow-hidden">
						{#each labelList as label}
							<button
								onclick={() => toggleLabel(label.id)}
								disabled={labelUpdating}
								class="flex items-center gap-2 w-full px-3 py-2 text-sm hover:bg-secondary transition-colors text-left"
								class:bg-secondary={selectedLabelIds.includes(label.id)}
							>
								<span class="w-3 h-3 rounded-full shrink-0" style="background-color:{label.color}"></span>
								<span class="text-foreground text-xs">{label.name}</span>
								{#if selectedLabelIds.includes(label.id)}<span class="ml-auto text-primary text-xs">✓</span>{/if}
							</button>
						{/each}
						{#if labelList.length === 0 && !showCreateLabel}
							<p class="px-3 py-2 text-xs text-muted-foreground">No labels available</p>
						{/if}
						{#if showCreateLabel}
							<div class="p-2 border-t border-border">
								<input
									type="text"
									bind:value={newLabelName}
									placeholder="Label name"
									class="w-full rounded border border-border bg-background px-2 py-1 text-xs text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-ring mb-1.5"
								/>
								<div class="flex items-center gap-1.5 mb-1.5">
									<span class="text-xs text-muted-foreground">Color</span>
									{#each ['#da3633', '#e3b341', '#238636', '#1f6feb', '#8250df', '#0075ca', '#6e40c9', '#f0883e'] as c}
										<button
											type="button"
											onclick={() => (newLabelColor = c)}
											class="w-4 h-4 rounded-full border-2 transition-all {newLabelColor === c ? 'border-foreground scale-110' : 'border-transparent'}"
											style="background-color:{c}"
										></button>
									{/each}
									<input type="color" bind:value={newLabelColor} class="w-4 h-4 rounded cursor-pointer border-0 bg-transparent p-0" title="Custom color" />
								</div>
								<div class="flex gap-1.5">
									<button
										onclick={createLabel}
										disabled={labelCreating || !newLabelName.trim()}
										class="flex-1 rounded bg-primary px-2 py-1 text-xs font-medium text-primary-foreground disabled:opacity-50 hover:opacity-90 transition-opacity"
										>{labelCreating ? 'Creating…' : 'Create'}</button
									>
									<button
										onclick={() => {
											showCreateLabel = false;
											newLabelName = '';
										}}
										class="rounded border border-border px-2 py-1 text-xs text-muted-foreground hover:text-foreground transition-colors">Cancel</button
									>
								</div>
							</div>
						{:else}
							<button
								onclick={() => (showCreateLabel = true)}
								class="flex items-center gap-1.5 w-full px-3 py-2 text-xs text-primary hover:bg-secondary transition-colors border-t border-border">+ Create new label</button
							>
						{/if}
					</div>
				{/if}
				{#if (pr.labels ?? []).length === 0}
					<p class="text-xs text-muted-foreground">None yet</p>
				{:else}
					<div class="flex flex-wrap gap-1.5">
						{#each pr.labels as label}
							<span class="px-2.5 py-0.5 rounded-full text-xs font-medium" style="background-color:{label.color};color:{textColorForBg(label.color)}">{label.name}</span>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Participants -->
			<div class="py-4">
				<div class="text-xs font-semibold text-foreground mb-2">
					{new Set([pr.author_id, ...comments.map((c) => c.author_id)]).size} participant{new Set([pr.author_id, ...comments.map((c) => c.author_id)]).size !== 1 ? 's' : ''}
				</div>
				<div class="flex flex-wrap gap-1">
					{#if pr.author}
						<a
							href="/{pr.author.username}"
							class="w-6 h-6 rounded-full bg-primary/20 flex items-center justify-center text-primary font-bold text-xs hover:ring-2 hover:ring-primary transition-all overflow-hidden"
							title={pr.author.username}
						>
							{#if pr.author.avatar_url}
								<img src={mediaUrl(pr.author.avatar_url)} alt={pr.author.username} class="h-full w-full object-cover" />
							{:else}
								{avatarLetter(pr.author.username)}
							{/if}
						</a>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}
