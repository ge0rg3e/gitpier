<script context="module">
	// Type declarations for Turnstile
	declare global {
		interface Window {
			turnstile?: {
				render: (
					container: HTMLElement | string,
					options: {
						sitekey: string;
						theme?: 'light' | 'dark';
						callback?: (token: string) => void;
					}
				) => string;
				reset: (container?: HTMLElement | string) => void;
				remove: (container?: HTMLElement | string) => void;
				getResponse: (container?: HTMLElement | string) => string | undefined;
			};
		}
	}
</script>

<script lang="ts">
	import { goto } from '$app/navigation';
	import { page } from '$app/state';
	import { authStore } from '$lib/stores/auth.svelte';
	import { auth } from '$lib/api/client';
	import { getPublicRuntimeConfig } from '$lib/runtime-config';
	import { Eye, EyeOff, Loader } from '@lucide/svelte';
	import { Button } from '$lib/components/ui/button/index.js';
	import { onMount } from 'svelte';

	let username = $state('');
	let email = $state('');
	let password = $state('');
	let showPassword = $state(false);
	let loading = $state(false);
	let error = $state('');
	let info = $state('');
	let turnstileToken = $state('');
	let registrationToken = $state('');
	let otpCode = $state('');
	let otpRequested = $state(false);
	let otpExpiresInSeconds = $state(0);
	let turnstileContainerRef = $state<HTMLDivElement | null>(null);
	let turnstileReady = $state(false);
	let turnstileSiteKey = getPublicRuntimeConfig().turnstileSiteKey;
	let usernameAvailable = $state<boolean | null>(null);
	let checkingUsernameAvailability = $state(false);
	let usernameAvailabilityMessage = $state('');
	let lastCheckedUsername = $state('');
	let usernameCheckRequestId = 0;

	function getAuthenticatedRedirectTarget() {
		const redirectParam = page.url.searchParams.get('redirect')?.trim() ?? '';
		const returnToParam = page.url.searchParams.get('return_to')?.trim() ?? '';
		const candidate = redirectParam || returnToParam || '/';

		try {
			const parsed = new URL(candidate, page.url.origin);
			if (parsed.origin !== page.url.origin) return '/';
			if (parsed.pathname === '/login' || parsed.pathname === '/signup') return '/';
			return `${parsed.pathname}${parsed.search}${parsed.hash}` || '/';
		} catch {
			return '/';
		}
	}

	function initTurnstile() {
		if (!turnstileContainerRef || !window.turnstile || !turnstileSiteKey) return;
		window.turnstile.render(turnstileContainerRef, {
			sitekey: turnstileSiteKey,
			theme: document.documentElement.getAttribute('data-theme') === 'dark' ? 'dark' : 'light',
			callback: (token: string) => {
				turnstileToken = token;
			}
		});
		turnstileReady = true;
	}

	const usernameValid = $derived(username.length >= 1 && username.length <= 39 && /^[a-zA-Z0-9][a-zA-Z0-9\-]*[a-zA-Z0-9]?$/.test(username));
	const emailValid = $derived(/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email.trim()));
	const passwordValid = $derived(password.length >= 8);
	const otpCodeValid = $derived(/^[0-9]{6}$/.test(otpCode.trim()));
	const canRequestOtp = $derived(
		usernameValid &&
			usernameAvailable === true &&
			!checkingUsernameAvailability &&
			emailValid &&
			passwordValid &&
			(!turnstileSiteKey || !!turnstileToken)
	);
	const canVerifyOtp = $derived(otpCodeValid);

	function normalizeOtp(value: string) {
		return value.replace(/\D/g, '').slice(0, 6);
	}

	onMount(() => {
		// Load Turnstile script
		if (!window.turnstile && turnstileSiteKey) {
			const script = document.createElement('script');
			script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js';
			script.async = true;
			script.defer = true;
			script.onload = initTurnstile;
			document.head.appendChild(script);
		} else if (turnstileSiteKey) {
			initTurnstile();
		}

		return () => {
			// Cleanup: reset Turnstile when component unmounts
			if (turnstileContainerRef && window.turnstile) {
				try {
					window.turnstile.reset(turnstileContainerRef);
				} catch (e) {
					// Ignore errors during cleanup
				}
			}
		};
	});

	async function requestOTP() {
		if (!usernameValid) {
			error = 'Username can only contain alphanumeric characters and hyphens.';
			return;
		}
		if (checkingUsernameAvailability) {
			error = 'Please wait while we check username availability.';
			return;
		}
		if (usernameAvailable !== true) {
			error = 'That username is not available. Please choose another one.';
			return;
		}
		if (!emailValid) {
			error = 'Please enter a valid email address.';
			return;
		}
		if (!passwordValid) {
			error = 'Password must be at least 8 characters.';
			return;
		}
		if (turnstileSiteKey && !turnstileToken) {
			error = 'Please complete the CAPTCHA verification.';
			return;
		}

		error = '';
		info = '';
		try {
			const result = await auth.requestRegisterOTP(username.trim(), email.trim(), password, true, turnstileToken);
			registrationToken = result.registration_token;
			otpRequested = true;
			otpCode = '';
			otpExpiresInSeconds = result.expires_in_seconds;
			info = 'We sent a 6-digit verification code to your email. Enter it below to finish creating your account.';

			if (turnstileContainerRef && window.turnstile) {
				window.turnstile.reset(turnstileContainerRef);
				turnstileToken = '';
			}
		} catch (e: any) {
			error = e.message ?? 'Registration failed. Please try again.';
			// Reset Turnstile on error so user can try again
			if (turnstileContainerRef && window.turnstile) {
				window.turnstile.reset(turnstileContainerRef);
				turnstileToken = '';
			}
		}
	}

	async function handleSubmit(e: Event) {
		e.preventDefault();
		if (loading) return;

		loading = true;
		if (!otpRequested) {
			if (!canRequestOtp) {
				error = 'Please fill in all required fields correctly.';
				loading = false;
				return;
			}
			await requestOTP();
			loading = false;
			return;
		}

		if (!registrationToken) {
			error = 'Registration session expired. Please request a new code.';
			loading = false;
			return;
		}
		if (!canVerifyOtp) {
			error = 'Please enter a valid 6-digit OTP code.';
			loading = false;
			return;
		}

		error = '';
		info = '';
		try {
			const { token, user } = await auth.verifyRegisterOTP(registrationToken, otpCode.trim());
			authStore.setAuth(token, user);
			goto('/');
		} catch (e: any) {
			error = e.message ?? 'OTP verification failed. Please try again.';
		} finally {
			loading = false;
		}
	}

	// Keep a fallback observer for token updates for environments where callback timing is unreliable.
	$effect(() => {
		if (!authStore.loading && authStore.isAuthenticated) {
			goto(getAuthenticatedRedirectTarget(), { replaceState: true });
			return;
		}
	});

	$effect(() => {
		const normalized = normalizeOtp(otpCode);
		if (normalized !== otpCode) {
			otpCode = normalized;
		}
	});

	$effect(() => {
		if (otpRequested) return;

		const rawUsername = username.trim();
		if (!rawUsername) {
			checkingUsernameAvailability = false;
			usernameAvailable = null;
			usernameAvailabilityMessage = '';
			lastCheckedUsername = '';
			return;
		}
		if (!usernameValid) {
			checkingUsernameAvailability = false;
			usernameAvailable = null;
			usernameAvailabilityMessage = '';
			lastCheckedUsername = '';
			return;
		}
		if (rawUsername === lastCheckedUsername) return;

		checkingUsernameAvailability = true;
		usernameAvailabilityMessage = '';
		const requestId = ++usernameCheckRequestId;
		const timeout = setTimeout(async () => {
			try {
				const result = await auth.checkUsernameAvailability(rawUsername);
				if (requestId !== usernameCheckRequestId) return;
				usernameAvailable = result.available;
				usernameAvailabilityMessage = result.available ? 'Username is available.' : 'This username is already taken.';
				lastCheckedUsername = rawUsername;
			} catch (e: any) {
				if (requestId !== usernameCheckRequestId) return;
				usernameAvailable = null;
				usernameAvailabilityMessage = 'Could not verify username availability. Please try again.';
				lastCheckedUsername = '';
			} finally {
				if (requestId === usernameCheckRequestId) {
					checkingUsernameAvailability = false;
				}
			}
		}, 900);

		return () => {
			clearTimeout(timeout);
		};
	});

	$effect(() => {
		if (turnstileSiteKey && typeof window !== 'undefined' && window.turnstile && turnstileContainerRef) {
			const container = turnstileContainerRef;
			const observer = new MutationObserver(() => {
				if (window.turnstile) {
					const token = window.turnstile.getResponse(container);
					if (token) {
						turnstileToken = token;
					}
				}
			});

			observer.observe(turnstileContainerRef, { attributes: true, subtree: true });

			return () => {
				observer.disconnect();
			};
		}
	});
</script>

<svelte:head>
	<title>Join GitPier</title>
</svelte:head>

<div class="auth-register-page">
	<div class="grid min-h-[95vh] w-full lg:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)]">
		<section class="flex items-center justify-center p-6 sm:p-8 lg:p-10">
			<div class="w-full max-w-md">
				<a href="/" class="mb-8 inline-flex">
					<img src="/images/logo.png" alt="GitPier" class="h-10 w-10 object-contain" />
				</a>

				<h1 class="text-3xl font-semibold tracking-tight text-foreground">Create an account</h1>
				<p class="mt-2 text-sm text-muted-foreground">
					{#if otpRequested}
						Enter the verification code we sent to your email to finish creating your account.
					{:else}
						Let&apos;s get started with your GitPier account.
					{/if}
				</p>

				{#if error}
					<div class="mt-6 rounded-xl border border-red-800/50 bg-red-900/30 px-4 py-3 text-sm text-red-400">{error}</div>
				{/if}
				{#if info}
					<div class="mt-6 rounded-xl border border-emerald-800/50 bg-emerald-900/30 px-4 py-3 text-sm text-emerald-300">{info}</div>
				{/if}

				<form onsubmit={handleSubmit} class="mt-6 space-y-4" novalidate>
					<div>
						<label for="username" class="mb-1.5 block text-sm font-semibold text-foreground">Username</label>
						<input
							id="username"
							type="text"
							bind:value={username}
							required
							disabled={otpRequested}
							autofocus
							class="h-10 w-full rounded-xl border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
							class:border-emerald-500={username && usernameValid && usernameAvailable === true}
							class:border-red-500={username && (!usernameValid || usernameAvailable === false)}
							class:border-border={!username}
						/>
						{#if username && !usernameValid}
							<p class="mt-1 text-xs text-red-400">Username may only contain alphanumeric characters or single hyphens.</p>
						{:else if username && checkingUsernameAvailability}
							<p class="mt-1 text-xs text-muted-foreground">Checking username availability...</p>
						{:else if username && usernameAvailabilityMessage}
							<p class="mt-1 text-xs" class:text-emerald-400={usernameAvailable === true} class:text-red-400={usernameAvailable === false} class:text-muted-foreground={usernameAvailable === null}>
								{usernameAvailabilityMessage}
							</p>
						{/if}
					</div>

					<div>
						<label for="email" class="mb-1.5 block text-sm font-semibold text-foreground">Email address</label>
						<input
							id="email"
							type="email"
							bind:value={email}
							required
							disabled={otpRequested}
							class="h-10 w-full rounded-xl border border-border bg-background px-3 text-sm text-foreground placeholder:text-muted-foreground transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
						/>
					</div>

					<div>
						<label for="password" class="mb-1.5 block text-sm font-semibold text-foreground">Password</label>
						<div class="relative">
							<input
								id="password"
								type={showPassword ? 'text' : 'password'}
								bind:value={password}
								required
								disabled={otpRequested}
								class="h-10 w-full rounded-xl border border-border bg-background px-3 pr-10 text-sm text-foreground placeholder:text-muted-foreground transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
							/>
							<button
								type="button"
								onclick={() => (showPassword = !showPassword)}
								class="absolute right-3 top-1/2 -translate-y-1/2 text-muted-foreground transition-colors hover:text-foreground"
							>
								{#if showPassword}<EyeOff class="h-4 w-4" />{:else}<Eye class="h-4 w-4" />{/if}
							</button>
						</div>
						<p class="mt-1 text-xs text-muted-foreground">At least 8 characters.</p>
					</div>

					{#if turnstileSiteKey && !otpRequested}
						<div class="flex justify-center py-1">
							<div bind:this={turnstileContainerRef} class="cf-turnstile" data-sitekey={turnstileSiteKey}></div>
						</div>
					{/if}

					{#if otpRequested}
						<div>
							<label for="otp_code" class="mb-1.5 block text-sm font-semibold text-foreground">Email OTP code</label>
							<input
								id="otp_code"
								type="text"
								inputmode="numeric"
								maxlength="6"
								bind:value={otpCode}
								oninput={(e) => (otpCode = normalizeOtp((e.currentTarget as HTMLInputElement).value))}
								required
								class="h-10 w-full rounded-xl border border-border bg-background px-3 text-sm tracking-[0.25em] text-foreground placeholder:text-muted-foreground transition-all focus:border-primary focus:outline-none focus:ring-1 focus:ring-primary"
								placeholder="123456"
							/>
							<p class="mt-1 text-xs text-muted-foreground">Code expires in about {Math.max(1, Math.ceil(otpExpiresInSeconds / 60))} minutes.</p>
						</div>
					{/if}

					<Button
						variant="brand"
						type="submit"
						class="mt-1 h-10 w-full rounded-xl"
						disabled={loading || (otpRequested ? !canVerifyOtp : !canRequestOtp)}
					>
						{#if loading}<Loader class="h-4 w-4 animate-spin" />{/if}
						{#if otpRequested}Verify code & create account{:else}Continue with email verification{/if}
					</Button>

					{#if otpRequested}
						<button
							type="button"
							onclick={async () => {
								if (loading) return;
								loading = true;
								await requestOTP();
								loading = false;
							}}
							class="w-full text-center text-xs text-primary hover:underline"
						>
							Resend verification code
						</button>
					{/if}
				</form>

				<div class="mt-6 text-center text-sm text-muted-foreground">
					Already have an account?
					<a href="/login" class="ml-1 font-semibold text-primary hover:underline">Sign in</a>
				</div>
			</div>
		</section>

		<aside class="auth-register-hero relative hidden overflow-hidden p-10 lg:flex lg:flex-col lg:justify-between">
			<div class="relative z-10">
				<p class="text-3xl font-semibold tracking-tight text-white">GitPier</p>
			</div>
			<div class="auth-register-bars" aria-hidden="true">
				<span style="--x:8%; --h:22%; --t:18%; --d:0.1s;"></span>
				<span style="--x:15%; --h:34%; --t:47%; --d:0.22s;"></span>
				<span style="--x:24%; --h:16%; --t:24%; --d:0.35s;"></span>
				<span style="--x:35%; --h:44%; --t:30%; --d:0.44s;"></span>
				<span style="--x:45%; --h:27%; --t:14%; --d:0.5s;"></span>
				<span style="--x:56%; --h:38%; --t:41%; --d:0.62s;"></span>
				<span style="--x:66%; --h:52%; --t:20%; --d:0.75s;"></span>
				<span style="--x:74%; --h:24%; --t:12%; --d:0.86s;"></span>
				<span style="--x:84%; --h:62%; --t:52%; --d:0.95s;"></span>
			</div>
			<div class="relative z-10 max-w-md">
				<h2 class="text-5xl font-semibold leading-[1.02] tracking-tight text-white">Build, review, and ship from one place.</h2>
				<p class="mt-4 text-lg leading-relaxed text-white/88">Manage repositories, issues, pull requests, and workflows with your team on GitPier.</p>
			</div>
		</aside>
	</div>
</div>

<style>
	.auth-register-page {
		background-image:
			radial-gradient(90rem 36rem at -20% -25%, color-mix(in oklch, var(--brand) 14%, transparent), transparent 65%),
			radial-gradient(80rem 24rem at 120% 115%, color-mix(in oklch, var(--brand) 8%, transparent), transparent 70%);
	}

	.auth-register-hero {
		background: linear-gradient(
			162deg,
			color-mix(in oklch, var(--brand) 48%, white 38%) 0%,
			color-mix(in oklch, var(--brand) 78%, black 20%) 58%,
			color-mix(in oklch, var(--brand) 88%, black 52%) 100%
		);
	}

	.auth-register-hero::before {
		content: '';
		position: absolute;
		inset: 0;
		background:
			radial-gradient(34rem 24rem at 20% 28%, color-mix(in oklch, white 20%, transparent), transparent 72%), linear-gradient(to top, rgb(0 0 0 / 44%), rgb(0 0 0 / 12%) 40%, transparent 66%);
		pointer-events: none;
	}

	.auth-register-bars {
		position: absolute;
		inset: 0;
	}

	.auth-register-bars span {
		position: absolute;
		left: var(--x);
		top: var(--t);
		width: 1.2rem;
		height: var(--h);
		border-radius: 0.55rem;
		background: linear-gradient(to bottom, rgb(255 255 255 / 30%), rgb(255 255 255 / 2%));
		box-shadow: 0 0 1.8rem color-mix(in oklch, var(--brand) 28%, transparent);
		animation: pulse-bar 4.8s ease-in-out infinite;
		animation-delay: var(--d);
	}

	.auth-register-bars span::before {
		content: '';
		position: absolute;
		left: 50%;
		top: -18%;
		width: 2px;
		height: 136%;
		transform: translateX(-50%);
		background: linear-gradient(to bottom, rgb(255 255 255 / 42%), transparent);
	}

	@keyframes pulse-bar {
		0%,
		100% {
			opacity: 0.34;
			transform: translateY(0);
		}
		50% {
			opacity: 0.92;
			transform: translateY(-0.5rem);
		}
	}
</style>
