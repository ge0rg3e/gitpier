<script lang="ts">
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { API_BASE, repos, type Repository } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { ChevronRight, Code, FileText, Pencil, X, GitCommit, Clipboard, Trash2, Ellipsis } from '@lucide/svelte';
	import FileTreeSidebar from '$lib/components/FileTreeSidebar.svelte';
	import CodeViewer from '$lib/components/CodeViewer.svelte';
	import CodeEditor from '$lib/components/CodeEditor.svelte';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import { marked } from 'marked';
	import DOMPurify from 'dompurify';
	import { toast } from 'svelte-sonner';

	// Repo layout context
	const repoLayout = getContext<{ repo: Repository | null; branches: string[]; currentBranch: string } | null>('repoLayout');

	let content = $state('');
	let editContent = $state('');
	let size = $state(0);
	let loading = $state(true);
	let error = $state('');
	let copied = $state(false);
	let viewMode = $state<'raw' | 'preview'>('raw');
	let editMode = $state(false);
	let commitDialogOpen = $state(false);
	let deleteDialogOpen = $state(false);
	let commitMessage = $state('');
	let commitDescription = $state('');
	let deleteCommitMessage = $state('');
	let deleteCommitDescription = $state('');
	let committing = $state(false);
	let commitError = $state('');
	let mediaSizeLoading = $state(false);
	let loadSeq = 0;

	const username = $derived(page.params.username ?? '');
	const repoName = $derived(page.params.repo ?? '');
	const path = $derived(page.params.path ?? '');
	const ref = $derived(page.url.searchParams.get('ref') ?? undefined);

	// Determine the branch to commit to: if ref looks like a commit SHA (40 hex) it's not editable
	const isCommitSHA = $derived(/^[0-9a-f]{40}$/i.test(ref ?? ''));
	const effectiveBranch = $derived(isCommitSHA ? null : (ref ?? repoLayout?.repo?.default_branch ?? repoLayout?.currentBranch ?? 'main'));

	// Check write permission:
	// - User is authenticated
	// - We're on a branch (not a commit SHA)
	// Actual enforcement is done on the backend; here we just hide the button if
	// the user is clearly not the owner (best-effort, no collaborator API call needed).
	const currentUser = $derived(authStore.user);
	const repoObj = $derived(repoLayout?.repo ?? null);
	const isOwner = $derived(currentUser != null && repoObj != null && (repoObj.owner?.username === currentUser.username || repoObj.org?.login === username));
	// Show edit button if logged in AND on a branch (collaborators will get a 403 from backend if not allowed;
	// showing the button for all logged-in users avoids an extra API call)
	const canEdit = $derived(currentUser != null && effectiveBranch != null && !repoObj?.is_archived);

	function parseSizeFromHeaders(res: Response): number | null {
		const contentLength = res.headers.get('content-length');
		if (contentLength) {
			const parsed = Number.parseInt(contentLength, 10);
			if (Number.isFinite(parsed) && parsed > 0) return parsed;
		}

		const contentRange = res.headers.get('content-range');
		if (contentRange) {
			const m = /\/(\d+)$/.exec(contentRange.trim());
			if (m?.[1]) {
				const parsed = Number.parseInt(m[1], 10);
				if (Number.isFinite(parsed) && parsed > 0) return parsed;
			}
		}

		return null;
	}

	async function resolveMediaSize(): Promise<number | null> {
		// Fast path: HEAD usually returns content-length.
		const headRes = await fetch(rawFileUrl, { method: 'HEAD', credentials: 'include' });
		if (headRes.ok) {
			const headSize = parseSizeFromHeaders(headRes);
			if (headSize != null) return headSize;
		}

		// Fallback: range request can expose full size in content-range.
		const rangeRes = await fetch(rawFileUrl, {
			method: 'GET',
			headers: { Range: 'bytes=0-0' },
			credentials: 'include'
		});
		const rangeSize = parseSizeFromHeaders(rangeRes);
		if (rangeSize != null) return rangeSize;

		// Final fallback: blob API always returns explicit size.
		const blobData = await repos.blob(username, repoName, path, ref);
		return blobData.size > 0 ? blobData.size : null;
	}

	$effect(() => {
		const seq = ++loadSeq;
		if (!username || !repoName || !path) {
			loading = false;
			error = 'Invalid route parameters';
			content = '';
			size = 0;
			return;
		}

		// Exit edit mode when file/ref changes
		editMode = false;
		commitError = '';

		loading = true;
		error = '';
		content = '';
		size = 0;
		mediaSizeLoading = false;
		viewMode = 'raw';

		if (isMediaFile) {
			mediaSizeLoading = true;
			void resolveMediaSize()
				.then((nextSize) => {
					if (seq !== loadSeq) return;
					if (nextSize != null) size = nextSize;
				})
				.catch(() => {})
				.finally(() => {
					if (seq !== loadSeq) return;
					mediaSizeLoading = false;
				});
			loading = false;
			return;
		}

		repos
			.blob(username, repoName, path, ref)
			.then((data) => {
				if (seq !== loadSeq) return;
				content = data.content;
				size = data.size;
				editContent = data.content;
				// Default commit message
				commitMessage = `Update ${path.split('/').pop() ?? path}`;
			})
			.catch((e: any) => {
				if (seq !== loadSeq) return;
				error = e?.message ?? 'Failed to load file';
			})
			.finally(() => {
				if (seq !== loadSeq) return;
				loading = false;
			});
	});

	const segments = $derived(path ? path.split('/').filter(Boolean) : []);
	const fileName = $derived(segments[segments.length - 1] ?? '');
	const fileExt = $derived(fileName.includes('.') ? (fileName.split('.').pop()?.toLowerCase() ?? '') : '');
	const isImageFile = $derived(['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'avif', 'ico', 'tif', 'tiff'].includes(fileExt));
	const isVideoFile = $derived(['mp4', 'webm', 'ogg', 'ogv', 'mov', 'm4v'].includes(fileExt));
	const isMediaFile = $derived(isImageFile || isVideoFile);
	const rawFileUrl = $derived(
		`${API_BASE}/api/v1/repos/${username}/${repoName}/raw?${new URLSearchParams({
			path,
			...(ref ? { ref } : {})
		})}`
	);
	const lines = $derived(content.split('\n'));
	const isMarkdown = $derived(fileName.toLowerCase().endsWith('.md'));
	const renderedMarkdown = $derived(isMarkdown && viewMode === 'preview' ? DOMPurify.sanitize(marked.parse(editMode ? editContent : content) as string) : '');

	function getLanguage(filename: string): string {
		const ext = filename.split('.').pop()?.toLowerCase();
		const map: Record<string, string> = {
			ts: 'TypeScript',
			js: 'JavaScript',
			svelte: 'Svelte',
			go: 'Go',
			py: 'Python',
			rs: 'Rust',
			java: 'Java',
			cs: 'C#',
			cpp: 'C++',
			c: 'C',
			html: 'HTML',
			css: 'CSS',
			json: 'JSON',
			yaml: 'YAML',
			yml: 'YAML',
			md: 'Markdown',
			sh: 'Shell',
			sql: 'SQL',
			dockerfile: 'Dockerfile',
			toml: 'TOML',
			xml: 'XML'
		};
		return map[ext ?? ''] ?? ext?.toUpperCase() ?? 'Text';
	}

	async function copyContent() {
		await navigator.clipboard.writeText(editMode ? editContent : content);
		copied = true;
		setTimeout(() => (copied = false), 2000);
	}

	function formatSize(bytes: number): string {
		if (bytes < 1024) return `${bytes} B`;
		if (bytes < 1048576) return `${(bytes / 1024).toFixed(1)} KB`;
		return `${(bytes / 1048576).toFixed(1)} MB`;
	}

	function enterEditMode() {
		editContent = content;
		commitMessage = `Update ${path.split('/').pop() ?? path}`;
		deleteCommitMessage = `Delete ${path.split('/').pop() ?? path}`;
		commitDescription = '';
		deleteCommitDescription = '';
		commitError = '';
		commitDialogOpen = false;
		deleteDialogOpen = false;
		editMode = true;
	}

	function cancelEdit() {
		editMode = false;
		editContent = content;
		commitError = '';
	}

	function openDeleteDialog() {
		deleteCommitMessage = `Delete ${path.split('/').pop() ?? path}`;
		deleteCommitDescription = '';
		commitError = '';
		deleteDialogOpen = true;
	}

	async function commitChanges() {
		if (!effectiveBranch || !commitMessage.trim()) return;
		commitError = '';
		committing = true;
		const fullMessage = commitDescription.trim() ? `${commitMessage.trim()}\n\n${commitDescription.trim()}` : commitMessage.trim();
		try {
			await repos.updateBlob(username, repoName, {
				path,
				content: editContent,
				message: fullMessage,
				branch: effectiveBranch
			});
			content = editContent;
			size = new TextEncoder().encode(editContent).length;
			editMode = false;
			commitDialogOpen = false;
			toast.success('Changes committed successfully');
			// Navigate to refresh the page with the new commit
			await goto(`/${username}/${repoName}/blob/${path}${effectiveBranch ? `?ref=${effectiveBranch}` : ''}`);
		} catch (e: any) {
			const msg = e?.message ?? 'Failed to commit changes';
			if (e?.status === 403) {
				commitError = 'You do not have write access to this repository.';
			} else if (e?.status === 422) {
				commitError = 'No changes to commit.';
			} else {
				commitError = msg;
			}
		} finally {
			committing = false;
		}
	}

	async function deleteFile() {
		if (!effectiveBranch || !deleteCommitMessage.trim()) return;
		commitError = '';
		committing = true;
		const fullMessage = deleteCommitDescription.trim()
			? `${deleteCommitMessage.trim()}\n\n${deleteCommitDescription.trim()}`
			: deleteCommitMessage.trim();
		try {
			await repos.deleteBlob(username, repoName, {
				path,
				message: fullMessage,
				branch: effectiveBranch
			});
			deleteDialogOpen = false;
			editMode = false;
			toast.success('File deleted successfully');
			const parentPath = segments.length > 1 ? segments.slice(0, -1).join('/') : '';
			const target = parentPath
				? `/${username}/${repoName}/tree/${parentPath}${effectiveBranch ? `?ref=${effectiveBranch}` : ''}`
				: `/${username}/${repoName}${effectiveBranch ? `?ref=${effectiveBranch}` : ''}`;
			await goto(target);
		} catch (e: any) {
			const msg = e?.message ?? 'Failed to delete file';
			if (e?.status === 403) {
				commitError = 'You do not have write access to this repository.';
			} else if (e?.status === 404) {
				commitError = 'File not found on this branch.';
			} else if (e?.status === 422) {
				commitError = 'No changes to commit.';
			} else {
				commitError = msg;
			}
		} finally {
			committing = false;
		}
	}
</script>

<!-- Breadcrumb -->
<nav class="flex items-center gap-1 mb-3 text-sm flex-wrap">
	<a href="/{username}/{repoName}" class="text-primary hover:underline font-semibold">{repoName}</a>
	{#each segments as seg, i}
		<ChevronRight class="h-3.5 w-3.5 text-muted-foreground" />
		{#if i < segments.length - 1}
			<a href="/{username}/{repoName}/tree/{segments.slice(0, i + 1).join('/')}{ref ? `?ref=${ref}` : ''}" class="text-primary hover:underline">{seg}</a>
		{:else}
			<span class="font-semibold text-foreground">{seg}</span>
		{/if}
	{/each}
</nav>

<div class="flex gap-4 items-start">
	<!-- File tree sidebar -->
	<div class="hidden xl:block shrink-0">
		<FileTreeSidebar {username} repo={repoName} {ref} currentPath={path} />
	</div>

	<!-- Main content -->
	<div class="flex-1 min-w-0">
		{#if loading}
			<div class="rounded-md border border-border overflow-hidden">
				<div class="h-10 bg-card border-b border-border animate-pulse"></div>
				<div class="bg-background p-4 space-y-2">
					{#each Array(15) as _, i}
						<div class="h-4 bg-card rounded animate-pulse" style="width: {30 + ((i * 7) % 55)}%"></div>
					{/each}
				</div>
			</div>
		{:else if error}
			<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
		{:else}
			<div class="rounded-md border border-border overflow-hidden">
				<!-- File header bar -->
				<div class="flex items-center justify-between gap-3 border-b border-border bg-card px-4 py-2.5">
					<div class="flex items-center gap-3 text-xs text-muted-foreground">
						{#if isMediaFile}
							<span>Binary media file</span>
							<span class="text-border">·</span>
							{#if mediaSizeLoading}
								<span class="inline-block h-3 w-16 rounded bg-secondary animate-pulse"></span>
							{:else if size > 0}
								<span>{formatSize(size)}</span>
							{:else}
								<span>Size unknown</span>
							{/if}
						{:else if !editMode}
							<span>{lines.length} lines</span>
							<span class="text-border">·</span>
							<span>{formatSize(size)}</span>
							<span class="text-border">·</span>
						{/if}
						<span>{getLanguage(fileName)}</span>
						{#if editMode}
							<span class="text-border">·</span>
							<span class="text-yellow-500">Editing</span>
							{#if effectiveBranch}
								<span class="text-border">·</span>
								<span class="flex items-center gap-1"><GitCommit class="h-3 w-3" />{effectiveBranch}</span>
							{/if}
						{/if}
					</div>
					<div class="flex items-center gap-2">
						{#if isMediaFile}
							<a
								href={rawFileUrl}
								target="_blank"
								rel="noopener noreferrer"
								class="flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground hover:border-primary"
							>
								Open raw
							</a>
						{:else if !editMode}
							{#if isMarkdown}
								<div class="flex items-center rounded-md border border-border overflow-hidden">
									<button
										onclick={() => (viewMode = 'raw')}
										class="flex items-center gap-1.5 px-2.5 py-1 text-xs transition-colors {viewMode === 'raw'
											? 'bg-secondary text-foreground'
											: 'text-muted-foreground hover:text-foreground'}"
									>
										<Code class="h-3.5 w-3.5" />
										Code
									</button>
									<button
										onclick={() => (viewMode = 'preview')}
										class="flex items-center gap-1.5 px-2.5 py-1 text-xs transition-colors border-l border-border {viewMode === 'preview'
											? 'bg-secondary text-foreground'
											: 'text-muted-foreground hover:text-foreground'}"
									>
										<FileText class="h-3.5 w-3.5" />
										Preview
									</button>
								</div>
							{/if}
							<button
								onclick={copyContent}
								class="flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 py-1 text-xs transition-colors {copied
									? 'text-brand border-brand/50'
									: 'text-muted-foreground hover:text-foreground hover:border-primary'}"
							>
								<Clipboard class="h-3.5 w-3.5" />
								{copied ? 'Copied!' : 'Copy'}
							</button>
							{#if canEdit}
								<button
									onclick={enterEditMode}
									class="flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 py-1 text-xs transition-colors text-muted-foreground hover:text-foreground hover:border-primary"
									title="Edit this file"
								>
									<Pencil class="h-3.5 w-3.5" />
									Edit
								</button>
								<DropdownMenu.Root>
									<DropdownMenu.Trigger
										class="flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground hover:border-primary"
										aria-label="File actions"
										title="File actions"
									>
										<Ellipsis class="h-3.5 w-3.5" />
									</DropdownMenu.Trigger>
									<DropdownMenu.Content align="end" class="w-44">
										<DropdownMenu.Item class="text-destructive focus:text-destructive" onclick={openDeleteDialog}>
											<span class="flex items-center gap-2"><Trash2 class="h-3.5 w-3.5" />Delete file</span>
										</DropdownMenu.Item>
									</DropdownMenu.Content>
								</DropdownMenu.Root>
							{/if}
						{:else}
							<button
								onclick={copyContent}
								class="flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 py-1 text-xs transition-colors {copied
									? 'text-brand border-brand/50'
									: 'text-muted-foreground hover:text-foreground hover:border-primary'}"
							>
								<Clipboard class="h-3.5 w-3.5" />
								{copied ? 'Copied!' : 'Copy'}
							</button>
							{#if isMarkdown}
								<div class="flex items-center rounded-md border border-border overflow-hidden">
									<button
										onclick={() => (viewMode = 'raw')}
										class="flex items-center gap-1.5 px-2.5 py-1 text-xs transition-colors {viewMode === 'raw'
											? 'bg-secondary text-foreground'
											: 'text-muted-foreground hover:text-foreground'}"
									>
										<Code class="h-3.5 w-3.5" />
										Code
									</button>
									<button
										onclick={() => (viewMode = 'preview')}
										class="flex items-center gap-1.5 px-2.5 py-1 text-xs transition-colors border-l border-border {viewMode === 'preview'
											? 'bg-secondary text-foreground'
											: 'text-muted-foreground hover:text-foreground'}"
									>
										<FileText class="h-3.5 w-3.5" />
										Preview
									</button>
								</div>
							{/if}
							<button
								onclick={cancelEdit}
								class="flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 py-1 text-xs transition-colors text-muted-foreground hover:text-foreground hover:border-primary"
							>
								<X class="h-3.5 w-3.5" />
								Cancel
							</button>
							<button
								onclick={() => (commitDialogOpen = true)}
								class="flex items-center gap-1.5 rounded-md bg-primary px-2.5 py-1 text-xs font-medium text-primary-foreground transition-opacity hover:opacity-90"
							>
								<GitCommit class="h-3.5 w-3.5" />
								Commit changes
							</button>
						{/if}
					</div>
				</div>
				<!-- Content -->
				{#if isImageFile}
					<div class="bg-background p-4">
						<img src={rawFileUrl} alt={fileName} class="mx-auto max-h-[75vh] max-w-full rounded-md border border-border bg-card object-contain" loading="lazy" />
					</div>
				{:else if isVideoFile}
					<div class="bg-background p-4">
						<video src={rawFileUrl} controls class="mx-auto max-h-[75vh] max-w-full rounded-md border border-border bg-card" preload="metadata">
							Your browser does not support this video format.
						</video>
					</div>
				{:else if editMode}
					{#if isMarkdown && viewMode === 'preview'}
						<div class="bg-background p-8">
							<div
								class="prose prose-invert max-w-none text-foreground
									prose-headings:text-foreground prose-headings:border-b prose-headings:border-secondary prose-headings:pb-2
									prose-a:text-primary prose-code:text-foreground prose-code:bg-card prose-code:rounded prose-code:px-1
									prose-pre:bg-card prose-pre:border prose-pre:border-border prose-blockquote:border-l-border prose-blockquote:text-muted-foreground
									prose-hr:border-secondary prose-strong:text-foreground"
							>
								{@html renderedMarkdown}
							</div>
						</div>
					{:else}
						<CodeEditor bind:value={editContent} filePath={fileName} />
					{/if}
				{:else if isMarkdown && viewMode === 'preview'}
					<div class="bg-background p-8">
						<div
							class="prose prose-invert max-w-none text-foreground
								prose-headings:text-foreground prose-headings:border-b prose-headings:border-secondary prose-headings:pb-2
								prose-a:text-primary prose-code:text-foreground prose-code:bg-card prose-code:rounded prose-code:px-1
								prose-pre:bg-card prose-pre:border prose-pre:border-border prose-blockquote:border-l-border prose-blockquote:text-muted-foreground
								prose-hr:border-secondary prose-strong:text-foreground"
						>
							{@html renderedMarkdown}
						</div>
					</div>
				{:else}
					<CodeViewer code={content} filePath={fileName} containerClass="border-0 rounded-none" />
				{/if}
			</div>
			<Dialog.Root bind:open={commitDialogOpen}>
				<Dialog.Content>
					<Dialog.Header>
						<Dialog.Title class="flex items-center gap-2">
							<GitCommit class="h-4 w-4" />
							Commit changes
						</Dialog.Title>
						<Dialog.Description>
							Committing to <span class="font-mono text-foreground">{effectiveBranch}</span>
						</Dialog.Description>
					</Dialog.Header>
					<div class="flex flex-col gap-3 py-2">
						<input
							type="text"
							bind:value={commitMessage}
							placeholder="Commit message"
							maxlength="72"
							class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary transition-colors"
						/>
						<textarea
							bind:value={commitDescription}
							placeholder="Add an optional extended description…"
							rows={3}
							class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary transition-colors resize-none"
						></textarea>
						{#if commitError}<p class="text-xs text-red-400">{commitError}</p>{/if}
					</div>
					<Dialog.Footer class="flex gap-2 justify-end">
						<button
							onclick={() => (commitDialogOpen = false)}
							class="rounded-md border border-border bg-secondary px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
						>
							Cancel
						</button>
						<button
							onclick={commitChanges}
							disabled={committing || !commitMessage.trim()}
							class="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-opacity disabled:opacity-50"
						>
							{#if committing}
								<span class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-primary-foreground border-t-transparent"></span>
								Committing…
							{:else}
								<GitCommit class="h-3.5 w-3.5" />
								Commit changes
							{/if}
						</button>
					</Dialog.Footer>
				</Dialog.Content>
			</Dialog.Root>
			<Dialog.Root bind:open={deleteDialogOpen}>
				<Dialog.Content>
					<Dialog.Header>
						<Dialog.Title class="flex items-center gap-2 text-red-300">
							<Trash2 class="h-4 w-4" />
							Delete file
						</Dialog.Title>
						<Dialog.Description>
							Delete <span class="font-mono text-foreground">{fileName}</span> from <span class="font-mono text-foreground">{effectiveBranch}</span>
						</Dialog.Description>
					</Dialog.Header>
					<div class="flex flex-col gap-3 py-2">
						<input
							type="text"
							bind:value={deleteCommitMessage}
							placeholder="Commit message"
							maxlength="72"
							class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary transition-colors"
						/>
						<textarea
							bind:value={deleteCommitDescription}
							placeholder="Add an optional extended description…"
							rows={3}
							class="w-full rounded-md border border-border bg-background px-3 py-2 text-sm text-foreground placeholder:text-muted-foreground outline-none focus:border-primary focus:ring-1 focus:ring-primary transition-colors resize-none"
						></textarea>
						{#if commitError}<p class="text-xs text-red-400">{commitError}</p>{/if}
					</div>
					<Dialog.Footer class="flex gap-2 justify-end">
						<button
							onclick={() => (deleteDialogOpen = false)}
							class="rounded-md border border-border bg-secondary px-3 py-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
						>
							Cancel
						</button>
						<button
							onclick={deleteFile}
							disabled={committing || !deleteCommitMessage.trim()}
							class="flex items-center gap-1.5 rounded-md bg-red-600 px-3 py-1.5 text-sm font-medium text-white transition-opacity hover:opacity-90 disabled:opacity-50"
						>
							{#if committing}
								<span class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-white border-t-transparent"></span>
								Deleting…
							{:else}
								<Trash2 class="h-3.5 w-3.5" />
								Delete file
							{/if}
						</button>
					</Dialog.Footer>
				</Dialog.Content>
			</Dialog.Root>
		{/if}
	</div>
</div>
