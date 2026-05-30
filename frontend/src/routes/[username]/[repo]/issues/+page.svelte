<script lang="ts">
	import { page } from '$app/state';
	import { issues, labels, type Issue, type Label } from '$lib/api/client';
	import { mediaUrl, timeAgo } from '$lib/utils';
	import { authStore } from '$lib/stores/auth.svelte';
	import { getContext } from 'svelte';
	import { CircleDot, CheckCircle, Plus, Tag, X, Pencil, Trash2 } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';

	let issueList = $state<Issue[]>([]);
	let labelList = $state<Label[]>([]);
	let loading = $state(true);
	let error = $state('');
	let filter = $state<'open' | 'closed'>('open');

	// Label management
	let showLabelPanel = $state(false);
	let newLabelName = $state('');
	let newLabelColor = $state('#0075ca');
	let newLabelDescription = $state('');
	let labelSaving = $state(false);
	let editingLabel = $state<Label | null>(null);

	const { username, repo } = $derived(page.params);
	const repoCtx: any = getContext('repoLayout');
	const isRepoArchived = $derived(Boolean(repoCtx?.repo?.is_archived));
	const isLoggedIn = $derived(authStore.user != null);

	async function load() {
		loading = true;
		error = '';
		try {
			const [issueData, labelData] = await Promise.all([issues.list(username!, repo!), labels.list(username!, repo!)]);
			issueList = issueData.issues ?? [];
			labelList = labelData.labels ?? [];
		} catch (e: any) {
			error = e.message;
		} finally {
			loading = false;
		}
	}

	$effect(() => {
		load();
	});

	const openIssues = $derived(issueList.filter((i) => i.status === 'open'));
	const closedIssues = $derived(issueList.filter((i) => i.status === 'closed'));
	const filtered = $derived(filter === 'open' ? openIssues : closedIssues);

	async function saveLabel() {
		if (isRepoArchived) {
			error = 'This repository is archived and read-only.';
			return;
		}
		if (!newLabelName.trim()) return;
		labelSaving = true;
		try {
			if (editingLabel) {
				const res = await labels.update(username!, repo!, editingLabel.id, {
					name: newLabelName,
					color: newLabelColor,
					description: newLabelDescription
				});
				labelList = labelList.map((l) => (l.id === editingLabel!.id ? res.label : l));
			} else {
				const res = await labels.create(username!, repo!, {
					name: newLabelName,
					color: newLabelColor,
					description: newLabelDescription
				});
				labelList = [...labelList, res.label];
			}
			resetLabelForm();
		} catch (e: any) {
			error = e.message;
		} finally {
			labelSaving = false;
		}
	}

	async function deleteLabel(label: Label) {
		if (isRepoArchived) {
			error = 'This repository is archived and read-only.';
			return;
		}
		if (!confirm(`Delete label "${label.name}"?`)) return;
		try {
			await labels.delete(username!, repo!, label.id);
			labelList = labelList.filter((l) => l.id !== label.id);
		} catch (e: any) {
			error = e.message;
		}
	}

	function startEditLabel(label: Label) {
		editingLabel = label;
		newLabelName = label.name;
		newLabelColor = label.color;
		newLabelDescription = label.description ?? '';
		showLabelPanel = true;
	}

	function resetLabelForm() {
		editingLabel = null;
		newLabelName = '';
		newLabelColor = '#0075ca';
		newLabelDescription = '';
		showLabelPanel = false;
	}

	function textColorForBg(hex: string): string {
		const r = parseInt(hex.slice(1, 3), 16);
		const g = parseInt(hex.slice(3, 5), 16);
		const b = parseInt(hex.slice(5, 7), 16);
		const luminance = (0.299 * r + 0.587 * g + 0.114 * b) / 255;
		return luminance > 0.5 ? '#000000' : '#ffffff';
	}

	function avatarLetter(u: string | undefined) {
		return (u ?? '?')[0].toUpperCase();
	}
</script>

<svelte:head>
	<title>Issues · {username}/{repo} · GitPier</title>
</svelte:head>

{#if loading}
	<div class="space-y-2">
		<div class="h-10 rounded-md border border-border bg-card animate-pulse"></div>
		{#each Array(3) as _}
			<div class="h-16 rounded-md border border-secondary bg-card animate-pulse"></div>
		{/each}
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else}
	<!-- Labels panel -->
	{#if showLabelPanel && !isRepoArchived}
		<div class="mb-4 rounded-md border border-border bg-card p-4">
			<h3 class="text-sm font-semibold text-foreground mb-3">{editingLabel ? 'Edit label' : 'Create label'}</h3>
			<div class="flex items-start gap-3 flex-wrap">
				<!-- Color preview -->
				<div class="flex items-center gap-2">
					<span class="text-xs text-muted-foreground">Color</span>
					<input type="color" bind:value={newLabelColor} class="w-8 h-8 rounded cursor-pointer border border-border bg-transparent" />
					<!-- Preview -->
					<span class="px-2 py-0.5 rounded-full text-xs font-medium" style="background-color:{newLabelColor};color:{textColorForBg(newLabelColor)}">
						{newLabelName || 'Preview'}
					</span>
				</div>
				<div class="flex-1 min-w-40">
					<input
						type="text"
						bind:value={newLabelName}
						placeholder="Label name"
						class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
					/>
				</div>
				<div class="flex-1 min-w-40">
					<input
						type="text"
						bind:value={newLabelDescription}
						placeholder="Description (optional)"
						class="h-8 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary"
					/>
				</div>
				<div class="flex gap-2">
					<Button size="sm" variant="brand" onclick={saveLabel} disabled={labelSaving || !newLabelName.trim()}>
						{editingLabel ? 'Save' : 'Create'}
					</Button>
					<Button size="sm" variant="outline" onclick={resetLabelForm}>Cancel</Button>
				</div>
			</div>
		</div>
	{/if}

	<!-- Filter + action bar -->
	<div class="flex items-center justify-between gap-3 mb-3">
		<div class="flex items-center gap-0.5 flex-1">
			<button
				onclick={() => (filter = 'open')}
				class="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors"
				class:bg-secondary={filter === 'open'}
				class:text-foreground={filter === 'open'}
				class:font-semibold={filter === 'open'}
				class:text-muted-foreground={filter !== 'open'}
			>
				<CircleDot class="h-4 w-4 text-[#3fb950]" />
				{openIssues.length} Open
			</button>
			<button
				onclick={() => (filter = 'closed')}
				class="flex items-center gap-1.5 px-3 py-1.5 text-sm rounded-md transition-colors"
				class:bg-secondary={filter === 'closed'}
				class:text-foreground={filter === 'closed'}
				class:font-semibold={filter === 'closed'}
				class:text-muted-foreground={filter !== 'closed'}
			>
				<CheckCircle class="h-4 w-4" />
				{closedIssues.length} Closed
			</button>
		</div>
		{#if isLoggedIn && !isRepoArchived}
			<div class="flex items-center gap-2">
				<Button
					size="sm"
					variant="outline"
					onclick={() => {
						showLabelPanel = !showLabelPanel;
						if (!showLabelPanel) resetLabelForm();
					}}
				>
					<Tag class="h-3.5 w-3.5" />
					Labels
					{#if labelList.length > 0}
						<span class="ml-1 rounded-full bg-secondary px-1.5 py-0.5 text-xs">{labelList.length}</span>
					{/if}
				</Button>
				<Button variant="brand" size="sm" href="/{username}/{repo}/issues/new">
					<Plus class="h-3.5 w-3.5" />
					New issue
				</Button>
			</div>
		{/if}
	</div>

	<!-- Labels list (shown when label panel open and has labels) -->
	{#if showLabelPanel && labelList.length > 0 && !isRepoArchived}
		<div class="mb-3 rounded-md border border-border bg-card divide-y divide-secondary overflow-hidden">
			{#each labelList as label}
				<div class="flex items-center justify-between px-4 py-2.5">
					<div class="flex items-center gap-3">
						<span class="px-2.5 py-0.5 rounded-full text-xs font-medium" style="background-color:{label.color};color:{textColorForBg(label.color)}">
							{label.name}
						</span>
						{#if label.description}
							<span class="text-xs text-muted-foreground">{label.description}</span>
						{/if}
					</div>
					<div class="flex items-center gap-1">
						<button onclick={() => startEditLabel(label)} class="p-1.5 rounded-md text-muted-foreground hover:text-foreground hover:bg-secondary transition-colors">
							<Pencil class="h-3.5 w-3.5" />
						</button>
						<button onclick={() => deleteLabel(label)} class="p-1.5 rounded-md text-muted-foreground hover:text-red-400 hover:bg-red-900/20 transition-colors">
							<Trash2 class="h-3.5 w-3.5" />
						</button>
					</div>
				</div>
			{/each}
		</div>
	{/if}

	{#if filtered.length === 0}
		<div class="rounded-md border border-border bg-card py-16 text-center">
			<CircleDot class="mx-auto h-12 w-12 text-muted-foreground mb-4" />
			<h3 class="text-base font-semibold text-foreground mb-2">
				There aren't any {filter} issues.
			</h3>
			<p class="text-sm text-muted-foreground">
				{#if filter === 'open'}Issues let you track bugs, enhancements, and other requests.{:else}No closed issues to show.{/if}
			</p>
		</div>
	{:else}
		<div class="rounded-md border border-border overflow-hidden divide-y divide-secondary">
			{#each filtered as issue}
				<a href="/{username}/{repo}/issues/{issue.number}" class="flex items-start gap-3 px-4 py-3 bg-card hover:bg-accent transition-colors">
					{#if issue.status === 'open'}
						<CircleDot class="h-4 w-4 text-[#3fb950] shrink-0 mt-0.5" />
					{:else}
						<CheckCircle class="h-4 w-4 text-[#a371f7] shrink-0 mt-0.5" />
					{/if}
					<div class="flex-1 min-w-0">
						<div class="flex items-center gap-2 flex-wrap">
							<p class="text-sm font-semibold text-foreground hover:text-primary truncate">{issue.title}</p>
							{#each issue.labels ?? [] as label}
								<span class="px-2 py-0.5 rounded-full text-xs font-medium" style="background-color:{label.color};color:{textColorForBg(label.color)}">
									{label.name}
								</span>
							{/each}
						</div>
						<div class="mt-0.5 flex items-center gap-1.5 text-xs text-muted-foreground">
							<div class="h-4 w-4 rounded-full bg-secondary border border-border overflow-hidden flex items-center justify-center text-[9px] font-semibold text-foreground shrink-0">
								{#if issue.author?.avatar_url}
									<img src={mediaUrl(issue.author.avatar_url)} alt={issue.author?.username ?? 'author'} class="h-full w-full object-cover" />
								{:else}
									{avatarLetter(issue.author?.username)}
								{/if}
							</div>
							<span>#{issue.number} opened {timeAgo(issue.created_at)} by {issue.author?.username ?? ''}</span>
						</div>
					</div>
				</a>
			{/each}
		</div>
	{/if}
{/if}
