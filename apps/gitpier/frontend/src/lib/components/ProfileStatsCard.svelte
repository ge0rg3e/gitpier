<script lang="ts">
	import { Star, GitPullRequest, CircleDot, GitCommitHorizontal, BookOpen } from '@lucide/svelte';
	import type { ProfileStats } from '$lib/api/client';

	interface Props {
		stats: ProfileStats;
		username: string;
	}

	const { stats, username }: Props = $props();

	// Compute a grade A++..D from total stars + commits + PRs
	function computeGrade(s: ProfileStats): string {
		const score = s.total_stars * 5 + s.total_commits * 0.5 + s.total_prs * 2 + s.total_issues;
		if (score >= 300) return 'A++';
		if (score >= 150) return 'A+';
		if (score >= 80) return 'A';
		if (score >= 40) return 'B+';
		if (score >= 20) return 'B';
		if (score >= 10) return 'C';
		return 'D';
	}

	// Score as 0–100 for the circular progress ring
	function computeScore(s: ProfileStats): number {
		const score = s.total_stars * 5 + s.total_commits * 0.5 + s.total_prs * 2 + s.total_issues;
		return Math.min(100, Math.round((score / 300) * 100));
	}

	const grade = $derived(computeGrade(stats));
	const score = $derived(computeScore(stats));

	// SVG ring params
	const R = 36;
	const CIRC = 2 * Math.PI * R;
	const dashOffset = $derived(CIRC - (score / 100) * CIRC);

	const GRADE_COLOR: Record<string, string> = {
		'A++': '#e2b714',
		'A+': '#58a6ff',
		'A': '#3fb950',
		'B+': '#bc8cff',
		'B': '#f78166',
		'C': '#d29922',
		'D': '#8b949e'
	};

	const gradeColor = $derived(GRADE_COLOR[grade] ?? '#8b949e');
</script>

<div class="rounded-xl border border-secondary bg-card p-5 flex items-center gap-5">
	<!-- Stats list -->
	<div class="flex-1 space-y-2.5 min-w-0">
		<p class="text-sm font-semibold text-foreground truncate">{username}'s Stats</p>
		<div class="space-y-1.5">
			<div class="flex items-center gap-2 text-sm text-muted-foreground">
				<Star class="h-3.5 w-3.5 shrink-0 text-yellow-400" />
				<span class="flex-1">Total Stars:</span>
				<strong class="text-foreground">{stats.total_stars.toLocaleString()}</strong>
			</div>
			<div class="flex items-center gap-2 text-sm text-muted-foreground">
				<GitCommitHorizontal class="h-3.5 w-3.5 shrink-0 text-green-400" />
				<span class="flex-1">Total Commits:</span>
				<strong class="text-foreground">{stats.total_commits.toLocaleString()}</strong>
			</div>
			<div class="flex items-center gap-2 text-sm text-muted-foreground">
				<GitPullRequest class="h-3.5 w-3.5 shrink-0 text-purple-400" />
				<span class="flex-1">Total PRs:</span>
				<strong class="text-foreground">{stats.total_prs.toLocaleString()}</strong>
			</div>
			<div class="flex items-center gap-2 text-sm text-muted-foreground">
				<CircleDot class="h-3.5 w-3.5 shrink-0 text-orange-400" />
				<span class="flex-1">Total Issues:</span>
				<strong class="text-foreground">{stats.total_issues.toLocaleString()}</strong>
			</div>
			<div class="flex items-center gap-2 text-sm text-muted-foreground">
				<BookOpen class="h-3.5 w-3.5 shrink-0 text-blue-400" />
				<span class="flex-1">Total Repos:</span>
				<strong class="text-foreground">{stats.total_repos.toLocaleString()}</strong>
			</div>
		</div>
	</div>

	<!-- Grade ring -->
	<div class="shrink-0 flex flex-col items-center gap-1">
		<svg width="90" height="90" viewBox="0 0 90 90">
			<!-- Background ring -->
			<circle
				cx="45" cy="45" r={R}
				fill="none"
				stroke="var(--color-secondary, #21262d)"
				stroke-width="7"
			/>
			<!-- Progress ring -->
			<circle
				cx="45" cy="45" r={R}
				fill="none"
				stroke={gradeColor}
				stroke-width="7"
				stroke-linecap="round"
				stroke-dasharray={CIRC}
				stroke-dashoffset={dashOffset}
				transform="rotate(-90 45 45)"
				style="transition: stroke-dashoffset 0.6s ease, stroke 0.3s ease;"
			/>
			<!-- Grade text -->
			<text
				x="45" y="52"
				text-anchor="middle"
				font-size={grade.length > 2 ? '14' : '18'}
				font-weight="bold"
				fill={gradeColor}
				font-family="monospace"
			>{grade}</text>
		</svg>
	</div>
</div>
