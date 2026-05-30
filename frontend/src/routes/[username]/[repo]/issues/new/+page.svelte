<script lang="ts">
	import { page } from '$app/state';
	import { goto } from '$app/navigation';
	import { issues, labels, milestones as milestones_api, repos, type Label, type Milestone, type User, type Collaborator } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { getContext } from 'svelte';
	import { AlertCircle, Loader, Settings, Bold, Italic, Heading, List, ListOrdered, Code, Link, Quote, AtSign, Paperclip, CheckSquare } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { renderMarkdownHtml } from '$lib/markdown';
	import { mediaUrl } from '$lib/utils';
	import { handleMarkdownPaste, handleMarkdownDrop, handleMarkdownDragOver, openMarkdownAssetPicker } from '$lib/hooks/markdown-assets';
	import { mentionAutocomplete } from '$lib/hooks/mention-autocomplete';
	import { mentionHoverCard } from '$lib/hooks/mention-hover-card';
	let selectedMilestoneId = $state<number | null>(null);
	let createMore = $state(false);

	// Load state
	let loading = $state(true);
	let error = $state('');
	let submitting = $state(false);
	let labelList = $state<Label[]>([]);
	let collaborators = $state<Collaborator[]>([]);
	let milestoneList = $state<Milestone[]>([]);

	// Form fields
	let title = $state('');
	let body = $state('');
	let selectedLabelIds = $state<number[]>([]);
	let newLabelName = $state('');
	let newLabelColor = $state('#1f6feb');
	let labelCreating = $state(false);
	let showCreateLabel = $state(false);
	let selectedAssigneeId = $state<number | null>(null);
	let selectedType = $state('');
	let descriptionTab = $state<'write' | 'preview'>('write');
	let textareaEl = $state<HTMLTextAreaElement | null>(null);
	let markdownUploading = $state(false);
	let markdownUploadError = $state('');

	// Dropdown visibility
	let openDropdown = $state<'assignees' | 'labels' | 'type' | 'milestone' | null>(null);

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
	const isLoggedIn = $derived(authStore.user != null);
	const currentUser = $derived(authStore.user);

	async function load() {
		loading = true;
		try {
			const [labelData, collabData, milestoneData] = await Promise.all([
				labels.list(username!, repo!),
				repos.collaborators.list(username!, repo!).catch(() => ({ collaborators: [] })),
				milestones_api.list(username!, repo!).catch(() => ({ milestones: [] }))
			]);
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

	const selectedLabels = $derived(labelList.filter((l) => selectedLabelIds.includes(l.id)));
	const selectedAssignee = $derived(selectedAssigneeId === currentUser?.id ? currentUser : (collaborators.find((c) => c.user_id === selectedAssigneeId)?.user ?? null));
	const selectedTypeObj = $derived(ISSUE_TYPES.find((t) => t.value === selectedType) ?? null);

	// Possible assignees: current user + collaborators' users (deduped)
	const possibleAssignees = $derived.by(() => {
		const result: User[] = [];
		if (currentUser) result.push(currentUser);
		for (const c of collaborators) {
			if (c.user && c.user_id !== currentUser?.id) result.push(c.user);
		}
		return result;
	});
	const mentionUsers = $derived(possibleAssignees.map((u) => ({ username: u.username, avatar_url: u.avatar_url })));

	function toggleLabel(id: number) {
		selectedLabelIds = selectedLabelIds.includes(id) ? selectedLabelIds.filter((l) => l !== id) : [...selectedLabelIds, id];
	}

	async function createLabel() {
		if (isRepoArchived) {
			error = 'This repository is archived and read-only.';
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
			error = e.message;
		} finally {
			labelCreating = false;
		}
	}

	function toggleAssignee(userId: number) {
		selectedAssigneeId = selectedAssigneeId === userId ? null : userId;
		openDropdown = null;
	}

	function assignSelf() {
		if (!currentUser) return;
		selectedAssigneeId = currentUser.id;
		openDropdown = null;
	}

	function selectType(value: string) {
		selectedType = selectedType === value ? '' : value;
		openDropdown = null;
	}

	function toggleDropdown(name: 'assignees' | 'labels' | 'type' | 'milestone') {
		openDropdown = openDropdown === name ? null : name;
	}

	function textColorForBg(hex: string): string {
		const r = parseInt(hex.slice(1, 3), 16);
		const g = parseInt(hex.slice(3, 5), 16);
		const b = parseInt(hex.slice(5, 7), 16);
		const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
		return luminance > 0.5 ? '#000000' : '#ffffff';
	}

	async function handleSubmit() {
		if (isRepoArchived) {
			error = 'This repository is archived and read-only.';
			return;
		}
		if (!title.trim()) return;
		submitting = true;
		error = '';
		try {
			const res = await issues.create(username!, repo!, {
				title: title.trim(),
				body: body || undefined,
				issue_type: selectedType || undefined,
				assignee_id: selectedAssigneeId ?? undefined,
				milestone_id: selectedMilestoneId ?? undefined,
				label_ids: selectedLabelIds.length > 0 ? selectedLabelIds : undefined
			});
			if (createMore) {
				title = '';
				body = '';
				selectedLabelIds = [];
				selectedAssigneeId = null;
				selectedType = '';
				selectedMilestoneId = null;
			} else {
				goto(`/${username}/${repo}/issues/${res.issue.number}`);
			}
		} catch (e: any) {
			error = e.message;
		} finally {
			submitting = false;
		}
	}

	function handleKeydown(e: KeyboardEvent) {
		if ((e.ctrlKey || e.metaKey) && e.key === 'Enter') {
			e.preventDefault();
			handleSubmit();
		}
	}

	function insertMarkdown(prefix: string, suffix = '', placeholder = '') {
		const el = textareaEl;
		if (!el) return;
		const start = el.selectionStart;
		const end = el.selectionEnd;
		const selected = body.slice(start, end) || placeholder;
		body = body.slice(0, start) + prefix + selected + suffix + body.slice(end);
		setTimeout(() => {
			el.focus();
			const newPos = start + prefix.length + selected.length;
			el.setSelectionRange(newPos, newPos);
		}, 0);
	}

	function markdownFieldForBody(textarea: HTMLTextAreaElement) {
		return {
			username: username!,
			repo: repo!,
			textarea,
			getValue: () => body,
			setValue: (next: string) => (body = next),
			onUploadState: (uploading: boolean) => (markdownUploading = uploading),
			onError: (message: string) => (markdownUploadError = message)
		};
	}

	async function handleBodyPaste(e: ClipboardEvent) {
		if (!textareaEl) return;
		await handleMarkdownPaste(e, markdownFieldForBody(textareaEl));
	}

	async function handleBodyDrop(e: DragEvent) {
		if (!textareaEl) return;
		await handleMarkdownDrop(e, markdownFieldForBody(textareaEl));
	}

	async function handleBodyPickFiles() {
		if (!textareaEl) return;
		await openMarkdownAssetPicker(markdownFieldForBody(textareaEl));
	}

	const renderedPreview = $derived(renderMarkdownHtml(body));
</script>

<svelte:head>
	<title>New issue</title>
</svelte:head>

<svelte:window
	onclick={(e) => {
		if (!(e.target as HTMLElement).closest('.sidebar-dropdown')) openDropdown = null;
	}}
/>

{#if !isLoggedIn}
	<div class="max-w-3xl mx-auto rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">You must be signed in to create an issue.</div>
{:else if isRepoArchived}
	<div class="max-w-3xl mx-auto rounded-md border border-amber-700/40 bg-amber-900/20 p-4 text-sm text-amber-300">This repository is archived and read-only. New issues cannot be created.</div>
{:else}
	<div>
		<h2 class="text-xl font-semibold text-foreground mb-5">Create new issue</h2>

		{#if error}
			<div class="mb-4 rounded-md border border-red-800/40 bg-red-900/20 px-4 py-3 text-sm text-red-400 flex items-center gap-2">
				<AlertCircle class="h-4 w-4 shrink-0" />{error}
			</div>
		{/if}

		<div class="flex gap-5 items-start">
			<!-- ── Main form ─────────────────────────────────────────────────── -->
			<div class="flex-1 min-w-0">
				<!-- Title -->
				<div class="mb-4">
					<label for="issue-title" class="block text-sm font-semibold text-foreground mb-1.5">
						Add a title <span class="text-red-400">*</span>
					</label>
					<input
						id="issue-title"
						type="text"
						bind:value={title}
						placeholder="Title"
						onkeydown={handleKeydown}
						class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				</div>

				<!-- Description editor -->
				<div class="mb-4">
					<label for="issue-body" class="block text-sm font-semibold text-foreground mb-1.5">Add a description</label>

					<div class="rounded-md border border-border overflow-hidden focus-within:ring-1 focus-within:ring-primary focus-within:border-primary">
						<!-- Tabs + toolbar -->
						<div class="flex items-center justify-between border-b border-border bg-secondary/30 px-2 py-1">
							<div class="flex items-center gap-0.5">
								<Button
									variant="ghost"
									size="sm"
									onclick={() => (descriptionTab = 'write')}
									class="px-3 h-7 text-xs rounded-md {descriptionTab === 'write' ? 'bg-background text-foreground font-semibold' : 'text-muted-foreground'}">Write</Button
								>
								<Button
									variant="ghost"
									size="sm"
									onclick={() => (descriptionTab = 'preview')}
									class="px-3 h-7 text-xs rounded-md {descriptionTab === 'preview' ? 'bg-background text-foreground font-semibold' : 'text-muted-foreground'}">Preview</Button
								>
							</div>
							{#if descriptionTab === 'write'}
								<div class="flex items-center gap-0.5">
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('**', '**', 'bold text')} title="Bold"><Bold class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('_', '_', 'italic text')} title="Italic"><Italic class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('## ', '', 'Heading')} title="Heading"><Heading class="h-3.5 w-3.5" /></Button>
									<div class="w-px h-4 bg-border mx-1"></div>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('`', '`', 'code')} title="Inline code"><Code class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('[', '](url)', 'link text')} title="Link"><Link class="h-3.5 w-3.5" /></Button>
									<div class="w-px h-4 bg-border mx-1"></div>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('- ', '', 'List item')} title="Unordered list"><List class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('1. ', '', 'List item')} title="Ordered list"><ListOrdered class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('- [ ] ', '', 'Task')} title="Task list"><CheckSquare class="h-3.5 w-3.5" /></Button>
									<div class="w-px h-4 bg-border mx-1"></div>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('> ', '', 'Quote')} title="Quote"><Quote class="h-3.5 w-3.5" /></Button>
									<Button variant="ghost" size="icon-sm" onclick={() => insertMarkdown('@')} title="Mention"><AtSign class="h-3.5 w-3.5" /></Button>
								</div>
							{/if}
						</div>

						{#if descriptionTab === 'write'}
							<textarea
								id="issue-body"
								bind:this={textareaEl}
								bind:value={body}
								use:mentionAutocomplete={{ users: mentionUsers }}
								rows={12}
								placeholder="Type your description here..."
								onkeydown={handleKeydown}
								onpaste={handleBodyPaste}
								ondragover={handleMarkdownDragOver}
								ondrop={handleBodyDrop}
								class="w-full bg-background px-3 py-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none resize-y min-h-40"
							></textarea>
						{:else}
							<div class="min-h-40 px-3 py-3">
								{#if renderedPreview}
									<div
										use:mentionHoverCard
										class="prose prose-invert prose-sm max-w-none text-foreground [&_a]:text-primary [&_code]:bg-secondary [&_code]:px-1 [&_code]:rounded [&_pre]:bg-secondary [&_pre]:p-3 [&_pre]:rounded-md"
									>
										{@html renderedPreview}
									</div>
								{:else}
									<p class="text-sm text-muted-foreground italic">Nothing to preview.</p>
								{/if}
							</div>
						{/if}

						<div class="flex items-center gap-3 border-t border-border bg-secondary/20 px-3 py-2">
							<Button variant="ghost" size="sm" class="gap-1.5 text-xs text-muted-foreground px-0 hover:bg-transparent" onclick={handleBodyPickFiles} disabled={markdownUploading}>
								<Paperclip class="h-3.5 w-3.5" />
								{markdownUploading ? 'Uploading…' : 'Paste, drop, or click to add files'}
							</Button>
							{#if markdownUploadError}
								<span class="text-xs text-red-400">{markdownUploadError}</span>
							{/if}
						</div>
					</div>
				</div>

				<!-- Actions -->
				<div class="flex items-center justify-end gap-2">
					<label class="flex items-center gap-2 cursor-pointer mr-auto">
						<input type="checkbox" bind:checked={createMore} class="rounded border-border bg-background" />
						<span class="text-sm text-muted-foreground">Create more</span>
					</label>
					<Button variant="outline" size="sm" href="/{username}/{repo}/issues">Cancel</Button>
					<Button onclick={handleSubmit} disabled={submitting || !title.trim()} size="sm" class="bg-brand hover:bg-[#2ea043] text-white gap-1.5">
						{#if submitting}<Loader class="h-4 w-4 animate-spin" />{/if}
						Create
						<kbd class="text-xs opacity-70 font-mono">⌃↵</kbd>
					</Button>
				</div>
			</div>

			<div class="w-56 shrink-0 divide-y divide-border border border-border rounded-md overflow-visible">
				<!-- Assignees -->
				<div class="px-4 py-3 sidebar-dropdown relative">
					<Button
						variant="ghost"
						onclick={(e) => {
							e.stopPropagation();
							toggleDropdown('assignees');
						}}
						class="group flex w-full items-center justify-between p-0 h-auto hover:bg-transparent"
					>
						<span class="text-xs font-semibold text-foreground">Assignees</span>
						<Settings class="h-3.5 w-3.5 text-muted-foreground group-hover:text-foreground transition-colors" />
					</Button>

					{#if selectedAssignee}
						<div class="mt-2 flex items-center gap-2">
							{#if selectedAssignee.avatar_url}
								<img src={mediaUrl(selectedAssignee.avatar_url)} alt={selectedAssignee.username} class="w-5 h-5 rounded-full" />
							{:else}
								<div class="w-5 h-5 rounded-full bg-secondary flex items-center justify-center text-[10px] font-bold text-muted-foreground">
									{selectedAssignee.username[0].toUpperCase()}
								</div>
							{/if}
							<span class="text-xs text-foreground">{selectedAssignee.username}</span>
							<Button
								variant="ghost"
								size="icon-xs"
								onclick={() => {
									selectedAssigneeId = null;
								}}
								class="ml-auto text-muted-foreground hover:text-red-400 hover:bg-transparent">✕</Button
							>
						</div>
					{:else}
						<div class="mt-1">
							<p class="text-xs text-muted-foreground">
								No one —
								<Button variant="link" size="xs" onclick={assignSelf} class="h-auto p-0 text-xs">Assign yourself</Button>
							</p>
						</div>
					{/if}

					{#if openDropdown === 'assignees'}
						<div class="absolute top-full left-0 right-0 z-30 mt-1 rounded-md border border-border bg-popover shadow-lg overflow-hidden">
							<div class="px-3 py-2 border-b border-border">
								<p class="text-xs text-muted-foreground font-semibold">Assign up to yourself</p>
							</div>
							{#each possibleAssignees as user}
								<Button
									variant="ghost"
									onclick={() => toggleAssignee(user.id)}
									class="h-auto w-full justify-start gap-2.5 px-3 py-2 text-xs {selectedAssigneeId === user.id ? 'bg-secondary hover:bg-secondary' : ''}"
								>
									{#if user.avatar_url}
										<img src={mediaUrl(user.avatar_url)} alt={user.username} class="w-5 h-5 rounded-full shrink-0" />
									{:else}
										<div class="w-5 h-5 rounded-full bg-border flex items-center justify-center text-[10px] font-bold shrink-0">
											{user.username[0].toUpperCase()}
										</div>
									{/if}
									<span class="text-foreground">{user.username}</span>
									{#if user.id === currentUser?.id}
										<span class="text-muted-foreground ml-1">(you)</span>
									{/if}
									{#if selectedAssigneeId === user.id}
										<span class="ml-auto text-primary">✓</span>
									{/if}
								</Button>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Labels -->
				<div class="px-4 py-3 sidebar-dropdown relative">
					<Button
						variant="ghost"
						onclick={(e) => {
							e.stopPropagation();
							toggleDropdown('labels');
						}}
						class="group flex w-full items-center justify-between p-0 h-auto hover:bg-transparent"
					>
						<span class="text-xs font-semibold text-foreground">Labels</span>
						<Settings class="h-3.5 w-3.5 text-muted-foreground group-hover:text-foreground transition-colors" />
					</Button>

					{#if selectedLabels.length === 0}
						<p class="mt-1 text-xs text-muted-foreground">No labels</p>
					{:else}
						<div class="flex flex-wrap gap-1 mt-2">
							{#each selectedLabels as label}
								<span class="px-2 py-0.5 rounded-full text-xs font-medium" style="background-color:{label.color};color:{textColorForBg(label.color)}">
									{label.name}
								</span>
							{/each}
						</div>
					{/if}

					{#if openDropdown === 'labels'}
						<div class="absolute top-full left-0 right-0 z-30 mt-1 rounded-md border border-border bg-popover shadow-lg overflow-hidden">
							{#if labelList.length === 0 && !showCreateLabel}
								<p class="px-3 py-2 text-xs text-muted-foreground">No labels available</p>
							{:else}
								{#each labelList as label}
									<Button
										variant="ghost"
										onclick={() => toggleLabel(label.id)}
										class="h-auto w-full justify-start gap-2.5 px-3 py-2 text-xs {selectedLabelIds.includes(label.id) ? 'bg-secondary hover:bg-secondary' : ''}"
									>
										<span class="w-3 h-3 rounded-full shrink-0" style="background-color:{label.color}"></span>
										<span class="text-foreground">{label.name}</span>
										{#if selectedLabelIds.includes(label.id)}
											<span class="ml-auto text-primary">✓</span>
										{/if}
									</Button>
								{/each}
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
				</div>

				<!-- Type -->
				<div class="px-4 py-3 sidebar-dropdown relative">
					<Button
						variant="ghost"
						onclick={(e) => {
							e.stopPropagation();
							toggleDropdown('type');
						}}
						class="group flex w-full items-center justify-between p-0 h-auto hover:bg-transparent"
					>
						<span class="text-xs font-semibold text-foreground">Type</span>
						<Settings class="h-3.5 w-3.5 text-muted-foreground group-hover:text-foreground transition-colors" />
					</Button>

					{#if selectedTypeObj}
						<div class="mt-2 flex items-center gap-2">
							<span class="w-2.5 h-2.5 rounded-full shrink-0" style="background-color:{selectedTypeObj.color}"></span>
							<span class="text-xs text-foreground capitalize">{selectedTypeObj.label}</span>
							<Button
								variant="ghost"
								size="icon-xs"
								onclick={() => {
									selectedType = '';
								}}
								class="ml-auto text-muted-foreground hover:text-red-400 hover:bg-transparent">✕</Button
							>
						</div>
					{:else}
						<p class="mt-1 text-xs text-muted-foreground">No type</p>
					{/if}

					{#if openDropdown === 'type'}
						<div class="absolute top-full left-0 right-0 z-30 mt-1 rounded-md border border-border bg-popover shadow-lg overflow-hidden">
							{#each ISSUE_TYPES as t}
								<Button
									variant="ghost"
									onclick={() => selectType(t.value)}
									class="h-auto w-full justify-start gap-2.5 px-3 py-2 text-xs {selectedType === t.value ? 'bg-secondary hover:bg-secondary' : ''}"
								>
									<span class="w-2.5 h-2.5 rounded-full shrink-0" style="background-color:{t.color}"></span>
									<span class="text-foreground">{t.label}</span>
									{#if selectedType === t.value}
										<span class="ml-auto text-primary">✓</span>
									{/if}
								</Button>
							{/each}
						</div>
					{/if}
				</div>

				<!-- Milestone -->
				<div class="px-4 py-3 sidebar-dropdown relative">
					<Button
						variant="ghost"
						onclick={(e) => {
							e.stopPropagation();
							toggleDropdown('milestone');
						}}
						class="group flex w-full items-center justify-between p-0 h-auto hover:bg-transparent"
					>
						<span class="text-xs font-semibold text-foreground">Milestone</span>
						<Settings class="h-3.5 w-3.5 text-muted-foreground group-hover:text-foreground transition-colors" />
					</Button>

					{#if selectedMilestoneId != null}
						{@const selectedMilestone = milestoneList.find((m) => m.id === selectedMilestoneId)}
						{#if selectedMilestone}
							<div class="mt-2 flex items-center gap-2">
								<span class="flex-1 text-xs text-foreground">{selectedMilestone.title}</span>
								<Button
									variant="ghost"
									size="icon-xs"
									onclick={() => {
										selectedMilestoneId = null;
									}}
									class="text-muted-foreground hover:text-red-400 hover:bg-transparent">✕</Button
								>
							</div>
						{/if}
					{:else}
						<p class="mt-1 text-xs text-muted-foreground">No milestone</p>
					{/if}

					{#if openDropdown === 'milestone'}
						<div class="absolute top-full left-0 right-0 z-30 mt-1 rounded-md border border-border bg-popover shadow-lg overflow-hidden">
							{#if milestoneList.length === 0}
								<p class="px-3 py-2 text-xs text-muted-foreground">No milestones available</p>
							{:else}
								{#each milestoneList as milestone}
									<Button
										variant="ghost"
										onclick={() => {
											selectedMilestoneId = milestone.id;
											openDropdown = null;
										}}
										class="h-auto w-full justify-start gap-2.5 px-3 py-2 text-xs {selectedMilestoneId === milestone.id ? 'bg-secondary hover:bg-secondary' : ''}"
									>
										<span class="flex-1 text-foreground text-left">{milestone.title}</span>
										{#if selectedMilestoneId === milestone.id}
											<span class="ml-auto text-primary">✓</span>
										{/if}
									</Button>
								{/each}
							{/if}
						</div>
					{/if}
				</div>
			</div>
		</div>
	</div>
{/if}
