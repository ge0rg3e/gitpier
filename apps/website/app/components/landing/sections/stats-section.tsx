import { Section } from '@/components/landing/ui/section';

interface StatsSectionProps extends React.HTMLAttributes<HTMLElement> {}

const stats = [
  { label: 'host', value: 'Repositories', description: 'Full Git hosting with branches, tags, file browsing, and complete commit history' },
  { label: 'review', value: 'Pull Requests', description: 'Streamlined code review with inline comments, approvals, and merge controls' },
  { label: 'automate', value: 'Workflows', description: 'Automated CI/CD pipelines triggered by pushes, pull requests, and custom events' },
  { label: 'collaborate', value: 'Teams', description: 'Organizations with fine grained roles, permissions, and access control' },
];

export function StatsSection({ className, ...props }: StatsSectionProps) {
  return (
    <Section className={className} {...props}>
      <div className="container mx-auto max-w-[960px]">
        <div className="grid grid-cols-2 gap-12 sm:grid-cols-4">
          {stats.map((item) => (
            <div key={item.label} className="flex flex-col items-start gap-3 text-left">
              {item.label && (
                <div className="text-muted-foreground text-sm font-semibold">{item.label}</div>
              )}
              <div className="flex items-baseline gap-2">
                <div className="from-foreground to-foreground dark:to-[#3a95ab] bg-linear-to-r bg-clip-text text-xl font-medium text-transparent drop-shadow-[2px_1px_24px_var(--brand)] transition-all duration-300 sm:text-2xl md:text-3xl">
                  {item.value}
                </div>
              </div>
              {item.description && (
                <div className="text-muted-foreground text-sm font-semibold text-pretty">{item.description}</div>
              )}
            </div>
          ))}
        </div>
      </div>
    </Section>
  );
}
