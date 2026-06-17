<script lang="ts">
	type Activity = {
		date: string;
		count: number;
		level?: number;
	};

	type Labels = {
		months?: string[];
		weekdays?: string[];
		totalCount?: string;
		legend?: {
			less?: string;
			more?: string;
		};
	};

	type Week = Array<Activity | undefined>;
	type MonthLabel = { weekIndex: number; label: string };

	const DEFAULT_MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
	const DEFAULT_LABELS: Labels = {
		months: DEFAULT_MONTHS,
		weekdays: ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'],
		totalCount: '{{count}} activities in {{year}}',
		legend: { less: 'Less', more: 'More' }
	};

	interface Props {
		data: Activity[];
		blockMargin?: number;
		blockRadius?: number;
		blockSize?: number;
		fontSize?: number;
		labels?: Labels;
		maxLevel?: number;
		totalCount?: number;
		weekStart?: number;
		class?: string;
	}

	const {
		data,
		blockMargin = 3,
		blockRadius = 2,
		blockSize = 10,
		fontSize = 11,
		labels: labelsProp = DEFAULT_LABELS,
		maxLevel: maxLevelProp = 4,
		totalCount: totalCountProp,
		weekStart = 0,
		class: className = ''
	}: Props = $props();

	function parseDateString(value: string): Date {
		const [year, month, day] = value.split('-').map(Number);
		return new Date(Date.UTC(year, month - 1, day));
	}

	function formatDateKey(date: Date): string {
		return `${date.getUTCFullYear()}-${String(date.getUTCMonth() + 1).padStart(2, '0')}-${String(date.getUTCDate()).padStart(2, '0')}`;
	}

	function differenceInCalendarDays(left: Date, right: Date): number {
		const ms = left.getTime() - right.getTime();
		return Math.floor(ms / 86400000);
	}

	function nextDay(date: Date, targetDay: number): Date {
		const current = date.getUTCDay();
		const delta = (targetDay - current + 7) % 7 || 7;
		const out = new Date(date);
		out.setUTCDate(out.getUTCDate() + delta);
		return out;
	}

	function subWeeks(date: Date, count: number): Date {
		const out = new Date(date);
		out.setUTCDate(out.getUTCDate() - count * 7);
		return out;
	}

	function fillHoles(activities: Activity[]): Activity[] {
		if (activities.length === 0) return [];
		const sorted = [...activities].sort((a, b) => a.date.localeCompare(b.date));
		const byDate = new Map<string, Activity>(sorted.map((a) => [a.date, a]));
		const first = parseDateString(sorted[0].date);
		const last = parseDateString(sorted[sorted.length - 1].date);
		const out: Activity[] = [];
		for (let cur = new Date(first); cur <= last; cur.setUTCDate(cur.getUTCDate() + 1)) {
			const key = formatDateKey(cur);
			out.push(byDate.get(key) ?? { date: key, count: 0, level: 0 });
		}
		return out;
	}

	function normalizeLevels(activities: Activity[], maxLevel: number): Activity[] {
		if (activities.length === 0) return [];
		const safeMax = Math.max(1, maxLevel);
		const maxCount = activities.reduce((acc, a) => Math.max(acc, a.count), 0);
		return activities.map((a) => {
			const explicit = typeof a.level === 'number';
			const computed = maxCount > 0 && a.count > 0 ? Math.max(1, Math.ceil((a.count / maxCount) * safeMax)) : 0;
			const level = explicit ? Math.max(0, Math.min(safeMax, a.level!)) : computed;
			return { ...a, level };
		});
	}

	function groupByWeeks(activities: Activity[], startDay = 0): Week[] {
		if (activities.length === 0) return [];
		const normalized = fillHoles(activities);
		const firstActivity = normalized[0];
		const firstDate = parseDateString(firstActivity.date);
		const firstCalendarDate = firstDate.getUTCDay() === startDay ? firstDate : subWeeks(nextDay(firstDate, startDay), 1);
		const pad = differenceInCalendarDays(firstDate, firstCalendarDate);
		const padded: Week = [...new Array(pad).fill(undefined), ...normalized];
		const weeks: Week[] = [];
		const numberOfWeeks = Math.ceil(padded.length / 7);
		for (let i = 0; i < numberOfWeeks; i++) {
			weeks.push(padded.slice(i * 7, i * 7 + 7));
		}
		return weeks;
	}

	function getMonthLabels(weeks: Week[], monthNames: string[] = DEFAULT_MONTHS): MonthLabel[] {
		return weeks
			.reduce<MonthLabel[]>((labels, week, weekIndex) => {
				const first = week.find((d) => d !== undefined);
				if (!first) return labels;
				const month = monthNames[parseDateString(first.date).getUTCMonth()];
				const prev = labels[labels.length - 1];
				if (weekIndex === 0 || !prev || prev.label !== month) {
					labels.push({ weekIndex, label: month });
				}
				return labels;
			}, [])
			.filter(({ weekIndex }, index, labels) => {
				const minWeeks = 3;
				if (index === 0) return !!labels[1] && labels[1].weekIndex - weekIndex >= minWeeks;
				if (index === labels.length - 1) return weeks.slice(weekIndex).length >= minWeeks;
				return true;
			});
	}

	function levelColor(level: number): string {
		// GitHub-like dark contribution colors
		if (level <= 0) return 'oklch(from var(--brand) l c h / 5%)';
		if (level === 1) return 'oklch(from var(--brand) l c h / 10%)';
		if (level === 2) return 'oklch(from var(--brand) l c h / 30%)';
		if (level === 3) return 'oklch(from var(--brand) l c h / 50%)';
		return 'var(--brand)';
	}

	function activityTitle(activity: Activity): string {
		const date = parseDateString(activity.date).toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' });
		return `${activity.count} contribution${activity.count === 1 ? '' : 's'} on ${date}`;
	}

	const labels = $derived({ ...DEFAULT_LABELS, ...labelsProp });
	const maxLevel = $derived(Math.max(1, maxLevelProp));
	const normalizedData = $derived(normalizeLevels(data, maxLevel));
	const weeks = $derived(groupByWeeks(normalizedData, weekStart));
	const monthLabels = $derived(getMonthLabels(weeks, labels.months ?? DEFAULT_MONTHS));
	const year = $derived(data.length > 0 ? parseDateString(data[0].date).getUTCFullYear() : new Date().getUTCFullYear());
	const totalCount = $derived(typeof totalCountProp === 'number' ? totalCountProp : data.reduce((sum, d) => sum + d.count, 0));

	const labelMargin = 6;
	const labelHeight = $derived(fontSize + labelMargin);
	const labelColumnWidth = $derived((labels.weekdays?.length ?? 0) > 0 ? 28 : 0);
	const pitch = $derived(blockSize + blockMargin);
	const gridX = $derived(labelColumnWidth + 8);
	const gridY = $derived(labelHeight + 6);
	const width = $derived(gridX + weeks.length * pitch - blockMargin + 2);
	const height = $derived(gridY + 7 * pitch - blockMargin + 2);
	const totalText = $derived((labels.totalCount ?? '{{count}} activities in {{year}}').replace('{{count}}', String(totalCount)).replace('{{year}}', String(year)));

	const weekdayTicks = [1, 3, 5];
</script>

{#if data.length > 0}
	<div class={className}>
		<div class="rounded-md border border-border bg-card p-4">
			<div class="mb-3 text-sm font-medium text-foreground">{totalText}</div>
			<svg viewBox={`0 0 ${width} ${height}`} preserveAspectRatio="xMinYMin meet" class="block h-auto w-full">
				{#each monthLabels as month}
					<text x={gridX + month.weekIndex * pitch} y={labelHeight - 2} fill="currentColor" style={`font-size:${fontSize}px`} class="text-muted-foreground">
						{month.label}
					</text>
				{/each}

				{#if labels.weekdays}
					{#each weekdayTicks as dayIndex}
						<text x="0" y={gridY + dayIndex * pitch + blockSize - 1} fill="currentColor" style={`font-size:${fontSize}px`} class="text-muted-foreground">
							{labels.weekdays[(weekStart + dayIndex) % 7]}
						</text>
					{/each}
				{/if}

				{#each weeks as week, weekIndex}
					{#each week as activity, dayIndex}
						{#if activity}
							<rect
								x={gridX + weekIndex * pitch}
								y={gridY + dayIndex * pitch}
								width={blockSize}
								height={blockSize}
								rx={blockRadius}
								ry={blockRadius}
								fill={levelColor(activity.level ?? 0)}
							>
								<title>{activityTitle(activity)}</title>
							</rect>
						{/if}
					{/each}
				{/each}
			</svg>

			<div class="mt-3 flex items-center justify-end gap-2 text-[11px] text-muted-foreground">
				<span>{labels.legend?.less ?? 'Less'}</span>
				{#each new Array(maxLevel + 1) as _, level}
					<span class="h-3 w-3 rounded-[2px]" style={`background-color:${levelColor(level)};`}></span>
				{/each}
				<span>{labels.legend?.more ?? 'More'}</span>
			</div>
		</div>
	</div>
{/if}
