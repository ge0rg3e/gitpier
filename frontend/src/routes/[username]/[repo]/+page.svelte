<script lang="ts">
	import { page } from '$app/state';
	import { env } from '$env/dynamic/public';
	import { getContext } from 'svelte';
	import { repos, releases, type FileEntry, type CommitInfo, type Release, type RepoStarEvent, type User } from '$lib/api/client';
	import { authStore } from '$lib/stores/auth.svelte';
	import { resolveRepoTreeIconUrl } from '$lib/icons/fileIcons';
	import CodeViewer from '$lib/components/CodeViewer.svelte';
	import { timeAgo, mediaUrl, isValidGitDate, commitAuthorAvatarUrl, commitAuthorHref, commitAuthorInitial, commitAuthorName } from '$lib/utils';
	import { Folder, File, GitCommit, Clock, Star, Eye, GitFork, Settings2, Link, Activity, Tag, FilePlus, Scale } from '@lucide/svelte';
	import { renderRepoMarkdownHtml } from '$lib/markdown';

	const layoutCtx = getContext<any>('repoLayout');

	const LANG_COLORS: Record<string, string> = {
		Go: '#00ADD8',
		JavaScript: '#f1e05a',
		TypeScript: '#3178c6',
		Python: '#3572A5',
		Ruby: '#701516',
		Rust: '#dea584',
		Java: '#b07219',
		Kotlin: '#A97BFF',
		Swift: '#F05138',
		C: '#555555',
		'C++': '#f34b7d',
		'C#': '#178600',
		PHP: '#4F5D95',
		HTML: '#e34c26',
		CSS: '#563d7c',
		Vue: '#41b883',
		Svelte: '#ff3e00',
		Shell: '#89e051',
		Lua: '#000080',
		R: '#198CE7',
		Scala: '#c22d40',
		Elixir: '#6e4a7e',
		Dart: '#00B4AB',
		Julia: '#a270ba',
		Zig: '#ec915c',
		Nim: '#ffc200',
		Haskell: '#5e5086',
		Clojure: '#db5855',
		Perl: '#0298c3',
		OCaml: '#3be133',
		Erlang: '#B83998',
		Fortran: '#4d41b1',
		Crystal: '#000100',
		D: '#ba595e',
		'Objective-C': '#438eff',
		Groovy: '#4298b8',
		HCL: '#844FBA',
		Nix: '#7e7eff',
		V: '#5d87bf',
		PowerShell: '#012456'
	};

	let files = $state<FileEntry[]>([]);
	let headCommit = $state<CommitInfo | null>(null);
	let readme = $state<string | null>(null);
	let readmeName = $state<string | null>(null);
	let license = $state<string | null>(null);
	let licenseName = $state<string | null>(null);
	let licensePath = $state<string | null>(null);
	let activeDoc = $state<'readme' | 'license'>('readme');
	let isEmpty = $state(false);
	let loading = $state(true);
	let error = $state('');
	let contributors = $state<User[]>([]);
	let latestRelease = $state<Release | null>(null);
	let releaseCount = $state<number | null>(null);
	let languages = $state<{ name: string; bytes: number; percent: number }[]>([]);
	type ActivityPoint = { date: string; count: number };
	type ChartTab = 'activity' | 'stars';
	let activityPoints = $state<ActivityPoint[]>([]);
	let loadingActivity = $state(true);
	let starsHistoryPoints = $state<ActivityPoint[]>([]);
	let loadingStarsHistory = $state(true);
	let activeChartTab = $state<ChartTab>('activity');
	let hoveredChartIndex = $state<number | null>(null);
	let hydratingFileMeta = $state(false);
	let invalidFileMetaPaths = $state<Set<string>>(new Set());
	let loadSeq = 0;

	const { username, repo } = $derived(page.params);
	const ref = $derived(page.url.searchParams.get('ref') ?? undefined);
	const layoutRepo = $derived(layoutCtx?.repo);
	const starCount = $derived(layoutCtx?.starCount ?? 0);
	const commitCount = $derived(layoutCtx?.stats?.commits ?? 0);
	function resolveSshCloneHost(raw?: string): string {
		const trimmed = (raw ?? '').trim();
		if (!trimmed) return 'localhost:2222';
		return trimmed
			.replace(/^ssh:\/\//i, '')
			.replace(/^[^@]+@/, '')
			.replace(/\/+$/, '');
	}
	const sshCloneHost = resolveSshCloneHost(env.PUBLIC_SSH_CLONE_HOST);
	const canAddFile = $derived(authStore.user != null && !/^[0-9a-f]{40}$/i.test(ref ?? '') && !layoutRepo?.is_archived);
	const addFileHref = $derived(`/${username}/${repo}/new${ref ? `?ref=${ref}` : ''}`);
	const setupNewRepoCmd = $derived(
		`echo "# ${repo}" >> README.md\ngit init\ngit add README.md\ngit commit -m "first commit"\ngit branch -M main\ngit remote add origin ssh://git@${sshCloneHost}/${username}/${repo}.git\ngit push -u origin main`
	);
	const setupExistingRepoCmd = $derived(`git remote add origin ssh://git@${sshCloneHost}/${username}/${repo}.git\ngit branch -M main\ngit push -u origin main`);

	function formatReleaseDate(value?: string | null): string {
		if (!value) return '';
		const d = new Date(value);
		if (Number.isNaN(d.getTime())) return '';
		return d.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
	}

	function hrefForWebsite(value?: string): string {
		const trimmed = value?.trim() ?? '';
		if (!trimmed) return '';
		if (/^https?:\/\//i.test(trimmed)) return trimmed;
		return `https://${trimmed}`;
	}

	function displayWebsite(value?: string): string {
		const trimmed = value?.trim() ?? '';
		return trimmed.replace(/^https?:\/\//i, '').replace(/\/$/, '');
	}

	function detectLicenseDisplayName(name: string | null, content: string | null): string | null {
		const raw = (content ?? '').trim();
		if (raw) {
			const upper = raw.toUpperCase();
			if (upper.includes('MIT LICENSE')) return 'MIT License';
			if (upper.includes('APACHE LICENSE') || upper.includes('APACHE-2.0')) return 'Apache License 2.0';
			if (upper.includes('GNU GENERAL PUBLIC LICENSE') || upper.includes('GPL-3.0')) return 'GNU GPLv3';
			if (upper.includes('BSD 3-CLAUSE') || upper.includes('REDISTRIBUTION AND USE IN SOURCE AND BINARY FORMS')) return 'BSD 3-Clause License';
			if (upper.includes('BSD 2-CLAUSE')) return 'BSD 2-Clause License';
			if (upper.includes('MOZILLA PUBLIC LICENSE') || upper.includes('MPL-2.0')) return 'Mozilla Public License 2.0';
			if (upper.includes('UNLICENSE')) return 'The Unlicense';
		}
		return name;
	}

	async function loadTree() {
		const seq = ++loadSeq;
		loading = true;
		error = '';
		readme = null;
		readmeName = null;
		license = null;
		licenseName = null;
		licensePath = null;
		activityPoints = [];
		loadingActivity = true;
		starsHistoryPoints = [];
		loadingStarsHistory = true;
		hoveredChartIndex = null;
		hydratingFileMeta = false;
		invalidFileMetaPaths = new Set();
		releaseCount = null;
		activeDoc = 'readme';
		try {
			// First paint: fast tree without per-file metadata.
			const treeData = await repos.tree(username!, repo!, ref, undefined, { includeMeta: false, includeHead: true });
			if (seq !== loadSeq) return;

			files = treeData.files ?? [];
			headCommit = treeData.head_commit;
			isEmpty = treeData.empty ?? false;
			loading = false;

			if (!isEmpty) {
				hydratingFileMeta = true;
				const readmeFile = files.find((f) => f.type === 'blob' && f.name.toLowerCase().startsWith('readme'));
				const licenseFile = files.find((f) => {
					if (f.type !== 'blob') return false;
					const name = f.name.toLowerCase();
					return ['license', 'license.md', 'license.txt', 'license.rst', 'licence', 'copying', 'unlicense'].includes(name);
				});

				if (readmeFile) {
					readmeName = readmeFile.name;
					void repos
						.blob(username!, repo!, readmeFile.path, ref)
						.then((blobData) => {
							if (seq !== loadSeq) return;
							readme = blobData.content;
						})
						.catch(() => {});
				}

				if (licenseFile) {
					licenseName = licenseFile.name;
					licensePath = licenseFile.path;
					void repos
						.blob(username!, repo!, licenseFile.path, ref)
						.then((blobData) => {
							if (seq !== loadSeq) return;
							license = blobData.content;
							if (!readme) {
								activeDoc = 'license';
							}
						})
						.catch(() => {});
				}

				// Hydrate per-file commit metadata shortly after first paint.
				setTimeout(() => {
					if (seq !== loadSeq) return;
					void repos
						.tree(username!, repo!, ref, undefined, { includeMeta: true, includeHead: false })
						.then((metaTreeData) => {
							if (seq !== loadSeq) return;
							const nextFiles = metaTreeData.files ?? files;
							const nextInvalidPaths = new Set<string>();

							files = nextFiles.map((file) => {
								const hasDate = !!file.date;
								const hasMessage = !!file.message?.trim();
								const validDate = !hasDate || isValidGitDate(file.date);

								if ((hasDate && !validDate) || (hasMessage && !validDate)) {
									nextInvalidPaths.add(file.path);
									return { ...file, date: undefined, message: undefined, author: undefined };
								}

								return file;
							});

							invalidFileMetaPaths = nextInvalidPaths;
							hydratingFileMeta = false;
						})
						.catch(() => {
							if (seq !== loadSeq) return;
							hydratingFileMeta = false;
						});
				}, 80);
			}

			// Side panel data should never block center content.
			void loadContributorsFromCommits(seq);

			void releases
				.getLatest(username!, repo!)
				.then((d) => {
					if (seq !== loadSeq) return;
					latestRelease = d.release;
				})
				.catch(() => {});

			void releases
				.list(username!, repo!)
				.then((d) => {
					if (seq !== loadSeq) return;
					releaseCount = d.releases?.length ?? 0;
				})
				.catch(() => {
					if (seq !== loadSeq) return;
					releaseCount = 0;
				});

			void repos
				.languages(username!, repo!)
				.then((d) => {
					if (seq !== loadSeq) return;
					languages = d.languages ?? [];
				})
				.catch(() => {});

			void repos
				.commits(username!, repo!, { ref, limit: 30 })
				.then((d) => {
					if (seq !== loadSeq) return;
					activityPoints = buildActivityPoints(d.commits ?? []);
				})
				.catch(() => {
					if (seq !== loadSeq) return;
					activityPoints = buildActivityPoints([]);
				})
				.finally(() => {
					if (seq !== loadSeq) return;
					loadingActivity = false;
				});

			void repos.star
				.history(username!, repo!)
				.then((d) => {
					if (seq !== loadSeq) return;
					starsHistoryPoints = buildStarsHistoryPoints(d.stars ?? []);
				})
				.catch(() => {
					if (seq !== loadSeq) return;
					starsHistoryPoints = buildStarsHistoryPoints([]);
				})
				.finally(() => {
					if (seq !== loadSeq) return;
					loadingStarsHistory = false;
				});
		} catch (e: any) {
			if (seq !== loadSeq) return;
			error = e.message;
			loading = false;
			loadingActivity = false;
			loadingStarsHistory = false;
		}
	}

	async function loadContributorsFromCommits(seq: number) {
		const byUsername = new Map<string, User>();
		let offset = 0;
		const limit = 100;
		for (let i = 0; i < 5; i++) {
			const data = await repos.commits(username!, repo!, { ref, limit, offset });
			if (seq !== loadSeq) return;
			for (const commit of data.commits ?? []) {
				const author = commit.author;
				const normalizedUsername = author.username?.trim().toLowerCase();
				if (!normalizedUsername) continue;
				if (layoutRepo?.owner?.username && normalizedUsername === layoutRepo.owner.username.toLowerCase()) continue;
				if (byUsername.has(normalizedUsername)) continue;
				byUsername.set(normalizedUsername, {
					id: 0,
					username: author.username!.trim(),
					display_name: author.name?.trim() || author.username!.trim(),
					email: '',
					bio: '',
					avatar_url: author.avatar_url ?? '',
					location: '',
					website: '',
					created_at: ''
				});
				if (byUsername.size >= 24) break;
			}
			if (!data.has_more || byUsername.size >= 24) break;
			offset += limit;
		}
		if (seq !== loadSeq) return;
		contributors = Array.from(byUsername.values());
	}

	function isRenderableCommit(commit: CommitInfo | null | undefined): commit is CommitInfo {
		if (!commit) return false;
		if (!commit.sha || !/^[0-9a-f]{7,40}$/i.test(commit.sha)) return false;
		if (!commit.message?.trim()) return false;
		if (!commit.author?.name?.trim()) return false;
		if (!isValidGitDate(commit.author?.date)) return false;
		return true;
	}

	$effect(() => {
		loadTree();
	});

	function formatMessage(msg: string) {
		return msg.split('\n')[0].trim();
	}

	const sortedFiles = $derived([
		...files.filter((f) => f.type === 'tree').sort((a, b) => a.name.localeCompare(b.name)),
		...files.filter((f) => f.type === 'blob').sort((a, b) => a.name.localeCompare(b.name))
	]);

	const renderedReadme = $derived(readme ? renderRepoMarkdownHtml(readme, username, repo, ref) : '');
	const isLicenseMarkdown = $derived(licenseName?.toLowerCase().endsWith('.md') ?? false);
	const renderedLicense = $derived(license ? renderRepoMarkdownHtml(license, username, repo, ref) : '');
	const showReadme = $derived(activeDoc === 'readme' && !!readme);
	const showLicense = $derived(activeDoc === 'license' && !!license);
	const showDocTabs = $derived(!!readme && !!license);
	const licenseDisplayName = $derived(detectLicenseDisplayName(licenseName, license));
	const licenseBlobHref = $derived.by(() => {
		if (!licensePath) return '';
		const encodedPath = licensePath
			.split('/')
			.map((segment) => encodeURIComponent(segment))
			.join('/');
		const refQuery = ref ? `?ref=${encodeURIComponent(ref)}` : '';
		return `/${username}/${repo}/blob/${encodedPath}${refQuery}`;
	});

	const ACTIVITY_DAYS = 21;
	const STAR_HISTORY_DAYS = 30;
	const CHART_W = 224;
	const CHART_H = 64;
	const CHART_PAD_X = 4;
	const CHART_PAD_Y = 6;

	function isoDateUTC(date: Date): string {
		return date.toISOString().slice(0, 10);
	}

	function buildActivityPoints(commits: CommitInfo[], days = ACTIVITY_DAYS): ActivityPoint[] {
		const end = new Date();
		end.setUTCHours(0, 0, 0, 0);

		const buckets = new Map<string, number>();
		for (let offset = days - 1; offset >= 0; offset--) {
			const day = new Date(end);
			day.setUTCDate(end.getUTCDate() - offset);
			buckets.set(isoDateUTC(day), 0);
		}

		for (const commit of commits) {
			const key = commit.author?.date?.slice(0, 10);
			if (key && buckets.has(key)) {
				buckets.set(key, (buckets.get(key) ?? 0) + 1);
			}
		}

		return Array.from(buckets.entries()).map(([date, count]) => ({ date, count }));
	}

	function buildStarsHistoryPoints(stars: RepoStarEvent[], days = STAR_HISTORY_DAYS): ActivityPoint[] {
		const end = new Date();
		end.setUTCHours(0, 0, 0, 0);

		const start = new Date(end);
		start.setUTCDate(end.getUTCDate() - (days - 1));

		const buckets = new Map<string, number>();
		for (let offset = days - 1; offset >= 0; offset--) {
			const day = new Date(end);
			day.setUTCDate(end.getUTCDate() - offset);
			buckets.set(isoDateUTC(day), 0);
		}

		let baseline = 0;
		for (const star of stars) {
			const starredAt = new Date(star.created_at);
			if (Number.isNaN(starredAt.getTime())) continue;
			starredAt.setUTCHours(0, 0, 0, 0);

			if (starredAt < start) {
				baseline += 1;
				continue;
			}

			const key = isoDateUTC(starredAt);
			if (buckets.has(key)) {
				buckets.set(key, (buckets.get(key) ?? 0) + 1);
			}
		}

		let running = baseline;
		return Array.from(buckets.entries()).map(([date, count]) => {
			running += count;
			return { date, count: running };
		});
	}

	function activityCoords(points: ActivityPoint[]): Array<{ x: number; y: number }> {
		if (points.length === 0) return [];
		const maxCount = Math.max(1, ...points.map((p) => p.count));
		const graphWidth = CHART_W - CHART_PAD_X * 2;
		const graphHeight = CHART_H - CHART_PAD_Y * 2;
		const step = points.length > 1 ? graphWidth / (points.length - 1) : 0;

		return points.map((point, index) => {
			const x = CHART_PAD_X + index * step;
			const y = CHART_H - CHART_PAD_Y - (point.count / maxCount) * graphHeight;
			return { x, y };
		});
	}

	function linePath(points: Array<{ x: number; y: number }>): string {
		if (points.length === 0) return '';
		return points.map((point, index) => `${index === 0 ? 'M' : 'L'} ${point.x.toFixed(2)} ${point.y.toFixed(2)}`).join(' ');
	}

	function formatChartDate(isoDate: string): string {
		const d = new Date(isoDate);
		if (Number.isNaN(d.getTime())) return isoDate;
		return d.toLocaleDateString(undefined, { month: 'short', day: 'numeric', year: 'numeric' });
	}

	function chartCountLabel(tab: ChartTab, count: number): string {
		if (tab === 'activity') return `${count} ${count === 1 ? 'commit' : 'commits'}`;
		return `${count} ${count === 1 ? 'star' : 'stars'} total`;
	}

	const activityCoordsData = $derived(activityCoords(activityPoints));
	const activityLine = $derived(linePath(activityCoordsData));
	const starsCoordsData = $derived(activityCoords(starsHistoryPoints));
	const starsLine = $derived(linePath(starsCoordsData));
	const activeChartPoints = $derived(activeChartTab === 'activity' ? activityPoints : starsHistoryPoints);
	const activeChartCoords = $derived(activeChartTab === 'activity' ? activityCoordsData : starsCoordsData);
	const activeChartPath = $derived(activeChartTab === 'activity' ? activityLine : starsLine);
	const activeChartLoading = $derived(activeChartTab === 'activity' ? loadingActivity : loadingStarsHistory);
	const activeChartColor = $derived(activeChartTab === 'activity' ? 'var(--brand)' : '#f6b73c');
	const activeChartLabel = $derived(activeChartTab === 'activity' ? 'Recent commit activity' : 'Stars history');
	const effectiveHoveredChartIndex = $derived(hoveredChartIndex);
	const hoveredChartPoint = $derived(effectiveHoveredChartIndex != null ? (activeChartPoints[effectiveHoveredChartIndex] ?? null) : null);
	const hoveredChartCoord = $derived(effectiveHoveredChartIndex != null ? (activeChartCoords[effectiveHoveredChartIndex] ?? null) : null);
	const hoveredChartDateLabel = $derived(hoveredChartPoint ? formatChartDate(hoveredChartPoint.date) : '');
	const hoveredChartCountLabel = $derived(hoveredChartPoint ? chartCountLabel(activeChartTab, hoveredChartPoint.count) : '');
	const hoveredChartIndexLabel = $derived(effectiveHoveredChartIndex != null ? `${effectiveHoveredChartIndex + 1} of ${activeChartPoints.length}` : '');
	const hoveredTooltipLeftPct = $derived(hoveredChartCoord ? Math.max(14, Math.min(86, (hoveredChartCoord.x / CHART_W) * 100)) : 50);
</script>

{#if loading}
	<div class="flex gap-4">
		<div class="flex-1 min-w-0 space-y-1">
			<div class="h-10 rounded-t-md border border-border bg-card animate-pulse"></div>
			{#each Array(7) as _}
				<div class="h-9 border-x border-b border-secondary bg-card animate-pulse"></div>
			{/each}
		</div>
		<div class="w-64 shrink-0 space-y-3">
			<div class="h-5 w-16 rounded bg-card animate-pulse"></div>
			<div class="h-4 rounded bg-card animate-pulse"></div>
			<div class="h-4 w-3/4 rounded bg-card animate-pulse"></div>
		</div>
	</div>
{:else if error}
	<div class="rounded-md border border-red-800/40 bg-red-900/20 p-4 text-sm text-red-400">{error}</div>
{:else if isEmpty}
	<!-- Empty repo setup instructions -->
	<div class="rounded-md border border-border bg-card p-8">
		<h3 class="text-lg font-semibold text-foreground mb-1">Quick setup</h3>
		<p class="text-sm text-muted-foreground mb-6">Get started by pushing an existing repository or creating a new one.</p>
		<div class="space-y-5 text-sm">
			<div>
				<p class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">…create a new repository on the command line</p>
				<CodeViewer code={setupNewRepoCmd} filePath="setup-new-repo.sh" />
			</div>
			<div>
				<p class="text-xs font-semibold text-muted-foreground uppercase tracking-wider mb-2">…or push an existing repository from the command line</p>
				<CodeViewer code={setupExistingRepoCmd} filePath="setup-existing-repo.sh" />
			</div>
		</div>
	</div>
{:else}
	<!-- Two-column layout: file list + About sidebar -->
	<div class="flex gap-6 items-start">
		<!-- Left: file list + README -->
		<div class="flex-1 min-w-0">
			<!-- File list -->
			<div class="rounded-md border border-border overflow-hidden">
				<!-- Last commit header -->
				{#if isRenderableCommit(headCommit)}
					{@const authorLabel = commitAuthorName(headCommit.author)}
					{@const authorHref = commitAuthorHref(headCommit.author)}
					{@const avatarUrl = commitAuthorAvatarUrl(headCommit.author)}
					<div class="flex items-center gap-2 border-b border-border px-4 py-2.5 bg-card">
						<!-- Author avatar -->
						<div class="h-5 w-5 rounded-full bg-secondary border border-border flex items-center justify-center text-xs font-bold text-primary shrink-0">
							{#if avatarUrl}
								<img src={avatarUrl} alt={authorLabel} class="h-full w-full rounded-full object-cover" />
							{:else}
								{commitAuthorInitial(headCommit.author)}
							{/if}
						</div>
						<!-- Author name -->
						{#if authorHref}
							<a href={authorHref} class="text-sm font-semibold text-foreground hover:underline shrink-0">{authorLabel}</a>
						{:else}
							<span class="text-sm font-semibold text-foreground shrink-0" title="Not registered on GitPier">{authorLabel}</span>
						{/if}
						<!-- Commit message -->
						<a href="/{username}/{repo}/commit/{headCommit.sha}" class="text-sm text-foreground hover:text-primary hover:underline truncate flex-1">
							{formatMessage(headCommit.message)}
						</a>
						<!-- SHA + time -->
						<div class="flex items-center gap-3 shrink-0 text-xs text-muted-foreground">
							{#if canAddFile}
								<a
									href={addFileHref}
									class="flex items-center gap-1.5 rounded-md border border-border bg-secondary px-2.5 py-1 text-xs text-muted-foreground transition-colors hover:text-foreground hover:border-primary"
								>
									<FilePlus class="h-3.5 w-3.5" />
									Add file
								</a>
							{/if}
							<a href="/{username}/{repo}/commit/{headCommit.sha}" class="font-mono hover:text-primary transition-colors">
								{headCommit.sha.slice(0, 7)}
							</a>
							<span>{timeAgo(headCommit.author.date)}</span>
							<!-- Commits count with clock -->
							<a href="/{username}/{repo}/commits" class="flex items-center gap-1 hover:text-primary transition-colors font-semibold">
								<Clock class="h-3.5 w-3.5" />
								{commitCount.toLocaleString()}
								{commitCount === 1 ? 'Commit' : 'Commits'}
							</a>
						</div>
					</div>
				{/if}

				<!-- Files -->
				<div class="divide-y divide-secondary">
					{#each sortedFiles as file}
						{@const fileHref = `/${username}/${repo}/${file.type === 'tree' ? 'tree' : 'blob'}/${file.path}${ref ? `?ref=${ref}` : ''}`}
						{@const commitMessage = file.commit_message ?? file.message}
						{@const commitDate = file.commit_date ?? file.date}
						{@const commitSHA = file.commit_sha ?? ''}
						<div class="flex items-center gap-3 px-4 py-2 bg-card hover:bg-accent transition-colors group">
							<a href={fileHref} class="flex min-w-0 flex-1 items-center gap-3">
								<span class="shrink-0">
									{#if file.type === 'tree'}
										{#if resolveRepoTreeIconUrl(file.name, 'tree')}
											<img src={resolveRepoTreeIconUrl(file.name, 'tree')} alt="" class="h-4 w-4" />
										{:else}
											<Folder class="h-4 w-4 text-primary" />
										{/if}
									{:else}
										{#if resolveRepoTreeIconUrl(file.name, 'blob')}
											<img src={resolveRepoTreeIconUrl(file.name, 'blob')} alt="" class="h-4 w-4" />
										{:else}
											<File class="h-4 w-4 text-muted-foreground" />
										{/if}
									{/if}
								</span>
								<span class="text-sm group-hover:underline font-medium truncate">{file.name}</span>
							</a>
							<div class="ml-auto hidden sm:flex items-center justify-end gap-4 max-w-[50%]">
								{#if commitMessage}
									{#if commitSHA}
										<a
											href="/{username}/{repo}/commit/{commitSHA}"
											class="text-sm text-muted-foreground truncate text-right max-w-88 hidden md:block hover:text-primary hover:underline">{commitMessage}</a
										>
									{:else}
										<span class="text-sm text-muted-foreground truncate text-right max-w-88 hidden md:block">{commitMessage}</span>
									{/if}
								{:else if hydratingFileMeta || invalidFileMetaPaths.has(file.path)}
									<span class="hidden md:flex justify-end w-28">
										<span class="block h-3 w-28 rounded bg-secondary animate-pulse"></span>
									</span>
								{/if}
								{#if isValidGitDate(commitDate)}
									<span class="text-sm text-muted-foreground shrink-0">{timeAgo(commitDate)}</span>
								{:else if hydratingFileMeta || invalidFileMetaPaths.has(file.path)}
									<span class="shrink-0">
										<span class="block h-3 w-20 rounded bg-secondary animate-pulse"></span>
									</span>
								{/if}
							</div>
						</div>
					{/each}
				</div>
			</div>

			<!-- Repository documents -->
			{#if readme || license}
				<div class="mt-4 rounded-md border border-border overflow-hidden">
					<div class="flex items-center gap-2 border-b border-border px-4 py-2 bg-card">
						<File class="h-4 w-4 text-muted-foreground" />
						{#if showDocTabs}
							<div class="flex items-center gap-1">
								<button
									type="button"
									onclick={() => (activeDoc = 'readme')}
									class="rounded-md border px-2.5 py-1 text-sm font-semibold transition-colors hover:bg-secondary"
									class:bg-secondary={activeDoc === 'readme'}
									class:border-border={activeDoc === 'readme'}
									class:border-transparent={activeDoc !== 'readme'}
									class:text-foreground={activeDoc === 'readme'}
									class:text-muted-foreground={activeDoc !== 'readme'}
								>
									{readmeName ?? 'README.md'}
								</button>
								<button
									type="button"
									onclick={() => (activeDoc = 'license')}
									class="rounded-md border px-2.5 py-1 text-sm font-semibold transition-colors hover:bg-secondary"
									class:bg-secondary={activeDoc === 'license'}
									class:border-border={activeDoc === 'license'}
									class:border-transparent={activeDoc !== 'license'}
									class:text-foreground={activeDoc === 'license'}
									class:text-muted-foreground={activeDoc !== 'license'}
								>
									{licenseName ?? 'LICENSE'}
								</button>
							</div>
						{:else if readme}
							<span class="text-sm font-semibold text-foreground">{readmeName ?? 'README.md'}</span>
						{:else}
							<span class="text-sm font-semibold text-foreground">{licenseName ?? 'LICENSE'}</span>
						{/if}
					</div>
					{#if showReadme}
						<div
							class="p-8 bg-background prose prose-sm prose-invert max-w-none
                        prose-headings:text-foreground prose-headings:border-b prose-headings:border-secondary prose-headings:pb-2
                        prose-p:text-foreground prose-a:text-primary
                        prose-code:text-foreground prose-code:bg-card prose-code:rounded prose-code:px-1
                        prose-pre:bg-card prose-pre:border prose-pre:border-border
                        prose-blockquote:border-l-border prose-blockquote:text-muted-foreground
                        prose-hr:border-secondary prose-li:text-foreground prose-strong:text-foreground"
						>
							{@html renderedReadme}
						</div>
					{:else if showLicense && isLicenseMarkdown}
						<div
							class="p-8 bg-background prose prose-sm prose-invert max-w-none
                        prose-headings:text-foreground prose-headings:border-b prose-headings:border-secondary prose-headings:pb-2
                        prose-p:text-foreground prose-a:text-primary
                        prose-code:text-foreground prose-code:bg-card prose-code:rounded prose-code:px-1
                        prose-pre:bg-card prose-pre:border prose-pre:border-border
                        prose-blockquote:border-l-border prose-blockquote:text-muted-foreground
                        prose-hr:border-secondary prose-li:text-foreground prose-strong:text-foreground"
						>
							{@html renderedLicense}
						</div>
					{:else if showLicense}
						<pre class="bg-background p-6 text-sm text-foreground whitespace-pre-wrap wrap-break-word overflow-x-auto">{license}</pre>
					{/if}
				</div>
			{/if}
		</div>

		<!-- Right: About sidebar -->
		<aside class="w-64 shrink-0 space-y-6 hidden lg:block">
			<!-- About -->
			<div>
				<div class="flex items-center justify-between mb-3">
					<h2 class="text-sm font-semibold text-foreground">About</h2>
				</div>

				{#if layoutRepo?.description}
					<p class="text-sm text-foreground mb-3">{layoutRepo.description}</p>
				{:else}
					<p class="text-sm text-muted-foreground mb-3 italic">No description provided yet.</p>
				{/if}

				{#if layoutRepo?.website}
					<a
						href={hrefForWebsite(layoutRepo.website)}
						target="_blank"
						rel="noopener noreferrer"
						class="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors"
					>
						<Link class="h-4 w-4" />
						<span class="truncate">{displayWebsite(layoutRepo.website)}</span>
					</a>
				{/if}

				{#if licenseName && licenseBlobHref}
					<div class="mt-2">
						<a href={licenseBlobHref} class="inline-flex items-center gap-1.5 text-sm text-muted-foreground hover:text-foreground transition-colors">
							<Scale class="h-4 w-4" />
							<span class="truncate">{licenseDisplayName ?? licenseName}</span>
						</a>
					</div>
				{/if}
			</div>

			<hr class="border-secondary" />

			<!-- Activity and stars -->
			<div>
				<div class="flex items-center justify-between mb-2">
					<h2 class="text-sm font-semibold text-foreground">Insights</h2>
					<div class="inline-flex rounded-md border border-border bg-card overflow-hidden">
						<button
							type="button"
							onclick={() => {
								activeChartTab = 'activity';
								hoveredChartIndex = null;
							}}
							class="px-2.5 py-1 text-xs font-semibold transition-colors"
							class:bg-secondary={activeChartTab === 'activity'}
							class:text-foreground={activeChartTab === 'activity'}
							class:text-muted-foreground={activeChartTab !== 'activity'}
						>
							Activity
						</button>
						<button
							type="button"
							onclick={() => {
								activeChartTab = 'stars';
								hoveredChartIndex = null;
							}}
							class="px-2.5 py-1 text-xs font-semibold transition-colors"
							class:bg-secondary={activeChartTab === 'stars'}
							class:text-foreground={activeChartTab === 'stars'}
							class:text-muted-foreground={activeChartTab !== 'stars'}
						>
							Stars
						</button>
					</div>
				</div>

				{#if activeChartLoading}
					<div class="h-16"></div>
				{:else}
					<div class="relative">
						<svg class="w-full h-16" viewBox={`0 0 ${CHART_W} ${CHART_H}`} role="img" aria-label={activeChartLabel} onmouseleave={() => (hoveredChartIndex = null)}>
							<path d={activeChartPath} fill="none" stroke={activeChartColor} stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
							{#if hoveredChartCoord}
								<circle cx={hoveredChartCoord.x} cy={hoveredChartCoord.y} r="2.5" fill={activeChartColor} />
							{/if}
							{#each activeChartCoords as coord, index}
								<circle
									cx={coord.x}
									cy={coord.y}
									r="7"
									fill="transparent"
									onmouseenter={() => (hoveredChartIndex = index)}
									onmousemove={() => (hoveredChartIndex = index)}
									onfocus={() => (hoveredChartIndex = index)}
									tabindex="0"
									aria-label={`${formatChartDate(activeChartPoints[index]?.date ?? '')}: ${chartCountLabel(activeChartTab, activeChartPoints[index]?.count ?? 0)}`}
								></circle>
							{/each}
						</svg>
						{#if hoveredChartCoord && hoveredChartPoint}
							<div
								class="pointer-events-none absolute z-10 -translate-x-1/2 whitespace-nowrap rounded-md border border-border bg-card px-2 py-1 text-left shadow-sm"
								style="left:{hoveredTooltipLeftPct}%; top:{Math.max(0, (hoveredChartCoord.y / CHART_H) * 100 - 24)}%;"
							>
								<p class="text-[11px] font-semibold text-foreground leading-tight">{hoveredChartCountLabel}</p>
								<p class="text-[10px] text-muted-foreground leading-tight">{hoveredChartDateLabel}</p>
							</div>
						{/if}
					</div>
					{#if hoveredChartPoint}
						<p class="mt-1 text-xs text-muted-foreground">{hoveredChartDateLabel} • {hoveredChartCountLabel} • {hoveredChartIndexLabel}</p>
					{/if}
				{/if}
			</div>

			{#if languages.length > 0}
				<hr class="border-secondary" />

				<!-- Languages -->
				<div>
					<h2 class="text-sm font-semibold text-foreground mb-3">Languages</h2>
					<!-- Segmented bar -->
					<div class="flex h-2 rounded-full overflow-hidden mb-3 gap-px">
						{#each languages as lang}
							<div title="{lang.name} {lang.percent.toFixed(1)}%" style="width:{lang.percent}%; background-color:{LANG_COLORS[lang.name] ?? '#8b949e'}"></div>
						{/each}
					</div>
					<!-- Legend -->
					<div class="flex flex-wrap gap-x-4 gap-y-1.5">
						{#each languages as lang}
							<span class="flex items-center gap-1.5 text-xs text-muted-foreground">
								<span class="h-2.5 w-2.5 rounded-full shrink-0" style="background-color:{LANG_COLORS[lang.name] ?? '#8b949e'}"></span>
								<span class="text-foreground font-medium">{lang.name}</span>
								<span>{lang.percent.toFixed(1)}%</span>
							</span>
						{/each}
					</div>
				</div>
			{/if}

			<hr class="border-secondary" />

			<!-- Releases -->
			<div>
				<div class="flex items-center justify-between mb-3">
					<h2 class="text-sm font-semibold text-foreground">
						<a href="/{username}/{repo}/releases" class="hover:text-primary transition-colors">Releases</a>
						{#if releaseCount !== null}
							<span class="font-normal text-muted-foreground ml-1">{releaseCount}</span>
						{/if}
					</h2>
				</div>
				{#if latestRelease}
					<a href="/{username}/{repo}/releases/{latestRelease.id}" class="flex items-start gap-2 text-sm text-muted-foreground hover:text-foreground transition-colors group mb-2">
						<Tag class="h-4 w-4 text-muted-foreground shrink-0" />
						<div class="min-w-0 flex-1">
							<div class="flex items-center gap-2">
								<span class="font-mono text-foreground group-hover:underline truncate">{latestRelease.tag_name}</span>
								{#if latestRelease.is_prerelease}
									<span class="text-xs border border-orange-400/50 text-orange-400 rounded-full px-1.5 py-0.5 shrink-0">Pre-release</span>
								{:else}
									<span class="text-xs border border-primary/40 text-primary rounded-full px-1.5 py-0.5 shrink-0">Latest</span>
								{/if}
							</div>
							{#if formatReleaseDate(latestRelease.published_at ?? latestRelease.created_at)}
								<p class="text-xs text-muted-foreground mt-1">
									on {formatReleaseDate(latestRelease.published_at ?? latestRelease.created_at)}
								</p>
							{/if}
						</div>
					</a>
					{#if latestRelease.name && latestRelease.name !== latestRelease.tag_name}
						<p class="text-xs text-muted-foreground">{latestRelease.name}</p>
					{/if}
				{:else}
					<p class="text-xs text-muted-foreground mb-1">No releases published</p>
					{#if layoutRepo && authStore.user?.id === layoutRepo.owner_id}
						<a href="/{username}/{repo}/releases/new" class="text-xs text-primary hover:underline">Create a new release</a>
					{/if}
				{/if}
			</div>

			<hr class="border-secondary" />

			<!-- Contributors -->
			<div>
				<div class="flex items-center justify-between mb-3">
					<h2 class="text-sm font-semibold text-foreground">
						Contributors
						{#if layoutRepo}
							<span class="font-normal text-muted-foreground ml-1">{contributors.length + 1}</span>
						{/if}
					</h2>
				</div>
				<div class="flex flex-wrap gap-1.5">
					<!-- Owner always shown first -->
					{#if layoutRepo?.owner}
						<a href="/{layoutRepo.owner.username}" title={layoutRepo.owner.username}>
							<div
								class="h-8 w-8 rounded-full bg-secondary border border-border flex items-center justify-center text-xs font-bold text-primary overflow-hidden hover:ring-2 hover:ring-primary transition-all"
							>
								{#if layoutRepo.owner.avatar_url}
									<img src={mediaUrl(layoutRepo.owner.avatar_url)} alt={layoutRepo.owner.username} class="h-full w-full object-cover" />
								{:else}
									{layoutRepo.owner.username[0]?.toUpperCase()}
								{/if}
							</div>
						</a>
					{/if}
					{#each contributors as collab}
						<a href="/{collab.username}" title={collab.username}>
							<div
								class="h-8 w-8 rounded-full bg-secondary border border-border flex items-center justify-center text-xs font-bold text-primary overflow-hidden hover:ring-2 hover:ring-primary transition-all"
							>
								{#if collab.avatar_url}
									<img src={mediaUrl(collab.avatar_url)} alt={collab.username} class="h-full w-full object-cover" />
								{:else}
									{collab.username[0]?.toUpperCase()}
								{/if}
							</div>
						</a>
					{/each}
				</div>
			</div>
		</aside>
	</div>
{/if}
