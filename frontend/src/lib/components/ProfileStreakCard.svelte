<script lang="ts">
	import { Flame, TrendingUp } from '@lucide/svelte';

	interface Props {
		currentStreak: number;
		longestStreak: number;
	}

	const { currentStreak, longestStreak }: Props = $props();
</script>

<div class="rounded-xl border border-secondary bg-card p-5">
	<p class="text-sm font-semibold text-foreground mb-4">Commit Streak</p>
	<div class="grid grid-cols-2 gap-3">
		<div class="rounded-lg border border-border bg-background p-3 flex flex-col items-center gap-1">
			<Flame class="h-5 w-5 text-orange-400" />
			<span class="text-2xl font-bold text-foreground tabular-nums">{currentStreak}</span>
			<span class="text-xs text-muted-foreground">Current streak</span>
			<span class="text-[10px] text-muted-foreground">{currentStreak === 1 ? '1 day' : `${currentStreak} days`}</span>
		</div>
		<div class="rounded-lg border border-border bg-background p-3 flex flex-col items-center gap-1">
			<TrendingUp class="h-5 w-5 text-green-400" />
			<span class="text-2xl font-bold text-foreground tabular-nums">{longestStreak}</span>
			<span class="text-xs text-muted-foreground">Longest streak</span>
			<span class="text-[10px] text-muted-foreground">{longestStreak === 1 ? '1 day' : `${longestStreak} days`}</span>
		</div>
	</div>
</div>
