<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { renderMarkdownHtml } from '$lib/markdown';
	import {
		issues,
		labels,
		milestones,
		orgs,
		projects,
		repos,
		users,
		type Collaborator,
		type Label,
		type Milestone,
		type Organization,
		type Project,
		type ProjectColumn,
		type ProjectItem,
		type Repository,
		type User
	} from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import * as DropdownMenu from '$lib/components/ui/dropdown-menu/index.js';
	import {
		Plus,
		GripVertical,
		Circle,
		Trash2,
		Lock,
		Globe2,
		Settings2,
		Ellipsis,
		Check,
		X,
		FilePlus2,
		Heading,
		Bold,
		Italic,
		List,
		ListOrdered,
		ListChecks,
		Link2,
		Code2,
		ChevronDown,
		Columns3,
		UserRound,
		ExternalLink
	} from '@lucide/svelte';

	const projectID = $derived(page.params.projectID);
	const orgCtx = getContext<{
		isOrg: boolean;
		org: Organization | null;
		isOwner: boolean;
		loading: boolean;
	}>('org');

	let loading = $state(true);
	let error = $state('');
	let project = $state<Project | null>(null);
	let mutationError = $state('');

	let showColumnDialog = $state(false);
	let editingColumnID = $state<string | null>(null);
	let creatingColumn = $state(false);
	let newColumnName = $state('');
	let newColumnDescription = $state('');
	let newColumnColor = $state('#22c55e');
	let showDeleteColumnDialog = $state(false);
	let deletingColumn = $state(false);
	let deleteColumnID = $state<string | null>(null);
	let deleteColumnConfirmText = $state('');
	let activeCardColumnID = $state<string | null>(null);
	let showCreateIssueDialog = $state(false);
	let creatingIssue = $state(false);
	let issueTitle = $state('');
	let issueBody = $state('');
	let issueRepo = $state('');
	let issueTab = $state<'write' | 'preview'>('write');
	let issueTextareaEl = $state<HTMLTextAreaElement | null>(null);
	let issueMetaLoading = $state(false);
	let issueCollaborators = $state<Collaborator[]>([]);
	let issueLabels = $state<Label[]>([]);
	let issueMilestones = $state<Milestone[]>([]);
	let issueAssigneeID = $state<number | null>(null);
	let issueMilestoneID = $state<number | null>(null);
	let issueLabelIDs = $state<number[]>([]);
	let issueMetaMenu = $state<'assignee' | 'labels' | 'milestone' | null>(null);
	let ownerRepos = $state<Repository[]>([]);
	let loadingOwnerRepos = $state(false);
	let showItemDetailsDialog = $state(false);
	let savingItemDetails = $state(false);
	let deletingItemDetails = $state(false);
	let itemDetailsItemID = $state<string | null>(null);
	let itemDetailsTitle = $state('');
	let itemDetailsBody = $state('');
	let itemDetailsIssueRef = $state<{ owner: string; repo: string; number: number } | null>(null);
	let itemDetailsColumnID = $state('');
	let itemDetailsAssigneeID = $state<string | null>(null);
	let itemDetailsTab = $state<'write' | 'preview'>('write');
	let itemDetailsTextareaEl = $state<HTMLTextAreaElement | null>(null);

	let draggingItemID = $state<string | null>(null);
	let draggingFromColumnID = $state<string | null>(null);
	let movingItem = $state(false);
	let draggingColumnID = $state<string | null>(null);
	let movingColumn = $state(false);
	let columnDropTargetID = $state<string | null>(null);

	const columnPalette = ['#22c55e', '#3b82f6', '#16a34a', '#eab308', '#f97316', '#f43f5e', '#ec4899', '#8b5cf6'];

	const canManage = $derived.by(() => {
		if (!project || !authStore.user) return false;
		if (project.owner_user?.username) return project.owner_user.username === authStore.user.username;
		if (project.owner_org?.login) return orgCtx.isOrg && orgCtx.isOwner && orgCtx.org?.login === project.owner_org.login;
		return false;
	});

	const sortedColumns = $derived.by(() => {
		if (!project?.columns) return [] as ProjectColumn[];
		return [...project.columns].sort((a, b) => a.position - b.position);
	});

	const itemAssigneeCandidates = $derived.by(() => {
		const map = new Map<string, User>();
		const add = (u?: User | null) => {
			if (!u || !u.id) return;
			map.set(String(u.id), u);
		};
		add(authStore.user);
		add(project?.owner_user);
		add(project?.created_by);
		for (const column of sortedColumns) {
			for (const item of column.items ?? []) add(item.assignee_user);
		}
		return [...map.values()];
	});

	$effect(() => {
		const pid = projectID;
		if (!pid) return;
		loadProject(pid);
	});

	async function loadProject(pid: string) {
		loading = true;
		error = '';
		mutationError = '';
		try {
			const data = await projects.get(pid);
			if (projectID !== pid) return;
			project = data.project;
		} catch (e: any) {
			if (projectID !== pid) return;
			error = e?.message ?? 'Failed to load project';
			project = null;
		} finally {
			if (projectID === pid) loading = false;
		}
	}

	function filteredColumnItems(column: ProjectColumn): ProjectItem[] {
		const sorted = [...(column.items ?? [])].sort((a, b) => a.position - b.position);
		return sorted;
	}

	function openCreateColumnDialog() {
		editingColumnID = null;
		newColumnName = '';
		newColumnDescription = '';
		newColumnColor = '#22c55e';
		showColumnDialog = true;
	}

	function openEditColumnDialog(column: ProjectColumn) {
		editingColumnID = column.id;
		newColumnName = column.name;
		newColumnDescription = column.description ?? '';
		newColumnColor = column.color || '#22c55e';
		showColumnDialog = true;
	}

	function openDeleteColumnDialog(columnID: string) {
		deleteColumnID = columnID;
		deleteColumnConfirmText = '';
		showDeleteColumnDialog = true;
	}

	async function saveColumn() {
		if (!project || !newColumnName.trim()) return;
		creatingColumn = true;
		mutationError = '';
		try {
			if (editingColumnID) {
				await projects.columns.update(project.id, editingColumnID, {
					name: newColumnName.trim(),
					description: newColumnDescription.trim(),
					color: newColumnColor
				});
			} else {
				await projects.columns.create(project.id, {
					name: newColumnName.trim(),
					description: newColumnDescription.trim(),
					color: newColumnColor
				});
			}
			newColumnName = '';
			newColumnDescription = '';
			newColumnColor = '#22c55e';
			showColumnDialog = false;
			editingColumnID = null;
			await loadProject(project.id);
		} catch (e: any) {
			mutationError = e?.message ?? `Failed to ${editingColumnID ? 'update' : 'create'} column`;
		} finally {
			creatingColumn = false;
		}
	}

	async function confirmDeleteColumn() {
		if (!project || !deleteColumnID || deleteColumnConfirmText !== 'DELETE') return;
		deletingColumn = true;
		mutationError = '';
		try {
			await projects.columns.delete(project.id, deleteColumnID);
			await loadProject(project.id);
			showDeleteColumnDialog = false;
			deleteColumnID = null;
			deleteColumnConfirmText = '';
		} catch (e: any) {
			mutationError = e?.message ?? 'Failed to delete column';
		} finally {
			deletingColumn = false;
		}
	}

	async function createCard(columnID: string, title: string, body: string) {
		if (!project) return;
		const cleanTitle = title.trim();
		if (!cleanTitle) return;
		const cleanBody = body.trim();
		mutationError = '';
		try {
			await projects.items.create(project.id, {
				column_id: columnID,
				title: cleanTitle,
				body: cleanBody
			});
			await loadProject(project.id);
		} catch (e: any) {
			mutationError = e?.message ?? 'Failed to create card';
		}
	}

	async function loadOwnerRepos() {
		if (!project || loadingOwnerRepos) return;
		if (ownerRepos.length > 0) return;

		loadingOwnerRepos = true;
		try {
			if (project.owner_org?.login) {
				ownerRepos = await orgs.repos.list(project.owner_org.login);
			} else if (project.owner_user?.username) {
				const profile = await users.getProfile(project.owner_user.username, { limit: 200, offset: 0 });
				ownerRepos = profile.repos ?? [];
			} else {
				ownerRepos = [];
			}
			if (ownerRepos.length > 0 && !issueRepo) issueRepo = ownerRepos[0].name;
		} catch {
			ownerRepos = [];
		} finally {
			loadingOwnerRepos = false;
		}
	}

	async function openCreateIssueDialog(columnID: string) {
		activeCardColumnID = columnID;
		issueTitle = '';
		issueBody = '';
		issueTab = 'write';
		issueAssigneeID = null;
		issueMilestoneID = null;
		issueLabelIDs = [];
		issueMetaMenu = null;
		await loadOwnerRepos();
		if (issueRepo) await loadIssueMeta(issueRepo);
		showCreateIssueDialog = true;
	}

	async function submitCreateIssue() {
		if (!project || !activeCardColumnID) return;
		if (!issueTitle.trim() || !issueRepo) return;

		const owner = project.owner_org?.login ?? project.owner_user?.username;
		if (!owner) return;

		creatingIssue = true;
		mutationError = '';
		try {
			const payload: any = { title: issueTitle.trim(), body: issueBody.trim() };
			if (issueAssigneeID !== null) payload.assignee_id = issueAssigneeID;
			if (issueMilestoneID !== null) payload.milestone_id = issueMilestoneID;
			if (issueLabelIDs.length > 0) payload.label_ids = issueLabelIDs;

			const created = await issues.create(owner, issueRepo, payload);
			const issue = created.issue;
			const itemTitle = `${issue.title} #${issue.number}`;
			const itemBody = mergeBodyWithIssueRef(issueBody, { owner, repo: issueRepo, number: issue.number });
			await createCard(activeCardColumnID, itemTitle, itemBody);
			showCreateIssueDialog = false;
			issueTitle = '';
			issueBody = '';
		} catch (e: any) {
			mutationError = e?.message ?? 'Failed to create issue';
		} finally {
			creatingIssue = false;
		}
	}

	async function loadIssueMeta(repoName: string) {
		if (!project || !repoName) return;
		const owner = project.owner_org?.login ?? project.owner_user?.username;
		if (!owner) return;

		issueMetaLoading = true;
		try {
			const [collabRes, labelsRes, milestonesRes] = await Promise.all([
				repos.collaborators.list(owner, repoName).catch(() => ({ collaborators: [] })),
				labels.list(owner, repoName).catch(() => ({ labels: [] })),
				milestones.list(owner, repoName, 'open').catch(() => ({ milestones: [] }))
			]);
			issueCollaborators = collabRes.collaborators ?? [];
			issueLabels = labelsRes.labels ?? [];
			issueMilestones = milestonesRes.milestones ?? [];

			if (issueAssigneeID !== null && !issueCollaborators.some((c) => c.user_id === issueAssigneeID)) issueAssigneeID = null;
			if (issueMilestoneID !== null && !issueMilestones.some((m) => m.id === issueMilestoneID)) issueMilestoneID = null;
			issueLabelIDs = issueLabelIDs.filter((id) => issueLabels.some((l) => l.id === id));
		} finally {
			issueMetaLoading = false;
		}
	}

	function toggleIssueLabel(id: number) {
		if (issueLabelIDs.includes(id)) {
			issueLabelIDs = issueLabelIDs.filter((v) => v !== id);
		} else {
			issueLabelIDs = [...issueLabelIDs, id];
		}
	}

	function selectedAssigneeName(): string {
		if (issueAssigneeID === null) return 'Assignee';
		const c = issueCollaborators.find((v) => v.user_id === issueAssigneeID);
		return c?.user?.username ?? 'Assignee';
	}

	function selectedMilestoneName(): string {
		if (issueMilestoneID === null) return 'Milestone';
		const m = issueMilestones.find((v) => v.id === issueMilestoneID);
		return m?.title ?? 'Milestone';
	}

	function selectedLabelsPreview(): string {
		if (issueLabelIDs.length === 0) return 'Label';
		const selected = issueLabels.filter((label) => issueLabelIDs.includes(label.id));
		if (selected.length === 0) return 'Label';
		if (selected.length <= 2) return selected.map((label) => label.name).join(', ');
		return `${selected[0].name}, ${selected[1].name} +${selected.length - 2}`;
	}

	function userInitial(username: string): string {
		return username?.trim().charAt(0).toUpperCase() || '?';
	}

	function toggleIssueMetaMenu(menu: 'assignee' | 'labels' | 'milestone') {
		issueMetaMenu = issueMetaMenu === menu ? null : menu;
	}

	function wrapSelection(prefix: string, suffix = '') {
		const el = issueTextareaEl;
		if (!el) return;
		const start = el.selectionStart;
		const end = el.selectionEnd;
		const selected = issueBody.slice(start, end);
		const replacement = `${prefix}${selected}${suffix}`;
		issueBody = issueBody.slice(0, start) + replacement + issueBody.slice(end);
		requestAnimationFrame(() => {
			const cursor = start + replacement.length;
			el.focus();
			el.setSelectionRange(cursor, cursor);
		});
	}

	function insertLinePrefix(prefix: string) {
		const el = issueTextareaEl;
		if (!el) return;
		const start = el.selectionStart;
		const lineStart = issueBody.lastIndexOf('\n', start - 1) + 1;
		issueBody = `${issueBody.slice(0, lineStart)}${prefix}${issueBody.slice(lineStart)}`;
		requestAnimationFrame(() => {
			const cursor = start + prefix.length;
			el.focus();
			el.setSelectionRange(cursor, cursor);
		});
	}

	function openItemDetails(item: ProjectItem, columnID: string) {
		const parsed = splitIssueRefFromBody(item.body ?? '');
		itemDetailsItemID = item.id;
		itemDetailsTitle = item.title;
		itemDetailsBody = parsed.body;
		itemDetailsIssueRef = parsed.ref;
		itemDetailsColumnID = columnID;
		itemDetailsAssigneeID = item.assignee_user_id ?? null;
		itemDetailsTab = 'write';
		showItemDetailsDialog = true;
	}

	function itemDetailsAssigneeLabel(): string {
		if (!itemDetailsAssigneeID) return 'Assignee';
		const found = itemAssigneeCandidates.find((u) => String(u.id) === itemDetailsAssigneeID);
		return found?.username ?? 'Assignee';
	}

	function itemDetailsColumnLabel(): string {
		if (!itemDetailsColumnID) return 'Column';
		return sortedColumns.find((c) => c.id === itemDetailsColumnID)?.name ?? 'Column';
	}

	function itemBodyIssueRef(body: string): { owner: string; repo: string; number: number } | null {
		const m = body.match(/([a-zA-Z0-9-]+)\/([a-zA-Z0-9._-]+)#(\d+)/);
		if (!m) return null;
		return { owner: m[1], repo: m[2], number: Number(m[3]) };
	}

	function issueRefToString(ref: { owner: string; repo: string; number: number }): string {
		return `${ref.owner}/${ref.repo}#${ref.number}`;
	}

	function issueRefMetaComment(ref: { owner: string; repo: string; number: number }): string {
		return `<!--gitpier:issue_ref:${issueRefToString(ref)}-->`;
	}

	function splitIssueRefFromBody(raw: string): { body: string; ref: { owner: string; repo: string; number: number } | null } {
		const meta = raw.match(/<!--gitpier:issue_ref:([a-zA-Z0-9-]+)\/([a-zA-Z0-9._-]+)#(\d+)-->/);
		if (meta) {
			const ref = { owner: meta[1], repo: meta[2], number: Number(meta[3]) };
			const clean = raw.replace(meta[0], '').trim();
			return { body: clean, ref };
		}

		const inline = itemBodyIssueRef(raw);
		if (!inline) return { body: raw, ref: null };

		// Backward compatibility: migrate old bodies that contain leading issue reference text.
		const inlineRef = issueRefToString(inline);
		const withoutLeadingRef = raw.replace(new RegExp(`^\\s*${inlineRef}\\s*\\n*`), '').trim();
		return { body: withoutLeadingRef, ref: inline };
	}

	function mergeBodyWithIssueRef(body: string, ref: { owner: string; repo: string; number: number } | null): string {
		const clean = body.trim();
		if (!ref) return clean;
		const meta = issueRefMetaComment(ref);
		return clean ? `${clean}\n\n${meta}` : meta;
	}

	function wrapItemDetailsSelection(prefix: string, suffix = '') {
		const el = itemDetailsTextareaEl;
		if (!el) return;
		const start = el.selectionStart;
		const end = el.selectionEnd;
		const selected = itemDetailsBody.slice(start, end);
		const replacement = `${prefix}${selected}${suffix}`;
		itemDetailsBody = itemDetailsBody.slice(0, start) + replacement + itemDetailsBody.slice(end);
		requestAnimationFrame(() => {
			const cursor = start + replacement.length;
			el.focus();
			el.setSelectionRange(cursor, cursor);
		});
	}

	function insertItemDetailsLinePrefix(prefix: string) {
		const el = itemDetailsTextareaEl;
		if (!el) return;
		const start = el.selectionStart;
		const lineStart = itemDetailsBody.lastIndexOf('\n', start - 1) + 1;
		itemDetailsBody = `${itemDetailsBody.slice(0, lineStart)}${prefix}${itemDetailsBody.slice(lineStart)}`;
		requestAnimationFrame(() => {
			const cursor = start + prefix.length;
			el.focus();
			el.setSelectionRange(cursor, cursor);
		});
	}

	async function saveItemDetails() {
		if (!project || !itemDetailsItemID || !itemDetailsTitle.trim()) return;
		savingItemDetails = true;
		mutationError = '';
		try {
			await projects.items.update(project.id, itemDetailsItemID, {
				title: itemDetailsTitle.trim(),
				body: mergeBodyWithIssueRef(itemDetailsBody, itemDetailsIssueRef),
				...(itemDetailsAssigneeID ? { assignee_user_id: itemDetailsAssigneeID } : { clear_assignee: true })
			});

			const currentColumn = sortedColumns.find((c) => (c.items ?? []).some((i) => i.id === itemDetailsItemID));
			if (itemDetailsColumnID && currentColumn?.id !== itemDetailsColumnID) {
				const targetColumn = sortedColumns.find((c) => c.id === itemDetailsColumnID);
				await projects.items.move(project.id, itemDetailsItemID, {
					column_id: itemDetailsColumnID,
					position: targetColumn?.items?.length ?? 0
				});
			}

			await loadProject(project.id);
			showItemDetailsDialog = false;
		} catch (e: any) {
			mutationError = e?.message ?? 'Failed to update item';
		} finally {
			savingItemDetails = false;
		}
	}

	async function deleteItemFromDetails() {
		if (!project || !itemDetailsItemID) return;
		deletingItemDetails = true;
		mutationError = '';
		try {
			await projects.items.delete(project.id, itemDetailsItemID);
			await loadProject(project.id);
			showItemDetailsDialog = false;
		} catch (e: any) {
			mutationError = e?.message ?? 'Failed to delete card';
		} finally {
			deletingItemDetails = false;
		}
	}

	$effect(() => {
		if (!showCreateIssueDialog || !issueRepo) return;
		loadIssueMeta(issueRepo);
	});

	function handleDragStart(itemID: string, columnID: string) {
		draggingItemID = itemID;
		draggingFromColumnID = columnID;
	}

	function handleDragEnd() {
		draggingItemID = null;
		draggingFromColumnID = null;
	}

	function handleColumnDragStart(columnID: string) {
		draggingColumnID = columnID;
		columnDropTargetID = null;
	}

	function handleColumnDragEnd() {
		draggingColumnID = null;
		columnDropTargetID = null;
	}

	async function handleColumnDrop(targetColumnID: string) {
		if (!project || !draggingColumnID || movingColumn) return;
		if (draggingColumnID === targetColumnID) {
			handleColumnDragEnd();
			return;
		}
		columnDropTargetID = targetColumnID;

		movingColumn = true;
		mutationError = '';

		const prevColumns = (project.columns ?? []).map((column) => ({
			...column,
			items: [...(column.items ?? [])]
		}));

		try {
			const ordered = [...(project.columns ?? [])].sort((a, b) => a.position - b.position);
			const fromIndex = ordered.findIndex((column) => column.id === draggingColumnID);
			const toIndex = ordered.findIndex((column) => column.id === targetColumnID);
			if (fromIndex < 0 || toIndex < 0) throw new Error('Column not found');

			const [moved] = ordered.splice(fromIndex, 1);
			ordered.splice(toIndex, 0, moved);

			const nextColumns = ordered.map((column, index) => ({ ...column, position: index }));
			project = { ...project, columns: nextColumns };

			// Persist the full order to avoid duplicate positions when moving left/right.
			const changedColumns = nextColumns.filter((column) => {
				const prevColumn = prevColumns.find((prev) => prev.id === column.id);
				return prevColumn?.position !== column.position;
			});
			await Promise.all(changedColumns.map((column) => projects.columns.update(project.id, column.id, { position: column.position })));
			await loadProject(project.id);
		} catch (e: any) {
			project = { ...project, columns: prevColumns };
			mutationError = e?.message ?? 'Failed to reorder columns';
		} finally {
			movingColumn = false;
			handleColumnDragEnd();
		}
	}

	function handleBoardDrop(targetColumnID: string) {
		if (draggingColumnID) {
			void handleColumnDrop(targetColumnID);
			return;
		}
		void handleDrop(targetColumnID);
	}

	async function handleDrop(targetColumnID: string) {
		if (!project || !draggingItemID || movingItem) return;
		if (draggingFromColumnID === targetColumnID) {
			handleDragEnd();
			return;
		}
		movingItem = true;
		mutationError = '';
		const prevColumns = (project.columns ?? []).map((column) => ({
			...column,
			items: [...(column.items ?? [])]
		}));
		try {
			const nextColumns = (project.columns ?? []).map((column) => ({
				...column,
				items: [...(column.items ?? [])]
			}));
			const fromColumn = nextColumns.find((column) => column.id === draggingFromColumnID);
			const toColumn = nextColumns.find((column) => column.id === targetColumnID);
			if (!fromColumn || !toColumn) throw new Error('Column not found');

			const fromItems = fromColumn.items ?? [];
			const movingIndex = fromItems.findIndex((item) => item.id === draggingItemID);
			if (movingIndex < 0) throw new Error('Item not found');
			const [movingCard] = fromItems.splice(movingIndex, 1);
			toColumn.items = [...(toColumn.items ?? []), { ...movingCard, column_id: targetColumnID }];

			// Reindex positions locally to avoid a full board reload and prevent flicker.
			for (const column of nextColumns) {
				const items = column.items ?? [];
				column.items = items.map((item, index) => ({ ...item, position: index }));
			}
			project = { ...project, columns: nextColumns };

			const targetPosition = (toColumn.items?.length ?? 1) - 1;
			await projects.items.move(project.id, draggingItemID, { column_id: targetColumnID, position: targetPosition });
		} catch (e: any) {
			project = { ...project, columns: prevColumns };
			mutationError = e?.message ?? 'Failed to move card';
		} finally {
			movingItem = false;
			handleDragEnd();
		}
	}

	async function deleteCard(itemID: string) {
		if (!project) return;
		mutationError = '';
		try {
			await projects.items.delete(project.id, itemID);
			await loadProject(project.id);
		} catch (e: any) {
			mutationError = e?.message ?? 'Failed to delete card';
		}
	}
</script>

<svelte:head>
	<title>{project?.title ?? 'Project'} · Board · GitPier</title>
</svelte:head>

<div class="min-h-[78vh] bg-background px-3 py-4">
	<div class="mx-auto flex max-w-[1600px] flex-col gap-3">
		{#if loading}
			<div class="flex items-center max-w-container mx-auto w-full px-5">
				<div class="flex items-center gap-3">
					<div class="h-4 w-44 animate-pulse rounded bg-card"></div>
					<div class="h-8 w-20 animate-pulse rounded-md border border-secondary bg-card"></div>
				</div>
			</div>
			<div class="overflow-x-auto pb-2">
				<div class="mx-auto flex w-max min-w-max items-start gap-2.5">
					{#each Array(4) as _}
						<div class="flex h-[72vh] w-[292px] shrink-0 flex-col rounded-md border border-secondary bg-card">
							<div class="flex items-center justify-between border-b border-secondary px-2.5 py-2">
								<div class="flex items-center gap-2">
									<div class="h-3.5 w-3.5 animate-pulse rounded bg-background"></div>
									<div class="h-3 w-3 animate-pulse rounded-full bg-background"></div>
									<div class="h-4 w-24 animate-pulse rounded bg-background"></div>
									<div class="h-4 w-8 animate-pulse rounded-full border border-secondary bg-background"></div>
								</div>
								<div class="h-4 w-10 animate-pulse rounded bg-background"></div>
							</div>
							<div class="flex-1 space-y-1.5 overflow-hidden px-2 py-2">
								{#each Array(2) as __}
									<div class="rounded-md border border-secondary bg-background p-2">
										<div class="mb-2 h-3 w-16 animate-pulse rounded bg-card"></div>
										<div class="h-4 w-4/5 animate-pulse rounded bg-card"></div>
										<div class="mt-2 h-3 w-1/2 animate-pulse rounded bg-card"></div>
									</div>
								{/each}
							</div>
						</div>
					{/each}
					<div class="h-8 w-8 animate-pulse rounded-md border border-secondary bg-card"></div>
			</div>
			</div>
		{:else if error || !project}
			<div class="rounded-md border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive">{error || 'Project not found'}</div>
		{:else}
			<div class="flex items-center max-w-container mx-auto w-full md:px-5">
				<div class="flex items-center gap-3">
					<p class="text-sm font-semibold text-foreground">
						<a class="hover:underline" href={`/${project?.owner_user?.username || project?.owner_org?.login || page.params.username}`}>
							{project?.owner_user?.username || project?.owner_org?.login || page.params.username}
						</a>
						<span>/</span>
						<a class="hover:underline" href={`/${page.params.username}/projects/${project?.id}/settings`}>{project?.title}</a>
					</p>
					<Button variant="ghost" size="sm" class="h-8 px-2 text-xs" onclick={() => goto(`/${page.params.username}/projects/${project?.id}/settings`)}>
						<Settings2 class="h-3.5 w-3.5" />
						<span>Settings</span>
					</Button>
				</div>
			</div>

			{#if mutationError}
				<div class="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">{mutationError}</div>
			{/if}

			<div class="overflow-x-auto pb-2">
				<div class="mx-auto flex w-max min-w-max items-start gap-2.5">
					{#each sortedColumns as column (column.id)}
						{@const items = filteredColumnItems(column)}
						<div
							class="flex h-[72vh] w-[292px] shrink-0 flex-col rounded-md border border-secondary bg-card"
							role="region"
							ondragover={(event) => {
								event.preventDefault();
								if (draggingColumnID) columnDropTargetID = column.id;
							}}
							ondrop={() => handleBoardDrop(column.id)}
						>
							{#if draggingColumnID && columnDropTargetID === column.id}
								<div class="mx-2 mt-2 h-1.5 rounded-full bg-primary/80"></div>
							{/if}
							<div class="flex items-center justify-between border-b border-secondary px-2.5 py-2">
								<div class="flex items-center gap-2 min-w-0">
									{#if canManage}
										<button
											class="rounded p-0.5 text-muted-foreground hover:bg-secondary"
											aria-label="Drag column"
											draggable="true"
											ondragstart={() => handleColumnDragStart(column.id)}
											ondragend={handleColumnDragEnd}
										>
											<GripVertical class="h-3.5 w-3.5" />
										</button>
									{/if}
									<Circle class="h-3 w-3 shrink-0" style={`color:${column.color}`} fill={column.color} />
									<span class="truncate text-sm font-semibold text-foreground">{column.name}</span>
									<span class="rounded-full border border-border bg-background px-1.5 py-0.5 text-[11px] text-muted-foreground">{items.length}/{column.items?.length ?? 0}</span>
								</div>
								<div class="flex items-center gap-1 text-muted-foreground">
									<DropdownMenu.Root>
										<DropdownMenu.Trigger class="rounded p-1 hover:bg-secondary" aria-label="Column options">
											<Ellipsis class="h-3.5 w-3.5" />
										</DropdownMenu.Trigger>
										<DropdownMenu.Content align="end" class="w-40">
											<DropdownMenu.Item onclick={() => openEditColumnDialog(column)}>Edit column</DropdownMenu.Item>
											<DropdownMenu.Separator />
											<DropdownMenu.Item class="text-destructive focus:text-destructive" onclick={() => openDeleteColumnDialog(column.id)}>Delete column</DropdownMenu.Item>
										</DropdownMenu.Content>
									</DropdownMenu.Root>
									{#if canManage}
										<DropdownMenu.Root>
											<DropdownMenu.Trigger class="rounded p-1 hover:bg-secondary" aria-label="Add item">
												<Plus class="h-3.5 w-3.5" />
											</DropdownMenu.Trigger>
											<DropdownMenu.Content align="end" class="w-56">
												<DropdownMenu.Item onclick={() => openCreateIssueDialog(column.id)}>
													<span class="flex items-center gap-2"><FilePlus2 class="h-4 w-4 text-muted-foreground" />Create new issue</span>
												</DropdownMenu.Item>
											</DropdownMenu.Content>
										</DropdownMenu.Root>
									{/if}
								</div>
							</div>

							<div class="flex-1 space-y-1.5 overflow-y-auto px-2 py-2">
								{#each items as item (item.id)}
									{@const visibleBody = splitIssueRefFromBody(item.body ?? '').body}
									<div
										class="rounded-md border border-secondary bg-background p-2 transition-colors hover:border-border hover:bg-secondary/20"
										role="button"
										tabindex="0"
										aria-label={`Open item ${item.title}`}
										onclick={() => openItemDetails(item, column.id)}
										onkeydown={(event) => {
											if (event.key === 'Enter' || event.key === ' ') {
												event.preventDefault();
												openItemDetails(item, column.id);
											}
										}}
										draggable={canManage}
										ondragstart={() => handleDragStart(item.id, column.id)}
										ondragend={handleDragEnd}
									>
										<div class="mb-1 flex items-start justify-between gap-2">
											<div class="flex min-w-0 items-center gap-1.5 text-[11px] text-muted-foreground">
												<Circle class="h-3 w-3 shrink-0" style={`color:${column.color}`} />
												<span class="truncate">item #{item.position + 1}</span>
											</div>
											<div class="flex items-center gap-1">
												{#if canManage}<GripVertical class="h-3.5 w-3.5 text-muted-foreground" />{/if}
												{#if canManage}
													<button
														class="rounded p-0.5 text-muted-foreground hover:bg-secondary hover:text-destructive"
														onclick={(event) => {
															event.stopPropagation();
															deleteCard(item.id);
														}}
														aria-label="Delete card"
													>
														<Trash2 class="h-3 w-3" />
													</button>
												{/if}
											</div>
										</div>
										<div class="text-sm font-medium leading-5 text-foreground">{item.title}</div>
										{#if visibleBody}<p class="mt-1 line-clamp-3 text-xs text-muted-foreground">{visibleBody}</p>{/if}
									</div>
								{/each}
							</div>
						</div>
					{/each}

					{#if canManage}
						<button
							class="inline-flex h-8 w-8 items-center justify-center rounded-md border border-border bg-card text-muted-foreground hover:bg-secondary"
							onclick={openCreateColumnDialog}
							aria-label="Add column"
						>
							<Plus class="h-4 w-4" />
						</button>
					{/if}
				</div>
			</div>
		{/if}
	</div>
</div>

<Dialog.Root bind:open={showColumnDialog}>
	<Dialog.Content class="max-w-md rounded-2xl">
		<Dialog.Header>
			<Dialog.Title class="text-xl">{editingColumnID ? 'Edit column' : 'New column'}</Dialog.Title>
			<Dialog.Description></Dialog.Description>
		</Dialog.Header>
		<div class="space-y-4 pt-1">
			<div>
				<label class="mb-1 block text-sm font-semibold text-foreground" for="col-name">Label text *</label>
				<input
					id="col-name"
					bind:value={newColumnName}
					class="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring"
				/>
			</div>

			<div>
				<p class="mb-1.5 block text-sm font-semibold text-foreground">Color</p>
				<div class="flex flex-wrap gap-1.5">
					{#each columnPalette as color}
						<button
							type="button"
							class="relative h-8 w-8 rounded-md border border-secondary"
							style={`background:${color}`}
							onclick={() => (newColumnColor = color)}
							aria-label={`Pick ${color}`}
						>
							{#if newColumnColor === color}
								<span class="absolute inset-0 flex items-center justify-center">
									<Check class="h-3.5 w-3.5 text-white" />
								</span>
							{/if}
						</button>
					{/each}
				</div>
			</div>

			<div>
				<label class="mb-1 block text-sm font-semibold text-foreground" for="col-description">Description</label>
				<textarea
					id="col-description"
					bind:value={newColumnDescription}
					rows="3"
					class="w-full rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring"
					placeholder="Column description"
				></textarea>
			</div>
		</div>
		<Dialog.Footer class="mt-4 gap-2">
			<Button variant="outline" onclick={() => (showColumnDialog = false)} disabled={creatingColumn}>Cancel</Button>
			<Button variant="brand" onclick={saveColumn} disabled={creatingColumn || !newColumnName.trim()}>
				{creatingColumn ? (editingColumnID ? 'Saving...' : 'Creating...') : editingColumnID ? 'Save' : 'Create'}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={showDeleteColumnDialog}>
	<Dialog.Content class="max-w-md rounded-2xl">
		<Dialog.Header>
			<Dialog.Title class="text-xl">Delete column</Dialog.Title>
			<Dialog.Description>
				This action cannot be undone. To confirm, type <span class="font-semibold text-foreground">DELETE</span>.
			</Dialog.Description>
		</Dialog.Header>
		<div class="space-y-3 pt-1">
			<input
				bind:value={deleteColumnConfirmText}
				placeholder="Type DELETE to confirm"
				class="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring"
			/>
		</div>
		<Dialog.Footer class="mt-4 gap-2">
			<Button
				variant="outline"
				onclick={() => {
					showDeleteColumnDialog = false;
					deleteColumnID = null;
					deleteColumnConfirmText = '';
				}}
				disabled={deletingColumn}
			>
				Cancel
			</Button>
			<Button variant="destructive" onclick={confirmDeleteColumn} disabled={deletingColumn || deleteColumnConfirmText !== 'DELETE'}>
				{deletingColumn ? 'Deleting...' : 'Delete column'}
			</Button>
		</Dialog.Footer>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={showItemDetailsDialog}>
	<Dialog.Content
		showCloseButton={false}
		class="!max-w-[min(1280px,calc(100vw-2rem))] sm:!max-w-[min(1280px,calc(100vw-3rem))] rounded-2xl p-0"
	>
		<div class="flex items-center justify-between border-b border-secondary px-4 py-3">
			<div class="flex min-w-0 items-center gap-2">
				<p class="truncate text-xl font-semibold text-foreground">{itemDetailsTitle || 'Item details'}</p>
				{#if itemDetailsItemID}
					<span class="shrink-0 rounded border border-border bg-background px-1.5 py-0.5 text-xs text-muted-foreground">#{itemDetailsItemID.slice(0, 8)}</span>
				{/if}
			</div>
			<button class="rounded p-1 text-muted-foreground hover:bg-secondary" onclick={() => (showItemDetailsDialog = false)} aria-label="Close">
				<X class="h-4 w-4" />
			</button>
		</div>

		<div class="grid gap-0 lg:grid-cols-[1fr_280px]">
			<div class="space-y-4 border-b border-secondary p-4 lg:border-b-0 lg:border-r">
				<div>
					<label class="mb-1 block text-sm font-semibold text-foreground" for="item-details-title">Title <span class="text-destructive">*</span></label>
					<input
						id="item-details-title"
						bind:value={itemDetailsTitle}
						disabled={!canManage}
						class="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring disabled:cursor-not-allowed disabled:opacity-80"
					/>
				</div>

				<div>
					<p class="mb-1 block text-sm font-semibold text-foreground">Description</p>
					<div class="overflow-hidden rounded-md border border-input bg-background">
						<div class="flex items-center justify-between border-b border-secondary">
							<div class="flex items-center text-sm">
								<button
									class="border-r border-secondary px-4 py-2 font-semibold {itemDetailsTab === 'write' ? 'bg-background text-foreground' : 'text-muted-foreground'}"
									onclick={() => (itemDetailsTab = 'write')}
								>
									Write
								</button>
								<button class="px-4 py-2 {itemDetailsTab === 'preview' ? 'bg-background text-foreground' : 'text-muted-foreground'}" onclick={() => (itemDetailsTab = 'preview')}>
									Preview
								</button>
							</div>
							<div class="mr-2 flex items-center gap-1 text-muted-foreground">
								<button class="rounded p-1 hover:bg-secondary disabled:opacity-50" disabled={!canManage} onclick={() => insertItemDetailsLinePrefix('# ')} aria-label="Heading"
									><Heading class="h-4 w-4" /></button
								>
								<button class="rounded p-1 hover:bg-secondary disabled:opacity-50" disabled={!canManage} onclick={() => wrapItemDetailsSelection('**', '**')} aria-label="Bold"
									><Bold class="h-4 w-4" /></button
								>
								<button class="rounded p-1 hover:bg-secondary disabled:opacity-50" disabled={!canManage} onclick={() => wrapItemDetailsSelection('*', '*')} aria-label="Italic"
									><Italic class="h-4 w-4" /></button
								>
								<button class="rounded p-1 hover:bg-secondary disabled:opacity-50" disabled={!canManage} onclick={() => wrapItemDetailsSelection('`', '`')} aria-label="Code"
									><Code2 class="h-4 w-4" /></button
								>
								<button class="rounded p-1 hover:bg-secondary disabled:opacity-50" disabled={!canManage} onclick={() => wrapItemDetailsSelection('[', '](https://)')} aria-label="Link"
									><Link2 class="h-4 w-4" /></button
								>
								<div class="mx-1 h-5 w-px bg-secondary"></div>
								<button class="rounded p-1 hover:bg-secondary disabled:opacity-50" disabled={!canManage} onclick={() => insertItemDetailsLinePrefix('- ')} aria-label="Bulleted list"
									><List class="h-4 w-4" /></button
								>
								<button class="rounded p-1 hover:bg-secondary disabled:opacity-50" disabled={!canManage} onclick={() => insertItemDetailsLinePrefix('1. ')} aria-label="Numbered list"
									><ListOrdered class="h-4 w-4" /></button
								>
								<button class="rounded p-1 hover:bg-secondary disabled:opacity-50" disabled={!canManage} onclick={() => insertItemDetailsLinePrefix('- [ ] ')} aria-label="Task list"
									><ListChecks class="h-4 w-4" /></button
								>
							</div>
						</div>

						{#if itemDetailsTab === 'write'}
							<textarea
								bind:this={itemDetailsTextareaEl}
								bind:value={itemDetailsBody}
								rows="16"
								disabled={!canManage}
								class="w-full resize-none border-0 bg-transparent px-4 py-3 text-sm text-foreground outline-none disabled:cursor-not-allowed disabled:opacity-80"
								placeholder="Type description..."
							></textarea>
						{:else}
							<div class="min-h-[24rem] px-4 py-3 text-sm prose prose-invert max-w-none">
								{#if itemDetailsBody.trim()}
									{@html renderMarkdownHtml(itemDetailsBody)}
								{:else}
									<p class="text-muted-foreground">Nothing to preview.</p>
								{/if}
							</div>
						{/if}
					</div>
				</div>
			</div>

			<div class="space-y-4 p-4">
				<div>
					<p class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Assignee</p>
					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							class="inline-flex w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground hover:bg-secondary disabled:opacity-80"
							disabled={!canManage}
						>
							<span class="inline-flex items-center gap-2 truncate">
								<UserRound class="h-4 w-4 text-muted-foreground" />
								{itemDetailsAssigneeLabel()}
							</span>
							<ChevronDown class="h-4 w-4 text-muted-foreground" />
						</DropdownMenu.Trigger>
						<DropdownMenu.Content align="start" class="w-[240px]">
							<DropdownMenu.Item
								onclick={() => {
									itemDetailsAssigneeID = null;
								}}
							>
								<span class="flex w-full items-center justify-between"
									>None {#if itemDetailsAssigneeID === null}<Check class="h-3.5 w-3.5 text-muted-foreground" />{/if}</span
								>
							</DropdownMenu.Item>
							{#each itemAssigneeCandidates as assignee}
								<DropdownMenu.Item
									onclick={() => {
										itemDetailsAssigneeID = String(assignee.id);
									}}
								>
									<span class="flex w-full items-center justify-between truncate"
										>{assignee.username}
										{#if itemDetailsAssigneeID === String(assignee.id)}<Check class="h-3.5 w-3.5 text-muted-foreground" />{/if}</span
									>
								</DropdownMenu.Item>
							{/each}
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				</div>

				<div>
					<p class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Column</p>
					<DropdownMenu.Root>
						<DropdownMenu.Trigger
							class="inline-flex w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground hover:bg-secondary disabled:opacity-80"
							disabled={!canManage}
						>
							<span class="inline-flex items-center gap-2 truncate">
								<Columns3 class="h-4 w-4 text-muted-foreground" />
								{itemDetailsColumnLabel()}
							</span>
							<ChevronDown class="h-4 w-4 text-muted-foreground" />
						</DropdownMenu.Trigger>
						<DropdownMenu.Content align="start" class="w-[240px]">
							{#each sortedColumns as col}
								<DropdownMenu.Item
									onclick={() => {
										itemDetailsColumnID = col.id;
									}}
								>
									<span class="inline-flex w-full items-center justify-between gap-2 truncate">
										<span class="inline-flex items-center gap-2 truncate">
											<Circle class="h-3 w-3 shrink-0" style={`color:${col.color}`} fill={col.color} />
											{col.name}
										</span>
										{#if itemDetailsColumnID === col.id}<Check class="h-3.5 w-3.5 text-muted-foreground" />{/if}
									</span>
								</DropdownMenu.Item>
							{/each}
						</DropdownMenu.Content>
					</DropdownMenu.Root>
				</div>

				<div>
					<p class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Linked issue</p>
					{#if itemDetailsIssueRef}
						{@const ref = itemDetailsIssueRef}
						<a
							class="inline-flex w-full items-center justify-between rounded-md border border-input bg-background px-3 py-2 text-sm text-foreground hover:bg-secondary"
							href="/{ref.owner}/{ref.repo}/issues/{ref.number}"
							target="_blank"
							rel="noreferrer"
						>
							<span class="truncate">{ref.owner}/{ref.repo}#{ref.number}</span>
							<ExternalLink class="h-3.5 w-3.5 text-muted-foreground" />
						</a>
					{:else}
						<p class="rounded-md border border-input bg-background px-3 py-2 text-sm text-muted-foreground">No linked issue reference in item description</p>
					{/if}
				</div>
			</div>
		</div>

		<div class="flex items-center justify-between border-t border-secondary px-4 py-3">
			<Button variant="destructive" onclick={deleteItemFromDetails} disabled={deletingItemDetails || !canManage}>
				{deletingItemDetails ? 'Deleting...' : 'Delete'}
			</Button>
			<div class="flex items-center gap-2">
				<Button variant="outline" onclick={() => (showItemDetailsDialog = false)} disabled={savingItemDetails || deletingItemDetails}>Cancel</Button>
				<Button variant="brand" onclick={saveItemDetails} disabled={savingItemDetails || !itemDetailsTitle.trim() || !canManage}>
					{savingItemDetails ? 'Saving...' : 'Save changes'}
				</Button>
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={showCreateIssueDialog}>
	<Dialog.Content
		showCloseButton={false}
		class="!max-w-[min(1280px,calc(100vw-2rem))] sm:!max-w-[min(1280px,calc(100vw-3rem))] rounded-2xl p-0"
	>
		<div class="flex items-center justify-between border-b border-secondary px-4 py-3">
			<p class="truncate text-xl font-semibold text-foreground">Create New Issue</p>
			<button class="rounded p-1 text-muted-foreground hover:bg-secondary" onclick={() => (showCreateIssueDialog = false)} aria-label="Close">
				<X class="h-4 w-4" />
			</button>
		</div>

		<div class="grid gap-0 lg:grid-cols-[1fr_280px]">
			<div class="space-y-4 border-b border-secondary p-4 lg:border-b-0 lg:border-r">
				<div>
					<label class="mb-1 block text-sm font-semibold text-foreground" for="issue-title">Title <span class="text-destructive">*</span></label>
					<input
						id="issue-title"
						bind:value={issueTitle}
						class="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring"
					/>
				</div>

				<div>
					<p class="mb-1 block text-sm font-semibold text-foreground">Description</p>
					<div class="overflow-hidden rounded-md border border-input bg-background">
						<div class="flex items-center justify-between border-b border-secondary">
							<div class="flex items-center text-sm">
								<button
									class="border-r border-secondary px-4 py-2 font-semibold {issueTab === 'write' ? 'bg-background text-foreground' : 'text-muted-foreground'}"
									onclick={() => (issueTab = 'write')}>Write</button
								>
								<button class="px-4 py-2 {issueTab === 'preview' ? 'bg-background text-foreground' : 'text-muted-foreground'}" onclick={() => (issueTab = 'preview')}>Preview</button>
							</div>
							<div class="mr-2 flex items-center gap-1 text-muted-foreground">
								<button class="rounded p-1 hover:bg-secondary" onclick={() => insertLinePrefix('# ')} aria-label="Heading"><Heading class="h-4 w-4" /></button>
								<button class="rounded p-1 hover:bg-secondary" onclick={() => wrapSelection('**', '**')} aria-label="Bold"><Bold class="h-4 w-4" /></button>
								<button class="rounded p-1 hover:bg-secondary" onclick={() => wrapSelection('*', '*')} aria-label="Italic"><Italic class="h-4 w-4" /></button>
								<button class="rounded p-1 hover:bg-secondary" onclick={() => wrapSelection('`', '`')} aria-label="Code"><Code2 class="h-4 w-4" /></button>
								<button class="rounded p-1 hover:bg-secondary" onclick={() => wrapSelection('[', '](https://)')} aria-label="Link"><Link2 class="h-4 w-4" /></button>
								<div class="mx-1 h-5 w-px bg-secondary"></div>
								<button class="rounded p-1 hover:bg-secondary" onclick={() => insertLinePrefix('- ')} aria-label="Bulleted list"><List class="h-4 w-4" /></button>
								<button class="rounded p-1 hover:bg-secondary" onclick={() => insertLinePrefix('1. ')} aria-label="Numbered list"><ListOrdered class="h-4 w-4" /></button>
								<button class="rounded p-1 hover:bg-secondary" onclick={() => insertLinePrefix('- [ ] ')} aria-label="Task list"><ListChecks class="h-4 w-4" /></button>
							</div>
						</div>

						{#if issueTab === 'write'}
							<textarea
								bind:this={issueTextareaEl}
								bind:value={issueBody}
								rows="14"
								class="w-full resize-none border-0 bg-transparent px-4 py-3 text-sm text-foreground outline-none"
								placeholder="Type your description here..."
							></textarea>
						{:else}
							<div class="min-h-[22rem] px-4 py-3 text-sm prose prose-invert max-w-none">
								{#if issueBody.trim()}
									{@html renderMarkdownHtml(issueBody)}
								{:else}
									<p class="text-muted-foreground">Nothing to preview.</p>
								{/if}
							</div>
						{/if}
					</div>
				</div>
			</div>

			<div class="space-y-4 p-4">
				<div>
					<p class="mb-1 text-xs font-semibold uppercase tracking-wide text-muted-foreground">Repository</p>
					<select
						id="issue-repo"
						bind:value={issueRepo}
						disabled={loadingOwnerRepos || ownerRepos.length <= 1}
						class="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring disabled:cursor-not-allowed disabled:opacity-80"
					>
						{#if loadingOwnerRepos}
							<option value="">Loading repositories...</option>
						{:else if ownerRepos.length === 0}
							<option value="">No repositories available</option>
						{:else}
							{#each ownerRepos as repo}
								<option value={repo.name}>{repo.name}</option>
							{/each}
						{/if}
					</select>
				</div>

				<div class="flex flex-wrap items-center gap-2 text-xs">
					<div class="relative">
						<button
							class="inline-flex items-center rounded-md border border-secondary bg-secondary/60 px-2.5 py-1 text-muted-foreground hover:bg-secondary"
							onclick={() => toggleIssueMetaMenu('assignee')}
						>
							{selectedAssigneeName()}
						</button>
						{#if issueMetaMenu === 'assignee'}
							<div class="absolute left-0 top-8 z-50 w-64 overflow-hidden rounded-md border border-border bg-card shadow-lg">
								<div class="max-h-64 overflow-y-auto p-1">
									<button
										class="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm text-foreground hover:bg-secondary"
										onclick={() => {
											issueAssigneeID = null;
											issueMetaMenu = null;
										}}
									>
										<span>None</span>
										{#if issueAssigneeID === null}<Check class="h-3.5 w-3.5 text-muted-foreground" />{/if}
									</button>
									{#if issueMetaLoading}
										<div class="px-2 py-1.5 text-sm text-muted-foreground">Loading...</div>
									{:else if issueCollaborators.length === 0}
										<div class="px-2 py-1.5 text-sm text-muted-foreground">No collaborators</div>
									{:else}
										{#each issueCollaborators as collab}
											<button
												class="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm text-foreground hover:bg-secondary"
												onclick={() => {
													issueAssigneeID = collab.user_id;
													issueMetaMenu = null;
												}}
											>
												<span class="flex min-w-0 items-center gap-2">
													<span
														class="inline-flex h-5 w-5 items-center justify-center rounded-full border border-secondary bg-background text-[10px] font-semibold text-muted-foreground"
													>
														{userInitial(collab.user.username)}
													</span>
													<span class="truncate">{collab.user.username}</span>
												</span>
												{#if issueAssigneeID === collab.user_id}<Check class="h-3.5 w-3.5 text-muted-foreground" />{/if}
											</button>
										{/each}
									{/if}
								</div>
							</div>
						{/if}
					</div>

					<div class="relative">
						<button
							class="inline-flex items-center rounded-md border border-secondary bg-secondary/60 px-2.5 py-1 text-muted-foreground hover:bg-secondary"
							onclick={() => toggleIssueMetaMenu('labels')}
						>
							{selectedLabelsPreview()}
						</button>
						{#if issueMetaMenu === 'labels'}
							<div class="absolute left-0 top-8 z-50 w-72 overflow-hidden rounded-md border border-border bg-card shadow-lg">
								<div class="max-h-64 overflow-y-auto p-1">
									{#if issueMetaLoading}
										<div class="px-2 py-1.5 text-sm text-muted-foreground">Loading...</div>
									{:else if issueLabels.length === 0}
										<div class="px-2 py-1.5 text-sm text-muted-foreground">No labels</div>
									{:else}
										{#each issueLabels as label}
											<button
												class="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm text-foreground hover:bg-secondary"
												onclick={() => toggleIssueLabel(label.id)}
											>
												<span class="flex min-w-0 items-center gap-2">
													<span class="h-3 w-3 shrink-0 rounded-full" style={`background-color:#${label.color}`}></span>
													<span class="truncate">{label.name}</span>
												</span>
												{#if issueLabelIDs.includes(label.id)}<Check class="h-3.5 w-3.5 text-muted-foreground" />{/if}
											</button>
										{/each}
									{/if}
								</div>
							</div>
						{/if}
					</div>

					<span class="rounded-md border border-secondary bg-secondary/60 px-2.5 py-1 text-muted-foreground">
						@{project?.owner_org?.login ?? project?.owner_user?.username}'s {project?.title ?? 'project'}
					</span>

					<div class="relative">
						<button
							class="inline-flex items-center rounded-md border border-secondary bg-secondary/60 px-2.5 py-1 text-muted-foreground hover:bg-secondary"
							onclick={() => toggleIssueMetaMenu('milestone')}
						>
							{selectedMilestoneName()}
						</button>
						{#if issueMetaMenu === 'milestone'}
							<div class="absolute left-0 top-8 z-50 w-72 overflow-hidden rounded-md border border-border bg-card shadow-lg">
								<div class="max-h-64 overflow-y-auto p-1">
									<button
										class="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm text-foreground hover:bg-secondary"
										onclick={() => {
											issueMilestoneID = null;
											issueMetaMenu = null;
										}}
									>
										<span>No milestone</span>
										{#if issueMilestoneID === null}<Check class="h-3.5 w-3.5 text-muted-foreground" />{/if}
									</button>
									{#if issueMetaLoading}
										<div class="px-2 py-1.5 text-sm text-muted-foreground">Loading...</div>
									{:else if issueMilestones.length === 0}
										<div class="px-2 py-1.5 text-sm text-muted-foreground">No milestones</div>
									{:else}
										{#each issueMilestones as milestone}
											<button
												class="flex w-full items-center justify-between rounded px-2 py-1.5 text-left text-sm text-foreground hover:bg-secondary"
												onclick={() => {
													issueMilestoneID = milestone.id;
													issueMetaMenu = null;
												}}
											>
												<span class="truncate">{milestone.title}</span>
												{#if issueMilestoneID === milestone.id}<Check class="h-3.5 w-3.5 text-muted-foreground" />{/if}
											</button>
										{/each}
									{/if}
								</div>
							</div>
						{/if}
					</div>
				</div>
			</div>
		</div>

		<div class="flex items-center justify-between border-t border-secondary px-4 py-3">
			<div></div>
			<div class="flex items-center gap-2">
				<Button variant="outline" onclick={() => (showCreateIssueDialog = false)} disabled={creatingIssue}>Cancel</Button>
				<Button variant="brand" onclick={submitCreateIssue} disabled={creatingIssue || !issueTitle.trim() || !issueRepo}>
					{#if creatingIssue}
						Creating...
					{:else}
						<span class="inline-flex items-center gap-2">
							<span>Create</span>
							<span class="rounded-sm bg-black/15 px-1 text-[10px] leading-4 text-white/85">↵</span>
						</span>
					{/if}
				</Button>
			</div>
		</div>
	</Dialog.Content>
</Dialog.Root>
