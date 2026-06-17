<script lang="ts">
	import { page } from '$app/state';
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { webhooks } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { Label } from '$lib/components/ui/label/index.js';
	import { Checkbox } from '$lib/components/ui/checkbox/index.js';
	import { ArrowLeft } from '@lucide/svelte';

	const username = $derived(page.params.username as string);
	const repoName = $derived(page.params.repo as string);

	const ALL_EVENTS = [
		{ value: 'push', label: 'Push', description: 'Any Git push to a repository' },
		{ value: 'issues', label: 'Issues', description: 'Issue opened, closed, or reopened' },
		{ value: 'issue_comment', label: 'Issue comments', description: 'Issue comment created' },
		{ value: 'pull_request', label: 'Pull requests', description: 'PR opened, closed, or merged' },
		{
			value: 'pull_request_review',
			label: 'PR reviews',
			description: 'Pull request review submitted'
		},
		{ value: 'release', label: 'Releases', description: 'Release published or updated' },
		{ value: 'create', label: 'Branch/tag created', description: 'Branch or tag created' },
		{ value: 'delete', label: 'Branch/tag deleted', description: 'Branch or tag deleted' }
	];

	let payloadURL = $state('');
	let contentType = $state('application/json');
	let secret = $state('');
	let insecureSSL = $state(false);
	let active = $state(true);
	// event selection mode: 'push' | 'all' | 'select'
	let eventMode = $state<'push' | 'all' | 'select'>('push');
	let selectedEvents = $state<Set<string>>(new Set(['push']));

	let saving = $state(false);
	let error = $state('');

	onMount(() => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
		}
	});

	function toggleEvent(value: string) {
		const s = new Set(selectedEvents);
		if (s.has(value)) {
			s.delete(value);
		} else {
			s.add(value);
		}
		selectedEvents = s;
	}

	function resolvedEvents(): string[] {
		if (eventMode === 'push') return ['push'];
		if (eventMode === 'all') return ['*'];
		return [...selectedEvents];
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (!payloadURL.trim()) {
			error = 'Payload URL is required.';
			return;
		}
		saving = true;
		error = '';
		try {
			const hook = await webhooks.create(username, repoName, {
				payload_url: payloadURL.trim(),
				content_type: contentType,
				secret: secret || undefined,
				insecure_ssl: insecureSSL,
				active,
				events: resolvedEvents()
			});
			goto(`/${username}/${repoName}/settings/webhooks/${hook.webhook.id}`);
		} catch (e: any) {
			error = e.message;
		} finally {
			saving = false;
		}
	}
</script>

<div class="container mx-auto max-w-2xl px-4 py-8">
	<a href={`/${username}/${repoName}/settings/webhooks`} class="text-muted-foreground mb-6 flex items-center gap-1 text-sm hover:underline">
		<ArrowLeft class="h-4 w-4" />
		Webhooks
	</a>

	<h1 class="mb-6 text-2xl font-bold">Add webhook</h1>

	<p class="text-muted-foreground mb-8 text-sm">
		We'll send a <code class="bg-muted rounded px-1 py-0.5 text-xs">POST</code> request to the URL below with details of any subscribed events.
	</p>

	<form onsubmit={handleSubmit} class="space-y-6">
		<!-- Payload URL -->
		<div class="space-y-2">
			<Label for="payload-url">Payload URL <span class="text-destructive">*</span></Label>
			<Input id="payload-url" type="url" placeholder="https://example.com/postreceive" bind:value={payloadURL} required />
		</div>

		<!-- Content type -->
		<div class="space-y-2">
			<Label for="content-type">Content type <span class="text-destructive">*</span></Label>
			<select
				id="content-type"
				bind:value={contentType}
				class="border-input bg-background focus-visible:ring-ring w-full rounded-md border px-3 py-2 text-sm focus-visible:ring-2 focus-visible:outline-none"
			>
				<option value="application/json">application/json</option>
				<option value="application/x-www-form-urlencoded">application/x-www-form-urlencoded</option>
			</select>
		</div>

		<!-- Secret -->
		<div class="space-y-2">
			<Label for="secret">Secret</Label>
			<Input id="secret" type="password" placeholder="Optional — used to sign payloads (X-Hub-Signature-256)" bind:value={secret} />
			<p class="text-muted-foreground text-xs">
				The secret is used to compute an HMAC hex digest of the payload. You can verify the signature in your endpoint using the <code class="bg-muted rounded px-1 py-0.5"
					>X-Hub-Signature-256</code
				> header.
			</p>
		</div>

		<!-- SSL verification -->
		<div class="space-y-3">
			<Label>SSL verification</Label>
			<p class="text-muted-foreground text-xs">By default, we verify SSL certificates when delivering payloads.</p>
			<div class="space-y-2">
				<label class="flex cursor-pointer items-center gap-2">
					<input type="radio" name="ssl" value={false} bind:group={insecureSSL} class="accent-primary" />
					<span class="text-sm font-medium">Enable SSL verification</span>
				</label>
				<label class="flex cursor-pointer items-center gap-2">
					<input type="radio" name="ssl" value={true} bind:group={insecureSSL} class="accent-primary" />
					<span class="text-sm font-medium text-yellow-600"> Disable (not recommended) </span>
				</label>
			</div>
		</div>

		<!-- Which events -->
		<div class="space-y-3">
			<Label>Which events would you like to trigger this webhook?</Label>
			<div class="space-y-2">
				<label class="flex cursor-pointer items-center gap-2">
					<input type="radio" name="events" value="push" bind:group={eventMode} class="accent-primary" />
					<span class="text-sm">Just the <strong>push</strong> event.</span>
				</label>
				<label class="flex cursor-pointer items-center gap-2">
					<input type="radio" name="events" value="all" bind:group={eventMode} class="accent-primary" />
					<span class="text-sm">Send me <strong>everything</strong>.</span>
				</label>
				<label class="flex cursor-pointer items-center gap-2">
					<input type="radio" name="events" value="select" bind:group={eventMode} class="accent-primary" />
					<span class="text-sm">Let me <strong>select individual events</strong>.</span>
				</label>
			</div>

			{#if eventMode === 'select'}
				<div class="bg-muted/40 mt-3 grid grid-cols-1 gap-2 rounded-lg border p-4 sm:grid-cols-2">
					{#each ALL_EVENTS as evt}
						<label class="flex cursor-pointer items-start gap-2">
							<Checkbox checked={selectedEvents.has(evt.value)} onCheckedChange={() => toggleEvent(evt.value)} class="mt-0.5" />
							<div>
								<p class="text-sm font-medium">{evt.label}</p>
								<p class="text-muted-foreground text-xs">{evt.description}</p>
							</div>
						</label>
					{/each}
				</div>
			{/if}
		</div>

		<!-- Active -->
		<div class="flex items-start gap-3">
			<Checkbox id="active" bind:checked={active} class="mt-0.5" />
			<div>
				<label for="active" class="cursor-pointer text-sm font-medium">Active</label>
				<p class="text-muted-foreground text-xs">We will deliver event details when this hook is triggered.</p>
			</div>
		</div>

		{#if error}
			<p class="text-destructive text-sm">{error}</p>
		{/if}

		<Button type="submit" disabled={saving}>
			{saving ? 'Adding webhook…' : 'Add webhook'}
		</Button>
	</form>
</div>
