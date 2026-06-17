<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { getContext } from 'svelte';
	import { projects, type Organization, type Project } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Switch } from '$lib/components/ui/switch/index.js';

	const projectID = $derived(page.params.projectID);
	const username = $derived(page.params.username);

	const orgCtx = getContext<{
		isOrg: boolean;
		org: Organization | null;
		isOwner: boolean;
		loading: boolean;
	}>('org');

	let loading = $state(true);
	let saving = $state(false);
	let error = $state('');
	let success = $state('');

	let project = $state<Project | null>(null);
	let title = $state('');
	let isPublic = $state(true);

	const canManage = $derived.by(() => {
		if (!project || !authStore.user) return false;
		if (project.owner_user?.username) return project.owner_user.username === authStore.user.username;
		if (project.owner_org?.login) return orgCtx.isOrg && orgCtx.isOwner && orgCtx.org?.login === project.owner_org.login;
		return false;
	});

	$effect(() => {
		if (!projectID) return;
		loadProject(projectID);
	});

	async function loadProject(pid: string) {
		loading = true;
		error = '';
		try {
			const data = await projects.get(pid);
			if (projectID !== pid) return;
			project = data.project;
			title = data.project.title;
			isPublic = data.project.is_public;
		} catch (e: any) {
			if (projectID !== pid) return;
			error = e?.message ?? 'Failed to load project';
			project = null;
		} finally {
			if (projectID === pid) loading = false;
		}
	}

	async function saveProject() {
		if (!project || !canManage) return;
		saving = true;
		error = '';
		success = '';
		try {
			const res = await projects.update(project.id, {
				title: title.trim(),
				is_public: isPublic
			});
			project = res.project;
			title = res.project.title;
			isPublic = res.project.is_public;
			success = 'Project settings saved.';
		} catch (e: any) {
			error = e?.message ?? 'Failed to save project settings';
		} finally {
			saving = false;
		}
	}
</script>

<svelte:head>
	<title>{project?.title ?? 'Project'} · Settings · GitPier</title>
</svelte:head>

<div class="mx-auto w-full max-w-2xl space-y-5 px-4 py-6">
	<div class="flex items-center justify-between">
		<div>
			<h1 class="text-xl font-semibold text-foreground">Project settings</h1>
			<p class="text-sm text-muted-foreground">Update project name and visibility.</p>
		</div>
		<Button variant="outline" onclick={() => goto(`/${username}/projects/${projectID}`)}>Back to board</Button>
	</div>

	{#if loading}
		<div class="rounded-md border border-border bg-card p-4 text-sm text-muted-foreground">Loading...</div>
	{:else if error && !project}
		<div class="rounded-md border border-destructive/40 bg-destructive/10 p-4 text-sm text-destructive">{error}</div>
	{:else if project}
		<div class="space-y-4 rounded-md border border-border bg-card p-4">
			{#if !canManage}
				<div class="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">You do not have permission to edit this project.</div>
			{/if}

			<div>
				<label class="mb-1 block text-sm font-semibold text-foreground" for="project-name">Project name</label>
				<input
					id="project-name"
					bind:value={title}
					disabled={!canManage || saving}
					class="h-10 w-full rounded-md border border-input bg-background px-3 text-sm text-foreground outline-none focus:ring-1 focus:ring-ring disabled:opacity-70"
				/>
			</div>

			<div class="flex items-center justify-between rounded-md border border-input bg-background px-3 py-2">
				<div>
					<p class="text-sm font-semibold text-foreground">Visibility</p>
					<p class="text-xs text-muted-foreground">{isPublic ? 'Public project' : 'Private project'}</p>
				</div>
				<Switch bind:checked={isPublic} disabled={!canManage || saving} />
			</div>

			{#if error}
				<div class="rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">{error}</div>
			{/if}
			{#if success}
				<div class="rounded-md border border-emerald-600/40 bg-emerald-600/10 p-3 text-sm text-emerald-500">{success}</div>
			{/if}

			<div class="flex justify-end">
				<Button variant="brand" onclick={saveProject} disabled={!canManage || saving || !title.trim()}>
					{saving ? 'Saving...' : 'Save changes'}
				</Button>
			</div>
		</div>
	{/if}
</div>
