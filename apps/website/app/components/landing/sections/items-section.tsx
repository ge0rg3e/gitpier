import {
  GitFork,
  GitPullRequest,
  CircleDot,
  Workflow,
  KeyRound,
  Users,
  Package,
  SearchCode,
} from 'lucide-react';
import { Section } from '@/components/landing/ui/section';
import { LandingItem } from '@/components/landing/ui/landing-item';
import { ItemTitle } from '@/components/landing/ui/item-title';
import { ItemDescription } from '@/components/landing/ui/item-description';
import { ItemIcon } from '@/components/landing/ui/item-icon';
import type { LucideIcon } from 'lucide-react';

interface ItemsSectionProps extends React.HTMLAttributes<HTMLElement> {}

const items: { title: string; description: string; icon: LucideIcon }[] = [
  { title: 'Repository hosting', description: 'Full Git hosting with branches, tags, file browsing, and complete commit history', icon: GitFork },
  { title: 'Pull requests', description: 'Streamlined code review with inline comments, approvals, and merge controls', icon: GitPullRequest },
  { title: 'Issue tracking', description: 'Organize work with issues, labels, milestones, and assignees across projects', icon: CircleDot },
  { title: 'CI/CD workflows', description: 'Automated pipelines triggered by pushes, pull requests, and custom events', icon: Workflow },
  { title: 'SSH access', description: 'Secure, key-based authentication for fast and safe push and pull operations', icon: KeyRound },
  { title: 'Organizations', description: 'Collaborate in teams with fine-grained roles, permissions, and access control', icon: Users },
  { title: 'Package registry', description: 'Host and distribute packages directly on GitPier', icon: Package },
  { title: 'Powerful search', description: 'Find code, repositories, issues, and users instantly across GitPier', icon: SearchCode },
];

export function ItemsSection({ className, ...props }: ItemsSectionProps) {
  return (
    <Section className={className} {...props}>
      <div className="max-w-container mx-auto flex flex-col items-center gap-6 sm:gap-20">
        <h2 className="mx-auto max-w-[24ch] text-center text-3xl leading-tight font-semibold text-balance sm:max-w-[30ch] sm:text-5xl sm:leading-tight">
          Everything your team needs.<br />Nothing you don&apos;t.
        </h2>
        <div className="grid auto-rows-fr grid-cols-2 gap-0 sm:grid-cols-3 sm:gap-4 lg:grid-cols-4">
          {items.map((item) => (
            <LandingItem key={item.title}>
              <ItemTitle className="flex items-center gap-2">
                <ItemIcon>
                  <item.icon className="size-5 stroke-1" />
                </ItemIcon>
                {item.title}
              </ItemTitle>
              <ItemDescription>{item.description}</ItemDescription>
            </LandingItem>
          ))}
        </div>
      </div>
    </Section>
  );
}
