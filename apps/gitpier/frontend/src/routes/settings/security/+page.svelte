<script lang="ts">
	import { onMount } from 'svelte';
	import { goto } from '$app/navigation';
	import { authStore } from '$lib/stores/auth.svelte';
	import { auth } from '$lib/api/client';
	import QRCode from 'qrcode';
	import { Button } from '$lib/components/ui/button/index.js';
	import { Copy, Download, Loader } from '@lucide/svelte';

	let statusLoading = $state(true);
	let enabled = $state(false);
	let hasPendingSetup = $state(false);

	// Change password
	let changeCurrentPassword = $state('');
	let changeNewPassword = $state('');
	let changeConfirmPassword = $state('');
	let changeBusy = $state(false);
	let changeError = $state('');
	let changeSuccess = $state('');

	let setupPassword = $state('');
	let setupSecret = $state('');
	let setupOtpAuthURL = $state('');
	let setupQrDataUrl = $state('');
	let setupCode = $state('');

	let disablePassword = $state('');
	let disableCode = $state('');
	let disableRecoveryCode = $state('');
	let disableUseRecovery = $state(false);

	let regenPassword = $state('');
	let regenCode = $state('');
	let regenRecoveryCode = $state('');
	let regenUseRecovery = $state(false);

	let recoveryCodes = $state<string[]>([]);
	let busy = $state(false);
	let qrLoading = $state(false);
	let error = $state('');
	let success = $state('');

	function recoveryCodesText() {
		return recoveryCodes.join('\n');
	}

	async function copyRecoveryCodes() {
		if (recoveryCodes.length === 0) return;
		error = '';
		try {
			await navigator.clipboard.writeText(recoveryCodesText());
			success = 'Recovery codes copied to clipboard.';
		} catch {
			error = 'Could not copy recovery codes. Please copy them manually.';
		}
	}

	function downloadRecoveryCodes() {
		if (recoveryCodes.length === 0) return;
		error = '';
		const content = `GitPier recovery codes\nGenerated: ${new Date().toISOString()}\n\n${recoveryCodesText()}\n`;
		const blob = new Blob([content], { type: 'text/plain;charset=utf-8' });
		const url = URL.createObjectURL(blob);
		const a = document.createElement('a');
		a.href = url;
		a.download = 'GitPier-recovery-codes.txt';
		a.click();
		URL.revokeObjectURL(url);
		success = 'Recovery codes downloaded.';
	}

	async function generateSetupQr(otpAuthUrl: string) {
		if (!otpAuthUrl) {
			setupQrDataUrl = '';
			return;
		}
		qrLoading = true;
		try {
			setupQrDataUrl = await QRCode.toDataURL(otpAuthUrl, {
				margin: 1,
				width: 220
			});
		} catch {
			setupQrDataUrl = '';
		} finally {
			qrLoading = false;
		}
	}

	onMount(async () => {
		if (!authStore.isAuthenticated && !authStore.loading) {
			goto('/login');
			return;
		}
		await refreshStatus();
	});

	async function refreshStatus() {
		statusLoading = true;
		try {
			const status = await auth.twoFactor.status();
			enabled = status.enabled;
			hasPendingSetup = status.has_pending_setup;
		} catch (e: any) {
			error = e.message ?? 'Failed to load two-factor status.';
		} finally {
			statusLoading = false;
		}
	}

	async function handleChangePassword() {
		changeError = '';
		changeSuccess = '';
		if (!changeCurrentPassword || !changeNewPassword || !changeConfirmPassword) {
			changeError = 'All fields are required.';
			return;
		}
		if (changeNewPassword.length < 8) {
			changeError = 'New password must be at least 8 characters.';
			return;
		}
		if (changeNewPassword !== changeConfirmPassword) {
			changeError = 'New passwords do not match.';
			return;
		}
		if (changeCurrentPassword === changeNewPassword) {
			changeError = 'New password must differ from the current password.';
			return;
		}
		changeBusy = true;
		try {
			await auth.changePassword(changeCurrentPassword, changeNewPassword);
			changeCurrentPassword = '';
			changeNewPassword = '';
			changeConfirmPassword = '';
			changeSuccess = 'Password updated successfully. All other sessions have been signed out.';
		} catch (e: any) {
			changeError = e.message ?? 'Failed to change password.';
		} finally {
			changeBusy = false;
		}
	}

	async function startSetup() {
		if (!setupPassword || busy) return;
		busy = true;
		error = '';
		success = '';
		recoveryCodes = [];
		try {
			const setup = await auth.twoFactor.setup(setupPassword);
			setupSecret = setup.secret;
			setupOtpAuthURL = setup.otpauth_url;
			await generateSetupQr(setup.otpauth_url);
			hasPendingSetup = true;
			success = 'Setup secret generated. Add it to your authenticator app, then verify with a 6-digit code.';
		} catch (e: any) {
			error = e.message ?? 'Failed to start setup.';
		} finally {
			busy = false;
		}
	}

	async function enableTwoFactor() {
		if (!setupCode || busy) return;
		busy = true;
		error = '';
		success = '';
		try {
			const result = await auth.twoFactor.enable(setupCode);
			recoveryCodes = result.recovery_codes;
			enabled = true;
			hasPendingSetup = false;
			setupCode = '';
			setupPassword = '';
			success = 'Two-factor authentication is now enabled.';
			await refreshStatus();
		} catch (e: any) {
			error = e.message ?? 'Failed to enable two-factor authentication.';
		} finally {
			busy = false;
		}
	}

	async function disableTwoFactor() {
		if (!disablePassword || (!disableUseRecovery && !disableCode) || (disableUseRecovery && !disableRecoveryCode) || busy) return;
		busy = true;
		error = '';
		success = '';
		try {
			await auth.twoFactor.disable(disablePassword, disableUseRecovery ? undefined : disableCode, disableUseRecovery ? disableRecoveryCode : undefined);
			enabled = false;
			hasPendingSetup = false;
			setupSecret = '';
			setupOtpAuthURL = '';
			setupQrDataUrl = '';
			recoveryCodes = [];
			disablePassword = '';
			disableCode = '';
			disableRecoveryCode = '';
			success = 'Two-factor authentication has been disabled.';
			await refreshStatus();
		} catch (e: any) {
			error = e.message ?? 'Failed to disable two-factor authentication.';
		} finally {
			busy = false;
		}
	}

	async function regenerateRecoveryCodes() {
		if (!regenPassword || (!regenUseRecovery && !regenCode) || (regenUseRecovery && !regenRecoveryCode) || busy) return;
		busy = true;
		error = '';
		success = '';
		try {
			const result = await auth.twoFactor.regenerateRecoveryCodes(regenPassword, regenUseRecovery ? undefined : regenCode, regenUseRecovery ? regenRecoveryCode : undefined);
			recoveryCodes = result.recovery_codes;
			regenPassword = '';
			regenCode = '';
			regenRecoveryCode = '';
			success = 'Recovery codes regenerated. Save these new codes now; old ones no longer work.';
		} catch (e: any) {
			error = e.message ?? 'Failed to regenerate recovery codes.';
		} finally {
			busy = false;
		}
	}
</script>

<svelte:head>
	<title>Password and authentication</title>
</svelte:head>

<div class="max-w-2xl space-y-6">
	<h1 class="text-2xl font-semibold text-foreground">Password and authentication</h1>

	{#if error}
		<div class="rounded-md border border-red-800/50 bg-red-900/30 px-4 py-3 text-sm text-red-400">{error}</div>
	{/if}
	{#if success}
		<div class="rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">{success}</div>
	{/if}

	<!-- Change password -->
	<section class="rounded-md border border-border bg-card px-5 py-4 space-y-4">
		<div>
			<h2 class="text-sm font-semibold text-foreground">Change password</h2>
			<p class="text-xs text-muted-foreground mt-1">Changing your password will sign out all other active sessions.</p>
		</div>

		{#if changeError}
			<div class="rounded-md border border-red-800/50 bg-red-900/30 px-4 py-3 text-sm text-red-400">{changeError}</div>
		{/if}
		{#if changeSuccess}
			<div class="rounded-md border border-brand/40 bg-brand/10 px-4 py-3 text-sm text-[#3fb950]">{changeSuccess}</div>
		{/if}

		<div class="space-y-3">
			<div>
				<label for="current-password" class="block text-xs font-semibold text-foreground mb-1">Current password</label>
				<input
					id="current-password"
					type="password"
					bind:value={changeCurrentPassword}
					autocomplete="current-password"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>
			<div>
				<label for="new-password" class="block text-xs font-semibold text-foreground mb-1">New password</label>
				<input
					id="new-password"
					type="password"
					bind:value={changeNewPassword}
					autocomplete="new-password"
					placeholder="At least 8 characters"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>
			<div>
				<label for="confirm-password" class="block text-xs font-semibold text-foreground mb-1">Confirm new password</label>
				<input
					id="confirm-password"
					type="password"
					bind:value={changeConfirmPassword}
					autocomplete="new-password"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
			</div>
			<Button variant="brand" onclick={handleChangePassword} disabled={changeBusy || !changeCurrentPassword || !changeNewPassword || !changeConfirmPassword}>
				{#if changeBusy}<Loader class="h-4 w-4 animate-spin" />{/if}
				Update password
			</Button>
		</div>
	</section>

	<section class="rounded-md border border-border bg-card px-5 py-4 space-y-4">
		<div class="flex items-center justify-between gap-4">
			<div>
				<h2 class="text-sm font-semibold text-foreground">Two-factor authentication</h2>
				<p class="text-xs text-muted-foreground mt-1">Protect your account with a one-time code from Google Authenticator, Authy, Microsoft Authenticator, 1Password, or similar apps.</p>
			</div>
			{#if statusLoading}
				<span class="text-xs text-muted-foreground inline-flex items-center gap-1"><Loader class="h-3 w-3 animate-spin" />Checking...</span>
			{:else if enabled}
				<span class="text-xs font-semibold rounded-full border border-[#3fb950]/40 bg-[#3fb950]/10 text-[#3fb950] px-2 py-0.5">Enabled</span>
			{:else}
				<span class="text-xs font-semibold rounded-full border border-border bg-secondary text-muted-foreground px-2 py-0.5">Disabled</span>
			{/if}
		</div>

		{#if !enabled}
			<div class="space-y-3">
				<p class="text-xs text-muted-foreground">Confirm your password to start setup.</p>
				<input
					type="password"
					bind:value={setupPassword}
					placeholder="Current password"
					class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
				<Button variant="brand" onclick={startSetup} disabled={busy || !setupPassword}>Start 2FA setup</Button>
			</div>

			{#if setupSecret}
				<div class="rounded-md border border-border bg-background p-4 space-y-3">
					<p class="text-sm font-semibold text-foreground">Step 2: Add this key to your authenticator app</p>
					{#if qrLoading}
						<div class="rounded-md border border-border bg-card p-5 flex items-center justify-center text-xs text-muted-foreground">
							<Loader class="h-3 w-3 animate-spin mr-1" />Generating QR code...
						</div>
					{:else if setupQrDataUrl}
						<div class="rounded-md border border-border bg-card p-4 flex flex-col items-center gap-2">
							<img src={setupQrDataUrl} alt="Scan this QR code with your authenticator app" class="h-52 w-52 max-w-full rounded-md bg-white p-2" />
							<p class="text-xs text-muted-foreground text-center">Scan this code in your authenticator app for instant setup.</p>
						</div>
					{/if}
					<div>
						<p class="text-xs text-muted-foreground mb-1">Manual setup key</p>
						<div class="font-mono text-xs break-all rounded border border-border bg-card px-3 py-2 text-foreground">
							{setupSecret || 'Setup key is hidden. Restart setup to generate a new key.'}
						</div>
					</div>
					{#if setupOtpAuthURL}
						<div>
							<p class="text-xs text-muted-foreground mb-1">OTP URI</p>
							<div class="font-mono text-[11px] break-all rounded border border-border bg-card px-3 py-2 text-muted-foreground">{setupOtpAuthURL}</div>
						</div>
					{/if}
					<div>
						<label class="block text-xs font-semibold text-foreground mb-1">Step 3: Verify with a 6-digit code</label>
						<input
							type="text"
							inputmode="numeric"
							maxlength={6}
							bind:value={setupCode}
							placeholder="123456"
							class="h-9 w-full rounded-md border border-border bg-background px-3 text-sm tracking-[0.2em] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
						/>
					</div>
					<Button variant="brand" onclick={enableTwoFactor} disabled={busy || !setupCode}>Enable 2FA</Button>
				</div>
			{/if}
		{:else}
			<div class="rounded-md border border-border bg-background p-4 space-y-3">
				<p class="text-sm font-semibold text-foreground">Disable 2FA</p>
				<p class="text-xs text-muted-foreground">Enter your password and either a current authenticator code or a recovery code.</p>
				<input
					type="password"
					bind:value={disablePassword}
					placeholder="Current password"
					class="h-9 w-full rounded-md border border-border bg-card px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
				{#if !disableUseRecovery}
					<input
						type="text"
						inputmode="numeric"
						maxlength={6}
						bind:value={disableCode}
						placeholder="Authenticator code"
						class="h-9 w-full rounded-md border border-border bg-card px-3 text-sm tracking-[0.2em] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				{:else}
					<input
						type="text"
						bind:value={disableRecoveryCode}
						placeholder="Recovery code (XXXX-XXXX)"
						class="h-9 w-full rounded-md border border-border bg-card px-3 text-sm uppercase tracking-[0.15em] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				{/if}
				<div class="flex flex-wrap gap-2">
					<Button variant="outline" onclick={() => (disableUseRecovery = !disableUseRecovery)}>{disableUseRecovery ? 'Use authenticator code' : 'Use recovery code'}</Button>
					<Button
						variant="destructive"
						onclick={disableTwoFactor}
						disabled={busy || !disablePassword || (!disableUseRecovery && !disableCode) || (disableUseRecovery && !disableRecoveryCode)}
					>
						Disable 2FA
					</Button>
				</div>
			</div>

			<div class="rounded-md border border-border bg-background p-4 space-y-3">
				<p class="text-sm font-semibold text-foreground">Regenerate recovery codes</p>
				<p class="text-xs text-muted-foreground">Generating new codes invalidates all previous recovery codes.</p>
				<input
					type="password"
					bind:value={regenPassword}
					placeholder="Current password"
					class="h-9 w-full rounded-md border border-border bg-card px-3 text-sm text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
				/>
				{#if !regenUseRecovery}
					<input
						type="text"
						inputmode="numeric"
						maxlength={6}
						bind:value={regenCode}
						placeholder="Authenticator code"
						class="h-9 w-full rounded-md border border-border bg-card px-3 text-sm tracking-[0.2em] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				{:else}
					<input
						type="text"
						bind:value={regenRecoveryCode}
						placeholder="Recovery code (XXXX-XXXX)"
						class="h-9 w-full rounded-md border border-border bg-card px-3 text-sm uppercase tracking-[0.15em] text-foreground placeholder:text-muted-foreground focus:outline-none focus:ring-1 focus:ring-primary focus:border-primary"
					/>
				{/if}
				<div class="flex flex-wrap gap-2">
					<Button variant="outline" onclick={() => (regenUseRecovery = !regenUseRecovery)}>{regenUseRecovery ? 'Use authenticator code' : 'Use recovery code'}</Button>
					<Button variant="outline" onclick={regenerateRecoveryCodes} disabled={busy || !regenPassword || (!regenUseRecovery && !regenCode) || (regenUseRecovery && !regenRecoveryCode)}>
						Regenerate codes
					</Button>
				</div>
			</div>
		{/if}
	</section>

	{#if recoveryCodes.length > 0}
		<section class="rounded-md border border-amber-600/40 bg-amber-900/10 px-5 py-4 space-y-3">
			<h2 class="text-sm font-semibold text-amber-300">Recovery codes</h2>
			<p class="text-xs text-muted-foreground">Store these in a safe place. Each code can only be used once.</p>
			<div class="flex flex-wrap gap-2">
				<Button variant="outline" onclick={copyRecoveryCodes}>
					<Copy class="h-3.5 w-3.5" />
					Copy all codes
				</Button>
				<Button variant="outline" onclick={downloadRecoveryCodes}>
					<Download class="h-3.5 w-3.5" />
					Download .txt
				</Button>
			</div>
			<div class="grid sm:grid-cols-2 gap-2">
				{#each recoveryCodes as code}
					<div class="rounded border border-border bg-card px-3 py-2 font-mono text-xs tracking-[0.12em] text-foreground">{code}</div>
				{/each}
			</div>
		</section>
	{/if}
</div>
