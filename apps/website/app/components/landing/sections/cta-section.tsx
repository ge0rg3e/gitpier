import { cn } from '@/lib/cn';
import { Section } from '@/components/landing/ui/section';
import { Glow } from '@/components/landing/ui/glow';
import { Button } from '@/components/ui/button';

interface CTASectionProps extends React.HTMLAttributes<HTMLElement> {}

export function CTASection({ className, ...props }: CTASectionProps) {
  return (
    <Section className={cn('group relative overflow-hidden', className)} {...props}>
      <div className="max-w-container relative z-10 mx-auto flex flex-col items-center gap-6 text-center sm:gap-8">
        <h2 className="max-w-[640px] text-3xl leading-tight font-semibold sm:text-5xl sm:leading-tight">
          Your code deserves a better home.
        </h2>
        <p className="text-muted-foreground max-w-[480px] text-balance font-medium">
          Join developers already building on GitPier. Start for free, scale when you&apos;re ready.
        </p>
        <div className="flex flex-wrap items-center justify-center gap-3">
          <Button variant="default" size="lg" href="/docs/self-hosted/get-started">
            Self-host GitPier
          </Button>
          <Button variant="outline" size="lg" href="https://github.com/ge0rg3e/gitpier">
            Contribute to GitPier
          </Button>
        </div>
      </div>
      <div className="absolute top-0 left-0 h-full w-full translate-y-[1rem] opacity-80 transition-all duration-500 ease-in-out group-hover:translate-y-[-2rem] group-hover:opacity-100">
        <Glow variant="bottom" />
      </div>
    </Section>
  );
}
