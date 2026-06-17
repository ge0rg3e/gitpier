<script lang="ts">
	import { repos } from '$lib/api/client';
	import { resolveRepoTreeIconUrl } from '$lib/icons/fileIcons';
	import { Folder, FolderOpen, File, ChevronRight, ChevronDown, PanelLeftClose, PanelLeft } from '@lucide/svelte';

	interface TreeNode {
		name: string;
		path: string;
		type: 'blob' | 'tree';
		children: TreeNode[];
		loaded: boolean;
		expanded: boolean;
		loading: boolean;
	}

	interface Props {
		username: string;
		repo: string;
		ref?: string;
		currentPath?: string;
	}

	interface TreeResponse {
		files?: { name: string; path: string; type: 'blob' | 'tree' }[];
	}

	const TREE_CACHE_TTL_MS = 60_000;
	const treeCache = new Map<string, { expiresAt: number; data: TreeResponse }>();
	const treeInFlight = new Map<string, Promise<TreeResponse>>();

	let { username, repo, ref, currentPath = '' }: Props = $props();

	let collapsed = $state(false);
	let rootNodes = $state<TreeNode[]>([]);
	let rootLoading = $state(true);
	let loadSeq = 0;

	function treeCacheKey(path = '') {
		return `${username}:${repo}:${ref ?? ''}:${path}`;
	}

	async function fetchTreeFast(path?: string): Promise<TreeResponse> {
		const key = treeCacheKey(path ?? '');
		const now = Date.now();
		const cached = treeCache.get(key);
		if (cached && cached.expiresAt > now) {
			return cached.data;
		}

		const existing = treeInFlight.get(key);
		if (existing) {
			return existing;
		}

		const req = repos
			.tree(username, repo, ref, path, { includeMeta: false, includeHead: false })
			.then((data) => {
				treeCache.set(key, { expiresAt: Date.now() + TREE_CACHE_TTL_MS, data });
				return data;
			})
			.finally(() => {
				treeInFlight.delete(key);
			});

		treeInFlight.set(key, req);
		return req;
	}

	function buildNodes(entries: { name: string; path: string; type: 'blob' | 'tree' }[]): TreeNode[] {
		const sorted = [
			...entries.filter((f) => f.type === 'tree').sort((a, b) => a.name.localeCompare(b.name)),
			...entries.filter((f) => f.type === 'blob').sort((a, b) => a.name.localeCompare(b.name))
		];
		return sorted.map((f) => ({
			name: f.name,
			path: f.path,
			type: f.type,
			children: [],
			loaded: false,
			expanded: false,
			loading: false
		}));
	}

	$effect(() => {
		const seq = ++loadSeq;
		rootLoading = true;
		fetchTreeFast()
			.then((data) => {
				if (seq !== loadSeq) return;
				rootNodes = buildNodes(data.files ?? []);
				// Auto-expand ancestors of the current path
				if (currentPath) {
					autoExpand(rootNodes, currentPath);
				}
			})
			.catch(() => {})
			.finally(() => {
				if (seq !== loadSeq) return;
				rootLoading = false;
			});
	});

	async function autoExpand(nodes: TreeNode[], targetPath: string) {
		for (const node of nodes) {
			if (node.type === 'tree' && (targetPath === node.path || targetPath.startsWith(node.path + '/'))) {
				if (!node.loaded) {
					node.loading = true;
					try {
						const data = await fetchTreeFast(node.path);
						node.children = buildNodes(data.files ?? []);
						node.loaded = true;
					} catch {
						// ignore
					} finally {
						node.loading = false;
					}
				}
				node.expanded = true;
				autoExpand(node.children, targetPath);
			}
		}
	}

	async function toggleFolder(node: TreeNode) {
		if (node.type !== 'tree') return;
		if (!node.expanded && !node.loaded) {
			node.loading = true;
			try {
				const data = await fetchTreeFast(node.path);
				node.children = buildNodes(data.files ?? []);
				node.loaded = true;
			} catch {
				// silently ignore load errors
			} finally {
				node.loading = false;
			}
		}
		node.expanded = !node.expanded;
	}

	function isActive(path: string) {
		return currentPath === path || currentPath.startsWith(path + '/');
	}
</script>

<div class="flex flex-col" class:w-52={!collapsed} class:w-8={collapsed}>
	<!-- Header -->
	<div class="flex items-center justify-between px-2 py-1.5 border border-border rounded-md bg-card mb-1" class:px-1={collapsed}>
		{#if !collapsed}
			<span class="text-xs font-semibold text-muted-foreground uppercase tracking-wider select-none">Files</span>
		{/if}
		<button onclick={() => (collapsed = !collapsed)} class="text-muted-foreground hover:text-foreground transition-colors shrink-0" title={collapsed ? 'Expand file tree' : 'Collapse file tree'}>
			{#if collapsed}
				<PanelLeft class="h-4 w-4" />
			{:else}
				<PanelLeftClose class="h-4 w-4" />
			{/if}
		</button>
	</div>

	{#if !collapsed}
		<!-- Tree -->
		<div class="border border-border rounded-md bg-card overflow-y-auto max-h-[calc(100vh-12rem)] text-sm">
			{#if rootLoading}
				{#each Array(6) as _}
					<div class="h-7 mx-2 my-1 rounded bg-secondary animate-pulse"></div>
				{/each}
			{:else}
				{#each rootNodes as node}
					{@render treeNode(node, 0)}
				{/each}
			{/if}
		</div>
	{/if}
</div>

{#snippet treeNode(node: TreeNode, depth: number)}
	<div>
		{#if node.type === 'tree'}
			<button
				onclick={() => toggleFolder(node)}
				class="flex w-full items-center gap-1.5 py-1.5 hover:bg-accent transition-colors text-left"
				class:bg-accent={isActive(node.path)}
				style="padding-left: {0.5 + depth * 0.875}rem"
			>
				{#if node.loading}
					<span class="h-3.5 w-3.5 shrink-0 animate-spin rounded-full border border-border border-t-primary"></span>
				{:else if node.expanded}
					<ChevronDown class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
				{:else}
					<ChevronRight class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
				{/if}
				{#if node.expanded}
					{#if resolveRepoTreeIconUrl(node.name, 'tree', { opened: true })}
						<img src={resolveRepoTreeIconUrl(node.name, 'tree', { opened: true })} alt="" class="h-3.5 w-3.5 shrink-0" />
					{:else}
						<FolderOpen class="h-3.5 w-3.5 shrink-0 text-primary" />
					{/if}
				{:else}
					{#if resolveRepoTreeIconUrl(node.name, 'tree', { opened: false })}
						<img src={resolveRepoTreeIconUrl(node.name, 'tree', { opened: false })} alt="" class="h-3.5 w-3.5 shrink-0" />
					{:else}
						<Folder class="h-3.5 w-3.5 shrink-0 text-primary" />
					{/if}
				{/if}
				<span class="truncate text-xs font-medium text-foreground">{node.name}</span>
			</button>
			{#if node.expanded && node.children.length > 0}
				{#each node.children as child}
					{@render treeNode(child, depth + 1)}
				{/each}
			{/if}
		{:else}
			<a
				href="/{username}/{repo}/blob/{node.path}{ref ? `?ref=${ref}` : ''}"
				class="flex w-full items-center gap-1.5 py-1.5 hover:bg-accent transition-colors"
				class:bg-accent={isActive(node.path)}
				class:text-primary={isActive(node.path)}
				style="padding-left: {0.5 + depth * 0.875}rem"
			>
				{#if resolveRepoTreeIconUrl(node.name, 'blob')}
					<img src={resolveRepoTreeIconUrl(node.name, 'blob')} alt="" class="h-3.5 w-3.5 shrink-0" />
				{:else}
					<File class="h-3.5 w-3.5 shrink-0 text-muted-foreground" />
				{/if}
				<span class="truncate text-xs text-foreground">{node.name}</span>
			</a>
		{/if}
	</div>
{/snippet}
