<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { issues, labels, milestones as milestones_api, repos, type Issue, type IssueComment, type Label, type Milestone, type Collaborator, type User } from '$lib/api/client';
	import { mediaUrl, timeAgo } from '$lib/utils';
	import { authStore } from '$lib/stores/auth.svelte';
	import { getContext } from 'svelte';
	import {
		CircleDot,
		CheckCircle,
		AlertCircle,
		Loader,
		Pencil,
		Trash2,
		Settings,
		Bold,
		Italic,
		Heading,
		Code,
		Link as LinkIcon,
		List,
		ListOrdered,
		CheckSquare,
		Quote,
		AtSign
	} from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { renderMarkdownHtml } from '$lib/markdown';
	import { handleMarkdownPaste, handleMarkdownDrop, handleMarkdownDragOver, openMarkdownAssetPicker } from '$lib/hooks/markdown-assets';
	import { mentionAutocomplete } from '$lib/hooks/mention-autocomplete';
	import { mentionHoverCard } from '$lib/hooks/mention-hover-card';

	let issue = $state<Issue | null>(null);
	let comments = $state<IssueComment[]>([]);
	let labelList = $state<Label[]>([]);
	let collaborators = $state<Collaborator[]>([]);
	let milestoneList = $state<Milestone[]>([]);
	let loading = $state(true);
	let error = $state('');

	// Editing issue
	let editingTitle = $state(false);
	let editingBody = $state(false);
	let editTitle = $state('');
	let editBody = $state('');
	let editBodyTab = $state<'write' | 'preview'>('write');
	let editSaving = $state(false);

	// Comment editor
	let newComment = $state('');
	let commentSaving = $state(false);
	let commentTab = $state<'write' | 'preview'>('write');
	let commentTextareaEl = $state<HTMLTextAreaElement | null>(null);
	let editingCommentId = $state<number | null>(null);
	let editCommentBody = $state('');
	let editCommentTab = $state<'write' | 'preview'>('write');
	let commentActionError = $state('');
	let markdownUploading = $state(false);
	let markdownUploadError = $state('');

	// Sidebar
	let openSidebarDropdown = $state<'assignees' | 'labels' | 'type' | 'milestone' | null>(null);
	let labelUpdating = $state(false);
	let newLabelName = $state('');
	let newLabelColor = $state('#1f6feb');
	let labelCreating = $state(false);
	let showCreateLabel = $state(false);

	// Action states
	let actionLoading = $state(false);
	let actionError = $state('');

	const ISSUE_TYPES = [
		{ value: 'bug', label: 'Bug', color: '#da3633' },
		{ value: 'enhancement', label: 'Enhancement', color: '#1f6feb' },
		{ value: 'question', label: 'Question', color: '#8250df' },
		{ value: 'documentation', label: 'Documentation', color: '#0075ca' },
		{ value: 'good first issue', label: 'Good first issue', color: '#238636' },
		{ value: 'help wanted', label: 'Help wanted', color: '#e3b341' }
	];

	const { username, repo } = $derived(page.params);
	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const issueNumber = $derived(Number(page.params.issueNumber));
	const currentUser = $derived(authStore.user);
	const isLoggedIn = $derived(currentUser != null);
	const isAuthor = $derived(isLoggedIn && issue?.author_id === currentUser?.id);

	const possibleAssignees = $derived.by(() => {
		const result: User[] = [];
		if (currentUser) result.push(currentUser);
		for (const c of collaborators) {
			if (c.user && c.user_id !== currentUser?.id) result.push(c.user);
		}
		return result;
	});
	const mentionUsers = $derived(possibleAssignees.map((u) => ({ username: u.username, avatar_url: u.avatar_url })));

	async function load() {
		loading = true;
		error = '';
		try {
			const [issueData, commentData, labelData, collabData, milestoneData] = await Promise.all([
				issues.get(username!, repo!, issueNumber),
				issues.comments.list(username!, repo!, issueNumber),
				labels.list(username!, repo!),
				repos.collaborators.list(username!, repo!).catch(() => ({ collaborators: [] })),
				milestones_api.list(username!, repo!).catch(() => ({ milestones: [] }))
			]);
			issue = issueData.issue;
			comments = commentData.comments ?? [];
			labelList = labelData.labels ?? [];
			collaborators = collabData.collaborators ?? [];
			milestoneList = milestoneData.milestones ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	const renderedBody = $derived(renderMarkdownHtml(issue?.body));
	const renderedCommentPreview = $derived(renderMarkdownHtml(newComment));
	const renderedEditBodyPreview = $derived(renderMarkdownHtml(editBody));
	const renderedEditCommentPreview = $derived(renderMarkdownHtml(editCommentBody));

	function renderMarkdown(text: string): string {
		return renderMarkdownHtml(text);
	}

	function avatarLetter(u: string | undefined) {
		return (u ?? '?')[0].toUpperCase();
	}

	async function handleClose() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		actionLoading = true;
		actionError = '';
		try {
			const data = await issues.close(username!, repo!, issueNumber);
			issue = data.issue;
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
			const data = await issues.reopen(username!, repo!, issueNumber);
			issue = data.issue;
		} catch (e: any) {
			actionError = e.message;
		} finally {
			actionLoading = false;
		}
	}

	async function handleDelete() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!confirm('Permanently delete this issue? This cannot be undone.')) return;
		actionLoading = true;
		actionError = '';
		try {
			await issues.delete(username!, repo!, issueNumber);
			goto(`/${username}/${repo}/issues`);
		} catch (e: any) {
			actionError = e.message;
			actionLoading = false;
		}
	}

	function startEditTitle() {
		if (isRepoArchived) return;
		editTitle = issue!.title;
		editingTitle = true;
	}

	function startEditBody() {
		if (isRepoArchived) return;
		editBody = issue!.body ?? '';
		editBodyTab = 'write';
		editingBody = true;
	}

	async function saveEditTitle() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		editSaving = true;
		try {
			const data = await issues.update(username!, repo!, issueNumber, { title: editTitle });
			issue = data.issue;
			editingTitle = false;
		} catch (e: any) {
			actionError = e.message;
		} finally {
			editSaving = false;
		}
	}

	async function saveEditBody() {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		editSaving = true;
		try {
			const data = await issues.update(username!, repo!, issueNumber, { body: editBody });
			issue = data.issue;
			editingBody = false;
		} catch (e: any) {
			actionError = e.message;
		} finally {
			editSaving = false;
		}
	}

	async function submitComment() {
		if (isRepoArchived) {
			commentActionError = 'This repository is archived and read-only.';
			return;
		}
		if (!newComment.trim()) return;
		commentSaving = true;
		commentActionError = '';
		try {
			const data = await issues.comments.create(username!, repo!, issueNumber, newComment.trim());
			comments = [...comments, data.comment];
			newComment = '';
		} catch (e: any) {
			commentActionError = e.message;
		} finally {
			commentSaving = false;
		}
	}

	function startEditComment(comment: IssueComment) {
		if (isRepoArchived) return;
		editingCommentId = comment.id;
		editCommentBody = comment.body;
		editCommentTab = 'write';
	}

	async function saveEditComment(commentId: number) {
		if (isRepoArchived) {
			commentActionError = 'This repository is archived and read-only.';
			return;
		}
		if (!editCommentBody.trim()) return;
		try {
			const data = await issues.comments.update(username!, repo!, issueNumber, commentId, editCommentBody.trim());
			comments = comments.map((c) => (c.id === commentId ? data.comment : c));
			editingCommentId = null;
			editCommentBody = '';
		} catch (e: any) {
			commentActionError = e.message;
		}
	}

	async function deleteComment(commentId: number) {
		if (isRepoArchived) {
			commentActionError = 'This repository is archived and read-only.';
			return;
		}
		if (!confirm('Delete this comment?')) return;
		try {
			await issues.comments.delete(username!, repo!, issueNumber, commentId);
			comments = comments.filter((c) => c.id !== commentId);
		} catch (e: any) {
			commentActionError = e.message;
		}
	}

	const selectedLabelIds = $derived((issue?.labels ?? []).map((l) => l.id));

	async function toggleLabel(labelId: number) {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!issue) return;
		labelUpdating = true;
		const current = issue.labels.map((l) => l.id);
		const updated = current.includes(labelId) ? current.filter((id) => id !== labelId) : [...current, labelId];
		try {
			const data = await issues.setLabels(username!, repo!, issueNumber, updated);
			issue = data.issue;
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
		if (!issue) return;
		try {
			const isSame = issue.assignee_id === userId;
			const data = await issues.update(username!, repo!, issueNumber, isSame ? { clear_assignee: true } : { assignee_id: userId });
			issue = data.issue;
			openSidebarDropdown = null;
		} catch (e: any) {
			actionError = e.message;
		}
	}

	async function updateType(typeValue: string) {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!issue) return;
		try {
			const isSame = issue.issue_type === typeValue;
			const data = await issues.update(username!, repo!, issueNumber, { issue_type: isSame ? '' : typeValue });
			issue = data.issue;
			openSidebarDropdown = null;
		} catch (e: any) {
			actionError = e.message;
		}
	}

	async function updateMilestone(milestoneId: number) {
		if (isRepoArchived) {
			actionError = 'This repository is archived and read-only.';
			return;
		}
		if (!issue) return;
		try {
			const isSame = issue.milestone_id === milestoneId;
			const data = await issues.update(username!, repo!, issueNumber, isSame ? { clear_milestone: true } : { milestone_id: milestoneId });
			issue = data.issue;
			openSidebarDropdown = null;
		} catch (e: any) {
			actionError = e.message;
		}
	}

	function insertCommentMarkdown(prefix: string, suffix = '', placeholder = '') {
		const el = commentTextareaEl;
		if (!el) return;
		const start = el.selectionStart;
		const end = el.selectionEnd;
		const selected = newComment.slice(start, end) || placeholder;
		newComment = newComment.slice(0, start) + prefix + selected + suffix + newComment.slice(end);
		setTimeout(() => {
			el.focus();
			const newPos = start + prefix.length + selected.length;
			el.setSelectionRange(newPos, newPos);
		}, 0);
	}

	function fieldBinding(textarea: HTMLTextAreaElement, getValue: () => string, setValue: (next: string) => void) {
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

	async function handleEditBodyPaste(e: ClipboardEvent) {
		const textarea = e.currentTarget as HTMLTextAreaElement | null;
		if (!textarea) return;
		await handleMarkdownPaste(
			e,
			fieldBinding(
				textarea,
				() => editBody,
				(next) => (editBody = next)
			)
		);
	}

	async function handleEditBodyDrop(e: DragEvent) {
		const textarea = e.currentTarget as HTMLTextAreaElement | null;
		if (!textarea) return;
		await handleMarkdownDrop(
			e,
			fieldBinding(
				textarea,
				() => editBody,
				(next) => (editBody = next)
			)
		);
	}

	async function handleCommentPaste(e: ClipboardEvent) {
		if (!commentTextareaEl) return;
		await handleMarkdownPaste(
			e,
			fieldBinding(
				commentTextareaEl,
				() => newComment,
				(next) => (newComment = next)
			)
		);
	}

	async function handleCommentDrop(e: DragEvent) {
		if (!commentTextareaEl) return;
		await handleMarkdownDrop(
			e,
			fieldBinding(
				commentTextareaEl,
				() => newComment,
				(next) => (newComment = next)
			)
		);
	}

	async function handleEditCommentPaste(e: ClipboardEvent) {
		const textarea = e.currentTarget as HTMLTextAreaElement | null;
		if (!textarea) return;
		await handleMarkdownPaste(
			e,
			fieldBinding(
				textarea,
				() => editCommentBody,
				(next) => (editCommentBody = next)
			)
		);
	}

	async function handleEditCommentDrop(e: DragEvent) {
		const textarea = e.currentTarget as HTMLTextAreaElement | null;
		if (!textarea) return;
		await handleMarkdownDrop(
			e,
			fieldBinding(
				textarea,
				() => editCommentBody,
				(next) => (editCommentBody = next)
			)
		);
	}

	async function pickCommentFiles() {
		if (!commentTextareaEl) return;
		await openMarkdownAssetPicker(
			fieldBinding(
				commentTextareaEl,
				() => newComment,
				(next) => (newComment = next)
			)
		);
	}
</script>

<svelte:head>
	<title>Issue #{issueNumber}{issue ? ' · ' + issue.title : ''}</title>
</svelte:head>

<svelte:window
	onclick={(e) => {
		if (!(e.target as HTMLElement).closest('.sidebar-section')) openSidebarDropdown = null;
	}}
/>

{#if loading}
	<div class="space-y-3">
		<div class="h-8 w-2/3 rounded-md bg-card animate-pulse"></div>
		<div class="h-48 rounded-md border border-border bg-card animate-pulse"></div>
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else if issue}
	{#if isRepoArchived}
		<div class="mb-4 rounded-md border border-amber-700/40 bg-amber-900/20 px-4 py-3 text-sm text-amber-300">This repository is archived. Issues and comments are read-only.</div>
	{/if}

	<!-- Title row -->
	<div class="flex items-start justify-between gap-4 mb-2">
		{#if editingTitle}
			<input
				type="text"
				bind:value={editTitle}
				class="h-9 flex-1 rounded-md border border-border bg-background px-3 text-xl font-semibold text-foreground focus:outline-none focus:ring-1 focus:ring-primary"
			/>
		{:else}
			<h1 class="text-2xl font-semibold text-foreground leading-snug">
				{issue.title}
				<span class="text-muted-foreground font-normal ml-1">#{issue.number}</span>
			</h1>
		{/if}
		<div class="flex gap-2 shrink-0">
			{#if isAuthor && !isRepoArchived}
				{#if editingTitle}
					<Button size="sm" variant="brand" onclick={saveEditTitle} disabled={editSaving || !editTitle.trim()}>
						{#if editSaving}<Loader class="h-4 w-4 animate-spin" />{/if}
						Save
					</Button>
					<Button size="sm" variant="outline" onclick={() => (editingTitle = false)}>Cancel</Button>
				{:else}
					<Button size="sm" variant="outline" onclick={startEditTitle}>Edit</Button>
				{/if}
			{/if}
			{#if !isRepoArchived}
				<Button size="sm" href="/{username}/{repo}/issues/new">New issue</Button>
			{/if}
		</div>
	</div>

	<!-- Status + meta -->
	<div class="flex items-center gap-3 flex-wrap mb-4">
		{#if issue.status === 'open'}
			<span class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-sm font-semibold bg-brand text-white">
				<CircleDot class="h-4 w-4" />
				Open
			</span>
		{:else}
			<span class="inline-flex items-center gap-1.5 px-3 py-1 rounded-full text-sm font-semibold bg-[#6e40c9] text-white">
				<CheckCircle class="h-4 w-4" />
				Closed
			</span>
		{/if}
		<span class="text-sm text-muted-foreground">
			<a href="/{issue.author?.username}" class="font-semibold text-foreground hover:text-primary">{issue.author?.username ?? ''}</a>
			opened this issue {timeAgo(issue.created_at)} ·
			{comments.length} comment{comments.length !== 1 ? 's' : ''}
		</span>
	</div>

	<div class="border-t border-border mb-5"></div>

	{#if actionError}
		<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400 flex items-center gap-2">
			<AlertCircle class="h-4 w-4 shrink-0" />{actionError}
		</div>
	{/if}

	<!-- Main layout -->
	<div class="flex gap-6 items-start">
		<!-- Timeline -->
		<div class="flex-1 min-w-0 space-y-4">
			<!-- Issue body -->
			<div class="flex gap-3 items-start">
				<div class="w-10 h-10 rounded-full bg-secondary shrink-0 flex items-center justify-center text-sm font-semibold text-foreground border border-border overflow-hidden">
					{#if issue.author?.avatar_url}
						<img src={mediaUrl(issue.author.avatar_url)} alt={issue.author?.username ?? 'author'} class="h-full w-full object-cover" />
					{:else}
						{avatarLetter(issue.author?.username)}
					{/if}
				</div>
				<div class="flex-1 rounded-md border border-border bg-card overflow-hidden">
					<div class="flex items-center justify-between px-3 py-2 border-b border-border bg-secondary/40">
						<span class="text-sm text-muted-foreground">
							<a href="/{issue.author?.username}" class="font-semibold text-foreground hover:text-primary">{issue.author?.username ?? ''}</a>
							opened this issue {timeAgo(issue.created_at)}
						</span>
						<div class="flex items-center gap-1.5">
							{#if isAuthor && !isRepoArchived}
								<span class="text-xs rounded-full border border-primary/30 text-primary px-2 py-0.5">Author</span>
								{#if !editingBody}
									<button onclick={startEditBody} class="p-1 text-muted-foreground hover:text-foreground hover:bg-secondary rounded transition-colors">
										<Pencil class="h-3.5 w-3.5" />
									</button>
								{/if}
							{/if}
						</div>
					</div>
					<div class="px-4 py-4">
						{#if editingBody}
							<div class="mb-3 rounded-md border border-border overflow-hidden">
								<div class="flex items-center gap-0.5 border-b border-border bg-secondary/30 px-2 py-1">
									<Button variant="ghost" size="sm" class={editBodyTab === 'write' ? 'bg-card text-foreground' : 'text-muted-foreground'} onclick={() => (editBodyTab = 'write')}
										>Write</Button
									>
									<Button variant="ghost" size="sm" class={editBodyTab === 'preview' ? 'bg-card text-foreground' : 'text-muted-foreground'} onclick={() => (editBodyTab = 'preview')}
										>Preview</Button
									>
								</div>
								{#if editBodyTab === 'write'}
									<textarea
										bind:value={editBody}
										use:mentionAutocomplete={{ users: mentionUsers }}
										rows={8}
										onpaste={handleEditBodyPaste}
										ondragover={handleMarkdownDragOver}
										ondrop={handleEditBodyDrop}
										class="w-full border-0 bg-background px-3 py-2.5 text-sm text-foreground focus:outline-none resize-y"
									></textarea>
								{:else}
									<div class="min-h-28 bg-background px-3 py-2.5">
										{#if renderedEditBodyPreview}
											<div
												use:mentionHoverCard
												class="prose prose-invert prose-sm max-w-none text-foreground [&_a]:text-primary [&_code]:bg-secondary [&_code]:px-1 [&_code]:rounded [&_pre]:bg-secondary [&_pre]:p-3 [&_pre]:rounded-md [&_pre]:overflow-auto"
											>
												{@html renderedEditBodyPreview}
											</div>
										{:else}
											<p class="text-sm text-muted-foreground italic">Nothing to preview</p>
										{/if}
									</div>
								{/if}
							</div>
							<div class="flex gap-2">
								<Button size="sm" variant="brand" onclick={saveEditBody} disabled={editSaving}>
									{#if editSaving}<Loader class="h-4 w-4 animate-spin" />{/if}
									Save
								</Button>
								<Button size="sm" variant="outline" onclick={() => (editingBody = false)}>Cancel</Button>
							</div>
						{:else if renderedBody}
							<div
								use:mentionHoverCard
								class="prose prose-invert prose-sm max-w-none text-foreground [&_a]:text-primary [&_code]:bg-secondary [&_code]:px-1 [&_code]:rounded [&_pre]:bg-secondary [&_pre]:p-3 [&_pre]:rounded-md [&_pre]:overflow-auto"
							>
								{@html renderedBody}
							</div>
						{:else}
							<p class="text-sm text-muted-foreground italic">No description provided.</p>
						{/if}
					</div>
				</div>
			</div>

			<!-- Comments -->
			{#each comments as comment}
				<div class="flex gap-3 items-start">
					<div class="w-10 h-10 rounded-full bg-secondary shrink-0 flex items-center justify-center text-sm font-semibold text-foreground border border-border overflow-hidden">
						{#if comment.author?.avatar_url}
							<img src={mediaUrl(comment.author.avatar_url)} alt={comment.author?.username ?? 'author'} class="h-full w-full object-cover" />
						{:else}
							{avatarLetter(comment.author?.username)}
						{/if}
					</div>
					<div class="flex-1 rounded-md border border-border bg-card overflow-hidden">
						<div class="flex items-center justify-between px-3 py-2 border-b border-border bg-secondary/40">
							<span class="text-sm text-muted-foreground">
								<a href="/{comment.author?.username}" class="font-semibold text-foreground hover:text-primary">{comment.author?.username ?? ''}</a>
								commented {timeAgo(comment.created_at)}
							</span>
							{#if isLoggedIn && !isRepoArchived && comment.author_id === currentUser?.id}
								<div class="flex items-center gap-1">
									<button onclick={() => startEditComment(comment)} class="p-1 text-muted-foreground hover:text-foreground hover:bg-secondary rounded transition-colors">
										<Pencil class="h-3.5 w-3.5" />
									</button>
									<button onclick={() => deleteComment(comment.id)} class="p-1 text-muted-foreground hover:text-red-400 hover:bg-red-900/20 rounded transition-colors">
										<Trash2 class="h-3.5 w-3.5" />
									</button>
								</div>
							{/if}
						</div>
						<div class="px-4 py-4">
							{#if editingCommentId === comment.id}
								<div class="mb-3 rounded-md border border-border overflow-hidden">
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
											bind:value={editCommentBody}
											use:mentionAutocomplete={{ users: mentionUsers }}
											rows={5}
											onpaste={handleEditCommentPaste}
											ondragover={handleMarkdownDragOver}
											ondrop={handleEditCommentDrop}
											class="w-full border-0 bg-background px-3 py-2.5 text-sm text-foreground focus:outline-none resize-y"
										></textarea>
									{:else}
										<div class="min-h-24 bg-background px-3 py-2.5">
											{#if renderedEditCommentPreview}
												<div
													use:mentionHoverCard
													class="prose prose-invert prose-sm max-w-none text-foreground [&_a]:text-primary [&_code]:bg-secondary [&_code]:px-1 [&_code]:rounded [&_pre]:bg-secondary [&_pre]:p-3 [&_pre]:rounded-md [&_pre]:overflow-auto"
												>
													{@html renderedEditCommentPreview}
												</div>
											{:else}
												<p class="text-sm text-muted-foreground italic">Nothing to preview</p>
											{/if}
										</div>
									{/if}
								</div>
								<div class="flex gap-2">
									<Button size="sm" variant="brand" onclick={() => saveEditComment(comment.id)} disabled={!editCommentBody.trim()}>Save</Button>
									<Button
										size="sm"
										variant="outline"
										onclick={() => {
											editingCommentId = null;
											editCommentBody = '';
										}}>Cancel</Button
									>
								</div>
							{:else}
								<div
									use:mentionHoverCard
									class="prose prose-invert prose-sm max-w-none text-foreground [&_a]:text-primary [&_code]:bg-secondary [&_code]:px-1 [&_code]:rounded [&_pre]:bg-secondary [&_pre]:p-3 [&_pre]:rounded-md [&_pre]:overflow-auto"
								>
									{@html renderMarkdown(comment.body)}
								</div>
							{/if}
						</div>
					</div>
				</div>
			{/each}

			{#if commentActionError}
				<div class="rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400">{commentActionError}</div>
			{/if}

			<!-- Add a comment -->
			{#if isLoggedIn && !isRepoArchived}
				<div class="flex gap-3 items-start">
					<div class="w-10 h-10 rounded-full bg-secondary shrink-0 flex items-center justify-center text-sm font-semibold text-foreground border border-border overflow-hidden">
						{#if currentUser?.avatar_url}
							<img src={mediaUrl(currentUser.avatar_url)} alt={currentUser?.username ?? 'author'} class="h-full w-full object-cover" />
						{:else}
							{avatarLetter(currentUser?.username)}
						{/if}
					</div>
					<div class="flex-1 rounded-md border border-border bg-card overflow-hidden">
						<!-- Tabs + toolbar -->
						<div class="flex items-center justify-between px-2 py-1.5 border-b border-border bg-secondary/40">
							<div class="flex gap-0.5">
								<Button variant="ghost" size="sm" class={commentTab === 'write' ? 'bg-card text-foreground' : 'text-muted-foreground'} onclick={() => (commentTab = 'write')}
									>Write</Button
								>
								<Button variant="ghost" size="sm" class={commentTab === 'preview' ? 'bg-card text-foreground' : 'text-muted-foreground'} onclick={() => (commentTab = 'preview')}
									>Preview</Button
								>
							</div>
							{#if commentTab === 'write'}
								<div class="flex gap-0.5">
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('**', '**', 'bold text')} title="Bold"><Bold class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('_', '_', 'italic text')} title="Italic"><Italic class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('## ', '', 'heading')} title="Heading"><Heading class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('`', '`', 'code')} title="Code"><Code class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('[', '](url)', 'link text')} title="Link"><LinkIcon class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('\n- ', '', 'list item')} title="Unordered list"><List class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('\n1. ', '', 'list item')} title="Ordered list"
										><ListOrdered class="h-3.5 w-3.5" /></Button
									>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('- [ ] ', '', 'task')} title="Task list"><CheckSquare class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('\n> ', '', 'quote')} title="Quote"><Quote class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertCommentMarkdown('@', '', 'username')} title="Mention"><AtSign class="h-3.5 w-3.5" /></Button>
								</div>
							{/if}
						</div>
						<!-- Body -->
						{#if commentTab === 'write'}
							<textarea
								bind:this={commentTextareaEl}
								bind:value={newComment}
								use:mentionAutocomplete={{ users: mentionUsers }}
								rows={5}
								placeholder="Leave a comment"
								onpaste={handleCommentPaste}
								ondragover={handleMarkdownDragOver}
								ondrop={handleCommentDrop}
								class="w-full min-h-30 px-4 py-3 text-sm text-foreground placeholder:text-muted-foreground bg-transparent border-0 focus:outline-none resize-y"
							></textarea>
						{:else}
							<div class="px-4 py-3 min-h-30">
								{#if newComment}
									<div
										use:mentionHoverCard
										class="prose prose-invert prose-sm max-w-none text-foreground [&_a]:text-primary [&_code]:bg-secondary [&_code]:px-1 [&_code]:rounded [&_pre]:bg-secondary [&_pre]:p-3 [&_pre]:rounded-md [&_pre]:overflow-auto"
									>
										{@html renderedCommentPreview}
									</div>
								{:else}
									<p class="text-sm text-muted-foreground italic">Nothing to preview</p>
								{/if}
							</div>
						{/if}
						<!-- Footer -->
						<div class="flex items-center justify-between px-3 py-2 border-t border-border bg-secondary/20">
							<button type="button" class="text-xs text-muted-foreground hover:text-foreground transition-colors" onclick={pickCommentFiles} disabled={markdownUploading}>
								{markdownUploading ? 'Uploading…' : 'Paste, drop, or click to add files'}
							</button>
							<div class="flex gap-2">
								{#if issue.status === 'open' && isAuthor && !isRepoArchived}
									<Button size="sm" variant="outline" onclick={handleClose} disabled={actionLoading}>
										{#if actionLoading}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
										Close issue
									</Button>
								{:else if issue.status === 'closed' && isAuthor && !isRepoArchived}
									<Button size="sm" variant="outline" onclick={handleReopen} disabled={actionLoading}>
										{#if actionLoading}<Loader class="h-3.5 w-3.5 animate-spin" />{/if}
										Reopen issue
									</Button>
								{/if}
								<Button variant="brand" size="sm" onclick={submitComment} disabled={commentSaving || !newComment.trim()}>
									{#if commentSaving}<Loader class="h-4 w-4 animate-spin" />{/if}
									Comment
								</Button>
							</div>
						</div>
						{#if markdownUploadError}
							<div class="px-3 pb-2 text-xs text-red-400">{markdownUploadError}</div>
						{/if}
					</div>
				</div>
			{:else if !isLoggedIn}
				<div class="text-center text-sm text-muted-foreground py-4">
					<a href="/login" class="text-primary hover:underline">Sign in</a> to leave a comment.
				</div>
			{:else}
				<div class="text-center text-sm text-muted-foreground py-4">Comments are disabled because this repository is archived.</div>
			{/if}
		</div>

		<!-- Sidebar -->
		<div class="w-60 shrink-0 divide-y divide-border">
			<!-- Assignees -->
			<div class="py-4 relative sidebar-section">
				<div class="flex items-center justify-between mb-2">
					<h4 class="text-xs font-semibold text-foreground">Assignees</h4>
					{#if isAuthor && !isRepoArchived}
						<button onclick={() => (openSidebarDropdown = openSidebarDropdown === 'assignees' ? null : 'assignees')} class="text-muted-foreground hover:text-foreground transition-colors">
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
								class:bg-secondary={issue.assignee_id === user.id}
							>
								<div class="w-5 h-5 rounded-full bg-secondary border border-border flex items-center justify-center text-xs font-semibold shrink-0">
									{avatarLetter(user.username)}
								</div>
								<span class="text-foreground text-xs">{user.username}</span>
								{#if issue.assignee_id === user.id}
									<span class="ml-auto text-primary text-xs">✓</span>
								{/if}
							</button>
						{/each}
						{#if possibleAssignees.length === 0}
							<p class="px-3 py-2 text-xs text-muted-foreground">No assignees available</p>
						{/if}
					</div>
				{/if}
				{#if issue.assignee}
					<div class="flex items-center gap-2">
						<div class="w-5 h-5 rounded-full bg-secondary border border-border flex items-center justify-center text-xs font-semibold shrink-0">
							{avatarLetter(issue.assignee.username)}
						</div>
						<span class="text-xs text-foreground">{issue.assignee.username}</span>
					</div>
				{:else}
					<p class="text-xs text-muted-foreground">
						No one
						{#if isAuthor && !isRepoArchived}
							— <button onclick={() => updateAssignee(currentUser!.id)} class="text-primary hover:underline">Assign yourself</button>
						{/if}
					</p>
				{/if}
			</div>

			<!-- Labels -->
			<div class="py-4 relative sidebar-section">
				<div class="flex items-center justify-between mb-2">
					<h4 class="text-xs font-semibold text-foreground">Labels</h4>
					{#if isAuthor && !isRepoArchived}
						<button onclick={() => (openSidebarDropdown = openSidebarDropdown === 'labels' ? null : 'labels')} class="text-muted-foreground hover:text-foreground transition-colors">
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
								{#if selectedLabelIds.includes(label.id)}
									<span class="ml-auto text-primary text-xs">✓</span>
								{/if}
							</button>
						{/each}
						{#if labelList.length === 0 && !showCreateLabel}
							<p class="px-3 py-2 text-xs text-muted-foreground">No labels available</p>
						{/if}
						<!-- Create label form -->
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
				{#if issue.labels.length === 0}
					<p class="text-xs text-muted-foreground">No labels</p>
				{:else}
					<div class="flex flex-wrap gap-1.5">
						{#each issue.labels as label}
							<span class="px-2.5 py-0.5 rounded-full text-xs font-medium" style="background-color:{label.color};color:{textColorForBg(label.color)}">
								{label.name}
							</span>
						{/each}
					</div>
				{/if}
			</div>

			<!-- Type -->
			<div class="py-4 relative sidebar-section">
				<div class="flex items-center justify-between mb-2">
					<h4 class="text-xs font-semibold text-foreground">Type</h4>
					{#if isAuthor && !isRepoArchived}
						<button onclick={() => (openSidebarDropdown = openSidebarDropdown === 'type' ? null : 'type')} class="text-muted-foreground hover:text-foreground transition-colors">
							<Settings class="h-3.5 w-3.5" />
						</button>
					{/if}
				</div>
				{#if openSidebarDropdown === 'type'}
					<div class="absolute right-0 top-10 z-20 w-56 rounded-md border border-border bg-card shadow-lg overflow-hidden">
						{#each ISSUE_TYPES as type}
							<button
								onclick={() => updateType(type.value)}
								class="flex items-center gap-2 w-full px-3 py-2 text-sm hover:bg-secondary transition-colors text-left"
								class:bg-secondary={issue.issue_type === type.value}
							>
								<span class="w-2.5 h-2.5 rounded-full shrink-0" style="background-color:{type.color}"></span>
								<span class="text-foreground text-xs">{type.label}</span>
								{#if issue.issue_type === type.value}
									<span class="ml-auto text-primary text-xs">✓</span>
								{/if}
							</button>
						{/each}
					</div>
				{/if}
				{#if issue.issue_type}
					{@const typeObj = ISSUE_TYPES.find((t) => t.value === issue!.issue_type)}
					{#if typeObj}
						<div class="flex items-center gap-2">
							<span class="w-2.5 h-2.5 rounded-full shrink-0" style="background-color:{typeObj.color}"></span>
							<span class="text-xs text-foreground">{typeObj.label}</span>
						</div>
					{:else}
						<span class="text-xs text-foreground">{issue.issue_type}</span>
					{/if}
				{:else}
					<p class="text-xs text-muted-foreground">No type</p>
				{/if}
			</div>

			<!-- Projects (static) -->
			<div class="py-4">
				<div class="flex items-center justify-between mb-2">
					<h4 class="text-xs font-semibold text-foreground">Projects</h4>
					<button class="text-muted-foreground hover:text-foreground transition-colors"><Settings class="h-3.5 w-3.5" /></button>
				</div>
				<p class="text-xs text-muted-foreground">No projects</p>
			</div>

			<!-- Milestone -->
			<div class="py-4 sidebar-section">
				<div class="flex items-center justify-between mb-2">
					<h4 class="text-xs font-semibold text-foreground">Milestone</h4>
					{#if isLoggedIn}
						<button onclick={() => (openSidebarDropdown = openSidebarDropdown === 'milestone' ? null : 'milestone')} class="text-muted-foreground hover:text-foreground transition-colors"
							><Settings class="h-3.5 w-3.5" /></button
						>
					{/if}
				</div>
				{#if openSidebarDropdown === 'milestone'}
					<div class="mt-1 border border-border rounded-md bg-popover shadow-lg z-10 overflow-hidden">
						{#if milestoneList.length === 0}
							<p class="px-3 py-2 text-xs text-muted-foreground">No milestones available</p>
						{:else}
							{#each milestoneList as milestone}
								<button class="w-full flex items-center gap-2 px-3 py-1.5 text-xs hover:bg-accent transition-colors text-left" onclick={() => updateMilestone(milestone.id)}>
									<span class="flex-1">{milestone.title}</span>
									{#if issue?.milestone_id === milestone.id}
										<CheckCircle class="h-3.5 w-3.5 text-primary shrink-0" />
									{/if}
								</button>
							{/each}
						{/if}
					</div>
				{:else if issue?.milestone}
					<p class="text-xs text-foreground font-medium">{issue.milestone.title}</p>
					{#if issue.milestone.description}
						<p class="text-xs text-muted-foreground mt-0.5">{issue.milestone.description}</p>
					{/if}
				{:else}
					<p class="text-xs text-muted-foreground">No milestone</p>
				{/if}
			</div>

			<!-- Danger zone -->
			{#if isAuthor}
				<div class="py-4">
					<button onclick={handleDelete} class="flex items-center gap-1.5 text-xs text-red-400 hover:text-red-300 transition-colors">
						<Trash2 class="h-3.5 w-3.5" />
						Delete issue
					</button>
				</div>
			{/if}
		</div>
	</div>
{/if}
