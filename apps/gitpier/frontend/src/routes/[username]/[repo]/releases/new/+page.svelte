<script lang="ts">
	import { page } from '$app/state';
	import { releases, type Release, type TagInfo } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { getContext, onMount } from 'svelte';
	import { Tag, Upload, X, FileArchive, ChevronDown, AlertCircle } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Textarea } from '$lib/components/ui/textarea/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { handleMarkdownPaste, handleMarkdownDrop, handleMarkdownDragOver, openMarkdownAssetPicker } from '$lib/hooks/markdown-assets';
	import { renderMarkdownHtml } from '$lib/markdown';

	const username = $derived(page.params.username);
	const repoName = $derived(page.params.repo);
	const editId = $derived(page.url.searchParams.get('edit'));
	const isEdit = $derived(editId !== null);

	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const branches = $derived<string[]>(repoCtx?.branches ?? []);

	// Form state
	let tagName = $state('');
	let targetCommit = $state('');
	let name = $state('');
	let body = $state('');
	let bodyTab = $state<'write' | 'preview'>('write');
	let bodyTextareaEl = $state<HTMLTextAreaElement | null>(null);
	let markdownUploading = $state(false);
	let markdownUploadError = $state('');
	let isDraft = $state(false);
	let isPrerelease = $state(false);

	// Existing tags (for dropdown)
	let existingTags = $state<TagInfo[]>([]);
	let showTagDropdown = $state(false);

	// File uploads (only for create mode initially; edit can also add)
	let selectedFiles = $state<File[]>([]);
	let uploading = $state(false);
	let submitting = $state(false);
	let error = $state('');
	let loadingEdit = $state(false);

	let editRelease = $state<Release | null>(null);
	const renderedBodyPreview = $derived(renderMarkdownHtml(body));

	onMount(async () => {
		// Load tags
		try {
			const data = await releases.getTags(username!, repoName!);
			existingTags = data.tags ?? [];
		} catch {}

		// If editing, load the release
		if (isEdit) {
			loadingEdit = true;
			try {
				if (!editId || !editId.trim()) {
					throw new Error('Invalid release id for edit.');
				}
				const data = await releases.get(username!, repoName!, editId);
				editRelease = data.release;
				tagName = editRelease.tag_name;
				targetCommit = editRelease.target_commit;
				name = editRelease.name;
				body = editRelease.body;
				isDraft = editRelease.is_draft;
				isPrerelease = editRelease.is_prerelease;
			} catch (e: any) {
				error = e.message;
			} finally {
				loadingEdit = false;
			}
		} else {
			// Default target to default branch from layout context
			targetCommit = repoCtx?.currentBranch ?? '';
		}
	});

	function handleFileSelect(e: Event) {
		const input = e.target as HTMLInputElement;
		if (!input.files) return;
		const newFiles = Array.from(input.files);
		// Deduplicate by name
		const existing = new Set(selectedFiles.map((f) => f.name));
		selectedFiles = [...selectedFiles, ...newFiles.filter((f) => !existing.has(f.name))];
		input.value = '';
	}

	function removeFile(index: number) {
		selectedFiles = selectedFiles.filter((_, i) => i !== index);
	}

	function formatBytes(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
	}

	async function submit() {
		if (isRepoArchived) {
			error = 'This repository is archived and read-only.';
			return;
		}
		if (!tagName.trim()) {
			error = 'Tag name is required.';
			return;
		}
		submitting = true;
		uploading = false;
		error = '';
		try {
			let saved: Release;
			if (isEdit) {
				if (!editId || !editId.trim()) {
					throw new Error('Invalid release id for edit.');
				}
				const data = await releases.update(username!, repoName!, editId, { name, body, is_draft: isDraft, is_prerelease: isPrerelease });
				saved = data.release;
			} else {
				const data = await releases.create(username!, repoName!, {
					tag_name: tagName.trim(),
					target_commitish: targetCommit.trim() || undefined,
					name: name.trim(),
					body,
					is_draft: isDraft,
					is_prerelease: isPrerelease
				});
				saved = data.release;
			}

			// Upload any selected asset files
			if (selectedFiles.length > 0) {
				uploading = true;
				for (const file of selectedFiles) {
					await releases.uploadAsset(username!, repoName!, saved.id, file);
				}
			}

			goto(`/${username}/${repoName}/releases/${saved.id}`);
		} catch (e: any) {
			error = e.message ?? 'Something went wrong.';
			submitting = false;
			uploading = false;
		}
	}

	function selectTag(t: TagInfo) {
		tagName = t.name;
		showTagDropdown = false;
	}

	function bodyField() {
		if (!bodyTextareaEl) return null;
		return {
			username: username!,
			repo: repoName!,
			textarea: bodyTextareaEl,
			getValue: () => body,
			setValue: (next: string) => (body = next),
			onUploadState: (uploading: boolean) => (markdownUploading = uploading),
			onError: (message: string) => (markdownUploadError = message)
		};
	}

	async function handleBodyPaste(e: ClipboardEvent) {
		const field = bodyField();
		if (!field) return;
		await handleMarkdownPaste(e, field);
	}

	async function handleBodyDrop(e: DragEvent) {
		const field = bodyField();
		if (!field) return;
		await handleMarkdownDrop(e, field);
	}

	async function pickBodyFiles() {
		const field = bodyField();
		if (!field) return;
		await openMarkdownAssetPicker(field);
	}
</script>

<svelte:window
	onclick={(e) => {
		if (!(e.target as HTMLElement).closest('.tag-dropdown-wrap')) showTagDropdown = false;
	}}
/>

<div class="max-w-3xl">
	{#if isRepoArchived}
		<div class="mb-4 rounded-md border border-amber-700/40 bg-amber-900/20 px-4 py-3 text-sm text-amber-300">This repository is archived and read-only. Releases cannot be changed.</div>
	{/if}

	<h1 class="text-xl font-bold text-foreground mb-6">
		{isEdit ? 'Edit release' : 'Create a new release'}
	</h1>

	{#if isRepoArchived}
		<div class="flex items-center gap-3">
			<Button type="button" variant="outline" onclick={() => goto(`/${username}/${repoName}/releases`)}>Back to releases</Button>
		</div>
	{:else if loadingEdit}
		<div class="animate-pulse space-y-4">
			<div class="h-8 w-full bg-secondary rounded"></div>
			<div class="h-8 w-full bg-secondary rounded"></div>
			<div class="h-40 w-full bg-secondary rounded"></div>
		</div>
	{:else}
		<form
			onsubmit={(e) => {
				e.preventDefault();
				submit();
			}}
			class="space-y-6"
		>
			<!-- Tag name -->
			<div class="space-y-1.5">
				<Label for="tag-name">Tag name <span class="text-destructive">*</span></Label>
				<div class="tag-dropdown-wrap relative">
					<div class="flex gap-2">
						<div class="relative flex-1">
							<Input id="tag-name" bind:value={tagName} placeholder="v1.0.0" class="font-mono pr-24" disabled={isEdit} required />
							{#if !isEdit && existingTags.length > 0}
								<button
									type="button"
									onclick={() => (showTagDropdown = !showTagDropdown)}
									class="absolute right-2 top-1/2 -translate-y-1/2 flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground transition-colors"
								>
									Existing <ChevronDown class="h-3.5 w-3.5" />
								</button>
							{/if}
						</div>
					</div>
					{#if showTagDropdown && existingTags.length > 0}
						<div class="absolute z-20 top-full mt-1 left-0 w-64 bg-popover border border-border rounded-md shadow-lg overflow-y-auto max-h-48">
							{#each existingTags as t}
								<button
									type="button"
									class="w-full flex items-center gap-2 px-3 py-2 text-sm text-foreground hover:bg-secondary text-left transition-colors"
									onclick={() => selectTag(t)}
								>
									<Tag class="h-3.5 w-3.5 text-muted-foreground shrink-0" />
									<span class="font-mono truncate">{t.name}</span>
								</button>
							{/each}
						</div>
					{/if}
				</div>
				{#if !isEdit}
					<p class="text-xs text-muted-foreground">Enter a new tag, or pick an existing one. New tags are created at the target branch.</p>
				{/if}
			</div>

			<!-- Target branch/commit (create only) -->
			{#if !isEdit}
				<div class="space-y-1.5">
					<Label for="target">Target branch or commit</Label>
					<Input id="target" bind:value={targetCommit} placeholder={repoCtx?.currentBranch ?? 'main'} list="target-suggestions" />
					<datalist id="target-suggestions">
						{#each branches as b}
							<option value={b}></option>
						{/each}
					</datalist>
					<p class="text-xs text-muted-foreground">Branch or commit SHA to create the tag from (if the tag doesn't exist yet).</p>
				</div>
			{/if}

			<!-- Release title -->
			<div class="space-y-1.5">
				<Label for="release-name">Release title</Label>
				<Input id="release-name" bind:value={name} placeholder={tagName || 'Release title'} />
			</div>

			<!-- Release notes -->
			<div class="space-y-1.5">
				<Label for="body">Release notes (Markdown)</Label>
				<div class="rounded-md border border-border overflow-hidden">
					<div class="flex items-center gap-0.5 border-b border-border bg-secondary/30 px-2 py-1">
						<Button variant="ghost" size="sm" class={bodyTab === 'write' ? 'bg-card text-foreground' : 'text-muted-foreground'} onclick={() => (bodyTab = 'write')}>Write</Button>
						<Button variant="ghost" size="sm" class={bodyTab === 'preview' ? 'bg-card text-foreground' : 'text-muted-foreground'} onclick={() => (bodyTab = 'preview')}>Preview</Button>
					</div>
					{#if bodyTab === 'write'}
						<Textarea
							id="body"
							bind:ref={bodyTextareaEl}
							bind:value={body}
							placeholder="Describe what changed in this release…"
							onpaste={handleBodyPaste}
							ondragover={handleMarkdownDragOver}
							ondrop={handleBodyDrop}
							class="min-h-50 border-0 font-mono text-sm resize-y"
						/>
					{:else}
						<div class="min-h-50 bg-background px-3 py-2.5">
							{#if renderedBodyPreview}
								<div
									class="prose prose-invert prose-sm max-w-none text-foreground [&_a]:text-primary [&_code]:bg-secondary [&_code]:px-1 [&_code]:rounded [&_pre]:bg-secondary [&_pre]:p-3 [&_pre]:rounded-md [&_pre]:overflow-auto"
								>
									{@html renderedBodyPreview}
								</div>
							{:else}
								<p class="text-sm text-muted-foreground italic">Nothing to preview</p>
							{/if}
						</div>
					{/if}
				</div>
				<div class="flex items-center gap-3 text-xs text-muted-foreground">
					<button type="button" class="hover:text-foreground transition-colors" onclick={pickBodyFiles} disabled={markdownUploading}>
						{markdownUploading ? 'Uploading…' : 'Paste, drop, or click to add files'}
					</button>
					{#if markdownUploadError}
						<span class="text-red-400">{markdownUploadError}</span>
					{/if}
				</div>
			</div>

			<!-- Flags -->
			<div class="flex items-center gap-6 flex-wrap">
				<label class="flex items-center gap-2 cursor-pointer select-none">
					<input type="checkbox" bind:checked={isDraft} class="h-4 w-4 rounded border-border accent-brand" />
					<span class="text-sm text-foreground">Save as draft</span>
				</label>
				<label class="flex items-center gap-2 cursor-pointer select-none">
					<input type="checkbox" bind:checked={isPrerelease} class="h-4 w-4 rounded border-border accent-brand" />
					<span class="text-sm text-foreground">Pre-release</span>
				</label>
			</div>

			<!-- Asset uploads -->
			<div class="space-y-2">
				<Label>Attach binaries</Label>
				<label
					class="flex flex-col items-center justify-center gap-2 w-full min-h-25 border-2 border-dashed border-border rounded-lg cursor-pointer hover:border-brand hover:bg-brand/5 transition-colors p-4 text-center"
				>
					<Upload class="h-6 w-6 text-muted-foreground" />
					<span class="text-sm text-muted-foreground">Drag & drop or <span class="text-brand font-medium">choose files</span></span>
					<input type="file" multiple onchange={handleFileSelect} class="hidden" />
				</label>

				{#if selectedFiles.length > 0}
					<ul class="space-y-1 mt-2">
						{#each selectedFiles as file, i}
							<li class="flex items-center gap-2 text-sm text-foreground border border-border rounded-md px-3 py-2">
								<FileArchive class="h-4 w-4 text-muted-foreground shrink-0" />
								<span class="flex-1 truncate">{file.name}</span>
								<span class="text-xs text-muted-foreground shrink-0">{formatBytes(file.size)}</span>
								<button type="button" onclick={() => removeFile(i)} class="shrink-0 text-muted-foreground hover:text-destructive transition-colors">
									<X class="h-4 w-4" />
								</button>
							</li>
						{/each}
					</ul>
				{/if}
			</div>

			{#if error}
				<div class="flex items-center gap-2 text-destructive text-sm p-3 border border-destructive/30 rounded-lg">
					<AlertCircle class="h-4 w-4 shrink-0" />{error}
				</div>
			{/if}

			<div class="flex items-center gap-3">
				<Button type="submit" variant="brand" disabled={submitting}>
					{#if uploading}
						Uploading assets…
					{:else if submitting}
						{isEdit ? 'Saving…' : 'Publishing…'}
					{:else}
						{isEdit ? 'Save changes' : isDraft ? 'Save draft' : 'Publish release'}
					{/if}
				</Button>
				<Button type="button" variant="outline" onclick={() => goto(`/${username}/${repoName}/releases`)}>Cancel</Button>
			</div>
		</form>
	{/if}
</div>
