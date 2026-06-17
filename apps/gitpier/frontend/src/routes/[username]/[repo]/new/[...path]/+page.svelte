<script lang="ts">
	import { page } from '$app/state';
	import { getContext, tick } from 'svelte';
	import { repos, type Repository } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { goto } from '$app/navigation';
	import { ChevronRight, GitCommit, X } from '@lucide/svelte';
	import FileTreeSidebar from '$lib/components/FileTreeSidebar.svelte';
	import CodeEditor from '$lib/components/CodeEditor.svelte';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { toast } from 'svelte-sonner';

	const repoLayout = getContext<{ repo: Repository | null; branches: string[]; currentBranch: string } | null>('repoLayout');

	const username = $derived(page.params.username ?? '');
	const repoName = $derived(page.params.repo ?? '');
	// [...]path is the pre-filled directory, e.g. "src/components"
	const initialDir = $derived(page.params.path ?? '');
	const ref = $derived(page.url.searchParams.get('ref') ?? undefined);

	const currentUser = $derived(authStore.user);
	const isCommitSHA = $derived(/^[0-9a-f]{40}$/i.test(ref ?? ''));
	const effectiveBranch = $derived(isCommitSHA ? null : (ref ?? repoLayout?.repo?.default_branch ?? repoLayout?.currentBranch ?? 'main'));
	const isArchived = $derived(repoLayout?.repo?.is_archived ?? false);
	const canCreate = $derived(currentUser != null && effectiveBranch != null && !isArchived);

	// ── Path input state ──────────────────────────────────────────────────────
	// dirParts = locked folder segments shown as chips
	// fileInput = what the user is currently typing (the filename portion)
	let dirParts = $state<string[]>([]);
	let fileInput = $state('');
	let inputEl = $state<HTMLInputElement | null>(null);

	// Initialise dirParts from the URL path once the derived is ready
	let inited = false;
	$effect(() => {
		if (!inited && initialDir !== undefined) {
			inited = true;
			dirParts = initialDir ? initialDir.split('/').filter(Boolean) : [];
		}
	});

	// Full path for the commit (dir segments + filename)
	const fullPath = $derived([...dirParts, fileInput.trim()].filter(Boolean).join('/'));

	function handlePathKeydown(e: KeyboardEvent) {
		if (e.key === '/') {
			e.preventDefault();
			const part = fileInput.trim();
			if (part) {
				dirParts = [...dirParts, part];
				fileInput = '';
			}
		} else if (e.key === 'Backspace' && fileInput === '' && dirParts.length > 0) {
			// Move last dir segment back into the input
			const last = dirParts[dirParts.length - 1];
			dirParts = dirParts.slice(0, -1);
			fileInput = last;
		}
	}

	function removeDir(i: number) {
		dirParts = dirParts.filter((_, idx) => idx !== i);
		void tick().then(() => inputEl?.focus());
	}

	// ── Content + commit ─────────────────────────────────────────────────────
	let fileContent = $state('');
	let commitDialogOpen = $state(false);
	let commitMessage = $state('');
	let commitDescription = $state('');
	let committing = $state(false);
	let commitError = $state('');

	// Auto-fill commit message when path changes
	$effect(() => {
		const name = fileInput.trim() || 'new file';
		commitMessage = `Create ${name}`;
	});

	async function createFile() {
		if (!effectiveBranch || !fullPath || !commitMessage.trim()) return;
		if (!fileInput.trim()) {
			commitError = 'Please enter a file name.';
			return;
		}
		commitError = '';
		committing = true;
		const fullMessage = commitDescription.trim() ? `${commitMessage.trim()}\n\n${commitDescription.trim()}` : commitMessage.trim();
		try {
			await repos.updateBlob(username, repoName, {
				path: fullPath,
				content: fileContent,
				message: fullMessage,
				branch: effectiveBranch
			});
			commitDialogOpen = false;
			toast.success('File created successfully');
			await goto(`/${username}/${repoName}/blob/${fullPath}?ref=${effectiveBranch}`);
		} catch (e: any) {
			if (e?.status === 403) {
				commitError = 'You do not have write access to this repository.';
			} else if (e?.status === 422) {
				commitError = 'No changes to commit (file already exists with identical content).';
			} else {
				commitError = e?.message ?? 'Failed to create file.';
			}
		} finally {
			committing = false;
		}
	}

	// Breadcrumb segments (for navigation, not the editable path input)
	const breadcrumbDirs = $derived(dirParts);
</script>

<!-- Breadcrumb -->
<nav class="flex items-center gap-1 mb-3 text-sm flex-wrap">
	<a href="/{username}/{repoName}" class="text-primary hover:underline font-semibold">{repoName}</a>
	{#each breadcrumbDirs as seg, i}
		<ChevronRight class="h-3.5 w-3.5 text-muted-foreground" />
		<a href="/{username}/{repoName}/tree/{breadcrumbDirs.slice(0, i + 1).join('/')}{ref ? `?ref=${ref}` : ''}" class="text-primary hover:underline">{seg}</a>
	{/each}
	<ChevronRight class="h-3.5 w-3.5 text-muted-foreground" />
	<span class="text-muted-foreground">new file</span>
</nav>

<div class="flex gap-4 items-start">
	<!-- File tree sidebar -->
	<div class="hidden xl:block shrink-0">
		<FileTreeSidebar {username} repo={repoName} {ref} currentPath={initialDir} />
	</div>

	<!-- Main content -->
	<div class="flex-1 min-w-0">
		{#if !canCreate}
			<div class="rounded-md border border-border bg-card p-6 text-sm text-muted-foreground">
				{#if !currentUser}
					You must be <a href="/login" class="text-primary hover:underline">signed in</a> to create files.
				{:else if isArchived}
					This repository is archived and read-only. Unarchive it to create files.
				{:else if isCommitSHA}
					You cannot create files when viewing a specific commit. Switch to a branch first.
				{/if}
			</div>
		{:else}
			<div class="rounded-md border border-border overflow-hidden">
				<!-- Path input bar -->
				<div class="flex items-center gap-0 border-b border-border bg-card px-4 py-2.5">
					<!-- Locked folder chips -->
					<div class="flex flex-wrap items-center gap-1 flex-1 min-w-0 cursor-text" role="presentation" onclick={() => inputEl?.focus()}>
						{#each dirParts as part, i}
							<span class="inline-flex items-center gap-1 rounded bg-secondary px-2 py-0.5 text-xs font-mono text-foreground">
								{part}
								<button
									onclick={(e) => {
										e.stopPropagation();
										removeDir(i);
									}}
									class="text-muted-foreground hover:text-foreground ml-0.5"
									aria-label="Remove {part}"><X class="h-2.5 w-2.5" /></button
								>
							</span>
							<span class="text-muted-foreground text-sm select-none">/</span>
						{/each}
						<!-- Filename input -->
						<input
							bind:this={inputEl}
							bind:value={fileInput}
							onkeydown={handlePathKeydown}
							type="text"
							placeholder={dirParts.length === 0 ? 'Name your file… (type / to create a folder)' : 'filename.ext'}
							class="flex-1 min-w-32 bg-transparent text-sm text-foreground placeholder:text-muted-foreground outline-none font-mono"
							spellcheck="false"
							autocomplete="off"
						/>
					</div>
					{#if effectiveBranch}
						<span class="shrink-0 ml-4 flex items-center gap-1 text-xs text-muted-foreground">
							<GitCommit class="h-3 w-3" />
							{effectiveBranch}
						</span>
					{/if}
					<button
						onclick={() => (commitDialogOpen = true)}
						disabled={!fileInput.trim()}
						class="shrink-0 ml-3 flex items-center gap-1.5 rounded-md bg-primary px-2.5 py-1 text-xs font-medium text-primary-foreground transition-opacity hover:opacity-90 disabled:opacity-40"
					>
						<GitCommit class="h-3.5 w-3.5" />
						Commit new file
					</button>
				</div>

				<!-- Code editor -->
				<div class="bg-background">
					<CodeEditor bind:value={fileContent} filePath={fileInput} />
				</div>
			</div>
			<Dialog.Root bind:open={commitDialogOpen}>
				<Dialog.Content>
					<Dialog.Header>
						<Dialog.Title class="flex items-center gap-2">
							<GitCommit class="h-4 w-4" />
							Commit new file
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
							onclick={createFile}
							disabled={committing || !commitMessage.trim() || !fileInput.trim()}
							class="flex items-center gap-1.5 rounded-md bg-primary px-3 py-1.5 text-sm font-medium text-primary-foreground transition-opacity disabled:opacity-50"
						>
							{#if committing}
								<span class="h-3.5 w-3.5 animate-spin rounded-full border-2 border-primary-foreground border-t-transparent"></span>
								Committing…
							{:else}
								<GitCommit class="h-3.5 w-3.5" />
								Commit new file
							{/if}
						</button>
					</Dialog.Footer>
				</Dialog.Content>
			</Dialog.Root>
		{/if}
	</div>
</div>
