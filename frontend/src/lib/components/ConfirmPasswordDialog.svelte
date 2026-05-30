<script lang="ts">
	import * as Dialog from '$lib/components/ui/dialog/index.js';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Input } from '$lib/components/ui/input/index.js';
	import { ShieldAlert } from '@lucide/svelte';

	interface Props {
		open: boolean;
		title?: string;
		description?: string;
		confirmLabel?: string;
		/** Called with the verified password so the action can forward it to the API. */
		onconfirm: (password: string) => void | Promise<void>;
		oncancel?: () => void;
	}

	let {
		open = $bindable(false),
		title = 'Confirm action',
		description = 'This action is irreversible. Please enter your password to confirm.',
		confirmLabel = 'Confirm',
		onconfirm,
		oncancel
	}: Props = $props();

	let password = $state('');
	let error = $state('');
	let loading = $state(false);

	function reset() {
		password = '';
		error = '';
		loading = false;
	}

	async function handleConfirm(e: Event) {
		e.preventDefault();
		if (!password) {
			error = 'Password is required.';
			return;
		}
		loading = true;
		error = '';
		try {
			// Pass the password to the action — the actual API endpoint verifies it server-side.
			await onconfirm(password);
			open = false;
			reset();
		} catch (err: any) {
			if (err.status === 403) {
				error = 'Incorrect password.';
			} else {
				error = err.message ?? 'Action failed.';
			}
		} finally {
			loading = false;
		}
	}

	function handleCancel() {
		open = false;
		reset();
		oncancel?.();
	}

	$effect(() => {
		if (!open) reset();
	});
</script>

<Dialog.Root bind:open>
	<Dialog.Content showCloseButton={false}>
		<Dialog.Header>
			<Dialog.Title class="flex items-center gap-2 text-red-400">
				<ShieldAlert class="h-4 w-4" />
				{title}
			</Dialog.Title>
			<Dialog.Description>
				{description}
			</Dialog.Description>
		</Dialog.Header>

		<form onsubmit={handleConfirm} class="flex flex-col gap-3">
			<Input type="password" placeholder="Your password" bind:value={password} autocomplete="current-password" disabled={loading} class={error ? 'border-red-600' : ''} />
			{#if error}
				<p class="text-xs text-red-400">{error}</p>
			{/if}
			<Dialog.Footer class="flex gap-2 justify-end pt-1">
				<Button type="button" variant="ghost" onclick={handleCancel} disabled={loading}>Cancel</Button>
				<Button type="submit" variant="destructive" disabled={loading || !password}>
					{loading ? 'Verifying…' : confirmLabel}
				</Button>
			</Dialog.Footer>
		</form>
	</Dialog.Content>
</Dialog.Root>
